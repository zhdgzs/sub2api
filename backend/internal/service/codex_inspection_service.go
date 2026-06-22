package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"time"
)

type CodexInspectionService struct {
	repo           CodexInspectionRepository
	accountRepo    AccountRepository
	settingService *SettingService
	probe          *OpenAICodexUsageProbe
	decisionEngine *CodexInspectionDecisionEngine

	mu               sync.Mutex
	runningCancels   map[int64]context.CancelFunc
	runningFullRunID int64
}

func NewCodexInspectionService(
	repo CodexInspectionRepository,
	accountRepo AccountRepository,
	settingService *SettingService,
	probe *OpenAICodexUsageProbe,
	decisionEngine *CodexInspectionDecisionEngine,
) *CodexInspectionService {
	return &CodexInspectionService{
		repo:           repo,
		accountRepo:    accountRepo,
		settingService: settingService,
		probe:          probe,
		decisionEngine: decisionEngine,
		runningCancels: map[int64]context.CancelFunc{},
	}
}

func DefaultCodexInspectionSettings() CodexInspectionSettings {
	return CodexInspectionSettings{
		Enabled: false,
		Schedule: CodexInspectionSchedule{
			Mode:            CodexInspectionScheduleModeInterval,
			IntervalMinutes: 60,
			TimePoints:      []string{},
			TimeZone:        codexInspectionDefaultTimeZone,
		},
		Target: CodexInspectionTargetConfig{
			OnlyOpenAIOAuth:      true,
			AccountIDs:           []int64{},
			GroupIDs:             []int64{},
			IncludeUnschedulable: true,
			IncludeError:         false,
			OnlyStaleMinutes:     0,
			SampleSize:           0,
		},
		Probe: CodexInspectionProbeConfig{
			Workers:            codexInspectionDefaultWorkers,
			TimeoutMS:          codexInspectionDefaultTimeoutMS,
			Retries:            0,
			MinIntervalMinutes: 30,
			UserAgent:          "",
		},
		Decision: CodexInspectionDecisionConfig{
			UsedPercentThreshold: 100,
			ShortWindowPolicy:    CodexInspectionActionKeep,
			LongWindowPolicy:     CodexInspectionActionDisable,
		},
		Actions: CodexInspectionActionConfig{
			AutoApply:       false,
			AllowEnable:     false,
			AllowDisable:    false,
			AllowMarkReauth: false,
			AllowDelete:     false,
		},
	}
}

func (s *CodexInspectionService) GetSettings(ctx context.Context) (CodexInspectionSettings, error) {
	settings := DefaultCodexInspectionSettings()
	if s == nil || s.settingService == nil || s.settingService.settingRepo == nil {
		return settings, nil
	}
	raw, err := s.settingService.settingRepo.GetValue(ctx, SettingKeyCodexInspectionConfig)
	if err != nil {
		if errors.Is(err, ErrSettingNotFound) {
			return settings, nil
		}
		return settings, fmt.Errorf("get codex inspection settings: %w", err)
	}
	if strings.TrimSpace(raw) == "" {
		return settings, nil
	}
	if err := json.Unmarshal([]byte(raw), &settings); err != nil {
		return DefaultCodexInspectionSettings(), fmt.Errorf("parse codex inspection settings: %w", err)
	}
	return normalizeCodexInspectionSettings(settings), nil
}

func (s *CodexInspectionService) UpdateSettings(ctx context.Context, settings CodexInspectionSettings) (CodexInspectionSettings, error) {
	normalized := normalizeCodexInspectionSettings(settings)
	if err := validateCodexInspectionSettings(normalized); err != nil {
		return normalized, err
	}
	if s == nil || s.settingService == nil || s.settingService.settingRepo == nil {
		return normalized, nil
	}
	data, err := json.Marshal(normalized)
	if err != nil {
		return normalized, fmt.Errorf("marshal codex inspection settings: %w", err)
	}
	if err := s.settingService.settingRepo.Set(ctx, SettingKeyCodexInspectionConfig, string(data)); err != nil {
		return normalized, fmt.Errorf("save codex inspection settings: %w", err)
	}
	return normalized, nil
}

