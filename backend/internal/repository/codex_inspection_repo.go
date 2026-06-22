package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	dbaccount "github.com/Wei-Shaw/sub2api/ent/account"
	dbaccountgroup "github.com/Wei-Shaw/sub2api/ent/accountgroup"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"
)

type codexInspectionRepository struct {
	client *dbent.Client
	db     *sql.DB
}

func NewCodexInspectionRepository(client *dbent.Client, db *sql.DB) service.CodexInspectionRepository {
	return &codexInspectionRepository{client: client, db: db}
}

func (r *codexInspectionRepository) CreateRun(ctx context.Context, run *service.CodexInspectionRun) error {
	if run == nil {
		return service.ErrCodexInspectionInvalidRequest
	}
	settingsJSON, err := json.Marshal(run.SettingsSnapshot)
	if err != nil {
		return fmt.Errorf("marshal codex inspection settings: %w", err)
	}
	row := r.db.QueryRowContext(ctx, `
		INSERT INTO codex_inspection_runs (
			trigger_type, trigger_key, status, total_accounts, completed_accounts,
			success_count, error_count, keep_count, enable_count, disable_count,
			reauth_count, delete_count, settings_snapshot, started_at, error_message
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13::jsonb,COALESCE($14::timestamptz,NOW()),$15)
		RETURNING id, started_at`,
		run.TriggerType, run.TriggerKey, emptyDefault(run.Status, service.CodexInspectionRunStatusRunning),
		run.TotalAccounts, run.CompletedAccounts, run.SuccessCount, run.ErrorCount, run.KeepCount,
		run.EnableCount, run.DisableCount, run.ReauthCount, run.DeleteCount, string(settingsJSON),
		nullableTimeValue(run.StartedAt), run.ErrorMessage,
	)
	return row.Scan(&run.ID, &run.StartedAt)
}

func (r *codexInspectionRepository) UpdateRun(ctx context.Context, run *service.CodexInspectionRun) error {
	if run == nil || run.ID <= 0 {
		return service.ErrCodexInspectionInvalidRequest
	}
	settingsJSON, err := json.Marshal(run.SettingsSnapshot)
	if err != nil {
		return fmt.Errorf("marshal codex inspection settings: %w", err)
	}
	_, err = r.db.ExecContext(ctx, `
		UPDATE codex_inspection_runs
		SET trigger_type=$2, trigger_key=$3, status=$4, total_accounts=$5, completed_accounts=$6,
			success_count=$7, error_count=$8, keep_count=$9, enable_count=$10, disable_count=$11,
			reauth_count=$12, delete_count=$13, settings_snapshot=$14::jsonb,
			started_at=$15, finished_at=$16, error_message=$17
		WHERE id=$1`,
		run.ID, run.TriggerType, run.TriggerKey, run.Status, run.TotalAccounts, run.CompletedAccounts,
		run.SuccessCount, run.ErrorCount, run.KeepCount, run.EnableCount, run.DisableCount,
		run.ReauthCount, run.DeleteCount, string(settingsJSON), run.StartedAt, run.FinishedAt, run.ErrorMessage,
	)
	if err != nil {
		return fmt.Errorf("update codex inspection run: %w", err)
	}
	return nil
}

func (r *codexInspectionRepository) GetRun(ctx context.Context, id int64) (*service.CodexInspectionRun, error) {
	row := r.db.QueryRowContext(ctx, codexInspectionRunSelectSQL+` WHERE id=$1`, id)
	run, err := scanCodexInspectionRun(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, service.ErrCodexInspectionRunNotFound
	}
	return run, err
}

