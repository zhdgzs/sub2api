<template>
  <span
    class="inline-flex max-w-full items-center gap-1"
    data-testid="codex-credits-balance"
    :title="updatedAtTitle"
  >
    {{ t('admin.accounts.openaiQuotaReset.points') }}
    <span class="truncate tabular-nums">{{ creditsDisplay }}</span>
  </span>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { OpenAICredits } from '@/api/admin/accounts'

const props = defineProps<{
  credits?: OpenAICredits | null
  fetchedAt?: number
}>()
const { t } = useI18n()

const creditsDisplay = computed(() => {
  const credits = props.credits
  if (!credits) return '—'
  if (credits.unlimited) return t('admin.accounts.openaiQuotaReset.pointsUnlimited')
  if (!credits.has_credits) return '0'
  const balance = credits.balance?.trim()
  // Preserve the upstream decimal string, including fractional points.
  if (balance && Number.isFinite(Number(balance)) && Number(balance) >= 0) return balance
  return t('admin.accounts.openaiQuotaReset.pointsAvailable')
})

const updatedAtTitle = computed(() => {
  if (!props.fetchedAt || !Number.isFinite(props.fetchedAt)) return undefined
  const date = new Date(props.fetchedAt * 1000)
  if (Number.isNaN(date.getTime())) return undefined
  return t('admin.accounts.openaiQuotaReset.pointsUpdatedAt', { time: date.toLocaleString() })
})
</script>