func (s *CodexInspectionService) Overview(ctx context.Context) (*CodexInspectionOverview, error) {
	settings, err := s.GetSettings(ctx)
	if err != nil {
		return nil, err
	}
	latest, err := s.repo.GetLatestRun(ctx)
	if err != nil {
		return nil, err
	}
	running, err := s.repo.GetRunningRun(ctx)
	if err != nil {
		return nil, err
	}
	total, err := s.repo.CountOpenAIOAuthAccounts(ctx)
	if err != nil {
		return nil, err
	}
	disabled, err := s.repo.CountAccountsDisabledByInspection(ctx)
	if err != nil {
		return nil, err
	}
	items, err := s.loadAllLatestResults(ctx)
	if err != nil {
		return nil, err
	}
	threshold := settings.Decision.UsedPercentThreshold
	if threshold <= 0 {
		threshold = 100
	}
	overview := &CodexInspectionOverview{
		Settings:             settings,
		LatestRun:            latest,
		RunningRun:           running,
		TotalOpenAIOAuth:     total,
		DisabledByInspection: disabled,
	}
	for _, item := range items {
		if item.ProbeStatus == CodexInspectionProbeStatusFailed {
			overview.ProbeFailedAccounts++
		}
		if percentAtLeast(item.FiveHourUsedPercent, threshold) {
			overview.FiveHourFullAccounts++
		}
		if percentAtLeast(item.LongWindowUsedPercent, threshold) {
			overview.LongWindowFull++
		}
		switch item.RecommendedAction {
		case CodexInspectionActionReauth:
			overview.ReauthAccounts++
		case CodexInspectionActionDelete:
			overview.DeleteSuggested++
		case CodexInspectionActionKeep:
			if item.ProbeStatus == CodexInspectionProbeStatusSuccess &&
				!percentAtLeast(item.FiveHourUsedPercent, threshold) &&
				!percentAtLeast(item.LongWindowUsedPercent, threshold) {
				overview.HealthyAccounts++
			}
		}
	}
	return overview, nil
}

func (s *CodexInspectionService) StartRun(ctx context.Context, req CodexInspectionRunRequest) (*CodexInspectionRun, error) {
	if req.TriggerType == "" {
		req.TriggerType = CodexInspectionTriggerManual
	}
	settings, err := s.effectiveSettings(ctx, req.SettingsOverride)
	if err != nil {
		return nil, err
	}
	targetQuery := s.targetQueryFromRequest(req, settings)
	accounts, err := s.repo.ListTargetAccounts(ctx, targetQuery)
	if err != nil {
		return nil, err
	}

	isFullRun := req.TriggerType != CodexInspectionTriggerSingleAccount
	if isFullRun {
		if err := s.reserveFullRun(ctx); err != nil {
			return nil, err
		}
	}

	run := &CodexInspectionRun{
		TriggerType:      req.TriggerType,
		TriggerKey:       req.TriggerKey,
		Status:           CodexInspectionRunStatusRunning,
		TotalAccounts:    len(accounts),
		SettingsSnapshot: settings,
		StartedAt:        time.Now(),
	}
	if err := s.repo.CreateRun(ctx, run); err != nil {
		if isFullRun {
			s.releaseFullRun(0)
		}
		return nil, err
	}

	runCtx, cancel := context.WithCancel(context.Background())
	s.registerRunCancel(run.ID, cancel, isFullRun)
	go s.processRun(runCtx, run, accounts, req.ApplyActions)
	return run, nil
}

func (s *CodexInspectionService) ProbeAccount(ctx context.Context, accountID int64) (*CodexInspectionRunDetail, error) {
	if accountID <= 0 {
		return nil, ErrCodexInspectionInvalidRequest
	}
	settings, err := s.GetSettings(ctx)
	if err != nil {
		return nil, err
	}
	settings = normalizeCodexInspectionSettings(settings)
	query := CodexInspectionTargetQuery{
		AccountIDs:           []int64{accountID},
		IncludeUnschedulable: true,
		IncludeError:         true,
	}
	accounts, err := s.repo.ListTargetAccounts(ctx, query)
	if err != nil {
		return nil, err
	}
	if len(accounts) == 0 {
		return nil, ErrCodexInspectionPrecondition.WithMetadata(map[string]string{"account_id": fmt.Sprint(accountID)})
	}
	run := &CodexInspectionRun{
		TriggerType:      CodexInspectionTriggerSingleAccount,
		TriggerKey:       fmt.Sprint(accountID),
		Status:           CodexInspectionRunStatusRunning,
		TotalAccounts:    1,
		SettingsSnapshot: settings,
		StartedAt:        time.Now(),
	}
	if err := s.repo.CreateRun(ctx, run); err != nil {
		return nil, err
	}
	s.processRun(ctx, run, accounts, false)
	results, _, _ := s.repo.ListResults(ctx, CodexInspectionListResultsParams{RunID: run.ID, Page: 1, PageSize: 1})
	detail := &CodexInspectionRunDetail{Run: run, Results: results}
	if len(results) > 0 && results[0].ProbeStatus == CodexInspectionProbeStatusFailed {
		return detail, ErrCodexInspectionUpstreamProbeFail
	}
	return detail, nil
}

