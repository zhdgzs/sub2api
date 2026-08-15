<template>
  <div v-if="windows.length" class="grid min-w-44 gap-2">
    <div v-for="window in windows" :key="window.key" class="grid grid-cols-[4.5rem_1fr] items-center gap-2">
      <span class="truncate text-[11px] text-gray-500 dark:text-gray-400" :title="label(window.key)">
        {{ label(window.key) }}
      </span>
      <div class="min-w-24">
        <div class="flex items-center justify-between gap-2 text-[10px]">
          <span class="font-mono text-gray-700 dark:text-gray-300">{{ valueText(window) }}</span>
          <span v-if="window.resets_at" class="text-gray-400" :title="formatDateTime(window.resets_at)">
            {{ formatCountdownWithSuffix(window.resets_at) }}
          </span>
        </div>
        <div class="mt-1 h-1.5 overflow-hidden rounded-full bg-gray-100 dark:bg-dark-700">
          <div
            class="h-full rounded-full bg-cyan-600 transition-[width] dark:bg-cyan-400"
            :style="{ width: `${Math.min(100, Math.max(0, window.utilization ?? 0))}%` }"
          />
        </div>
      </div>
    </div>
  </div>
  <span v-else class="text-sm text-gray-400">-</span>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import type { SubscriptionAccountUsageWindow } from '@/api/subscriptionAccounts'
import { formatCountdownWithSuffix, formatDateTime } from '@/utils/format'

defineProps<{ windows: SubscriptionAccountUsageWindow[] }>()
const { t } = useI18n()

const knownLabels: Record<string, string> = {
  five_hour: '5h',
  seven_day: '7d',
  seven_day_sonnet: '7d Sonnet',
  seven_day_fable: '7d Fable',
  thirty_day: '30d',
  gemini_shared_daily: 'Shared / day',
  gemini_pro_daily: 'Pro / day',
  gemini_flash_daily: 'Flash / day',
  gemini_shared_minute: 'Shared / min',
  gemini_pro_minute: 'Pro / min',
  gemini_flash_minute: 'Flash / min',
  grok_requests: 'Requests',
  grok_tokens: 'Tokens',
}

const label = (key: string) => {
  if (knownLabels[key]) return knownLabels[key]
  if (key.startsWith('antigravity:')) return key.slice('antigravity:'.length)
  return t('subscriptionAccounts.usage.unknown')
}
const valueText = (window: SubscriptionAccountUsageWindow) => {
  if (window.used != null && window.limit != null) return `${window.used}/${window.limit}`
  if (window.utilization != null) return `${window.utilization.toFixed(1)}%`
  return '-'
}
</script>
