<template>
  <div class="grid min-w-36 grid-cols-1 gap-1.5">
    <CapacityLine
      :label="t('subscriptionAccounts.capacity.concurrency')"
      :used="capacity.current_concurrency"
      :limit="capacity.concurrency"
    />
    <CapacityLine
      v-if="capacity.window_cost_limit != null"
      :label="t('subscriptionAccounts.capacity.windowCost')"
      :used="capacity.current_window_cost ?? 0"
      :limit="capacity.window_cost_limit"
      prefix="$"
    />
    <CapacityLine
      v-if="capacity.max_sessions != null"
      :label="t('subscriptionAccounts.capacity.sessions')"
      :used="capacity.active_sessions ?? 0"
      :limit="capacity.max_sessions"
    />
    <CapacityLine
      v-if="capacity.base_rpm != null"
      label="RPM"
      :used="capacity.current_rpm ?? 0"
      :limit="capacity.base_rpm"
    />
    <CapacityLine
      v-if="capacity.quota_daily_limit != null"
      :label="t('subscriptionAccounts.capacity.daily')"
      :used="capacity.quota_daily_used ?? 0"
      :limit="capacity.quota_daily_limit"
      prefix="$"
    />
    <CapacityLine
      v-if="capacity.quota_weekly_limit != null"
      :label="t('subscriptionAccounts.capacity.weekly')"
      :used="capacity.quota_weekly_used ?? 0"
      :limit="capacity.quota_weekly_limit"
      prefix="$"
    />
    <CapacityLine
      v-if="capacity.quota_limit != null"
      :label="t('subscriptionAccounts.capacity.total')"
      :used="capacity.quota_used ?? 0"
      :limit="capacity.quota_limit"
      prefix="$"
    />
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import type { SubscriptionAccountCapacity } from '@/api/subscriptionAccounts'
import CapacityLine from './SubscriptionAccountCapacityLine.vue'

defineProps<{ capacity: SubscriptionAccountCapacity }>()
const { t } = useI18n()
</script>
