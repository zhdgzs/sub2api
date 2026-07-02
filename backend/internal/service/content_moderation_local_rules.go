package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const (
	ContentModerationLocalRulesActionRecord = "record"
	ContentModerationLocalRulesActionBlock  = "block"

	ContentModerationLocalRulesScanLatestUserInput = "latest_user_input"
	ContentModerationLocalRulesScanFullTextContext = "full_text_context"

	defaultContentModerationLocalRulesThreshold       = 50
	defaultContentModerationLocalRulesStrictThreshold = 90
	defaultContentModerationLocalRulesMaxTextLength   = 80 * 1024
	maxContentModerationLocalRulesThreshold           = 500
	maxContentModerationLocalRulesStrictThreshold     = 1000
	maxContentModerationLocalRulesMaxTextLength       = 1024 * 1024
	maxContentModerationLocalRuleNameRunes            = 120
	maxContentModerationLocalRuleCategoryRunes        = 120
	maxContentModerationLocalRulePatternRunes         = 4096
	maxContentModerationLocalRulesCustomRules         = 500
)

type ContentModerationLocalRulesConfig struct {
	Enabled              bool                                `json:"enabled"`
	Action               string                              `json:"action"`
	ScanScope            string                              `json:"scan_scope"`
	Threshold            int                                 `json:"threshold"`
	StrictThreshold      int                                 `json:"strict_threshold"`
	MaxTextLength        int                                 `json:"max_text_length"`
	SkipAPIAfterHit      bool                                `json:"skip_api_after_hit"`
	CountForAutoBan      bool                                `json:"count_for_auto_ban"`
	RecordHash           bool                                `json:"record_hash"`
	EmailOnHit           bool                                `json:"email_on_hit"`
	DisabledBuiltinRules []string                            `json:"disabled_builtin_rules"`
	CustomRules          []ContentModerationLocalRulePattern `json:"custom_rules"`
	BuiltinRules         []ContentModerationLocalRulePattern `json:"builtin_rules,omitempty"`
}

type ContentModerationLocalRulePattern struct {
	Name     string `json:"name"`
	Pattern  string `json:"pattern"`
	Weight   int    `json:"weight"`
	Category string `json:"category,omitempty"`
	Strict   bool   `json:"strict,omitempty"`
	Enabled  *bool  `json:"enabled,omitempty"`
}

type ContentModerationLocalRuleMatch struct {
	Name     string `json:"name"`
	Weight   int    `json:"weight"`
	Category string `json:"category,omitempty"`
	Strict   bool   `json:"strict,omitempty"`
}

type ContentModerationLocalRuleLogDetail struct {
	Source          string                            `json:"source,omitempty"`
	ScanScope       string                            `json:"scan_scope,omitempty"`
	Score           int                               `json:"score"`
	RawScore        int                               `json:"raw_score"`
	StrictScore     int                               `json:"strict_score"`
	Threshold       int                               `json:"threshold"`
	StrictThreshold int                               `json:"strict_threshold"`
	StrictHit       bool                              `json:"strict_hit"`
	HighestCategory string                            `json:"highest_category,omitempty"`
	Matches         []ContentModerationLocalRuleMatch `json:"matches,omitempty"`
	ContextPreview  string                            `json:"context_preview,omitempty"`
	ExtractedChars  int                               `json:"extracted_chars"`
}

type ContentModerationLocalRuleResult struct {
	Enabled         bool
	Hit             bool
	ScanTextHash    string
	Action          string
	Score           int
	RawScore        int
	StrictScore     int
	Threshold       int
	StrictThreshold int
	StrictHit       bool
	HighestCategory string
	Matches         []ContentModerationLocalRuleMatch
	TextPreview     string
	ContextPreview  string
	ExtractedChars  int
	ScanScope       string
}

type TestContentModerationLocalRulesInput struct {
	Text   string                             `json:"text"`
	Config *ContentModerationLocalRulesConfig `json:"config,omitempty"`
}

