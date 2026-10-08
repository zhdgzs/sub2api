import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { SubscriptionAccount } from '@/api/subscriptionAccounts'
import SubscriptionAccountTodayStats from '../SubscriptionAccountTodayStats.vue'
import SubscriptionAccountUsageWindows from '../SubscriptionAccountUsageWindows.vue'

const { getUsage, refreshOpenAIQuota, resetOpenAIQuota, refreshOpenAIReferrals, sendOpenAIReferralInvite } = vi.hoisted(() => ({
  getUsage: vi.fn(),
  refreshOpenAIQuota: vi.fn(),
  resetOpenAIQuota: vi.fn(),
  refreshOpenAIReferrals: vi.fn(),
  sendOpenAIReferralInvite: vi.fn(),
}))

vi.mock('@/api/admin/accounts', () => ({
  refreshOpenAIQuota,
  resetOpenAIQuota,
  refreshOpenAIReferrals,
  sendOpenAIReferralInvite,
}))

vi.mock('@/api/admin', () => ({
  adminAPI: { accounts: { getUsage } },
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return { ...actual, useI18n: () => ({ t: (key: string) => key }) }
})

function makeAccount(): SubscriptionAccount {
  return {
    id: 8,
    name: 'pool-8',
    platform: 'anthropic',
    type: 'oauth',
    capacity: { current_concurrency: 0, concurrency: 5 },
    status: 'active',
    schedulable: true,
    groups: [{ id: 3, name: 'Pro', platform: 'anthropic' }],
    usage: {
      updated_at: null,
      five_hour: {
        utilization: 42,
        resets_at: '2026-08-15T12:00:00Z',
        remaining_seconds: 3600,
      },
      seven_day: null,
      seven_day_sonnet: null,
    },
    supports_openai_quota_history: false,
    rate_multiplier: 1,
    created_at: '2026-08-15T00:00:00Z',
  }
}

describe('subscription account readonly cells', () => {
  beforeEach(() => {
    getUsage.mockReset()
    refreshOpenAIQuota.mockReset()
    resetOpenAIQuota.mockReset()
    refreshOpenAIReferrals.mockReset()
    sendOpenAIReferralInvite.mockReset()
    Object.defineProperty(window, 'matchMedia', {
      writable: true,
      value: vi.fn().mockReturnValue({
        matches: true,
        addListener: vi.fn(),
        removeListener: vi.fn(),
        addEventListener: vi.fn(),
        removeEventListener: vi.fn(),
      }),
    })
  })

  it('今日统计只显示请求数和 Token', () => {
    const wrapper = mount(SubscriptionAccountTodayStats, {
      props: { stats: { requests: 12, tokens: 3456, cost: 7.89, user_cost: 9.87 } },
    })

    expect(wrapper.text()).toContain('admin.accounts.stats.requests')
    expect(wrapper.text()).toContain('admin.accounts.stats.tokens')
    expect(wrapper.text()).not.toContain('usage.accountBilled')
    expect(wrapper.text()).not.toContain('usage.userBilled')
    expect(wrapper.text()).not.toContain('7.89')
    expect(wrapper.text()).not.toContain('9.87')
  })

  it.each([
    { name: 'decimal precision', credits: { has_credits: true, unlimited: false, balance: '12345678901234567890.0123' }, expected: '12345678901234567890.0123' },
    { name: 'zero', credits: { has_credits: false, unlimited: false, balance: '0' }, expected: '0' },
    { name: 'unlimited', credits: { has_credits: false, unlimited: true, balance: null }, expected: 'admin.accounts.openaiQuotaReset.pointsUnlimited' },
    { name: 'hidden balance', credits: { has_credits: true, unlimited: false, balance: null }, expected: 'admin.accounts.openaiQuotaReset.pointsAvailable' },
    { name: 'invalid balance', credits: { has_credits: true, unlimited: false, balance: 'NaN' }, expected: 'admin.accounts.openaiQuotaReset.pointsAvailable' },
    { name: 'negative balance', credits: { has_credits: true, unlimited: false, balance: '-1' }, expected: 'admin.accounts.openaiQuotaReset.pointsAvailable' },
    { name: 'unknown', credits: null, expected: '—' },
  ])('displays readonly Codex points: $name without administrator requests', async ({ credits, expected }) => {
    const account: SubscriptionAccount = {
      ...makeAccount(),
      platform: 'openai',
      codex_credits_snapshot: { credits, fetched_at: 1770000000 },
    }
    const wrapper = mount(SubscriptionAccountUsageWindows, { props: { account } })
    await flushPromises()

    const points = wrapper.get('[data-testid="subscription-codex-credits"]')
    expect(points.text()).toContain('admin.accounts.openaiQuotaReset.points')
    expect(points.text()).toContain(expected)
    expect(points.find('button').exists()).toBe(false)
    expect(points.get('[data-testid="codex-credits-balance"]').attributes('title')).toBe('admin.accounts.openaiQuotaReset.pointsUpdatedAt')
    expect(getUsage).not.toHaveBeenCalled()
    expect(refreshOpenAIQuota).not.toHaveBeenCalled()
    expect(resetOpenAIQuota).not.toHaveBeenCalled()
    expect(refreshOpenAIReferrals).not.toHaveBeenCalled()
    expect(sendOpenAIReferralInvite).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('refreshes the balance from list data and clears it when the snapshot disappears', async () => {
    const account: SubscriptionAccount = { ...makeAccount(), platform: 'openai', usage: undefined }
    const wrapper = mount(SubscriptionAccountUsageWindows, { props: { account } })
    const points = () => wrapper.get('[data-testid="subscription-codex-credits"]')
    expect(points().text()).toContain('—')

    await wrapper.setProps({ account: {
      ...account,
      codex_credits_snapshot: { credits: { has_credits: true, unlimited: false, balance: '12.50' }, fetched_at: 1770000000 },
    } })
    expect(points().text()).toContain('12.50')

    await wrapper.setProps({ account })
    expect(points().text()).toContain('—')
    expect(points().text()).not.toContain('12.50')
    expect(getUsage).not.toHaveBeenCalled()
    expect(refreshOpenAIQuota).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it.each([
    { platform: 'anthropic' as const, type: 'oauth' as const },
    { platform: 'openai' as const, type: 'apikey' as const },
  ])('does not display Codex points for $platform/$type', ({ platform, type }) => {
    const wrapper = mount(SubscriptionAccountUsageWindows, {
      props: { account: { ...makeAccount(), platform, type } },
    })
    expect(wrapper.find('[data-testid="subscription-codex-credits"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('用量窗口只消费列表数据且不请求管理员接口', async () => {
    const wrapper = mount(SubscriptionAccountUsageWindows, {
      props: { account: makeAccount() },
      global: {
        stubs: {
          UsageProgressBar: {
            props: ['label', 'utilization'],
            template: '<div data-testid="usage-bar">{{ label }} {{ utilization }}</div>',
          },
        },
      },
    })

    await flushPromises()

    expect(wrapper.get('[data-testid="usage-bar"]').text()).toBe('5h 42')
    expect(getUsage).not.toHaveBeenCalled()
  })
})
