package service

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestCodexInspectionExecuteActionsDeleteRequiresConfirmation(t *testing.T) {
	repo := &codexInspectionRepoStub{
		results: []CodexInspectionResult{{
			ID:                101,
			RunID:             7,
			AccountID:         33,
			RecommendedAction: CodexInspectionActionDelete,
		}},
	}
	svc := &CodexInspectionService{repo: repo}

	outcomes, err := svc.ExecuteActions(context.Background(), 7, CodexInspectionActionRequest{ResultIDs: []int64{101}})
	if !errors.Is(err, ErrCodexInspectionInvalidRequest) {
		t.Fatalf("err = %v, want %v", err, ErrCodexInspectionInvalidRequest)
	}
	if len(outcomes) != 0 {
		t.Fatalf("outcomes = %#v, want none", outcomes)
	}
	if repo.updateActionCalls != 0 {
		t.Fatalf("UpdateResultAction calls = %d, want 0", repo.updateActionCalls)
	}
}

func TestCodexInspectionShouldAutoApplyNeverDeletes(t *testing.T) {
	svc := &CodexInspectionService{}
	settings := DefaultCodexInspectionSettings()
	settings.Actions.AutoApply = true
	settings.Actions.AllowDelete = true

	if svc.shouldAutoApply(settings, CodexInspectionActionDelete) {
		t.Fatal("delete must never be auto-applied")
	}
}

func TestCodexInspectionEnableRequiresInspectionMarker(t *testing.T) {
	accountRepo := &codexInspectionAccountRepoStub{
		accounts: map[int64]*Account{
			33: {ID: 33, Extra: map[string]any{}},
		},
	}
	svc := &CodexInspectionService{accountRepo: accountRepo}

	status, message := svc.executeAction(context.Background(), 7, CodexInspectionResult{AccountID: 33}, CodexInspectionActionEnable, false, "", false)
	if status != CodexInspectionActionStatusSkipped {
		t.Fatalf("status = %s, want %s", status, CodexInspectionActionStatusSkipped)
	}
	if !strings.Contains(message, "not disabled by codex inspection") {
		t.Fatalf("message = %q, want inspection marker explanation", message)
	}
	if accountRepo.setSchedulableCalls != 0 {
		t.Fatalf("SetSchedulable calls = %d, want 0", accountRepo.setSchedulableCalls)
	}
}

func TestCodexInspectionAutomaticDeleteNeedsReview(t *testing.T) {
	accountRepo := &codexInspectionAccountRepoStub{}
	svc := &CodexInspectionService{accountRepo: accountRepo}

	status, _ := svc.executeAction(context.Background(), 7, CodexInspectionResult{AccountID: 33}, CodexInspectionActionDelete, false, codexInspectionDeleteConfirmation, true)
	if status != CodexInspectionActionStatusNeedsReview {
		t.Fatalf("status = %s, want %s", status, CodexInspectionActionStatusNeedsReview)
	}
	if accountRepo.deleteCalls != 0 {
		t.Fatalf("Delete calls = %d, want 0", accountRepo.deleteCalls)
	}
}

type codexInspectionRepoStub struct {
	CodexInspectionRepository
	results           []CodexInspectionResult
	updateActionCalls int
}

func (r *codexInspectionRepoStub) GetResultsByIDs(context.Context, []int64) ([]CodexInspectionResult, error) {
	return r.results, nil
}

func (r *codexInspectionRepoStub) UpdateResultAction(context.Context, int64, string, string) error {
	r.updateActionCalls++
	return nil
}

func (r *codexInspectionRepoStub) InsertLog(context.Context, *CodexInspectionLog) error {
	return nil
}

type codexInspectionAccountRepoStub struct {
	AccountRepository
	accounts             map[int64]*Account
	setSchedulableCalls  int
	deleteCalls          int
	setErrorCalls        int
	updateExtraCallCount int
}

func (r *codexInspectionAccountRepoStub) GetByID(_ context.Context, id int64) (*Account, error) {
	if account, ok := r.accounts[id]; ok {
		return account, nil
	}
	return nil, ErrAccountNotFound
}

func (r *codexInspectionAccountRepoStub) SetSchedulable(_ context.Context, _ int64, _ bool) error {
	r.setSchedulableCalls++
	return nil
}

func (r *codexInspectionAccountRepoStub) Delete(_ context.Context, _ int64) error {
	r.deleteCalls++
	return nil
}

func (r *codexInspectionAccountRepoStub) SetError(_ context.Context, _ int64, _ string) error {
	r.setErrorCalls++
	return nil
}

func (r *codexInspectionAccountRepoStub) UpdateExtra(_ context.Context, _ int64, _ map[string]any) error {
	r.updateExtraCallCount++
	return nil
}
