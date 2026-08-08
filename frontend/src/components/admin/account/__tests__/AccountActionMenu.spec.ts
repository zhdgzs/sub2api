import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'

import AccountActionMenu from '../AccountActionMenu.vue'
import type { Account } from '@/types'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string) => key
  })
}))

function buildAccount(overrides: Partial<Account> = {}): Account {
  return {
    id: 7,
    name: 'token-account',
    platform: 'openai',
    type: 'oauth',
    credentials: {},
    credentials_status: {},
    extra: {},
    proxy_id: null,
    concurrency: 1,
    priority: 10,
    rate_multiplier: 1,
    status: 'active',
    error_message: null,
    last_used_at: null,
    expires_at: null,
    auto_pause_on_expired: false,
    created_at: '2026-08-08T00:00:00Z',
    updated_at: '2026-08-08T00:00:00Z',
    schedulable: true,
    rate_limited_at: null,
    rate_limit_reset_at: null,
    overload_until: null,
    temp_unschedulable_until: null,
    temp_unschedulable_reason: null,
    session_window_start: null,
    session_window_end: null,
    session_window_status: null,
    ...overrides
  }
}

function mountMenu(account: Account) {
  return mount(AccountActionMenu, {
    props: {
      show: true,
      account,
      position: { top: 12, left: 24 }
    },
    global: {
      stubs: {
        Teleport: true,
        Icon: true
      }
    }
  })
}

describe('AccountActionMenu', () => {
  it('emits the access_token copy action from the row menu', async () => {
    const account = buildAccount()
    const wrapper = mountMenu(account)

    const action = wrapper.get('[data-test="copy-access-token-action"]')
    expect(action.text()).toContain('admin.accounts.copyAccessToken')

    await action.trigger('click')

    expect(wrapper.emitted('copy-access-token')?.[0]).toEqual([account])
    expect(wrapper.emitted('close')).toHaveLength(1)
  })

  it('emits the single-account export action from the row menu', async () => {
    const account = buildAccount()
    const wrapper = mountMenu(account)

    const action = wrapper.get('[data-test="export-account-action"]')
    expect(action.text()).toContain('admin.accounts.dataExportAccount')

    await action.trigger('click')

    expect(wrapper.emitted('export-account')?.[0]).toEqual([account])
    expect(wrapper.emitted('close')).toHaveLength(1)
  })
})