type ContentModerationLocalRulesTestResult struct {
	Hit             bool                              `json:"hit"`
	Action          string                            `json:"action"`
	Score           int                               `json:"score"`
	RawScore        int                               `json:"raw_score"`
	StrictScore     int                               `json:"strict_score"`
	StrictHit       bool                              `json:"strict_hit"`
	Threshold       int                               `json:"threshold"`
	StrictThreshold int                               `json:"strict_threshold"`
	HighestCategory string                            `json:"highest_category"`
	Matches         []ContentModerationLocalRuleMatch `json:"matches"`
	TextPreview     string                            `json:"text_preview"`
	ContextPreview  string                            `json:"context_preview"`
	ExtractedChars  int                               `json:"extracted_chars"`
}

func defaultContentModerationLocalRulesConfig() ContentModerationLocalRulesConfig {
	return normalizeContentModerationLocalRulesConfig(ContentModerationLocalRulesConfig{})
}

func normalizeContentModerationLocalRulesConfig(cfg ContentModerationLocalRulesConfig) ContentModerationLocalRulesConfig {
	switch strings.ToLower(strings.TrimSpace(cfg.Action)) {
	case ContentModerationLocalRulesActionBlock:
		cfg.Action = ContentModerationLocalRulesActionBlock
	default:
		cfg.Action = ContentModerationLocalRulesActionRecord
	}
	switch strings.ToLower(strings.TrimSpace(cfg.ScanScope)) {
	case ContentModerationLocalRulesScanFullTextContext:
		cfg.ScanScope = ContentModerationLocalRulesScanFullTextContext
	default:
		cfg.ScanScope = ContentModerationLocalRulesScanLatestUserInput
	}
	if cfg.Threshold <= 0 {
		cfg.Threshold = defaultContentModerationLocalRulesThreshold
	}
	if cfg.Threshold > maxContentModerationLocalRulesThreshold {
		cfg.Threshold = maxContentModerationLocalRulesThreshold
	}
	if cfg.StrictThreshold <= 0 {
		cfg.StrictThreshold = defaultContentModerationLocalRulesStrictThreshold
	}
	if cfg.StrictThreshold < cfg.Threshold {
		cfg.StrictThreshold = cfg.Threshold
	}
	if cfg.StrictThreshold > maxContentModerationLocalRulesStrictThreshold {
		cfg.StrictThreshold = maxContentModerationLocalRulesStrictThreshold
	}
	if cfg.MaxTextLength <= 0 {
		cfg.MaxTextLength = defaultContentModerationLocalRulesMaxTextLength
	}
	if cfg.MaxTextLength > maxContentModerationLocalRulesMaxTextLength {
		cfg.MaxTextLength = maxContentModerationLocalRulesMaxTextLength
	}
	cfg.DisabledBuiltinRules = normalizeContentModerationLocalRuleNames(cfg.DisabledBuiltinRules)
	cfg.CustomRules = normalizeContentModerationLocalRulePatterns(cfg.CustomRules)
	cfg.BuiltinRules = nil
	return cfg
}

func cloneContentModerationLocalRulesConfig(cfg ContentModerationLocalRulesConfig) ContentModerationLocalRulesConfig {
	cfg = normalizeContentModerationLocalRulesConfig(cfg)
	cfg.DisabledBuiltinRules = append([]string(nil), cfg.DisabledBuiltinRules...)
	cfg.CustomRules = cloneContentModerationLocalRulePatterns(cfg.CustomRules)
	cfg.BuiltinRules = cloneContentModerationLocalRulePatterns(cfg.BuiltinRules)
	return cfg
}

func contentModerationLocalRulesConfigView(cfg ContentModerationLocalRulesConfig) ContentModerationLocalRulesConfig {
	out := cloneContentModerationLocalRulesConfig(cfg)
	out.BuiltinRules = ContentModerationBuiltinLocalRulePatterns()
	return out
}

func normalizeContentModerationLocalRuleNames(names []string) []string {
	if len(names) == 0 {
		return []string{}
	}
	seen := make(map[string]struct{}, len(names))
	out := make([]string, 0, len(names))
	for _, name := range names {
		name = trimRunes(strings.TrimSpace(name), maxContentModerationLocalRuleNameRunes)
		if name == "" {
			continue
		}
		key := strings.ToLower(name)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, name)
	}
	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i]) < strings.ToLower(out[j])
	})
	return out
}

