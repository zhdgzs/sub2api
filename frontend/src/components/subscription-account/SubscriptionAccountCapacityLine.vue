<template>
  <div class="grid grid-cols-[2.75rem_1fr_auto] items-center gap-2 text-[11px]">
    <span class="text-gray-500 dark:text-gray-400">{{ label }}</span>
    <div class="h-1.5 w-16 overflow-hidden rounded-full bg-gray-100 dark:bg-dark-700">
      <div :class="barClass" class="h-full rounded-full transition-[width]" :style="{ width: `${percent}%` }" />
    </div>
    <span class="min-w-12 text-right font-mono text-gray-700 dark:text-gray-300">
      {{ prefix }}{{ formatValue(used) }}/{{ prefix }}{{ formatValue(limit) }}
    </span>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'

const props = withDefaults(defineProps<{
  label: string
  used: number
  limit: number
  prefix?: string
}>(), { prefix: '' })

const percent = computed(() => {
  if (props.limit <= 0) return 0
  return Math.min(100, Math.max(0, (props.used / props.limit) * 100))
})
const barClass = computed(() => {
  if (percent.value >= 95) return 'bg-red-500'
  if (percent.value >= 75) return 'bg-amber-500'
  return 'bg-emerald-500'
})
const formatValue = (value: number) => Number.isInteger(value) ? String(value) : value.toFixed(2)
</script>
