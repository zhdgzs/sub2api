import { beforeEach, describe, expect, it, vi } from 'vitest'

const { get } = vi.hoisted(() => ({
  get: vi.fn(),
}))

vi.mock('@/api/client', () => ({
  apiClient: {
    get,
  },
}))

import { exportData } from '@/api/admin/accounts'

describe('admin accounts api', () => {
  beforeEach(() => {
    get.mockReset()
    get.mockResolvedValue({
      data: {
        exported_at: '2026-06-06T00:00:00Z',
        proxies: [],
        accounts: [],
      },
    })
  })

  it('passes plan_type through account export filters', async () => {
    await exportData({
      includeProxies: false,
      filters: {
        platform: 'openai',
        type: 'oauth',
        plan_type: 'pro',
        status: 'active',
        group: '12',
        privacy_mode: 'training_set_cf_blocked',
        search: 'needle',
        sort_by: 'priority',
        sort_order: 'desc',
      },
    })

    expect(get).toHaveBeenCalledWith('/admin/accounts/data', {
      params: {
        platform: 'openai',
        type: 'oauth',
        plan_type: 'pro',
        status: 'active',
        group: '12',
        privacy_mode: 'training_set_cf_blocked',
        search: 'needle',
        sort_by: 'priority',
        sort_order: 'desc',
        include_proxies: 'false',
      },
    })
  })
})