func (r *codexInspectionRepository) GetLatestRun(ctx context.Context) (*service.CodexInspectionRun, error) {
	row := r.db.QueryRowContext(ctx, codexInspectionRunSelectSQL+` ORDER BY started_at DESC, id DESC LIMIT 1`)
	run, err := scanCodexInspectionRun(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return run, err
}

func (r *codexInspectionRepository) GetRunningRun(ctx context.Context) (*service.CodexInspectionRun, error) {
	row := r.db.QueryRowContext(ctx, codexInspectionRunSelectSQL+` WHERE status=$1 ORDER BY started_at DESC, id DESC LIMIT 1`, service.CodexInspectionRunStatusRunning)
	run, err := scanCodexInspectionRun(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return run, err
}

func (r *codexInspectionRepository) ListRuns(ctx context.Context, params service.CodexInspectionListRunsParams) ([]service.CodexInspectionRun, int64, error) {
	limit, offset := normalizeLimitOffset(params.Limit, params.Offset, 50, 200)
	where, args := []string{"1=1"}, []any{}
	if strings.TrimSpace(params.Status) != "" {
		args = append(args, strings.TrimSpace(params.Status))
		where = append(where, fmt.Sprintf("status=$%d", len(args)))
	}
	if strings.TrimSpace(params.TriggerType) != "" {
		args = append(args, strings.TrimSpace(params.TriggerType))
		where = append(where, fmt.Sprintf("trigger_type=$%d", len(args)))
	}
	whereSQL := " WHERE " + strings.Join(where, " AND ")
	var total int64
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM codex_inspection_runs`+whereSQL, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count codex inspection runs: %w", err)
	}
	args = append(args, limit, offset)
	rows, err := r.db.QueryContext(ctx, codexInspectionRunSelectSQL+whereSQL+fmt.Sprintf(" ORDER BY started_at DESC, id DESC LIMIT $%d OFFSET $%d", len(args)-1, len(args)), args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list codex inspection runs: %w", err)
	}
	defer rows.Close()
	items := make([]service.CodexInspectionRun, 0, limit)
	for rows.Next() {
		run, err := scanCodexInspectionRun(rows)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, *run)
	}
	return items, total, rows.Err()
}

func (r *codexInspectionRepository) InsertResult(ctx context.Context, result *service.CodexInspectionResult) error {
	if result == nil {
		return service.ErrCodexInspectionInvalidRequest
	}
	rawJSON, err := json.Marshal(nonNilMap(result.RawRateLimit))
	if err != nil {
		return fmt.Errorf("marshal raw rate limit: %w", err)
	}
	row := r.db.QueryRowContext(ctx, `
		INSERT INTO codex_inspection_results (
			run_id, account_id, account_name, account_status_snapshot, schedulable_snapshot,
			proxy_id_snapshot, chatgpt_account_id, probe_status, upstream_status_code,
			latency_ms, five_hour_used_percent, long_window_type, long_window_used_percent,
			recommended_action, action_reason, action_status, action_error, body_excerpt,
			raw_rate_limit, created_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19::jsonb,COALESCE($20::timestamptz,NOW()))
		RETURNING id, created_at`,
		result.RunID, result.AccountID, result.AccountName, result.AccountStatusSnapshot,
		result.SchedulableSnapshot, result.ProxyIDSnapshot, result.ChatGPTAccountID,
		result.ProbeStatus, result.UpstreamStatusCode, result.LatencyMS, result.FiveHourUsedPercent,
		emptyDefault(result.LongWindowType, service.CodexInspectionLongWindowNone), result.LongWindowUsedPercent,
		emptyDefault(result.RecommendedAction, service.CodexInspectionActionKeep), result.ActionReason,
		emptyDefault(result.ActionStatus, service.CodexInspectionActionStatusNone), result.ActionError,
		result.BodyExcerpt, string(rawJSON), nullableTimeValue(result.CreatedAt),
	)
	return row.Scan(&result.ID, &result.CreatedAt)
}

func (r *codexInspectionRepository) UpdateResultAction(ctx context.Context, resultID int64, status, actionError string) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE codex_inspection_results
		SET action_status=$2, action_error=$3
		WHERE id=$1`, resultID, status, actionError)
	if err != nil {
		return fmt.Errorf("update codex inspection result action: %w", err)
	}
	return nil
}

func (r *codexInspectionRepository) ListResults(ctx context.Context, params service.CodexInspectionListResultsParams) ([]service.CodexInspectionResult, int64, error) {
	where, args := codexInspectionResultFilters(params.RunID, params.Action, params.ProbeStatus, params.AccountStatus, params.QuotaWindow, params.Search, params.GroupIDs, params.OnlyStaleMinutes)
	page, pageSize := normalizePage(params.Page, params.PageSize, 50, 200)
	return r.listResultsWithWhere(ctx, where, args, page, pageSize, "created_at DESC, id DESC", "codex_inspection_results")
}

func (r *codexInspectionRepository) ListLatestAccountResults(ctx context.Context, params service.CodexInspectionLatestResultsParams) ([]service.CodexInspectionResult, int64, error) {
	where, args := codexInspectionResultFilters(0, params.Action, params.ProbeStatus, params.AccountStatus, params.QuotaWindow, params.Search, params.GroupIDs, params.OnlyStaleMinutes)
	page, pageSize := normalizePage(params.Page, params.PageSize, 50, 200)
	return r.listResultsWithWhere(ctx, where, args, page, pageSize, "account_name ASC, account_id ASC", `
		(
			SELECT DISTINCT ON (account_id) *
			FROM codex_inspection_results
			ORDER BY account_id, created_at DESC, id DESC
		) AS codex_inspection_results`)
}

func (r *codexInspectionRepository) listResultsWithWhere(ctx context.Context, where []string, args []any, page, pageSize int, orderBy, fromSQL string) ([]service.CodexInspectionResult, int64, error) {
	whereSQL := " WHERE " + strings.Join(where, " AND ")
	var total int64
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+fromSQL+whereSQL, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count codex inspection results: %w", err)
	}
	offset := (page - 1) * pageSize
	args = append(args, pageSize, offset)
	rows, err := r.db.QueryContext(ctx, codexInspectionResultSelectSQL+` FROM `+fromSQL+whereSQL+fmt.Sprintf(" ORDER BY %s LIMIT $%d OFFSET $%d", orderBy, len(args)-1, len(args)), args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list codex inspection results: %w", err)
	}
	defer rows.Close()
	items := make([]service.CodexInspectionResult, 0, pageSize)
	for rows.Next() {
		result, err := scanCodexInspectionResult(rows)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, *result)
	}
	return items, total, rows.Err()
}