func (s *CodexInspectionService) CancelRun(ctx context.Context, runID int64) (*CodexInspectionRun, error) {
	if runID <= 0 {
		return nil, ErrCodexInspectionInvalidRequest
	}
	s.mu.Lock()
	cancel := s.runningCancels[runID]
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	run, err := s.repo.GetRun(ctx, runID)
	if err != nil {
		return nil, err
	}
	if run.Status == CodexInspectionRunStatusRunning && cancel == nil {
		now := time.Now()
		run.Status = CodexInspectionRunStatusCanceled
		run.FinishedAt = &now
		_ = s.repo.UpdateRun(ctx, run)
	}
	return run, nil
}

func (s *CodexInspectionService) ListRuns(ctx context.Context, params CodexInspectionListRunsParams) (*CodexInspectionRunsPage, error) {
	items, total, err := s.repo.ListRuns(ctx, params)
	if err != nil {
		return nil, err
	}
	return &CodexInspectionRunsPage{Items: items, Total: total}, nil
}

func (s *CodexInspectionService) GetRun(ctx context.Context, id int64) (*CodexInspectionRunDetail, error) {
	run, err := s.repo.GetRun(ctx, id)
	if err != nil {
		return nil, err
	}
	results, _, _ := s.repo.ListResults(ctx, CodexInspectionListResultsParams{RunID: id, Page: 1, PageSize: 10})
	logs, _, _ := s.repo.ListLogs(ctx, CodexInspectionListLogsParams{RunID: id, Limit: 20})
	return &CodexInspectionRunDetail{Run: run, Results: results, Logs: logs}, nil
}

func (s *CodexInspectionService) ListResults(ctx context.Context, params CodexInspectionListResultsParams) (*CodexInspectionResultsPage, error) {
	page, pageSize := normalizeCodexInspectionPage(params.Page, params.PageSize)
	params.Page, params.PageSize = page, pageSize
	items, total, err := s.repo.ListResults(ctx, params)
	if err != nil {
		return nil, err
	}
	return &CodexInspectionResultsPage{Items: items, Total: total, Page: page, PageSize: pageSize, Pages: codexInspectionPages(total, pageSize)}, nil
}

func (s *CodexInspectionService) ListLatestAccountResults(ctx context.Context, params CodexInspectionLatestResultsParams) (*CodexInspectionResultsPage, error) {
	page, pageSize := normalizeCodexInspectionPage(params.Page, params.PageSize)
	params.Page, params.PageSize = page, pageSize
	items, total, err := s.repo.ListLatestAccountResults(ctx, params)
	if err != nil {
		return nil, err
	}
	return &CodexInspectionResultsPage{Items: items, Total: total, Page: page, PageSize: pageSize, Pages: codexInspectionPages(total, pageSize)}, nil
}

func (s *CodexInspectionService) ListLogs(ctx context.Context, params CodexInspectionListLogsParams) (*CodexInspectionLogsPage, error) {
	items, total, err := s.repo.ListLogs(ctx, params)
	if err != nil {
		return nil, err
	}
	return &CodexInspectionLogsPage{Items: items, Total: total}, nil
}

