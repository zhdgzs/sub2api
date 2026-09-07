import { apiClient } from './client'
import type {
  AccountPlatform,
  AccountType,
  AccountUsageInfo,
  OpenAIQuotaPeriod,
  PaginatedResponse,
  WindowStats,
} from '@/types'

export interface SubscriptionAccountGroup {
  id: number
  name: string
  platform: string
}

export interface SubscriptionAccountCapacity {
  current_concurrency: number
  concurrency: number
  current_window_cost?: number
  window_cost_limit?: number
  active_sessions?: number
  max_sessions?: number
  current_rpm?: number
  base_rpm?: number
  quota_used?: number
  quota_limit?: number
  quota_daily_used?: number
  quota_daily_limit?: number
  quota_weekly_used?: number
  quota_weekly_limit?: number
}

export interface SubscriptionAccount {
  id: number
  name: string
  platform: AccountPlatform
  type: AccountType
  capacity: SubscriptionAccountCapacity
  status: 'active' | 'disabled' | 'error' | string
  schedulable: boolean
  rate_limit_reset_at?: string
  overload_until?: string
  temp_unschedulable_until?: string
  today_stats?: WindowStats
  groups: SubscriptionAccountGroup[]
  usage?: AccountUsageInfo
  current_openai_quota_prediction?: number | null
  supports_openai_quota_history: boolean
  rate_multiplier: number
  last_used_at?: string
  created_at: string
}

export interface SubscriptionAccountListParams {
  page?: number
  page_size?: number
  search?: string
  group_id?: number
}

export async function list(
  params: SubscriptionAccountListParams,
  options?: { signal?: AbortSignal },
): Promise<PaginatedResponse<SubscriptionAccount>> {
  const { data } = await apiClient.get<PaginatedResponse<SubscriptionAccount>>(
    '/subscription-accounts',
    { params, signal: options?.signal },
  )
  return data
}

export async function getOpenAIQuotaPeriods(
  id: number,
  page: number = 1,
  pageSize: number = 20,
): Promise<PaginatedResponse<OpenAIQuotaPeriod>> {
  const { data } = await apiClient.get<PaginatedResponse<OpenAIQuotaPeriod>>(
    `/subscription-accounts/${id}/openai-quota-periods`,
    { params: { page, page_size: pageSize } },
  )
  return data
}

export default { list, getOpenAIQuotaPeriods }
