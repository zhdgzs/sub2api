<template>
  <div class="inline-flex min-w-24 flex-col items-start gap-1">
    <span :class="['badge text-xs', display.className]">{{ display.label }}</span>
    <span v-if="display.detail" class="text-[11px] text-gray-400 dark:text-gray-500">
      {{ display.detail }}
    </span>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { SubscriptionAccount } from '@/api/subscriptionAccounts'
import { formatCountdownWithSuffix } from '@/utils/format'

const props = defineProps<{ account: SubscriptionAccount }>()
const { t } = useI18n()

const isFuture = (value?: string) => Boolean(value && new Date(value).getTime() > Date.now())

const display = computed(() => {
  if (props.account.status === 'error') {
    return { label: t('subscriptionAccounts.status.error'), className: 'badge-danger', detail: '' }
  }
  if (props.account.status !== 'active') {
    return { label: t('subscriptionAccounts.status.disabled'), className: 'badge-gray', detail: '' }
  }
  if (isFuture(props.account.rate_limit_reset_at)) {
    return {
      label: t('subscriptionAccounts.status.rateLimited'),
      className: 'badge-warning',
      detail: formatCountdownWithSuffix(props.account.rate_limit_reset_at),
    }
  }
  if (isFuture(props.account.overload_until)) {
    return {
      label: t('subscriptionAccounts.status.overloaded'),
      className: 'badge-danger',
      detail: formatCountdownWithSuffix(props.account.overload_until),
    }
  }
  if (isFuture(props.account.temp_unschedulable_until)) {
    return {
      label: t('subscriptionAccounts.status.cooldown'),
      className: 'badge-warning',
      detail: formatCountdownWithSuffix(props.account.temp_unschedulable_until),
    }
  }
  if (!props.account.schedulable) {
    return { label: t('subscriptionAccounts.status.paused'), className: 'badge-gray', detail: '' }
  }
  return { label: t('subscriptionAccounts.status.available'), className: 'badge-success', detail: '' }
})
</script>
