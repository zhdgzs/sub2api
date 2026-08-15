<template>
  <AccountCapacityCell :account="readonlyAccount" />
</template>

<script setup lang="ts">
import { computed } from 'vue'
import type { SubscriptionAccount } from '@/api/subscriptionAccounts'
import AccountCapacityCell from '@/components/account/AccountCapacityCell.vue'
import { toReadonlyAccount } from './accountView'

const props = defineProps<{ account: SubscriptionAccount }>()
const readonlyAccount = computed(() => toReadonlyAccount(props.account))
</script>
