<template>
  <BaseDialog
    :show="show"
    :title="t('admin.accounts.quotaHistoryTitle', { name: account?.name ?? '' })"
    width="extra-wide"
    @close="emit('close')"
  >
    <div class="min-h-[24rem]">
      <div v-if="loading" class="flex h-[24rem] items-center justify-center">
        <LoadingSpinner />
      </div>

      <div
        v-else-if="error"
        class="flex h-[24rem] flex-col items-center justify-center gap-3 text-sm text-gray-500 dark:text-gray-400"
      >
        <span>{{ t('admin.accounts.quotaHistoryLoadFailed') }}</span>
        <button type="button" class="btn btn-secondary btn-sm" @click="loadPeriods">
          <Icon name="refresh" size="sm" />
          {{ t('admin.accounts.quotaHistoryRetry') }}
        </button>
      </div>

      <div
        v-else-if="periods.length === 0"
        class="flex h-[24rem] flex-col items-center justify-center gap-2 text-sm text-gray-500 dark:text-gray-400"
      >
        <Icon name="chartBar" size="lg" class="text-gray-300 dark:text-gray-600" />
        <span>{{ t('admin.accounts.quotaHistoryEmpty') }}</span>
      </div>

      <div v-else class="h-[24rem] min-h-[20rem] w-full">
        <Bar :data="chartData" :options="chartOptions" />
      </div>
    </div>

    <template v-if="!loading && !error && pagination.total > 0" #footer>
      <div class="flex w-full items-center justify-between gap-3">
        <span class="text-xs text-gray-500 dark:text-gray-400">
          {{ t('admin.accounts.quotaHistoryPage', { page: pagination.page, pages: pagination.pages }) }}
        </span>
        <div class="flex items-center gap-2">
          <button
            type="button"
            class="btn btn-secondary btn-sm"
            :disabled="pagination.page >= pagination.pages"
            @click="changePage(pagination.page + 1)"
          >
            <Icon name="chevronLeft" size="sm" />
            {{ t('admin.accounts.quotaHistoryOlder') }}
          </button>
          <button
            type="button"
            class="btn btn-secondary btn-sm"
            :disabled="pagination.page <= 1"
            @click="changePage(pagination.page - 1)"
          >
            {{ t('admin.accounts.quotaHistoryNewer') }}
            <Icon name="chevronRight" size="sm" />
          </button>
        </div>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  BarElement,
  CategoryScale,
  Chart as ChartJS,
  Legend,
  LinearScale,
  Tooltip,
  type ChartData,
  type ChartOptions
} from 'chart.js'
import { Bar } from 'vue-chartjs'
import { adminAPI } from '@/api/admin'
import BaseDialog from '@/components/common/BaseDialog.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import Icon from '@/components/icons/Icon.vue'
import type { Account, OpenAIQuotaPeriod } from '@/types'
import { formatCurrency, formatDateTime } from '@/utils/format'

ChartJS.register(BarElement, CategoryScale, LinearScale, Tooltip, Legend)

const props = defineProps<{
  show: boolean
  account: Account | null
}>()

const emit = defineEmits<{
  close: []
}>()

const { t } = useI18n()
const periods = ref<OpenAIQuotaPeriod[]>([])
const loading = ref(false)
const error = ref(false)
const pagination = reactive({ page: 1, pageSize: 20, total: 0, pages: 1 })
let requestSequence = 0

const chronologicalPeriods = computed(() => [...periods.value].reverse())

const periodLabel = (value: string) => {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return new Intl.DateTimeFormat(undefined, {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit'
  }).format(date)
}

const chartData = computed<ChartData<'bar'>>(() => ({
  labels: chronologicalPeriods.value.map((period) => periodLabel(period.started_at)),
  datasets: [
    {
      label: t('admin.accounts.quotaHistoryUsed'),
      data: chronologicalPeriods.value.map((period) => period.used_usd),
      backgroundColor: 'rgba(13, 148, 136, 0.78)',
      borderColor: 'rgb(13, 148, 136)',
      borderWidth: 1,
      borderRadius: 3
    },
    {
      label: t('admin.accounts.quotaHistoryPredicted'),
      data: chronologicalPeriods.value.map((period) => period.predicted_quota_usd ?? null),
      backgroundColor: 'rgba(245, 158, 11, 0.78)',
      borderColor: 'rgb(217, 119, 6)',
      borderWidth: 1,
      borderRadius: 3
    }
  ]
}))

const chartOptions = computed<ChartOptions<'bar'>>(() => ({
  responsive: true,
  maintainAspectRatio: false,
  interaction: { mode: 'index', intersect: false },
  scales: {
    x: { stacked: false, grid: { display: false } },
    y: {
      stacked: false,
      beginAtZero: true,
      ticks: { callback: (value) => formatCurrency(Number(value)) }
    }
  },
  plugins: {
    legend: { position: 'top' },
    tooltip: {
      callbacks: {
        title: (items) => {
          const period = chronologicalPeriods.value[items[0]?.dataIndex ?? -1]
          if (!period) return ''
          const end = period.ended_at
            ? formatDateTime(period.ended_at)
            : t('admin.accounts.quotaHistoryCurrent')
          return `${formatDateTime(period.started_at)} - ${end}`
        },
        label: (item) => `${item.dataset.label}: ${formatCurrency(Number(item.raw))}`,
        afterBody: (items) => {
          const period = chronologicalPeriods.value[items[0]?.dataIndex ?? -1]
          if (!period) return []
          return [
            `${t('admin.accounts.quotaHistoryUsed')}: ${formatCurrency(period.used_usd)}`,
            `${t('admin.accounts.quotaHistoryPredicted')}: ${period.predicted_quota_usd == null ? t('admin.accounts.quotaHistoryNoPrediction') : formatCurrency(period.predicted_quota_usd)}`
          ]
        }
      }
    }
  }
}))

const loadPeriods = async () => {
  if (!props.show || !props.account) return
  const sequence = ++requestSequence
  loading.value = true
  error.value = false
  try {
    const result = await adminAPI.accounts.getOpenAIQuotaPeriods(
      props.account.id,
      pagination.page,
      pagination.pageSize
    )
    if (sequence !== requestSequence) return
    periods.value = result.items
    pagination.total = result.total
    pagination.pages = result.pages
  } catch {
    if (sequence !== requestSequence) return
    periods.value = []
    error.value = true
  } finally {
    if (sequence === requestSequence) loading.value = false
  }
}

const changePage = (page: number) => {
  if (page < 1 || page > pagination.pages || page === pagination.page) return
  pagination.page = page
  void loadPeriods()
}

watch(
  () => [props.show, props.account?.id] as const,
  ([show, accountID], previous) => {
    if (!show || !accountID) {
      requestSequence++
      periods.value = []
      loading.value = false
      error.value = false
      return
    }
    if (!previous?.[0] || previous[1] !== accountID) {
      pagination.page = 1
      pagination.total = 0
      pagination.pages = 1
      periods.value = []
      void loadPeriods()
    }
  },
  { immediate: true }
)
</script>
