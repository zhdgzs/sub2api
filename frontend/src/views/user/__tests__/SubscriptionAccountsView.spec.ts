import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

const { list, fetchActiveSubscriptions, showError } = vi.hoisted(() => ({
  list: vi.fn(),
  fetchActiveSubscriptions: vi.fn(),
  showError: vi.fn(),
}))

vi.mock('@/api/subscriptionAccounts', () => ({
  default: { list },
}))

vi.mock('@/stores/subscriptions', () => ({
  useSubscriptionStore: () => ({
    activeSubscriptions: [
      {
        group: { id: 7, name: 'Pro', subscription_type: 'subscription' },
      },
    ],
    fetchActiveSubscriptions,
  }),
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showError }),
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return { ...actual, useI18n: () => ({ t: (key: string) => key }) }
})

import SubscriptionAccountsView from '../SubscriptionAccountsView.vue'

const passthrough = { template: '<div><slot /><slot name="filters" /><slot name="table" /><slot name="pagination" /></div>' }

describe('SubscriptionAccountsView', () => {
  beforeEach(() => {
    list.mockReset().mockResolvedValue({ items: [], total: 0, page: 1, page_size: 20, pages: 1 })
    fetchActiveSubscriptions.mockReset().mockResolvedValue([])
    showError.mockReset()
  })

  it('loads the current user subscription account page without admin endpoints', async () => {
    mount(SubscriptionAccountsView, {
      global: {
        stubs: {
          AppLayout: passthrough,
          TablePageLayout: passthrough,
          DataTable: true,
          Pagination: true,
          Select: true,
          Icon: true,
          PlatformTypeBadge: true,
          AccountTodayStatsCell: true,
          SubscriptionAccountCapacity: true,
          SubscriptionAccountStatus: true,
          SubscriptionAccountUsageWindows: true,
        },
      },
    })
    await flushPromises()

    expect(fetchActiveSubscriptions).toHaveBeenCalled()
    expect(list).toHaveBeenCalledWith(
      { page: 1, page_size: 20, search: undefined, group_id: undefined },
      expect.objectContaining({ signal: expect.any(AbortSignal) }),
    )
    expect(JSON.stringify(list.mock.calls)).not.toContain('/admin/accounts')
  })
})
