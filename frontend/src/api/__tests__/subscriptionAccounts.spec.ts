import { beforeEach, describe, expect, it, vi } from 'vitest'

const get = vi.hoisted(() => vi.fn())
vi.mock('@/api/client', () => ({ apiClient: { get } }))

import subscriptionAccountsAPI from '../subscriptionAccounts'

describe('subscription accounts API', () => {
  beforeEach(() => get.mockReset())

  it('uses the user-only route with server-side filters', async () => {
    const payload = { items: [], total: 0, page: 1, page_size: 20, pages: 1 }
    const controller = new AbortController()
    get.mockResolvedValue({ data: payload })

    const result = await subscriptionAccountsAPI.list(
      { page: 2, page_size: 50, search: 'openai', group_id: 7 },
      { signal: controller.signal },
    )

    expect(get).toHaveBeenCalledWith('/subscription-accounts', {
      params: { page: 2, page_size: 50, search: 'openai', group_id: 7 },
      signal: controller.signal,
    })
    expect(result).toBe(payload)
  })
})