func (s *CodexInspectionService) ExecuteActions(ctx context.Context, runID int64, req CodexInspectionActionRequest) ([]CodexInspectionActionOutcome, error) {
	if runID <= 0 || len(req.ResultIDs) == 0 {
		return nil, ErrCodexInspectionInvalidRequest
	}
	results, err := s.repo.GetResultsByIDs(ctx, req.ResultIDs)
	if err != nil {
		return nil, err
	}
	byID := make(map[int64]CodexInspectionResult, len(results))
	for _, result := range results {
		if result.RunID == runID {
			byID[result.ID] = result
		}
	}
	resolvedActions := make(map[int64]string, len(req.ResultIDs))
	for _, id := range req.ResultIDs {
		result, ok := byID[id]
		if !ok {
			continue
		}
		action := strings.TrimSpace(req.ActionOverride)
		if action == "" {
			action = result.RecommendedAction
		}
		if !validCodexInspectionAction(action) {
			return nil, ErrCodexInspectionInvalidRequest.WithMetadata(map[string]string{"field": "action_override"})
		}
		if action == CodexInspectionActionDelete && strings.TrimSpace(req.ConfirmationText) != codexInspectionDeleteConfirmation {
			return nil, ErrCodexInspectionInvalidRequest.WithMetadata(map[string]string{"field": "confirmation_text"})
		}
		resolvedActions[id] = action
	}
	outcomes := make([]CodexInspectionActionOutcome, 0, len(req.ResultIDs))
	for _, id := range req.ResultIDs {
		result, ok := byID[id]
		if !ok {
			outcomes = append(outcomes, CodexInspectionActionOutcome{ResultID: id, Status: CodexInspectionActionStatusSkipped, Message: "result not found in run"})
			continue
		}
		action := resolvedActions[id]
		status, message := s.executeAction(ctx, runID, result, action, req.Force, req.ConfirmationText, false)
		if err := s.repo.UpdateResultAction(ctx, result.ID, status, message); err != nil {
			status, message = CodexInspectionActionStatusFailed, err.Error()
		}
		accountID := result.AccountID
		_ = s.repo.InsertLog(ctx, &CodexInspectionLog{
			RunID:     runID,
			AccountID: &accountID,
			Level:     "info",
			Message:   "codex inspection action executed",
			Detail: map[string]any{
				"result_id": result.ID,
				"action":    action,
				"status":    status,
				"message":   message,
			},
		})
		outcomes = append(outcomes, CodexInspectionActionOutcome{ResultID: result.ID, AccountID: result.AccountID, Action: action, Status: status, Message: message})
	}
	return outcomes, nil
}

func (s *CodexInspectionService) processRun(ctx context.Context, run *CodexInspectionRun, accounts []*Account, applyActions bool) {
	if run == nil {
		return
	}
	defer s.unregisterRun(run.ID)
	_ = s.repo.InsertLog(context.Background(), &CodexInspectionLog{RunID: run.ID, Level: "info", Message: "codex inspection run started", Detail: map[string]any{"total_accounts": len(accounts)}})
	if len(accounts) == 0 {
		now := time.Now()
		run.Status = CodexInspectionRunStatusCompleted
		run.FinishedAt = &now
		_ = s.repo.UpdateRun(context.Background(), run)
		return
	}

	settings := normalizeCodexInspectionSettings(run.SettingsSnapshot)
	workers := settings.Probe.Workers
	jobs := make(chan *Account)
	var runMu sync.Mutex
	var wg sync.WaitGroup

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for account := range jobs {
				select {
				case <-ctx.Done():
					return
				default:
				}
				result := s.inspectAccount(ctx, run, account)
				if applyActions && s.shouldAutoApply(settings, result.RecommendedAction) {
					status, message := s.executeAction(ctx, run.ID, result, result.RecommendedAction, false, "", true)
					result.ActionStatus = status
					result.ActionError = message
				}
				if err := s.repo.InsertResult(ctx, &result); err != nil {
					_ = s.repo.InsertLog(context.Background(), &CodexInspectionLog{RunID: run.ID, Level: "error", Message: "failed to insert codex inspection result", Detail: map[string]any{"account_id": account.ID, "error": err.Error()}})
					continue
				}
				if result.ActionStatus != CodexInspectionActionStatusNone {
					_ = s.repo.UpdateResultAction(ctx, result.ID, result.ActionStatus, result.ActionError)
				}
				runMu.Lock()
				updateRunCounters(run, result)
				_ = s.repo.UpdateRun(context.Background(), run)
				runMu.Unlock()
			}
		}()
	}
sendLoop:
	for _, account := range accounts {
		select {
		case <-ctx.Done():
			break sendLoop
		case jobs <- account:
		}
	}
	close(jobs)
	wg.Wait()

	now := time.Now()
	runMu.Lock()
	if ctx.Err() != nil {
		run.Status = CodexInspectionRunStatusCanceled
	} else {
		run.Status = CodexInspectionRunStatusCompleted
	}
	run.FinishedAt = &now
	_ = s.repo.UpdateRun(context.Background(), run)
	runMu.Unlock()
	_ = s.repo.InsertLog(context.Background(), &CodexInspectionLog{RunID: run.ID, Level: "info", Message: "codex inspection run finished", Detail: map[string]any{"status": run.Status}})
}

