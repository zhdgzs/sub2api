import { apiClient } from '../client'

export type CodexInspectionAction = 'keep' | 'enable' | 'disable' | 'reauth' | 'delete'
export type CodexInspectionProbeStatus = 'success' | 'failed' | 'skipped'
export type CodexInspectionRunStatus = 'running' | 'completed' | 'failed' | 'canceled'
export type CodexInspectionTriggerType = 'manual' | 'scheduled' | 'single_account'

export interface CodexInspectionSettings {
  enabled: boolean
  schedule: {
    mode: 'interval' | 'time_points'
    interval_minutes: number
    time_points: string[]
    timezone: string
  }
  target: {
    only_openai_oauth: boolean
    account_ids: number[]
    group_ids: number[]
    include_unschedulable: boolean
    include_error: boolean
    only_stale_minutes: number
    sample_size: number
  }
  probe: {
    workers: number
    timeout_ms: number
    retries: number
    min_interval_minutes: number
    user_agent: string
  }
  decision: {
    used_percent_threshold: number
    short_window_policy: string
    long_window_policy: string
  }
  actions: {
    auto_apply: boolean
    allow_enable: boolean
    allow_disable: boolean
    allow_mark_reauth: boolean
    allow_delete: boolean
  }
}

export interface CodexInspectionRun {
  id: number
  trigger_type: CodexInspectionTriggerType
  trigger_key: string
  status: CodexInspectionRunStatus
  total_accounts: number
  completed_accounts: number
  success_count: number
  error_count: number
  keep_count: number
  enable_count: number
  disable_count: number
  reauth_count: number
  delete_count: number
  settings_snapshot: CodexInspectionSettings
  started_at: string
  finished_at: string | null
  error_message: string
}

export interface CodexInspectionResult {
  id: number
  run_id: number
  account_id: number
  account_name: string
  account_status_snapshot: string
  schedulable_snapshot: boolean
  proxy_id_snapshot: number | null
  chatgpt_account_id: string
  probe_status: CodexInspectionProbeStatus
  upstream_status_code: number | null
  latency_ms: number | null
  five_hour_used_percent: number | null
  long_window_type: 'weekly' | 'monthly' | 'generic' | 'none'
  long_window_used_percent: number | null
  recommended_action: CodexInspectionAction
  action_reason: string
  action_status: 'none' | 'pending' | 'success' | 'failed' | 'skipped' | 'needs_review'
  action_error: string
  body_excerpt: string
  raw_rate_limit: Record<string, unknown>
  created_at: string
}

export interface CodexInspectionLog {
  id: number
  run_id: number
  account_id: number | null
  level: 'debug' | 'info' | 'warning' | 'error'
  message: string
  detail: Record<string, unknown>
  created_at: string
}

export interface CodexInspectionOverview {
  settings: CodexInspectionSettings
  latest_run: CodexInspectionRun | null
  running_run: CodexInspectionRun | null
  total_openai_oauth: number
  healthy_accounts: number
  five_hour_full_accounts: number
  long_window_full_accounts: number
  reauth_accounts: number
  delete_suggested_accounts: number
  disabled_by_inspection_accounts: number
  probe_failed_accounts: number
}

export interface Page<T> {
  items: T[]
  total: number
  page?: number
  page_size?: number
  pages?: number
}

export interface CreateRunRequest {
  account_ids?: number[]
  filters?: {
    group_ids?: number[]
    include_unschedulable?: boolean
    include_error?: boolean
    only_stale_minutes?: number
  }
  apply_actions?: boolean
  settings_override?: CodexInspectionSettings | null
}

export interface ApplyActionsRequest {
  result_ids: number[]
  action_override?: '' | CodexInspectionAction
  force?: boolean
  confirmation_text?: string
}

export async function overview(): Promise<CodexInspectionOverview> {
  const { data } = await apiClient.get<CodexInspectionOverview>('/admin/codex-inspection/overview')
  return data
}

export async function getSettings(): Promise<CodexInspectionSettings> {
  const { data } = await apiClient.get<CodexInspectionSettings>('/admin/codex-inspection/settings')
  return data
}

export async function updateSettings(settings: CodexInspectionSettings): Promise<CodexInspectionSettings> {
  const { data } = await apiClient.put<CodexInspectionSettings>('/admin/codex-inspection/settings', settings)
  return data
}

export async function createRun(payload: CreateRunRequest): Promise<CodexInspectionRun> {
  const { data } = await apiClient.post<CodexInspectionRun>('/admin/codex-inspection/runs', payload)
  return data
}

export async function listRuns(params: Record<string, unknown> = {}): Promise<Page<CodexInspectionRun>> {
  const { data } = await apiClient.get<Page<CodexInspectionRun>>('/admin/codex-inspection/runs', { params })
  return data
}

export async function getRun(id: number): Promise<{ run: CodexInspectionRun; results: CodexInspectionResult[]; logs: CodexInspectionLog[] }> {
  const { data } = await apiClient.get(`/admin/codex-inspection/runs/${id}`)
  return data
}

export async function listRunResults(id: number, params: Record<string, unknown> = {}): Promise<Page<CodexInspectionResult>> {
  const { data } = await apiClient.get<Page<CodexInspectionResult>>(`/admin/codex-inspection/runs/${id}/results`, { params })
  return data
}

export async function cancelRun(id: number): Promise<CodexInspectionRun> {
  const { data } = await apiClient.post<CodexInspectionRun>(`/admin/codex-inspection/runs/${id}/cancel`)
  return data
}

export async function applyActions(id: number, payload: ApplyActionsRequest): Promise<{ items: Array<{ result_id: number; account_id: number; action: string; status: string; message: string }> }> {
  const { data } = await apiClient.post(`/admin/codex-inspection/runs/${id}/actions`, payload)
  return data
}

export async function probeAccount(accountId: number): Promise<{ run: CodexInspectionRun; results: CodexInspectionResult[]; logs: CodexInspectionLog[] }> {
  const { data } = await apiClient.post(`/admin/codex-inspection/accounts/${accountId}/probe`)
  return data
}

export async function latestAccounts(params: Record<string, unknown> = {}): Promise<Page<CodexInspectionResult>> {
  const { data } = await apiClient.get<Page<CodexInspectionResult>>('/admin/codex-inspection/accounts/latest', { params })
  return data
}

export async function listLogs(params: Record<string, unknown> = {}): Promise<Page<CodexInspectionLog>> {
  const { data } = await apiClient.get<Page<CodexInspectionLog>>('/admin/codex-inspection/logs', { params })
  return data
}

export const codexInspectionAPI = {
  overview,
  getSettings,
  updateSettings,
  createRun,
  listRuns,
  getRun,
  listRunResults,
  cancelRun,
  applyActions,
  probeAccount,
  latestAccounts,
  listLogs,
}

export default codexInspectionAPI
