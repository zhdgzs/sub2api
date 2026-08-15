<template>
  <div class="subscription-account-usage">
    <AccountUsageCell
      :account="readonlyAccount"
      :batched-usage="account.usage ?? null"
      :request-batched-usage="ignoreUsageRequest"
    />
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import type { SubscriptionAccount } from '@/api/subscriptionAccounts'
import type { Account } from '@/types'
import AccountUsageCell from '@/components/account/AccountUsageCell.vue'
import { toReadonlyAccount } from './accountView'

const props = defineProps<{ account: SubscriptionAccount }>()
const readonlyAccount = computed(() => toReadonlyAccount(props.account))

// 传入批量加载器后，原组件只消费用户接口随列表返回的数据，不会调用管理员接口。
const ignoreUsageRequest = (_account: Account, _options?: { force?: boolean }) => undefined
</script>

<style scoped>
/* 普通用户仅查看用量，隐藏查询、重置、次数及探测类按钮。 */
.subscription-account-usage :deep(button) {
  display: none;
}
</style>
