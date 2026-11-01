import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

const { list, getOpenAIQuotaPeriods, fetchActiveSubscriptions, showError } = vi.hoisted(() => ({
  list: vi.fn(),
  getOpenAIQuotaPeriods: vi.fn(),
  fetchActiveSubscriptions: vi.fn(),
  showError: vi.fn(),
}))

vi.mock('@/api/subscriptionAccounts', () => ({
  default: { list, getOpenAIQuotaPeriods },
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
import { formatDateTime } from '@/utils/format'

const passthrough = { template: '<div><slot /><slot name="filters" /><slot name="table" /><slot name="pagination" /></div>' }

function mountView() {
  return mount(SubscriptionAccountsView, {
    global: {
      stubs: {
        AppLayout: passthrough,
        TablePageLayout: passthrough,
        DataTable: {
          props: ['columns', 'data'],
          template: `<table>
            <thead><tr><th v-for="column in columns" :key="column.key">{{ column.label }}</th></tr></thead>
            <tbody><tr v-for="row in data" :key="row.id">
              <td v-for="column in columns" :key="column.key" :data-field="column.key">
                <slot :name="'cell-' + column.key" :row="row" :value="row[column.key]" />
              </td>
            </tr></tbody>
          </table>`,
        },
        Pagination: true,
        Select: true,
        Icon: true,
        PlatformTypeBadge: true,
        AccountGroupsCell: true,
        SubscriptionAccountCapacity: true,
        SubscriptionAccountStatus: true,
        SubscriptionAccountTodayStats: true,
        SubscriptionAccountUsageWindows: true,
        OpenAIQuotaHistoryModal: true,
      },
    },
  })
}

describe('SubscriptionAccountsView', () => {
  beforeEach(() => {
    list.mockReset().mockResolvedValue({ items: [], total: 0, page: 1, page_size: 20, pages: 1 })
    getOpenAIQuotaPeriods.mockReset().mockResolvedValue({
      items: [],
      total: 0,
      page: 1,
      page_size: 20,
      pages: 0,
    })
    fetchActiveSubscriptions.mockReset().mockResolvedValue([])
    showError.mockReset()
  })

  it('loads the current user subscription account page without admin endpoints', async () => {
    mountView()
    await flushPromises()

    expect(fetchActiveSubscriptions).toHaveBeenCalled()
    expect(list).toHaveBeenCalledWith(
      { page: 1, page_size: 20, search: undefined, group_id: undefined },
      expect.objectContaining({ signal: expect.any(AbortSignal) }),
    )
    expect(JSON.stringify(list.mock.calls)).not.toContain('/admin/accounts')
  })

  it.each(['2026-12-01T00:00:00Z', null])('displays priority and expiry %s without name or creation time', async (expiresAt) => {
    list.mockResolvedValue({
      items: [{
        id: 8,
        name: 'hidden-account-name',
        platform: 'openai',
        type: 'oauth',
        capacity: { current_concurrency: 0, concurrency: 5 },
        groups: [],
        priority: 3,
        expires_at: expiresAt,
        created_at: '2026-08-15T00:00:00Z',
        rate_multiplier: 1,
      }],
      total: 1,
    })
    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.get('[data-field="priority"]').text()).toBe('3')
    expect(wrapper.get('[data-field="expires_at"]').text()).toBe(expiresAt ? formatDateTime(expiresAt) : '-')
    expect(wrapper.find('[data-field="name"]').exists()).toBe(false)
    expect(wrapper.find('[data-field="created_at"]').exists()).toBe(false)
    expect(wrapper.text()).not.toContain('hidden-account-name')
    wrapper.unmount()
  })
})