func (s *CodexInspectionService) inspectAccount(ctx context.Context, run *CodexInspectionRun, account *Account) CodexInspectionResult {
	result := CodexInspectionResult{
		RunID:                 run.ID,
		AccountID:             account.ID,
		AccountName:           account.Name,
		AccountStatusSnapshot: account.Status,
		SchedulableSnapshot:   account.Schedulable,
		ProxyIDSnapshot:       account.ProxyID,
		ChatGPTAccountID:      account.GetChatGPTAccountID(),
		ProbeStatus:           CodexInspectionProbeStatusFailed,
		LongWindowType:        CodexInspectionLongWindowNone,
		RecommendedAction:     CodexInspectionActionKeep,
		ActionStatus:          CodexInspectionActionStatusNone,
		RawRateLimit:          map[string]any{},
		CreatedAt:             time.Now(),
	}
	outcome := s.probe.Probe(ctx, account, run.SettingsSnapshot.Probe)
	decision := s.decisionEngine.Decide(account, outcome, run.SettingsSnapshot.Decision)
	result.ProbeStatus = outcome.ProbeStatus
	result.UpstreamStatusCode = outcome.StatusCode
	result.LatencyMS = outcome.LatencyMS
	result.FiveHourUsedPercent = outcome.FiveHourUsedPercent
	result.LongWindowType = outcome.LongWindowType
	result.LongWindowUsedPercent = outcome.LongWindowUsedPercent
	result.RecommendedAction = decision.Action
	result.ActionReason = decision.Reason
	result.BodyExcerpt = outcome.BodyExcerpt
	result.RawRateLimit = outcome.RawRateLimit
	if outcome.Error != "" {
		result.ActionReason = strings.TrimSpace(result.ActionReason + "; " + outcome.Error)
	}
	if outcome.ProbeStatus == CodexInspectionProbeStatusSuccess {
		s.persistAccountSnapshot(ctx, account.ID, run.ID, outcome, decision)
	}
	return result
}

func (s *CodexInspectionService) persistAccountSnapshot(ctx context.Context, accountID, runID int64, outcome CodexInspectionProbeOutcome, decision CodexInspectionDecision) {
	updates := map[string]any{
		"codex_usage_updated_at":     time.Now().UTC().Format(time.RFC3339),
		codexInspectionLastRunIDKey:  runID,
		codexInspectionLastActionKey: decision.Action,
		codexInspectionLastReasonKey: decision.Reason,
	}
	if outcome.FiveHourUsedPercent != nil {
		updates["codex_5h_used_percent"] = *outcome.FiveHourUsedPercent
	}
	if outcome.FiveHourWindowMinutes != nil {
		updates["codex_5h_window_minutes"] = *outcome.FiveHourWindowMinutes
	}
	if outcome.LongWindowType == CodexInspectionLongWindowWeekly && outcome.LongWindowUsedPercent != nil {
		updates["codex_7d_used_percent"] = *outcome.LongWindowUsedPercent
	}
	if outcome.LongWindowType == CodexInspectionLongWindowWeekly && outcome.LongWindowMinutes != nil {
		updates["codex_7d_window_minutes"] = *outcome.LongWindowMinutes
	}
	if outcome.LongWindowType == CodexInspectionLongWindowMonthly && outcome.LongWindowUsedPercent != nil {
		updates["codex_30d_used_percent"] = *outcome.LongWindowUsedPercent
	}
	if outcome.LongWindowType == CodexInspectionLongWindowMonthly && outcome.LongWindowMinutes != nil {
		updates["codex_30d_window_minutes"] = *outcome.LongWindowMinutes
	}
	_ = s.accountRepo.UpdateExtra(ctx, accountID, updates)
}

