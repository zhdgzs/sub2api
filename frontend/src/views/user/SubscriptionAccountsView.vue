<template>
  <AppLayout>
    <TablePageLayout>
      <template #filters>
        <div class="flex flex-col justify-between gap-3 lg:flex-row lg:items-center">
          <div class="flex flex-1 flex-col gap-3 sm:flex-row">
            <div class="relative w-full sm:max-w-sm">
              <Icon
                name="search"
                size="md"
                class="absolute left-3 top-1/2 -translate-y-1/2 text-gray-400"
              />
              <input
                v-model="searchDraft"
                type="search"
                class="input pl-10"
                :placeholder="t('subscriptionAccounts.searchPlaceholder')"
                @keyup.enter="applySearch"
              />
            </div>
            <div class="w-full sm:w-64">
              <Select
                v-model="groupFilter"
                :options="groupOptions"
                @change="applyFilters"
              />
            </div>
          </div>
          <div class="flex justify-end gap-2">
            <button
              type="button"
              class="btn btn-secondary"
              :title="t('common.reset')"
              :disabled="loading"
              @click="resetFilters"
            >
              <Icon name="x" size="md" />
            </button>
            <button
              type="button"
              class="btn btn-secondary"
              :title="t('common.refresh')"
              :disabled="loading"
              @click="loadAccounts"
            >
              <Icon name="refresh" size="md" :class="{ 'animate-spin': loading }" />
            </button>
          </div>
        </div>
      </template>

      <template #table>
        <DataTable
          :columns="columns"
          :data="accounts"
          :loading="loading"
          row-key="id"
          :estimate-row-height="150"
        >
          <template #cell-name="{ row }">
            <span class="font-medium text-gray-900 dark:text-white">{{ row.name }}</span>
          </template>

          <template #cell-platform_type="{ row }">
            <PlatformTypeBadge :platform="row.platform" :type="row.type" />
          </template>

          <template #cell-capacity="{ row }">
            <SubscriptionAccountCapacity :account="row" />
          </template>

          <template #cell-status="{ row }">
            <SubscriptionAccountStatus :account="row" />
          </template>

          <template #cell-today_stats="{ row }">
            <SubscriptionAccountTodayStats :stats="row.today_stats ?? null" />
          </template>

          <template #cell-groups="{ row }">
            <AccountGroupsCell :groups="toReadonlyAccount(row).groups" :max-display="4" />
          </template>

          <template #cell-usage_windows="{ row }">
            <SubscriptionAccountUsageWindows :account="row" />
          </template>

          <template #cell-rate_multiplier="{ row }">
            <span class="font-mono text-sm text-gray-700 dark:text-gray-300">
              {{ formatMultiplier(row.rate_multiplier) }}x
            </span>
          </template>

          <template #cell-last_used_at="{ row }">
            <span class="whitespace-nowrap text-sm text-gray-600 dark:text-gray-300">
              {{ row.last_used_at ? formatDateTime(row.last_used_at) : '-' }}
            </span>
          </template>

          <template #cell-created_at="{ row }">
            <span class="whitespace-nowrap text-sm text-gray-600 dark:text-gray-300">
              {{ formatDateTime(row.created_at) }}
            </span>
          </template>

          <template #empty>
            <div class="flex flex-col items-center py-4">
              <Icon name="server" size="xl" class="mb-3 h-12 w-12 text-gray-300 dark:text-dark-500" />
              <p class="font-medium text-gray-700 dark:text-gray-200">
                {{ emptyLabel }}
              </p>
            </div>
          </template>
        </DataTable>
      </template>

      <template #pagination>
        <Pagination
          :total="pagination.total"
          :page="pagination.page"
          :page-size="pagination.pageSize"
          @update:page="changePage"
          @update:page-size="changePageSize"
        />
      </template>
    </TablePageLayout>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import TablePageLayout from '@/components/layout/TablePageLayout.vue'
