import { describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import OpenAIQuotaHistoryModal from '../OpenAIQuotaHistoryModal.vue'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key })
  }
})

vi.mock('vue-chartjs', () => ({
  Bar: {
    name: 'Bar',
    props: ['data', 'options'],
    template: '<div class="chart-data">{{ JSON.stringify(data) }}</div>'
  }
}))

describe('OpenAIQuotaHistoryModal', () => {
  it.each([true, false])('展示保存的预计总费用，兼容无快照历史（有快照：%s）', async (hasEstimate) => {
    const wrapper = mount(OpenAIQuotaHistoryModal, {
      props: {
        show: true,
        account: { id: 1, name: 'OpenAI Pro' },
        fetchPeriods: vi.fn().mockResolvedValue({
          items: [{
            id: 9,
            account_id: 1,
            started_at: '2026-10-01T00:00:00Z',
            ended_at: '2026-10-08T00:00:00Z',
            used_usd: 12.5,
            used_percent: 25,
            token_count: 1200,
            request_count: 42,
            predicted_quota_usd: 50,
            estimate: hasEstimate ? {
              total_cost: 120,
              window_started_at: '2026-09-29T00:00:00Z',
              window_cost: 30,
              used_percent: 25,
              sampled_at: '2026-10-07T00:00:00Z'
            } : null
          }],
          total: 1,
          page: 1,
          page_size: 20,
          pages: 1
        })
      },
      global: {
        stubs: {
          BaseDialog: { template: '<div><slot /></div>' },
          LoadingSpinner: true,
          Icon: true
        }
      }
    })
    await flushPromises()

    const chartData = JSON.parse(wrapper.find('.chart-data').text())
    expect(chartData.datasets).toHaveLength(2)
    expect(chartData.datasets[0].label).toBe('admin.accounts.quotaHistoryEstimated')
    expect(chartData.datasets[0].data).toEqual([hasEstimate ? 120 : null])
    expect(chartData.datasets[1].label).toBe('admin.accounts.quotaHistoryUsed')
    expect(chartData.datasets[1].data).toEqual([12.5])

    const options = wrapper.findComponent({ name: 'Bar' }).props('options')
    expect(options.plugins.tooltip.callbacks.label({
      datasetIndex: 0,
      dataset: { label: 'estimated' },
      raw: 0.004
    })).toBe('estimated: $0.00')
    expect(options.plugins.tooltip.callbacks.label({
      datasetIndex: 0,
      dataset: { label: 'estimated' },
      raw: 1.005
    })).toBe(`estimated: $${(1.005).toFixed(2)}`)
    const details = options.plugins.tooltip.callbacks.afterBody([{ dataIndex: 0 }])
    expect(details.slice(0, 3)).toEqual([
      'admin.accounts.quotaHistoryUsedPercent: 25.0%',
      'admin.accounts.quotaHistoryTokens: 1.2K',
      'admin.accounts.quotaHistoryRequests: 42'
    ])
    if (hasEstimate) {
      expect(details.some((line: string) => line.startsWith('admin.accounts.quotaHistoryEstimateWindow:'))).toBe(true)
      expect(details.some((line: string) => line.startsWith('admin.accounts.quotaHistoryEstimateSampledAt:'))).toBe(true)
      expect(details).toContain('admin.accounts.quotaHistoryEstimatePercent: 25.0%')
      expect(details).not.toContain('admin.accounts.quotaHistoryNoEstimate')
    } else {
      expect(details).toContain('admin.accounts.quotaHistoryNoEstimate')
    }
    wrapper.unmount()
  })
})
