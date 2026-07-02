package service

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"regexp"
	"regexp/syntax"
	"sort"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"
)

type contentModerationLocalRuleEngine struct {
	cfg          ContentModerationLocalRulesConfig
	patterns     []contentModerationCompiledLocalRulePattern
	literalIndex *contentModerationLocalRuleLiteralIndex
}

type contentModerationCompiledLocalRulePattern struct {
	cfg      ContentModerationLocalRulePattern
	re       *regexp.Regexp
	requires []string
}

type contentModerationLocalRuleLiteralIndex struct {
	literals []string
}

var contentModerationLocalRuleEngineCache sync.Map

func inspectContentModerationLocalRulesText(text string, cfg ContentModerationLocalRulesConfig) ContentModerationLocalRuleResult {
	cfg = normalizeContentModerationLocalRulesConfig(cfg)
	text = limitContentModerationLocalRuleScanText(text, cfg.MaxTextLength)
	result := ContentModerationLocalRuleResult{
		Enabled:         cfg.Enabled,
		ScanTextHash:    contentModerationLocalRuleHash(text),
		Action:          cfg.Action,
		Threshold:       cfg.Threshold,
		StrictThreshold: cfg.StrictThreshold,
		TextPreview:     previewContentModerationLocalRuleText(text, 500),
		ExtractedChars:  contentModerationLocalRuleExtractedChars(text),
		ScanScope:       cfg.ScanScope,
	}
	if !cfg.Enabled || strings.TrimSpace(text) == "" {
		return result
	}
	engine, err := contentModerationLocalRuleEngineForConfig(cfg)
	if err != nil {
		slog.Warn("content_moderation.local_rules_engine_unavailable", "error", err)
		return result
	}
	return engine.inspectText(text)
}

func contentModerationLocalRuleEngineForConfig(cfg ContentModerationLocalRulesConfig) (*contentModerationLocalRuleEngine, error) {
	cfg = normalizeContentModerationLocalRulesConfig(cfg)
	key := contentModerationLocalRuleEngineCacheKey(cfg)
	if cached, ok := contentModerationLocalRuleEngineCache.Load(key); ok {
		return cached.(*contentModerationLocalRuleEngine), nil
	}
	engine, err := newContentModerationLocalRuleEngine(cfg)
	if err != nil {
		return nil, err
	}
	actual, _ := contentModerationLocalRuleEngineCache.LoadOrStore(key, engine)
	return actual.(*contentModerationLocalRuleEngine), nil
}

func contentModerationLocalRuleEngineCacheKey(cfg ContentModerationLocalRulesConfig) string {
	cfg = normalizeContentModerationLocalRulesConfig(cfg)
	data, err := json.Marshal(struct {
		Enabled              bool                                `json:"enabled"`
		Action               string                              `json:"action"`
		ScanScope            string                              `json:"scan_scope"`
		Threshold            int                                 `json:"threshold"`
		StrictThreshold      int                                 `json:"strict_threshold"`
		MaxTextLength        int                                 `json:"max_text_length"`
		DisabledBuiltinRules []string                            `json:"disabled_builtin_rules"`
		CustomRules          []ContentModerationLocalRulePattern `json:"custom_rules"`
	}{
		Enabled:              cfg.Enabled,
		Action:               cfg.Action,
		ScanScope:            cfg.ScanScope,
		Threshold:            cfg.Threshold,
		StrictThreshold:      cfg.StrictThreshold,
		MaxTextLength:        cfg.MaxTextLength,
		DisabledBuiltinRules: cfg.DisabledBuiltinRules,
		CustomRules:          cfg.CustomRules,
	})
	if err != nil {
		return fmt.Sprintf("%t|%s|%s|%d|%d|%d|%v|%v", cfg.Enabled, cfg.Action, cfg.ScanScope, cfg.Threshold, cfg.StrictThreshold, cfg.MaxTextLength, cfg.DisabledBuiltinRules, cfg.CustomRules)
	}
	return string(data)
}