func (s *CodexInspectionService) executeAction(ctx context.Context, runID int64, result CodexInspectionResult, action string, force bool, confirmationText string, automatic bool) (string, string) {
	action = strings.TrimSpace(action)
	if !validCodexInspectionAction(action) {
		return CodexInspectionActionStatusFailed, "invalid action"
	}
	if action == CodexInspectionActionKeep {
		return CodexInspectionActionStatusSkipped, "keep action has no side effect"
	}
	if automatic && action == CodexInspectionActionDelete {
		return CodexInspectionActionStatusNeedsReview, "delete is never executed automatically"
	}
	account, err := s.accountRepo.GetByID(ctx, result.AccountID)
	if err != nil {
		return CodexInspectionActionStatusFailed, err.Error()
	}
	switch action {
	case CodexInspectionActionEnable:
		if !inspectionDisabled(account) && !force {
			return CodexInspectionActionStatusSkipped, "account was not disabled by codex inspection"
		}
		if err := s.accountRepo.SetSchedulable(ctx, account.ID, true); err != nil {
			return CodexInspectionActionStatusFailed, err.Error()
		}
		if err := s.repo.ClearAccountExtraKeys(ctx, account.ID, []string{codexInspectionDisabledByRunIDKey}); err != nil {
			return CodexInspectionActionStatusFailed, err.Error()
		}
		_ = s.accountRepo.UpdateExtra(ctx, account.ID, map[string]any{codexInspectionLastActionKey: action, codexInspectionLastReasonKey: result.ActionReason})
		return CodexInspectionActionStatusSuccess, "account schedulable enabled"
	case CodexInspectionActionDisable:
		if err := s.accountRepo.SetSchedulable(ctx, account.ID, false); err != nil {
			return CodexInspectionActionStatusFailed, err.Error()
		}
		_ = s.accountRepo.UpdateExtra(ctx, account.ID, map[string]any{
			codexInspectionDisabledByRunIDKey: runID,
			codexInspectionLastActionKey:      action,
			codexInspectionLastReasonKey:      result.ActionReason,
		})
		return CodexInspectionActionStatusSuccess, "account schedulable disabled"
	case CodexInspectionActionReauth:
		if err := s.accountRepo.SetError(ctx, account.ID, codexInspectionReauthErrorMessage); err != nil {
			return CodexInspectionActionStatusFailed, err.Error()
		}
		_ = s.accountRepo.UpdateExtra(ctx, account.ID, map[string]any{codexInspectionLastActionKey: action, codexInspectionLastReasonKey: result.ActionReason})
		return CodexInspectionActionStatusSuccess, "account marked reauth required"
	case CodexInspectionActionDelete:
		if strings.TrimSpace(confirmationText) != codexInspectionDeleteConfirmation {
			return CodexInspectionActionStatusNeedsReview, "DELETE confirmation is required"
		}
		if err := s.accountRepo.Delete(ctx, account.ID); err != nil {
			return CodexInspectionActionStatusFailed, err.Error()
		}
		return CodexInspectionActionStatusSuccess, "account deleted"
	default:
		return CodexInspectionActionStatusFailed, "invalid action"
	}
}

func (s *CodexInspectionService) shouldAutoApply(settings CodexInspectionSettings, action string) bool {
	if !settings.Actions.AutoApply {
		return false
	}
	switch action {
	case CodexInspectionActionEnable:
		return settings.Actions.AllowEnable
	case CodexInspectionActionDisable:
		return settings.Actions.AllowDisable
	case CodexInspectionActionReauth:
		return settings.Actions.AllowMarkReauth
	case CodexInspectionActionDelete:
		return false
	default:
		return false
	}
}

func updateRunCounters(run *CodexInspectionRun, result CodexInspectionResult) {
	run.CompletedAccounts++
	if result.ProbeStatus == CodexInspectionProbeStatusSuccess {
		run.SuccessCount++
	} else {
		run.ErrorCount++
	}
	switch result.RecommendedAction {
	case CodexInspectionActionEnable:
		run.EnableCount++
	case CodexInspectionActionDisable:
		run.DisableCount++
	case CodexInspectionActionReauth:
		run.ReauthCount++
	case CodexInspectionActionDelete:
		run.DeleteCount++
	default:
		run.KeepCount++
	}
}

func (s *CodexInspectionService) effectiveSettings(ctx context.Context, override *CodexInspectionSettings) (CodexInspectionSettings, error) {
	if override != nil {
		settings := normalizeCodexInspectionSettings(*override)
		return settings, validateCodexInspectionSettings(settings)
	}
	settings, err := s.GetSettings(ctx)
	if err != nil {
		return settings, err
	}
	settings = normalizeCodexInspectionSettings(settings)
	return settings, validateCodexInspectionSettings(settings)
}

