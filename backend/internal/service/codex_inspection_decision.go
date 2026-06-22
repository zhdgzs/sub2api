package service

import (
	"fmt"
	"math"
	"strings"
)

type CodexInspectionDecisionEngine struct{}

func NewCodexInspectionDecisionEngine() *CodexInspectionDecisionEngine {
	return &CodexInspectionDecisionEngine{}
}

func (e *CodexInspectionDecisionEngine) Decide(account *Account, probe CodexInspectionProbeOutcome, cfg CodexInspectionDecisionConfig) CodexInspectionDecision {
	threshold := cfg.UsedPercentThreshold
	if threshold <= 0 {
		threshold = 100
	}
	decision := CodexInspectionDecision{
		Action:         CodexInspectionActionKeep,
		Reason:         "no quota or auth rule matched",
		Confidence:     0.5,
		LongWindowType: probe.LongWindowType,
	}
	if probe.ProbeStatus != CodexInspectionProbeStatusSuccess || probe.StatusCode == nil {
		decision.Reason = "probe failed or upstream status is unavailable"
		decision.UnknownOrFailure = true
		return decision
	}

	body := strings.ToLower(probe.BodyText)
	statusCode := *probe.StatusCode
	if statusCode == 401 {
		switch {
		case strings.Contains(body, "expired"):
			return CodexInspectionDecision{Action: CodexInspectionActionReauth, Reason: "access token expired", Confidence: 0.9, LongWindowType: probe.LongWindowType}
		case strings.Contains(body, "invalidated") || strings.Contains(body, "unknown") || strings.Contains(body, "revoked"):
			return CodexInspectionDecision{Action: CodexInspectionActionDelete, Reason: "access token invalidated or unknown", Confidence: 0.85, LongWindowType: probe.LongWindowType}
		default:
			return CodexInspectionDecision{Action: CodexInspectionActionReauth, Reason: "upstream unauthorized", Confidence: 0.55, LongWindowType: probe.LongWindowType, UnknownOrFailure: true}
		}
	}
	if statusCode == 402 && strings.Contains(body, "deactivated_workspace") {
		return CodexInspectionDecision{Action: CodexInspectionActionDelete, Reason: "workspace is deactivated", Confidence: 0.9, LongWindowType: probe.LongWindowType}
	}

	fiveHourFull := percentAtLeast(probe.FiveHourUsedPercent, threshold) || windowLimitReached(probe.Windows, CodexInspectionLongWindowNone, true)
	longFull := percentAtLeast(probe.LongWindowUsedPercent, threshold) || windowLimitReached(probe.Windows, probe.LongWindowType, false)
	decision.FiveHourFull = fiveHourFull
	decision.LongWindowFull = longFull
	decision.IsQuotaIssue = fiveHourFull || longFull || statusCode == 402

	if longFull {
		action := cfg.LongWindowPolicy
		if action == "" {
			action = CodexInspectionActionDisable
		}
		return CodexInspectionDecision{
			Action:         action,
			Reason:         fmt.Sprintf("%s codex quota usage reached threshold %.2f%%", displayLongWindowType(probe.LongWindowType), threshold),
			Confidence:     0.85,
			IsQuotaIssue:   true,
			LongWindowType: probe.LongWindowType,
			FiveHourFull:   fiveHourFull,
			LongWindowFull: true,
		}
	}

	if inspectionDisabled(account) && longWindowRecovered(probe) {
		return CodexInspectionDecision{
			Action:         CodexInspectionActionEnable,
			Reason:         "long codex quota window recovered for inspection-disabled account",
			Confidence:     0.8,
			IsQuotaIssue:   false,
			LongWindowType: probe.LongWindowType,
			FiveHourFull:   fiveHourFull,
		}
	}

	if fiveHourFull {
		decision.Action = CodexInspectionActionKeep
		decision.Reason = "5h codex quota reached threshold; long window is still available"
		decision.Confidence = 0.8
		return decision
	}

	if statusCode >= 400 {
		decision.Reason = fmt.Sprintf("upstream returned status %d without a safe action rule", statusCode)
		decision.UnknownOrFailure = true
		return decision
	}

	decision.Reason = "codex quota and auth state look healthy"
	decision.Confidence = 0.8
	return decision
}