import DataTable from '@/components/common/DataTable.vue'
import Pagination from '@/components/common/Pagination.vue'
import Select from '@/components/common/Select.vue'
import Icon from '@/components/icons/Icon.vue'
import PlatformTypeBadge from '@/components/common/PlatformTypeBadge.vue'
import AccountGroupsCell from '@/components/account/AccountGroupsCell.vue'
import SubscriptionAccountCapacity from '@/components/subscription-account/SubscriptionAccountCapacity.vue'
import SubscriptionAccountStatus from '@/components/subscription-account/SubscriptionAccountStatus.vue'
import SubscriptionAccountTodayStats from '@/components/subscription-account/SubscriptionAccountTodayStats.vue'
import SubscriptionAccountUsageWindows from '@/components/subscription-account/SubscriptionAccountUsageWindows.vue'
import { toReadonlyAccount } from '@/components/subscription-account/accountView'
import subscriptionAccountsAPI, { type SubscriptionAccount } from '@/api/subscriptionAccounts'
import { useSubscriptionStore } from '@/stores/subscriptions'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'
import { formatDateTime } from '@/utils/format'
import { formatMultiplier } from '@/utils/formatters'

const { t } = useI18n()
const appStore = useAppStore()
const subscriptionStore = useSubscriptionStore()

const accounts = ref<SubscriptionAccount[]>([])
const loading = ref(false)
const searchDraft = ref('')
const appliedSearch = ref('')
const groupFilter = ref('')
const pagination = reactive({ page: 1, pageSize: 20, total: 0 })
let controller: AbortController | null = null

const columns = computed(() => [
  { key: 'name', label: t('subscriptionAccounts.columns.name'), sortable: false },
  { key: 'platform_type', label: t('subscriptionAccounts.columns.platformType'), sortable: false },
  { key: 'capacity', label: t('subscriptionAccounts.columns.capacity'), sortable: false },
  { key: 'status', label: t('subscriptionAccounts.columns.status'), sortable: false },
  { key: 'today_stats', label: t('subscriptionAccounts.columns.todayStats'), sortable: false },
  { key: 'groups', label: t('subscriptionAccounts.columns.groups'), sortable: false },
  { key: 'usage_windows', label: t('subscriptionAccounts.columns.usageWindows'), sortable: false },
  { key: 'rate_multiplier', label: t('subscriptionAccounts.columns.rateMultiplier'), sortable: false },
  { key: 'last_used_at', label: t('subscriptionAccounts.columns.lastUsed'), sortable: false },
  { key: 'created_at', label: t('subscriptionAccounts.columns.createdAt'), sortable: false },
])

const subscriptionGroups = computed(() => {
  const byID = new Map<number, { id: number; name: string }>()
  for (const subscription of subscriptionStore.activeSubscriptions) {
    const group = subscription.group
    if (group?.subscription_type !== 'subscription') continue
    byID.set(group.id, { id: group.id, name: group.name })
  }
  return [...byID.values()].sort((a, b) => a.name.localeCompare(b.name))
})

const groupOptions = computed(() => [
  { value: '', label: t('subscriptionAccounts.allGroups') },
  ...subscriptionGroups.value.map((group) => ({ value: String(group.id), label: group.name })),
])

const emptyLabel = computed(() =>
  subscriptionGroups.value.length === 0
    ? t('subscriptionAccounts.noSubscription')
    : t('subscriptionAccounts.empty'),
)

async function loadAccounts() {
  controller?.abort()
  controller = new AbortController()
  loading.value = true
  try {
    const result = await subscriptionAccountsAPI.list(
      {
        page: pagination.page,
        page_size: pagination.pageSize,
        search: appliedSearch.value || undefined,
        group_id: groupFilter.value ? Number(groupFilter.value) : undefined,
      },
      { signal: controller.signal },
    )
    accounts.value = result.items
    pagination.total = result.total
  } catch (error) {
    if ((error as { name?: string })?.name !== 'CanceledError') {
      appStore.showError(extractApiErrorMessage(error, t('common.error')))
    }
  } finally {
    loading.value = false
  }
}

function applySearch() {
  appliedSearch.value = searchDraft.value.trim()
  pagination.page = 1
  loadAccounts()
}

function applyFilters() {
  pagination.page = 1
  loadAccounts()
}

function resetFilters() {
  searchDraft.value = ''
  appliedSearch.value = ''
  groupFilter.value = ''
  pagination.page = 1
  loadAccounts()
}

function changePage(page: number) {
  pagination.page = page
  loadAccounts()
}

function changePageSize(pageSize: number) {
  pagination.pageSize = pageSize
  pagination.page = 1
  loadAccounts()
}

onMounted(async () => {
  await subscriptionStore.fetchActiveSubscriptions().catch(() => [])
  await loadAccounts()
})

onBeforeUnmount(() => controller?.abort())
</script>