func (s *CodexInspectionService) targetQueryFromRequest(req CodexInspectionRunRequest, settings CodexInspectionSettings) CodexInspectionTargetQuery {
	accountIDs := dedupeInt64(append([]int64{}, settings.Target.AccountIDs...))
	accountIDs = dedupeInt64(append(accountIDs, req.AccountIDs...))
	groupIDs := dedupeInt64(append([]int64{}, settings.Target.GroupIDs...))
	groupIDs = dedupeInt64(append(groupIDs, req.Filters.GroupIDs...))
	includeUnschedulable := settings.Target.IncludeUnschedulable || req.Filters.IncludeUnschedulable
	includeError := settings.Target.IncludeError || req.Filters.IncludeError
	onlyStale := settings.Target.OnlyStaleMinutes
	if req.Filters.OnlyStaleMinutes > 0 {
		onlyStale = req.Filters.OnlyStaleMinutes
	}
	return CodexInspectionTargetQuery{
		AccountIDs:           accountIDs,
		GroupIDs:             groupIDs,
		IncludeUnschedulable: includeUnschedulable,
		IncludeError:         includeError,
		OnlyStaleMinutes:     onlyStale,
		SampleSize:           settings.Target.SampleSize,
	}
}

func (s *CodexInspectionService) reserveFullRun(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.runningFullRunID != 0 {
		return ErrCodexInspectionRunConflict
	}
	if running, err := s.repo.GetRunningRun(ctx); err == nil && running != nil {
		return ErrCodexInspectionRunConflict
	}
	s.runningFullRunID = -1
	return nil
}

func (s *CodexInspectionService) releaseFullRun(runID int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if runID == 0 || s.runningFullRunID == runID || s.runningFullRunID == -1 {
		s.runningFullRunID = 0
	}
}

func (s *CodexInspectionService) registerRunCancel(runID int64, cancel context.CancelFunc, isFullRun bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.runningCancels[runID] = cancel
	if isFullRun {
		s.runningFullRunID = runID
	}
}

func (s *CodexInspectionService) unregisterRun(runID int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if cancel := s.runningCancels[runID]; cancel != nil {
		delete(s.runningCancels, runID)
	}
	if s.runningFullRunID == runID {
		s.runningFullRunID = 0
	}
}

func (s *CodexInspectionService) loadAllLatestResults(ctx context.Context) ([]CodexInspectionResult, error) {
	const pageSize = 200
	out := make([]CodexInspectionResult, 0, pageSize)
	for page := 1; ; page++ {
		items, total, err := s.repo.ListLatestAccountResults(ctx, CodexInspectionLatestResultsParams{Page: page, PageSize: pageSize})
		if err != nil {
			return nil, err
		}
		out = append(out, items...)
		if len(out) >= int(total) || len(items) < pageSize {
			return out, nil
		}
	}
}

func normalizeCodexInspectionSettings(settings CodexInspectionSettings) CodexInspectionSettings {
	def := DefaultCodexInspectionSettings()
	if strings.TrimSpace(settings.Schedule.Mode) == "" {
		settings.Schedule.Mode = def.Schedule.Mode
	}
	if settings.Schedule.IntervalMinutes <= 0 {
		settings.Schedule.IntervalMinutes = def.Schedule.IntervalMinutes
	}
	if settings.Schedule.TimeZone == "" {
		settings.Schedule.TimeZone = def.Schedule.TimeZone
	}
	if settings.Schedule.TimePoints == nil {
		settings.Schedule.TimePoints = []string{}
	}
	settings.Target.OnlyOpenAIOAuth = true
	if settings.Target.AccountIDs == nil {
		settings.Target.AccountIDs = []int64{}
	}
	if settings.Target.GroupIDs == nil {
		settings.Target.GroupIDs = []int64{}
	}
	settings.Target.AccountIDs = dedupeInt64(settings.Target.AccountIDs)
	settings.Target.GroupIDs = dedupeInt64(settings.Target.GroupIDs)
	settings.Probe = normalizeProbeConfig(settings.Probe)
	if settings.Decision.UsedPercentThreshold <= 0 {
		settings.Decision.UsedPercentThreshold = def.Decision.UsedPercentThreshold
	}
	if settings.Decision.ShortWindowPolicy == "" {
		settings.Decision.ShortWindowPolicy = def.Decision.ShortWindowPolicy
	}
	if settings.Decision.LongWindowPolicy == "" {
		settings.Decision.LongWindowPolicy = def.Decision.LongWindowPolicy
	}
	settings.Actions.AllowDelete = false
	return settings
}