func (r *codexInspectionRepository) GetResultsByIDs(ctx context.Context, ids []int64) ([]service.CodexInspectionResult, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	rows, err := r.db.QueryContext(ctx, codexInspectionResultSelectSQL+` FROM codex_inspection_results WHERE id = ANY($1) ORDER BY id`, pq.Array(ids))
	if err != nil {
		return nil, fmt.Errorf("get codex inspection results: %w", err)
	}
	defer rows.Close()
	items := make([]service.CodexInspectionResult, 0, len(ids))
	for rows.Next() {
		result, err := scanCodexInspectionResult(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, *result)
	}
	return items, rows.Err()
}

func (r *codexInspectionRepository) InsertLog(ctx context.Context, logRow *service.CodexInspectionLog) error {
	if logRow == nil {
		return nil
	}
	detailJSON, err := json.Marshal(nonNilMap(logRow.Detail))
	if err != nil {
		return fmt.Errorf("marshal codex inspection log detail: %w", err)
	}
	row := r.db.QueryRowContext(ctx, `
		INSERT INTO codex_inspection_logs (run_id, account_id, level, message, detail, created_at)
		VALUES ($1,$2,$3,$4,$5::jsonb,COALESCE($6::timestamptz,NOW()))
		RETURNING id, created_at`,
		logRow.RunID, logRow.AccountID, emptyDefault(logRow.Level, "info"), logRow.Message, string(detailJSON), nullableTimeValue(logRow.CreatedAt),
	)
	return row.Scan(&logRow.ID, &logRow.CreatedAt)
}

