import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { SubscriptionAccount } from '@/api/subscriptionAccounts'
import SubscriptionAccountTodayStats from '../SubscriptionAccountTodayStats.vue'
import SubscriptionAccountUsageWindows from '../SubscriptionAccountUsageWindows.vue'

const { getUsage } = vi.hoisted(() => ({ getUsage: vi.fn() }))

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
    rate_multiplier: 1,
    created_at: '2026-08-15T00:00:00Z',
  }
}

describe('subscription account readonly cells', () => {
  beforeEach(() => {
    getUsage.mockReset()
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