func normalizeContentModerationLocalRulePatterns(patterns []ContentModerationLocalRulePattern) []ContentModerationLocalRulePattern {
	if len(patterns) == 0 {
		return []ContentModerationLocalRulePattern{}
	}
	out := make([]ContentModerationLocalRulePattern, 0, len(patterns))
	seen := map[string]struct{}{}
	for _, pattern := range patterns {
		pattern.Name = trimRunes(strings.TrimSpace(pattern.Name), maxContentModerationLocalRuleNameRunes)
		pattern.Pattern = trimRunes(strings.TrimSpace(pattern.Pattern), maxContentModerationLocalRulePatternRunes)
		pattern.Category = trimRunes(strings.TrimSpace(pattern.Category), maxContentModerationLocalRuleCategoryRunes)
		if pattern.Name == "" && pattern.Pattern == "" && pattern.Weight <= 0 {
			continue
		}
		key := strings.ToLower(pattern.Name)
		if key != "" {
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
		}
		out = append(out, pattern)
		if len(out) >= maxContentModerationLocalRulesCustomRules {
			break
		}
	}
	return out
}

func cloneContentModerationLocalRulePatterns(patterns []ContentModerationLocalRulePattern) []ContentModerationLocalRulePattern {
	if len(patterns) == 0 {
		return []ContentModerationLocalRulePattern{}
	}
	out := make([]ContentModerationLocalRulePattern, len(patterns))
	copy(out, patterns)
	return out
}

func validateContentModerationLocalRulesConfig(cfg ContentModerationLocalRulesConfig) error {
	action := strings.ToLower(strings.TrimSpace(cfg.Action))
	if action != "" && action != ContentModerationLocalRulesActionRecord && action != ContentModerationLocalRulesActionBlock {
		return infraerrors.BadRequest("INVALID_LOCAL_RULE_ACTION", "本地规则命中后动作无效")
	}
	scanScope := strings.ToLower(strings.TrimSpace(cfg.ScanScope))
	if scanScope != "" && scanScope != ContentModerationLocalRulesScanLatestUserInput && scanScope != ContentModerationLocalRulesScanFullTextContext {
		return infraerrors.BadRequest("INVALID_LOCAL_RULE_SCAN_SCOPE", "本地规则扫描范围无效")
	}
	cfg = normalizeContentModerationLocalRulesConfig(cfg)
	builtin := map[string]struct{}{}
	for _, pattern := range contentModerationLocalRuleBuiltinPatterns {
		builtin[strings.ToLower(pattern.Name)] = struct{}{}
	}
	seenCustom := map[string]struct{}{}
	for _, pattern := range cfg.CustomRules {
		if strings.TrimSpace(pattern.Name) == "" {
			return infraerrors.BadRequest("INVALID_LOCAL_RULE_NAME", "本地规则名称不能为空")
		}
		if _, ok := builtin[strings.ToLower(pattern.Name)]; ok {
			return infraerrors.BadRequest("INVALID_LOCAL_RULE_NAME", "自定义规则名称不能与内置规则重复")
		}
		key := strings.ToLower(pattern.Name)
		if _, ok := seenCustom[key]; ok {
			return infraerrors.BadRequest("INVALID_LOCAL_RULE_NAME", "自定义规则名称重复")
		}
		seenCustom[key] = struct{}{}
		if strings.TrimSpace(pattern.Pattern) == "" {
			return infraerrors.BadRequest("INVALID_LOCAL_RULE_PATTERN", "本地规则正则不能为空")
		}
		if pattern.Weight <= 0 || pattern.Weight > maxContentModerationLocalRulesThreshold {
			return infraerrors.BadRequest("INVALID_LOCAL_RULE_WEIGHT", "本地规则权重无效")
		}
		if _, err := regexp.Compile(pattern.Pattern); err != nil {
			return infraerrors.BadRequest("INVALID_LOCAL_RULE_PATTERN", fmt.Sprintf("本地规则正则无效: %s", pattern.Name))
		}
	}
	return nil
}