func (r *codexInspectionRepository) ListLogs(ctx context.Context, params service.CodexInspectionListLogsParams) ([]service.CodexInspectionLog, int64, error) {
	limit, offset := normalizeLimitOffset(params.Limit, params.Offset, 50, 500)
	where, args := []string{"1=1"}, []any{}
	if params.RunID > 0 {
		args = append(args, params.RunID)
		where = append(where, fmt.Sprintf("run_id=$%d", len(args)))
	}
	if params.AccountID > 0 {
		args = append(args, params.AccountID)
		where = append(where, fmt.Sprintf("account_id=$%d", len(args)))
	}
	if strings.TrimSpace(params.Level) != "" {
		args = append(args, strings.TrimSpace(params.Level))
		where = append(where, fmt.Sprintf("level=$%d", len(args)))
	}
	whereSQL := " WHERE " + strings.Join(where, " AND ")
	var total int64
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM codex_inspection_logs`+whereSQL, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count codex inspection logs: %w", err)
	}
	args = append(args, limit, offset)
	rows, err := r.db.QueryContext(ctx, codexInspectionLogSelectSQL+whereSQL+fmt.Sprintf(" ORDER BY created_at DESC, id DESC LIMIT $%d OFFSET $%d", len(args)-1, len(args)), args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list codex inspection logs: %w", err)
	}
	defer rows.Close()
	items := make([]service.CodexInspectionLog, 0, limit)
	for rows.Next() {
		item, err := scanCodexInspectionLog(rows)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, *item)
	}
	return items, total, rows.Err()
}

func (r *codexInspectionRepository) ListTargetAccounts(ctx context.Context, query service.CodexInspectionTargetQuery) ([]*service.Account, error) {
	q := r.client.Account.Query().
		Where(
			dbaccount.DeletedAtIsNil(),
			dbaccount.PlatformEQ(service.PlatformOpenAI),
			dbaccount.TypeEQ(service.AccountTypeOAuth),
		)
	if len(query.AccountIDs) > 0 {
		q = q.Where(dbaccount.IDIn(query.AccountIDs...))
	}
	if len(query.GroupIDs) > 0 {
		q = q.Where(dbaccount.HasAccountGroupsWith(dbaccountgroup.GroupIDIn(query.GroupIDs...)))
	}
	if !query.IncludeUnschedulable {
		q = q.Where(dbaccount.SchedulableEQ(true))
	}
	if query.IncludeError {
		q = q.Where(dbaccount.StatusIn(service.StatusActive, service.StatusError))
	} else {
		q = q.Where(dbaccount.StatusEQ(service.StatusActive))
	}
	rows, err := q.Order(dbent.Asc(dbaccount.FieldID)).All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list codex inspection target accounts: %w", err)
	}
	helper := &accountRepository{client: r.client, sql: r.db}
	serviceRows, err := helper.accountsToService(ctx, rows)
	if err != nil {
		return nil, err
	}
	out := make([]*service.Account, 0, len(serviceRows))
	now := time.Now()
	for i := range serviceRows {
		account := serviceRows[i]
		if query.OnlyStaleMinutes > 0 && !codexInspectionAccountIsStale(account.Extra, query.OnlyStaleMinutes, now) {
			continue
		}
		out = append(out, &account)
		if query.SampleSize > 0 && len(out) >= query.SampleSize {
			break
		}
	}
	return out, nil
}

func (r *codexInspectionRepository) CountOpenAIOAuthAccounts(ctx context.Context) (int, error) {
	return r.client.Account.Query().
		Where(dbaccount.DeletedAtIsNil(), dbaccount.PlatformEQ(service.PlatformOpenAI), dbaccount.TypeEQ(service.AccountTypeOAuth)).
		Count(ctx)
}

func (r *codexInspectionRepository) CountAccountsDisabledByInspection(ctx context.Context) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM accounts
		WHERE deleted_at IS NULL
			AND platform=$1
			AND type=$2
			AND extra ? $3`,
		service.PlatformOpenAI, service.AccountTypeOAuth, "codex_inspection_disabled_by_run_id",
	).Scan(&count)
	return count, err
}