func newContentModerationLocalRuleEngine(cfg ContentModerationLocalRulesConfig) (*contentModerationLocalRuleEngine, error) {
	cfg = normalizeContentModerationLocalRulesConfig(cfg)
	disabled := map[string]struct{}{}
	for _, name := range cfg.DisabledBuiltinRules {
		disabled[strings.ToLower(strings.TrimSpace(name))] = struct{}{}
	}
	merged := append([]ContentModerationLocalRulePattern{}, contentModerationLocalRuleBuiltinPatterns...)
	merged = append(merged, cfg.CustomRules...)
	patterns := make([]contentModerationCompiledLocalRulePattern, 0, len(merged))
	for _, pattern := range merged {
		pattern.Name = strings.TrimSpace(pattern.Name)
		pattern.Pattern = strings.TrimSpace(pattern.Pattern)
		pattern.Category = strings.TrimSpace(pattern.Category)
		if pattern.Name == "" || pattern.Pattern == "" || pattern.Weight <= 0 {
			continue
		}
		if _, ok := disabled[strings.ToLower(pattern.Name)]; ok {
			continue
		}
		if pattern.Enabled != nil && !*pattern.Enabled {
			continue
		}
		re, err := regexp.Compile(pattern.Pattern)
		if err != nil {
			return nil, fmt.Errorf("compile local rule pattern %q: %w", pattern.Name, err)
		}
		patterns = append(patterns, contentModerationCompiledLocalRulePattern{
			cfg:      pattern,
			re:       re,
			requires: contentModerationLocalRulePatternRequires(pattern.Pattern),
		})
	}
	return &contentModerationLocalRuleEngine{
		cfg:          cfg,
		patterns:     patterns,
		literalIndex: buildContentModerationLocalRuleLiteralIndex(patterns),
	}, nil
}

func (engine *contentModerationLocalRuleEngine) inspectText(text string) ContentModerationLocalRuleResult {
	cfg := engine.cfg
	text = limitContentModerationLocalRuleScanText(text, cfg.MaxTextLength)
	result := ContentModerationLocalRuleResult{
		Enabled:         cfg.Enabled,
		ScanTextHash:    contentModerationLocalRuleHash(text),
		Action:          cfg.Action,
		Threshold:       cfg.Threshold,
		StrictThreshold: cfg.StrictThreshold,
		TextPreview:     previewContentModerationLocalRuleText(text, 500),
		ExtractedChars:  contentModerationLocalRuleExtractedChars(text),
		ScanScope:       cfg.ScanScope,
	}
	if !cfg.Enabled || strings.TrimSpace(text) == "" {
		return result
	}
	scanText := normalizeContentModerationLocalRuleScanText(text)
	if utf8.RuneCountInString(scanText) < 3 {
		return result
	}

	matchContexts := make([]string, 0, 3)
	recordContext := func(context string) {
		context = strings.TrimSpace(context)
		if context == "" || len(matchContexts) >= 3 {
			return
		}
		for _, existing := range matchContexts {
			if existing == context {
				return
			}
		}
		matchContexts = append(matchContexts, context)
	}
	matchesByName := map[string]ContentModerationLocalRuleMatch{}
	literalHits := engine.literalIndex.match(scanText)
	for _, pattern := range engine.patterns {
		if !contentModerationLocalRulePatternShouldRun(scanText, pattern, literalHits) {
			continue
		}
		if loc := pattern.re.FindStringIndex(scanText); loc != nil {
			match := ContentModerationLocalRuleMatch{
				Name:     pattern.cfg.Name,
				Weight:   pattern.cfg.Weight,
				Category: pattern.cfg.Category,
				Strict:   pattern.cfg.Strict,
			}
			recordContext(contentModerationLocalRuleRegexContext(scanText, loc))
			matchesByName[match.Name] = match
		}
	}

	matches := make([]ContentModerationLocalRuleMatch, 0, len(matchesByName))
	rawScore := 0
	strictScore := 0
	for _, match := range matchesByName {
		matches = append(matches, match)
		rawScore += match.Weight
		if match.Strict {
			strictScore += match.Weight
		}
	}
	sort.Slice(matches, func(i, j int) bool {
		if matches[i].Weight == matches[j].Weight {
			return matches[i].Name < matches[j].Name
		}
		return matches[i].Weight > matches[j].Weight
	})
	score := rawScore
	if score > 0 {
		score -= contentModerationLocalRuleDefensiveContextDiscount(scanText)
		if score < 0 {
			score = 0
		}
	}
	strictHit := strictScore >= cfg.StrictThreshold
	hit := score >= cfg.Threshold || strictHit
	result.Hit = hit
	result.Score = score
	result.RawScore = rawScore
	result.StrictScore = strictScore
	result.StrictHit = strictHit
	result.Matches = matches
	result.HighestCategory = contentModerationLocalRuleHighestCategory(matches)
	if len(matchContexts) > 0 {
		result.ContextPreview = strings.Join(matchContexts, "\n---\n")
	}
	return result
}

