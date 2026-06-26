import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

import AccountsView from '../AccountsView.vue'

const {
  listAccounts,
  listWithEtag,
  getBatchTodayStats,
  getAllProxies,
  getAllGroups,
  updateAccount
} = vi.hoisted(() => ({
  listAccounts: vi.fn(),
  listWithEtag: vi.fn(),
  getBatchTodayStats: vi.fn(),
  getAllProxies: vi.fn(),
  getAllGroups: vi.fn(),
  updateAccount: vi.fn()
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    accounts: {
      list: listAccounts,
      listWithEtag,
      getBatchTodayStats,
      update: updateAccount,
      delete: vi.fn(),
      batchClearError: vi.fn(),
      batchRefresh: vi.fn(),
      toggleSchedulable: vi.fn()
    },
    proxies: {
      getAll: getAllProxies
    },
    groups: {
      getAll: getAllGroups
    }
  }
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError: vi.fn(),
    showSuccess: vi.fn(),
    showInfo: vi.fn()
  })
}))

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => ({
    token: 'test-token'
  })
}))

vi.mock('vue-router', () => ({
  useRoute: () => ({
    query: {}
  })
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key
    })
  }
})

const DataTableStub = {
  props: ['columns', 'data'],
  methods: {
    hasColumn(key: string) {
      return this.columns.some((column: { key: string }) => column.key === key)
    }
  },
  template: `
    <div data-test="data-table">
      <span v-for="column in columns" :key="column.key" data-test="column-key">{{ column.key }}</span>
      <div v-for="row in data" :key="row.id">
        <slot v-if="hasColumn('priority')" name="cell-priority" :value="row.priority" :row="row" />
        <slot v-if="hasColumn('created_at')" name="cell-created_at" :value="row.created_at" :row="row" />
      </div>
    </div>
  `
}

const AccountBulkActionsBarStub = {
  props: ['selectedIds'],
  emits: ['edit-filtered'],
  template: '<button data-test="edit-filtered" @click="$emit(\'edit-filtered\')">edit filtered</button>'
}

const BulkEditAccountModalStub = {
  props: ['show', 'target'],
  template: '<div data-test="bulk-edit-modal" :data-show="String(show)" :data-target-mode="target?.mode ?? \'\'"></div>'
}

const baseStubs = {
  AppLayout: { template: '<div><slot /></div>' },
  TablePageLayout: {
    template: '<div><slot name="filters" /><slot name="table" /><slot name="pagination" /></div>'
  },
  DataTable: DataTableStub,
  Pagination: true,
  ConfirmDialog: true,
  AccountTableActions: { template: '<div><slot name="beforeCreate" /><slot name="after" /></div>' },
  AccountTableFilters: { template: '<div></div>' },
  AccountBulkActionsBar: AccountBulkActionsBarStub,
  AccountActionMenu: true,
  ImportDataModal: true,
  ReAuthAccountModal: true,
  AccountTestModal: true,
  AccountStatsModal: true,
  ScheduledTestsPanel: true,
  SyncFromCrsModal: true,
  TempUnschedStatusModal: true,
  ErrorPassthroughRulesModal: true,
  TLSFingerprintProfilesModal: true,
  CreateAccountModal: true,
  EditAccountModal: true,
  BulkEditAccountModal: BulkEditAccountModalStub,
  PlatformTypeBadge: true,
  AccountCapacityCell: true,
  AccountStatusIndicator: true,
  AccountTodayStatsCell: true,
  AccountGroupsCell: true,
  AccountUsageCell: true,
  Icon: true
}

const InteractiveAccountTableFiltersStub = {
  emits: ['update:filters', 'change', 'update:searchQuery'],
  template: `
    <div>
      <button data-test="set-platform-filter" @click="$emit('update:filters', { platform: 'openai' }); $emit('change')">set platform</button>
      <button data-test="set-search-filter" @click="$emit('update:searchQuery', 'saved-account')">set search</button>
    </div>
  `
}

const mountAccountsView = (stubs: Record<string, unknown> = {}) => mount(AccountsView, {
  global: {
    stubs: {
      ...baseStubs,
      ...stubs
    }
  }
})

const buildAccount = (overrides: Record<string, unknown> = {}) => ({
  id: 1,
  name: 'test-account',
  platform: 'anthropic',
  type: 'oauth',
  status: 'active',
  schedulable: true,
  priority: 10,
  created_at: '2026-03-07T10:00:00Z',
  updated_at: '2026-03-07T10:00:00Z',
  ...overrides
})