func (r *codexInspectionRepository) ClearAccountExtraKeys(ctx context.Context, accountID int64, keys []string) error {
	if accountID <= 0 || len(keys) == 0 {
		return nil
	}
	_, err := r.db.ExecContext(ctx, `
		UPDATE accounts
		SET extra = COALESCE(extra, '{}'::jsonb) - $2::text[], updated_at = NOW()
		WHERE id=$1 AND deleted_at IS NULL`,
		accountID, pq.Array(keys),
	)
	if err != nil {
		return fmt.Errorf("clear account extra keys: %w", err)
	}
	return nil
}

const codexInspectionRunSelectSQL = `
	SELECT id, trigger_type, trigger_key, status, total_accounts, completed_accounts,
		success_count, error_count, keep_count, enable_count, disable_count,
		reauth_count, delete_count, settings_snapshot, started_at, finished_at, error_message
	FROM codex_inspection_runs`

const codexInspectionResultSelectSQL = `
	SELECT id, run_id, account_id, account_name, account_status_snapshot,
		schedulable_snapshot, proxy_id_snapshot, chatgpt_account_id, probe_status,
		upstream_status_code, latency_ms, five_hour_used_percent, long_window_type,
		long_window_used_percent, recommended_action, action_reason, action_status,
		action_error, body_excerpt, raw_rate_limit, created_at`

const codexInspectionLogSelectSQL = `
	SELECT id, run_id, account_id, level, message, detail, created_at
	FROM codex_inspection_logs`

type sqlScanner interface {
	Scan(dest ...any) error
}

func scanCodexInspectionRun(scanner sqlScanner) (*service.CodexInspectionRun, error) {
	var run service.CodexInspectionRun
	var settingsRaw []byte
	if err := scanner.Scan(
		&run.ID, &run.TriggerType, &run.TriggerKey, &run.Status, &run.TotalAccounts,
		&run.CompletedAccounts, &run.SuccessCount, &run.ErrorCount, &run.KeepCount,
		&run.EnableCount, &run.DisableCount, &run.ReauthCount, &run.DeleteCount,
		&settingsRaw, &run.StartedAt, &run.FinishedAt, &run.ErrorMessage,
	); err != nil {
		return nil, err
	}
	run.SettingsSnapshot = service.DefaultCodexInspectionSettings()
	if len(settingsRaw) > 0 {
		_ = json.Unmarshal(settingsRaw, &run.SettingsSnapshot)
	}
	return &run, nil
}

func scanCodexInspectionResult(scanner sqlScanner) (*service.CodexInspectionResult, error) {
	var result service.CodexInspectionResult
	var rawRateLimit []byte
	if err := scanner.Scan(
		&result.ID, &result.RunID, &result.AccountID, &result.AccountName,
		&result.AccountStatusSnapshot, &result.SchedulableSnapshot, &result.ProxyIDSnapshot,
		&result.ChatGPTAccountID, &result.ProbeStatus, &result.UpstreamStatusCode,
		&result.LatencyMS, &result.FiveHourUsedPercent, &result.LongWindowType,
		&result.LongWindowUsedPercent, &result.RecommendedAction, &result.ActionReason,
		&result.ActionStatus, &result.ActionError, &result.BodyExcerpt, &rawRateLimit,
		&result.CreatedAt,
	); err != nil {
		return nil, err
	}
	result.RawRateLimit = map[string]any{}
	if len(rawRateLimit) > 0 {
		_ = json.Unmarshal(rawRateLimit, &result.RawRateLimit)
	}
	return &result, nil
}

