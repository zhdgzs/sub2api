<template>
  <AccountStatusIndicator :account="readonlyAccount" />
</template>

<script setup lang="ts">
import { computed } from 'vue'
import type { SubscriptionAccount } from '@/api/subscriptionAccounts'
import AccountStatusIndicator from '@/components/account/AccountStatusIndicator.vue'
import { toReadonlyAccount } from './accountView'

const props = defineProps<{ account: SubscriptionAccount }>()
const readonlyAccount = computed(() => toReadonlyAccount(props.account))
</script>