func normalizeContentModerationLocalRuleScanText(text string) string {
	text = strings.ReplaceAll(text, "```", " ")
	text = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) && r != '\n' && r != '\r' && r != '\t' {
			return ' '
		}
		return unicode.ToLower(r)
	}, text)
	return strings.Join(strings.Fields(text), " ")
}

func contentModerationLocalRulePatternShouldRun(text string, pattern contentModerationCompiledLocalRulePattern, literalHits map[string]bool) bool {
	for _, required := range pattern.requires {
		if required == "" {
			continue
		}
		if literalHits != nil {
			if !literalHits[required] {
				return false
			}
			continue
		}
		if !strings.Contains(text, required) {
			return false
		}
	}
	return true
}

func buildContentModerationLocalRuleLiteralIndex(patterns []contentModerationCompiledLocalRulePattern) *contentModerationLocalRuleLiteralIndex {
	seen := map[string]struct{}{}
	out := &contentModerationLocalRuleLiteralIndex{}
	for _, pattern := range patterns {
		for _, literal := range pattern.requires {
			literal = strings.TrimSpace(literal)
			if literal == "" {
				continue
			}
			if _, ok := seen[literal]; ok {
				continue
			}
			seen[literal] = struct{}{}
			out.literals = append(out.literals, literal)
		}
	}
	return out
}

func (idx *contentModerationLocalRuleLiteralIndex) match(text string) map[string]bool {
	if idx == nil || len(idx.literals) == 0 || text == "" {
		return nil
	}
	hits := make(map[string]bool, len(idx.literals))
	for _, literal := range idx.literals {
		if strings.Contains(text, literal) {
			hits[literal] = true
		}
	}
	return hits
}

func contentModerationLocalRulePatternRequires(pattern string) []string {
	parsed, err := syntax.Parse(pattern, syntax.Perl)
	if err != nil {
		return nil
	}
	return contentModerationLocalRuleRegexpRequiredLiterals(parsed.Simplify())
}

func contentModerationLocalRuleRegexpRequiredLiterals(re *syntax.Regexp) []string {
	literals := contentModerationLocalRuleRequiredLiteralSet(re)
	if len(literals) == 0 {
		return nil
	}
	out := make([]string, 0, len(literals))
	seen := map[string]struct{}{}
	for literal := range literals {
		literal = strings.TrimSpace(literal)
		if utf8.RuneCountInString(literal) < 4 {
			continue
		}
		if _, ok := seen[literal]; ok {
			continue
		}
		seen[literal] = struct{}{}
		out = append(out, literal)
	}
	sort.Slice(out, func(i, j int) bool {
		if len(out[i]) == len(out[j]) {
			return out[i] < out[j]
		}
		return len(out[i]) > len(out[j])
	})
	return out
}