func scanCodexInspectionLog(scanner sqlScanner) (*service.CodexInspectionLog, error) {
	var item service.CodexInspectionLog
	var detailRaw []byte
	if err := scanner.Scan(&item.ID, &item.RunID, &item.AccountID, &item.Level, &item.Message, &detailRaw, &item.CreatedAt); err != nil {
		return nil, err
	}
	item.Detail = map[string]any{}
	if len(detailRaw) > 0 {
		_ = json.Unmarshal(detailRaw, &item.Detail)
	}
	return &item, nil
}

func codexInspectionResultFilters(runID int64, action, probeStatus, accountStatus, quotaWindow, search string, groupIDs []int64, onlyStaleMinutes int) ([]string, []any) {
	where, args := []string{"1=1"}, []any{}
	if runID > 0 {
		args = append(args, runID)
		where = append(where, fmt.Sprintf("run_id=$%d", len(args)))
	}
	if strings.TrimSpace(action) != "" {
		args = append(args, strings.TrimSpace(action))
		where = append(where, fmt.Sprintf("recommended_action=$%d", len(args)))
	}
	if strings.TrimSpace(probeStatus) != "" {
		args = append(args, strings.TrimSpace(probeStatus))
		where = append(where, fmt.Sprintf("probe_status=$%d", len(args)))
	}
	if strings.TrimSpace(accountStatus) != "" {
		args = append(args, strings.TrimSpace(accountStatus))
		where = append(where, fmt.Sprintf("account_status_snapshot=$%d", len(args)))
	}
	switch strings.TrimSpace(quotaWindow) {
	case "five_full":
		where = append(where, "five_hour_used_percent >= 100")
	case "long_full":
		where = append(where, "long_window_used_percent >= 100")
	case "normal":
		where = append(where, "COALESCE(five_hour_used_percent, 0) < 100 AND COALESCE(long_window_used_percent, 0) < 100")
	}
	if s := strings.TrimSpace(search); s != "" {
		args = append(args, "%"+s+"%")
		where = append(where, fmt.Sprintf("(account_name ILIKE $%d OR action_reason ILIKE $%d OR chatgpt_account_id ILIKE $%d)", len(args), len(args), len(args)))
	}
	if len(groupIDs) > 0 {
		args = append(args, pq.Array(groupIDs))
		where = append(where, fmt.Sprintf(`EXISTS (
			SELECT 1 FROM account_groups ag
			WHERE ag.account_id = codex_inspection_results.account_id
				AND ag.group_id = ANY($%d)
		)`, len(args)))
	}
	if onlyStaleMinutes > 0 {
		args = append(args, time.Now().Add(-time.Duration(onlyStaleMinutes)*time.Minute))
		where = append(where, fmt.Sprintf("created_at <= $%d", len(args)))
	}
	return where, args
}

func normalizePage(page, pageSize, defaultPageSize, maxPageSize int) (int, int) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = defaultPageSize
	}
	if maxPageSize > 0 && pageSize > maxPageSize {
		pageSize = maxPageSize
	}
	return page, pageSize
}

func normalizeLimitOffset(limit, offset, defaultLimit, maxLimit int) (int, int) {
	if limit <= 0 {
		limit = defaultLimit
	}
	if maxLimit > 0 && limit > maxLimit {
		limit = maxLimit
	}
	if offset < 0 {
		offset = 0
	}
	return limit, offset
}

func emptyDefault(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func nonNilMap(in map[string]any) map[string]any {
	if in == nil {
		return map[string]any{}
	}
	return in
}

func nullableTimeValue(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t
}

func codexInspectionAccountIsStale(extra map[string]any, minutes int, now time.Time) bool {
	if len(extra) == 0 {
		return true
	}
	raw, ok := extra["codex_usage_updated_at"]
	if !ok || raw == nil {
		return true
	}
	updatedAt, err := time.Parse(time.RFC3339, strings.TrimSpace(fmt.Sprint(raw)))
	if err != nil {
		return true
	}
	return now.Sub(updatedAt) >= time.Duration(minutes)*time.Minute
}