func (s *ContentModerationService) TestLocalRules(ctx context.Context, input TestContentModerationLocalRulesInput) (*ContentModerationLocalRulesTestResult, error) {
	if strings.TrimSpace(input.Text) == "" {
		return nil, infraerrors.BadRequest("INVALID_LOCAL_RULE_TEST_TEXT", "测试文本不能为空")
	}
	cfg, err := s.loadConfig(ctx)
	if err != nil {
		return nil, err
	}
	localCfg := cfg.LocalRules
	if input.Config != nil {
		localCfg = *input.Config
	}
	localCfg.Enabled = true
	localCfg = normalizeContentModerationLocalRulesConfig(localCfg)
	if err := validateContentModerationLocalRulesConfig(localCfg); err != nil {
		return nil, err
	}
	result := inspectContentModerationLocalRulesText(input.Text, localCfg)
	return &ContentModerationLocalRulesTestResult{
		Hit:             result.Hit,
		Action:          result.Action,
		Score:           result.Score,
		RawScore:        result.RawScore,
		StrictScore:     result.StrictScore,
		StrictHit:       result.StrictHit,
		Threshold:       result.Threshold,
		StrictThreshold: result.StrictThreshold,
		HighestCategory: result.HighestCategory,
		Matches:         result.Matches,
		TextPreview:     result.TextPreview,
		ContextPreview:  result.ContextPreview,
		ExtractedChars:  result.ExtractedChars,
	}, nil
}

func (s *ContentModerationService) checkLocalRules(input ContentModerationCheckInput, cfg *ContentModerationConfig, content ContentModerationInput) *ContentModerationLocalRuleResult {
	if cfg == nil || !cfg.LocalRules.Enabled {
		return nil
	}
	localCfg := normalizeContentModerationLocalRulesConfig(cfg.LocalRules)
	var text string
	switch localCfg.ScanScope {
	case ContentModerationLocalRulesScanFullTextContext:
		text = ExtractContentModerationFullTextContext(input.Protocol, input.Endpoint, input.Body, localCfg.MaxTextLength)
	default:
		text = content.Text
	}
	result := inspectContentModerationLocalRulesText(text, localCfg)
	result.ScanTextHash = contentModerationLocalRuleHash(text)
	if !result.Hit {
		return &result
	}
	return &result
}

func (result ContentModerationLocalRuleResult) normalizedScore() float64 {
	score := 0.0
	if result.Threshold > 0 && result.Score > 0 {
		score = float64(result.Score) / float64(result.Threshold)
	}
	if result.StrictThreshold > 0 && result.StrictScore > 0 {
		if strict := float64(result.StrictScore) / float64(result.StrictThreshold); strict > score {
			score = strict
		}
	}
	if score > 1 {
		return 1
	}
	return score
}

func (result ContentModerationLocalRuleResult) logDetail() ContentModerationLocalRuleLogDetail {
	return ContentModerationLocalRuleLogDetail{
		Source:          "codex2api",
		ScanScope:       result.ScanScope,
		Score:           result.Score,
		RawScore:        result.RawScore,
		StrictScore:     result.StrictScore,
		Threshold:       result.Threshold,
		StrictThreshold: result.StrictThreshold,
		StrictHit:       result.StrictHit,
		HighestCategory: result.HighestCategory,
		Matches:         append([]ContentModerationLocalRuleMatch(nil), result.Matches...),
		ContextPreview:  result.ContextPreview,
		ExtractedChars:  result.ExtractedChars,
	}
}

func contentModerationLocalRuleHighestCategory(matches []ContentModerationLocalRuleMatch) string {
	if len(matches) == 0 {
		return "local_rule"
	}
	best := matches[0]
	for _, match := range matches[1:] {
		if match.Weight > best.Weight {
			best = match
		}
	}
	if strings.TrimSpace(best.Category) == "" {
		return "local_rule"
	}
	return best.Category
}

func previewContentModerationLocalRuleText(text string, maxRunes int) string {
	text = strings.Join(strings.Fields(strings.TrimSpace(text)), " ")
	if maxRunes <= 0 {
		return ""
	}
	runes := []rune(text)
	if len(runes) <= maxRunes {
		return redactContentModerationSecrets(text)
	}
	return redactContentModerationSecrets(string(runes[:maxRunes]) + "...")
}

func contentModerationLocalRuleExtractedChars(text string) int {
	return utf8.RuneCountInString(text)
}

func contentModerationLocalRuleHash(text string) string {
	h := sha256.New()
	_, _ = h.Write([]byte("local_rule_text:"))
	_, _ = h.Write([]byte(text))
	return hex.EncodeToString(h.Sum(nil))
}