func contentModerationLocalRuleRequiredLiteralSet(re *syntax.Regexp) map[string]struct{} {
	if re == nil {
		return nil
	}
	switch re.Op {
	case syntax.OpLiteral:
		literal := normalizeContentModerationLocalRuleScanText(string(re.Rune))
		if utf8.RuneCountInString(literal) < 4 {
			return nil
		}
		return map[string]struct{}{literal: {}}
	case syntax.OpCapture, syntax.OpPlus:
		if len(re.Sub) == 0 {
			return nil
		}
		return contentModerationLocalRuleRequiredLiteralSet(re.Sub[0])
	case syntax.OpConcat:
		out := map[string]struct{}{}
		for _, sub := range re.Sub {
			for literal := range contentModerationLocalRuleRequiredLiteralSet(sub) {
				out[literal] = struct{}{}
			}
		}
		if len(out) == 0 {
			return nil
		}
		return out
	case syntax.OpAlternate:
		var common map[string]struct{}
		initialized := false
		for _, sub := range re.Sub {
			literals := contentModerationLocalRuleRequiredLiteralSet(sub)
			if len(literals) == 0 {
				return nil
			}
			if !initialized {
				common = literals
				initialized = true
				continue
			}
			for literal := range common {
				if _, ok := literals[literal]; !ok {
					delete(common, literal)
				}
			}
			if len(common) == 0 {
				return nil
			}
		}
		return common
	}
	return nil
}

func contentModerationLocalRuleDefensiveContextDiscount(text string) int {
	discount := 0
	for _, pattern := range contentModerationLocalRuleDefensiveContextPatterns {
		if pattern.MatchString(text) {
			discount += 30
		}
	}
	if discount > 90 {
		return 90
	}
	return discount
}

var contentModerationLocalRuleDefensiveContextPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\b(defensive|defense|prevent|prevention|mitigation|detect|detection|hardening|patch|remediation|incident\s+response)\b`),
	regexp.MustCompile(`(?i)\b(do\s+not\s+provide|without\s+code|no\s+commands|high\s+level|non[-\s]?operational|refusal|unsafe)\b`),
	regexp.MustCompile(`防御|修复|检测|加固|不要提供|不提供代码`),
}

func contentModerationLocalRuleRegexContext(text string, loc []int) string {
	if len(loc) != 2 {
		return ""
	}
	start, end := loc[0], loc[1]
	if start < 0 || end < start || start > len(text) {
		return ""
	}
	if end > len(text) {
		end = len(text)
	}
	contextStart := contentModerationLocalRuleByteOffsetBefore(text, start, 80)
	contextEnd := contentModerationLocalRuleByteOffsetAfter(text, end, 80)
	before := strings.TrimSpace(text[contextStart:start])
	hit := strings.TrimSpace(text[start:end])
	after := strings.TrimSpace(text[end:contextEnd])
	parts := make([]string, 0, 3)
	if before != "" {
		parts = append(parts, before)
	}
	if hit != "" {
		parts = append(parts, "[HIT]"+hit+"[/HIT]")
	}
	if after != "" {
		parts = append(parts, after)
	}
	context := strings.Join(parts, " ")
	if contextStart > 0 {
		context = "..." + context
	}
	return trimRunes(redactContentModerationSecrets(context), maxModerationExcerptRunes)
}

func contentModerationLocalRuleByteOffsetBefore(text string, start int, maxRunes int) int {
	if start <= 0 || maxRunes <= 0 {
		return start
	}
	offsets := make([]int, 0, maxRunes+1)
	for idx := range text[:start] {
		offsets = append(offsets, idx)
	}
	if len(offsets) <= maxRunes {
		return 0
	}
	return offsets[len(offsets)-maxRunes]
}

func contentModerationLocalRuleByteOffsetAfter(text string, end int, maxRunes int) int {
	if end >= len(text) || maxRunes <= 0 {
		return end
	}
	count := 0
	for idx := range text[end:] {
		if count >= maxRunes {
			return end + idx
		}
		count++
	}
	return len(text)
}