describe('admin AccountsView bulk edit scope', () => {
  beforeEach(() => {
    localStorage.clear()

    listAccounts.mockReset()
    listWithEtag.mockReset()
    getBatchTodayStats.mockReset()
    getAllProxies.mockReset()
    getAllGroups.mockReset()
    updateAccount.mockReset()

    listAccounts.mockResolvedValue({
      items: [],
      total: 0,
      page: 1,
      page_size: 20,
      pages: 0
    })
    listWithEtag.mockResolvedValue({
      notModified: true,
      etag: null,
      data: null
    })
    getBatchTodayStats.mockResolvedValue({ stats: {} })
    getAllProxies.mockResolvedValue([])
    getAllGroups.mockResolvedValue([])
    updateAccount.mockResolvedValue(buildAccount())
  })

  it('opens bulk edit in filtered-results mode from the bulk actions dropdown', async () => {
    const wrapper = mountAccountsView()

    await flushPromises()
    await wrapper.get('[data-test="edit-filtered"]').trigger('click')
    await flushPromises()

    expect(wrapper.get('[data-test="bulk-edit-modal"]').attributes('data-show')).toBe('true')
    expect(wrapper.get('[data-test="bulk-edit-modal"]').attributes('data-target-mode')).toBe('filtered')
  })

  it('renders the created_at column by default', async () => {
    listAccounts.mockResolvedValue({
      items: [buildAccount()],
      total: 1,
      page: 1,
      page_size: 20,
      pages: 1
    })

    const wrapper = mountAccountsView()

    await flushPromises()

    const columnKeys = wrapper.findAll('[data-test="column-key"]').map(node => node.text())
    expect(columnKeys).toContain('created_at')
    const columns = wrapper.getComponent(DataTableStub).props('columns') as Array<{ key: string; label: string; sortable: boolean }>
    expect(columns.find(column => column.key === 'created_at')).toMatchObject({
      label: 'admin.accounts.columns.createdAt',
      sortable: true
    })
  })

  it('updates account priority directly from the list cell', async () => {
    const initialAccount = buildAccount({ priority: 10 })
    const updatedAccount = buildAccount({ priority: 3, updated_at: '2026-03-07T10:05:00Z' })

    listAccounts.mockResolvedValue({
      items: [initialAccount],
      total: 1,
      page: 1,
      page_size: 20,
      pages: 1
    })
    updateAccount.mockResolvedValue(updatedAccount)

    const wrapper = mountAccountsView()
    await flushPromises()

    const columnKeys = wrapper.findAll('[data-test="column-key"]').map(node => node.text())
    expect(columnKeys).toContain('priority')

    const priorityInput = wrapper.get('[data-test="account-priority-input"]')
    expect((priorityInput.element as HTMLInputElement).value).toBe('10')

    await priorityInput.setValue('3')
    await priorityInput.trigger('blur')
    await flushPromises()

    expect(updateAccount).toHaveBeenCalledWith(1, { priority: 3 })
    expect((wrapper.get('[data-test="account-priority-input"]').element as HTMLInputElement).value).toBe('3')
  })

  it('restores persisted account filters on mount', async () => {
    localStorage.setItem('account-table-filters', JSON.stringify({
      platform: 'openai',
      type: 'oauth',
      plan_type: 'pro',
      status: 'active',
      privacy_mode: 'training_off',
      group: '12',
      search: 'saved-account'
    }))

    mountAccountsView()
    await flushPromises()

    expect(listAccounts).toHaveBeenCalledWith(
      1,
      20,
      expect.objectContaining({
        platform: 'openai',
        type: 'oauth',
        plan_type: 'pro',
        status: 'active',
        privacy_mode: 'training_off',
        group: '12',
        search: 'saved-account'
      }),
      expect.any(Object)
    )
  })

  it('persists account filters when the user changes them', async () => {
    const wrapper = mountAccountsView({
      AccountTableFilters: InteractiveAccountTableFiltersStub
    })

    await flushPromises()
    await wrapper.get('[data-test="set-platform-filter"]').trigger('click')
    await wrapper.get('[data-test="set-search-filter"]').trigger('click')
    await flushPromises()

    expect(JSON.parse(localStorage.getItem('account-table-filters') || 'null')).toMatchObject({
      platform: 'openai',
      type: '',
      plan_type: '',
      status: '',
      privacy_mode: '',
      group: '',
      search: 'saved-account'
    })
  })
})
