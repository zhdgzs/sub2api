<template>
  <div v-if="stats" class="space-y-0.5 text-xs">
    <div class="flex items-center gap-1">
      <span class="text-gray-500 dark:text-gray-400">
        {{ t('admin.accounts.stats.requests') }}:
      </span>
      <span class="font-medium text-gray-700 dark:text-gray-300">
        {{ formatNumber(stats.requests) }}
      </span>
    </div>
    <div class="flex items-center gap-1">
      <span class="text-gray-500 dark:text-gray-400">
        {{ t('admin.accounts.stats.tokens') }}:
      </span>
      <span class="font-medium text-gray-700 dark:text-gray-300">
        {{ formatTokens(stats.tokens) }}
      </span>
    </div>
  </div>
  <div v-else class="text-xs text-gray-400">-</div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import type { WindowStats } from '@/types'
import { formatNumber } from '@/utils/format'

defineProps<{ stats?: WindowStats | null }>()

const { t } = useI18n()

const formatTokens = (tokens: number): string => {
  if (tokens >= 1_000_000) return `${(tokens / 1_000_000).toFixed(2)}M`
  if (tokens >= 1_000) return `${(tokens / 1_000).toFixed(1)}K`
  return formatNumber(tokens)
}
</script>
