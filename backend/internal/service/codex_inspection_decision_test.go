package service

import "testing"

func TestClassifyCodexInspectionWindow(t *testing.T) {
	tests := []struct {
		name    string
		seconds int
		minutes int
		want    string
	}{
		{name: "five hour seconds", seconds: 18000, want: "5h"},
		{name: "five hour minutes", minutes: 300, want: "5h"},
		{name: "weekly seconds", seconds: 604800, want: CodexInspectionLongWindowWeekly},
		{name: "monthly minutes", minutes: 43200, want: CodexInspectionLongWindowMonthly},
		{name: "generic long", minutes: 1440, want: CodexInspectionLongWindowGeneric},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := classifyCodexInspectionWindow(tt.seconds, tt.minutes); got != tt.want {
				t.Fatalf("classify = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestCodexInspectionDecisionAuthAndQuota(t *testing.T) {
	engine := NewCodexInspectionDecisionEngine()
	cfg := CodexInspectionDecisionConfig{UsedPercentThreshold: 100}

	status401 := 401
	status402 := 402
	status200 := 200
	tests := []struct {
		name    string
		account *Account
		probe   CodexInspectionProbeOutcome
		want    string
	}{
		{
			name:  "expired token reauth",
			probe: CodexInspectionProbeOutcome{ProbeStatus: CodexInspectionProbeStatusSuccess, StatusCode: &status401, BodyText: "token expired"},
			want:  CodexInspectionActionReauth,
		},
		{
			name:  "invalidated token delete",
			probe: CodexInspectionProbeOutcome{ProbeStatus: CodexInspectionProbeStatusSuccess, StatusCode: &status401, BodyText: "token invalidated"},
			want:  CodexInspectionActionDelete,
		},
		{
			name:  "deactivated workspace delete",
			probe: CodexInspectionProbeOutcome{ProbeStatus: CodexInspectionProbeStatusSuccess, StatusCode: &status402, BodyText: "deactivated_workspace"},
			want:  CodexInspectionActionDelete,
		},
		{
			name: "long window full disables",
			probe: CodexInspectionProbeOutcome{
				ProbeStatus:           CodexInspectionProbeStatusSuccess,
				StatusCode:            &status200,
				LongWindowType:        CodexInspectionLongWindowWeekly,
				LongWindowUsedPercent: floatPtr(100),
			},
			want: CodexInspectionActionDisable,
		},
		{
			name: "only five hour full keeps",
			probe: CodexInspectionProbeOutcome{
				ProbeStatus:         CodexInspectionProbeStatusSuccess,
				StatusCode:          &status200,
				FiveHourUsedPercent: floatPtr(100),
				LongWindowType:      CodexInspectionLongWindowWeekly,
			},
			want: CodexInspectionActionKeep,
		},
		{
			name:    "inspection disabled recovered enables",
			account: &Account{Extra: map[string]any{codexInspectionDisabledByRunIDKey: int64(12)}},
			probe: CodexInspectionProbeOutcome{
				ProbeStatus:           CodexInspectionProbeStatusSuccess,
				StatusCode:            &status200,
				LongWindowType:        CodexInspectionLongWindowWeekly,
				LongWindowUsedPercent: floatPtr(20),
			},
			want: CodexInspectionActionEnable,
		},
		{
			name:    "inspection disabled without long window keeps",
			account: &Account{Extra: map[string]any{codexInspectionDisabledByRunIDKey: int64(12)}},
			probe: CodexInspectionProbeOutcome{
				ProbeStatus:    CodexInspectionProbeStatusSuccess,
				StatusCode:     &status200,
				LongWindowType: CodexInspectionLongWindowNone,
			},
			want: CodexInspectionActionKeep,
		},
		{
			name:  "probe failed keeps",
			probe: CodexInspectionProbeOutcome{ProbeStatus: CodexInspectionProbeStatusFailed},
			want:  CodexInspectionActionKeep,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := engine.Decide(tt.account, tt.probe, cfg).Action; got != tt.want {
				t.Fatalf("action = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestCodexInspectionDecisionLongWindowPolicyKeep(t *testing.T) {
	engine := NewCodexInspectionDecisionEngine()
	status200 := 200

	decision := engine.Decide(nil, CodexInspectionProbeOutcome{
		ProbeStatus:           CodexInspectionProbeStatusSuccess,
		StatusCode:            &status200,
		LongWindowType:        CodexInspectionLongWindowWeekly,
		LongWindowUsedPercent: floatPtr(100),
	}, CodexInspectionDecisionConfig{UsedPercentThreshold: 100, LongWindowPolicy: CodexInspectionActionKeep})

	if decision.Action != CodexInspectionActionKeep {
		t.Fatalf("action = %s, want %s", decision.Action, CodexInspectionActionKeep)
	}
	if !decision.LongWindowFull || !decision.IsQuotaIssue {
		t.Fatalf("expected long window quota issue to remain true")
	}
}

func floatPtr(v float64) *float64 {
	return &v
}