func longWindowRecovered(probe CodexInspectionProbeOutcome) bool {
	return probe.LongWindowType != CodexInspectionLongWindowNone && probe.LongWindowUsedPercent != nil
}

func inspectionDisabled(account *Account) bool {
	if account == nil || len(account.Extra) == 0 {
		return false
	}
	v, ok := account.Extra[codexInspectionDisabledByRunIDKey]
	if !ok || v == nil {
		return false
	}
	return strings.TrimSpace(fmt.Sprint(v)) != ""
}

func percentAtLeast(value *float64, threshold float64) bool {
	if value == nil {
		return false
	}
	return !math.IsNaN(*value) && *value >= threshold
}

func windowLimitReached(windows []CodexInspectionRateLimitWindow, longType string, fiveHour bool) bool {
	for _, w := range windows {
		if !w.LimitReached {
			continue
		}
		classified := classifyCodexInspectionWindow(w.WindowSeconds, w.WindowMinutes)
		if fiveHour && classified == "5h" {
			return true
		}
		if !fiveHour && classified == longType && longType != CodexInspectionLongWindowNone {
			return true
		}
	}
	return false
}

func displayLongWindowType(t string) string {
	switch t {
	case CodexInspectionLongWindowWeekly:
		return "weekly"
	case CodexInspectionLongWindowMonthly:
		return "monthly"
	case CodexInspectionLongWindowGeneric:
		return "long"
	default:
		return "long"
	}
}

func classifyCodexInspectionWindow(seconds, minutes int) string {
	if seconds <= 0 && minutes > 0 {
		seconds = minutes * 60
	}
	if minutes <= 0 && seconds > 0 {
		minutes = seconds / 60
	}
	switch {
	case seconds == codexInspectionFiveHourSeconds || minutes == 300:
		return "5h"
	case seconds == codexInspectionWeekSeconds || minutes == 10_080:
		return CodexInspectionLongWindowWeekly
	case seconds == codexInspectionMonthSeconds || minutes == 43_200:
		return CodexInspectionLongWindowMonthly
	case seconds > codexInspectionFiveHourSeconds || minutes > 300:
		return CodexInspectionLongWindowGeneric
	default:
		return CodexInspectionLongWindowNone
	}
}

func fillCodexInspectionProbeWindowSummary(out *CodexInspectionProbeOutcome) {
	if out == nil {
		return
	}
	out.LongWindowType = CodexInspectionLongWindowNone
	var weekly, monthly, generic *CodexInspectionRateLimitWindow
	for i := range out.Windows {
		w := &out.Windows[i]
		switch classifyCodexInspectionWindow(w.WindowSeconds, w.WindowMinutes) {
		case "5h":
			if out.FiveHourUsedPercent == nil {
				out.FiveHourUsedPercent = w.UsedPercent
			}
			if out.FiveHourWindowMinutes == nil && w.WindowMinutes > 0 {
				v := w.WindowMinutes
				out.FiveHourWindowMinutes = &v
			}
		case CodexInspectionLongWindowWeekly:
			if weekly == nil {
				weekly = w
			}
		case CodexInspectionLongWindowMonthly:
			if monthly == nil {
				monthly = w
			}
		case CodexInspectionLongWindowGeneric:
			if generic == nil {
				generic = w
			}
		}
	}
	selectedType, selected := CodexInspectionLongWindowNone, (*CodexInspectionRateLimitWindow)(nil)
	if weekly != nil {
		selectedType, selected = CodexInspectionLongWindowWeekly, weekly
	} else if monthly != nil {
		selectedType, selected = CodexInspectionLongWindowMonthly, monthly
	} else if generic != nil {
		selectedType, selected = CodexInspectionLongWindowGeneric, generic
	}
	if selected == nil {
		out.LongWindowType = CodexInspectionLongWindowNone
		return
	}
	out.LongWindowType = selectedType
	out.LongWindowUsedPercent = selected.UsedPercent
	if selected.WindowMinutes > 0 {
		v := selected.WindowMinutes
		out.LongWindowMinutes = &v
	}
}
