<template>
  <BaseDialog :show="show" :title="`${t('admin.accounts.usageWindow.modelDetailsTitle')} · ${accountName}`" width="wide" @close="emit('close')">
    <div class="flex gap-2" role="group" :aria-label="t('admin.accounts.usageWindow.modelDetailsTitle')">
      <button v-for="window in windows" :key="window" type="button" :aria-pressed="selected === window" :class="['rounded-lg px-4 py-2 text-sm focus-visible:ring-2 focus-visible:ring-primary-500', selected === window ? 'bg-primary-600 text-white' : 'bg-gray-100 text-gray-700 dark:bg-dark-700 dark:text-gray-200']" @click="selectWindow(window)">{{ window }}</button>
    </div>
    <p class="my-3 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.accounts.usageWindow.recordedOnly') }}</p>
    <p v-if="loading" role="status" class="py-6 text-sm text-gray-500">{{ t('admin.accounts.usageWindow.loading') }}</p>
    <div v-else-if="failed" role="alert" class="py-6 text-sm text-red-600 dark:text-red-400">
      {{ t('admin.accounts.usageWindow.failed') }}
      <button type="button" class="ml-2 rounded underline focus-visible:ring-2 focus-visible:ring-primary-500" @click="load(true)">{{ t('admin.accounts.usageWindow.retry') }}</button>
    </div>
    <template v-else-if="snapshot">
      <p data-test="window-period" class="mb-4 break-words text-xs text-gray-600 dark:text-gray-300">
        {{ periodLabel }} · {{ formatTime(snapshot.start_at) }} — {{ formatTime(snapshot.end_at) }}
      </p>
      <p v-if="snapshot.models.length === 0" class="py-6 text-sm text-gray-500">{{ t('admin.accounts.usageWindow.empty') }}</p>
      <table class="hidden w-full table-fixed text-sm md:table" data-test="model-table">
        <thead class="text-gray-500 dark:text-gray-400"><tr><th class="w-1/3 pb-3 text-left">{{ t('admin.accounts.usageWindow.model') }}</th><th v-for="column in columns" :key="column.key" class="pb-3 text-right text-xs">{{ t(`admin.accounts.usageWindow.${column.label}`) }}</th></tr></thead>
        <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
          <tr v-for="model in snapshot.models" :key="model.model"><td class="break-words py-3 pr-3 font-medium text-gray-800 dark:text-gray-100">{{ model.model }}</td><td v-for="column in columns" :key="column.key" class="break-words py-3 pl-2 text-right tabular-nums text-gray-700 dark:text-gray-200">{{ formatNumber(model[column.key]) }}</td></tr>
        </tbody>
        <tfoot class="border-t border-gray-200 font-semibold dark:border-dark-600"><tr><td class="py-3 text-gray-800 dark:text-gray-100">{{ t('admin.accounts.usageWindow.sum') }}</td><td v-for="column in columns" :key="column.key" class="break-words py-3 pl-2 text-right tabular-nums text-gray-800 dark:text-gray-100">{{ formatNumber(snapshot.totals[column.key]) }}</td></tr></tfoot>
      </table>
      <div class="space-y-3 md:hidden" data-test="model-cards">
        <div v-for="model in [...snapshot.models, { ...snapshot.totals, model: t('admin.accounts.usageWindow.sum') }]" :key="model.model" class="rounded-xl border border-gray-200 p-3 dark:border-dark-600">
          <div class="mb-2 break-words font-medium text-gray-800 dark:text-gray-100">{{ model.model }}</div>
          <dl class="grid grid-cols-2 gap-2 text-xs"><div v-for="column in columns" :key="column.key"><dt class="text-gray-500 dark:text-gray-400">{{ t(`admin.accounts.usageWindow.${column.label}`) }}</dt><dd class="break-words tabular-nums text-gray-800 dark:text-gray-100">{{ formatNumber(model[column.key]) }}</dd></div></dl>
        </div>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api/admin'
import BaseDialog from '@/components/common/BaseDialog.vue'
import type { AccountModelWindowStats, ModelWindowTokens, UsageWindow } from '@/types'

const props = defineProps<{ show: boolean; accountId: number; accountName: string; initialWindow: UsageWindow }>()
const emit = defineEmits<{ close: []; loaded: [snapshot: AccountModelWindowStats] }>()
const { t, locale } = useI18n()
const windows: UsageWindow[] = ['5h', '7d']
const columns: Array<{ key: keyof ModelWindowTokens; label: string }> = [
  { key: 'input_tokens', label: 'input' },
  { key: 'cache_read_tokens', label: 'cacheRead' },
  { key: 'cache_creation_tokens', label: 'cacheWrite' },
  { key: 'output_tokens', label: 'output' },
  { key: 'total_tokens', label: 'total' }
]
const selected = ref<UsageWindow>(props.initialWindow)
const snapshot = ref<AccountModelWindowStats | null>(null)
const loading = ref(false)
const failed = ref(false)
const loaded = new Map<UsageWindow, { snapshot: AccountModelWindowStats; at: number }>()
let generation = 0
let disposed = false
const formatNumber = (value: number) => new Intl.NumberFormat(locale.value || 'en').format(value)
const formatTime = (value: string) => new Date(value).toLocaleString(locale.value || 'en')
const periodLabel = computed(() => t(`admin.accounts.usageWindow.${snapshot.value?.period === 'cycle' ? 'cycle' : snapshot.value?.period === 'last_5_hours' ? 'last5h' : 'last7d'}`))

async function load(retry = false) {
  const requestGeneration = ++generation
  const accountId = props.accountId
  const window = selected.value
  snapshot.value = null
  failed.value = false
  const cached = loaded.get(window)
  if (!retry && cached && Date.now() - cached.at < 60_000) {
    snapshot.value = cached.snapshot
    loading.value = false
    emit('loaded', cached.snapshot)
    return
  }
  loading.value = true
  const current = () => !disposed && props.show && generation === requestGeneration && accountId === props.accountId && window === selected.value
  try {
    const result = await adminAPI.accounts.getUsageModelStats(accountId, window)
    if (!current()) return
    snapshot.value = result
    loaded.set(window, { snapshot: result, at: Date.now() })
    emit('loaded', result)
  } catch {
    if (current()) failed.value = true
  } finally {
    if (current()) loading.value = false
  }
}
function selectWindow(window: UsageWindow) {
  if (selected.value === window) return
  selected.value = window
  void load()
}
watch(() => [props.show, props.accountId, props.initialWindow] as const, (next, previous) => {
  generation++
  if (!previous || next[1] !== previous[1] || (next[0] && !previous[0])) loaded.clear()
  if (!props.show) return
  selected.value = props.initialWindow
  void load()
}, { immediate: true })
onBeforeUnmount(() => { disposed = true; generation++ })
</script>
