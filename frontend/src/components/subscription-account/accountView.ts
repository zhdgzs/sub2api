import type { SubscriptionAccount } from '@/api/subscriptionAccounts'
import type { Account } from '@/types'

// 将用户接口的安全字段适配为账号列表展示组件所需的只读视图。
export function toReadonlyAccount(account: SubscriptionAccount): Account {
  return {
    ...account,
    ...account.capacity,
    proxy_id: null,
    priority: 0,
    error_message: null,
    last_used_at: account.last_used_at ?? null,
    expires_at: null,
    auto_pause_on_expired: false,
    updated_at: account.created_at,
    rate_limited_at: null,
    rate_limit_reset_at: account.rate_limit_reset_at ?? null,
    overload_until: account.overload_until ?? null,
    temp_unschedulable_until: account.temp_unschedulable_until ?? null,
    temp_unschedulable_reason: null,
    session_window_start: null,
    session_window_end: null,
    session_window_status: null,
    groups: account.groups.map((group) => ({
      ...group,
      subscription_type: 'subscription',
    })) as Account['groups'],
  } as Account
}