func normalizeProbeConfig(cfg CodexInspectionProbeConfig) CodexInspectionProbeConfig {
	if cfg.Workers <= 0 {
		cfg.Workers = codexInspectionDefaultWorkers
	}
	if cfg.Workers > codexInspectionMaxWorkers {
		cfg.Workers = codexInspectionMaxWorkers
	}
	if cfg.TimeoutMS <= 0 {
		cfg.TimeoutMS = codexInspectionDefaultTimeoutMS
	}
	if cfg.TimeoutMS < codexInspectionMinTimeoutMS {
		cfg.TimeoutMS = codexInspectionMinTimeoutMS
	}
	if cfg.TimeoutMS > codexInspectionMaxTimeoutMS {
		cfg.TimeoutMS = codexInspectionMaxTimeoutMS
	}
	if cfg.Retries < 0 {
		cfg.Retries = 0
	}
	if cfg.Retries > 3 {
		cfg.Retries = 3
	}
	if cfg.MinIntervalMinutes < 0 {
		cfg.MinIntervalMinutes = 0
	}
	cfg.UserAgent = strings.TrimSpace(cfg.UserAgent)
	return cfg
}

func validateCodexInspectionSettings(settings CodexInspectionSettings) error {
	switch settings.Schedule.Mode {
	case CodexInspectionScheduleModeInterval, CodexInspectionScheduleModeTimePoints:
	default:
		return ErrCodexInspectionInvalidRequest.WithMetadata(map[string]string{"field": "schedule.mode"})
	}
	if settings.Schedule.IntervalMinutes < 1 || settings.Schedule.IntervalMinutes > 10_080 {
		return ErrCodexInspectionInvalidRequest.WithMetadata(map[string]string{"field": "schedule.interval_minutes"})
	}
	if _, err := time.LoadLocation(settings.Schedule.TimeZone); err != nil {
		return ErrCodexInspectionInvalidRequest.WithMetadata(map[string]string{"field": "schedule.timezone"})
	}
	for _, point := range settings.Schedule.TimePoints {
		if _, err := time.Parse("15:04", strings.TrimSpace(point)); err != nil {
			return ErrCodexInspectionInvalidRequest.WithMetadata(map[string]string{"field": "schedule.time_points"})
		}
	}
	if settings.Decision.UsedPercentThreshold <= 0 || settings.Decision.UsedPercentThreshold > 1000 {
		return ErrCodexInspectionInvalidRequest.WithMetadata(map[string]string{"field": "decision.used_percent_threshold"})
	}
	if settings.Decision.ShortWindowPolicy != CodexInspectionActionKeep {
		return ErrCodexInspectionInvalidRequest.WithMetadata(map[string]string{"field": "decision.short_window_policy"})
	}
	switch settings.Decision.LongWindowPolicy {
	case CodexInspectionActionKeep, CodexInspectionActionDisable:
	default:
		return ErrCodexInspectionInvalidRequest.WithMetadata(map[string]string{"field": "decision.long_window_policy"})
	}
	return nil
}

func dedupeInt64(in []int64) []int64 {
	if len(in) == 0 {
		return []int64{}
	}
	seen := make(map[int64]struct{}, len(in))
	out := make([]int64, 0, len(in))
	for _, id := range in {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func normalizeCodexInspectionPage(page, pageSize int) (int, int) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = codexInspectionDefaultResultLimit
	}
	if pageSize > codexInspectionMaxResultPageSize {
		pageSize = codexInspectionMaxResultPageSize
	}
	return page, pageSize
}

func codexInspectionPages(total int64, pageSize int) int {
	if pageSize <= 0 {
		return 1
	}
	pages := int(math.Ceil(float64(total) / float64(pageSize)))
	if pages < 1 {
		return 1
	}
	return pages
}

func validCodexInspectionAction(action string) bool {
	switch action {
	case CodexInspectionActionKeep, CodexInspectionActionEnable, CodexInspectionActionDisable, CodexInspectionActionReauth, CodexInspectionActionDelete:
		return true
	default:
		return false
	}
}
