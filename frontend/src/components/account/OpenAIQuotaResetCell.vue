<template>
  <div v-if="visible" class="space-y-1">
    <!--
      Unified action row. Parents that already render their own "local query"
      affordance (e.g. AccountUsageCell's active-sampling refresh) pass it in
      via the #pre-actions slot so the user sees a single row of related
      buttons rather than two near-duplicate "查询" rows.

      The 5h / 7d window bars are deliberately NOT rendered here — the local
      active-sampling display (UsageProgressBar in AccountUsageCell) already
      owns that real estate. This cell is purely about the rate-limit reset
      credit: query its count, consume one if needed.
    -->
    <div class="flex flex-wrap items-center gap-1.5">
      <slot name="pre-actions" />

      <button
        type="button"
        data-testid="quota-count-button"
        class="inline-flex items-center gap-0.5 rounded px-1.5 py-0.5 text-[10px] font-medium text-blue-600 transition-colors hover:bg-blue-50 disabled:cursor-not-allowed disabled:opacity-50 dark:text-blue-400 dark:hover:bg-blue-900/30"
        :disabled="loading || resetting"
        :title="countButtonTitle"
        @click="handleQuery()"
      >
        <svg
          class="h-2.5 w-2.5"
          :class="{ 'animate-spin': loading }"
          fill="none"
          stroke="currentColor"
          viewBox="0 0 24 24"
        >
          <path
            stroke-linecap="round"
            stroke-linejoin="round"
            stroke-width="2"
            d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15"
          />
        </svg>
        {{ t('admin.accounts.openaiQuotaReset.count') }}<span v-if="data"> {{ availableResetCount }}</span>
      </button>

      <button
        type="button"
        data-testid="quota-schedule-button"
        class="inline-flex items-center gap-0.5 rounded px-1.5 py-0.5 text-[10px] font-medium text-emerald-700 transition-colors hover:bg-emerald-50 disabled:cursor-not-allowed disabled:opacity-50 dark:text-emerald-400 dark:hover:bg-emerald-900/30"
        :disabled="scheduling || loading || isShadow || (!currentTask && !canSchedule)"
        :title="scheduleButtonTitle"
        @click="openScheduleDialog"
      >
        <Icon name="clock" size="xs" :class="{ 'animate-spin': scheduling }" />
        {{ currentTask ? taskStatusLabel(currentTask.status) : t('admin.accounts.openaiQuotaReset.schedule') }}
      </button>

      <button
        type="button"
        data-testid="quota-reset-button"
        class="inline-flex items-center gap-0.5 rounded px-1.5 py-0.5 text-[10px] font-medium text-orange-600 transition-colors hover:bg-orange-50 disabled:cursor-not-allowed disabled:opacity-50 dark:text-orange-400 dark:hover:bg-orange-900/30"
        :disabled="resetting || loading || !canReset"
        :title="resetButtonTitle"
        @click="openResetConfirm"
      >
        <svg
          class="h-2.5 w-2.5"
          :class="{ 'animate-spin': resetting }"
          fill="none"
          stroke="currentColor"
          viewBox="0 0 24 24"
        >
          <path
            stroke-linecap="round"
            stroke-linejoin="round"
            stroke-width="2"
            d="M20 12a8 8 0 11-2.343-5.657L20 8m0 0V4m0 4h-4"
          />
        </svg>
        {{ t('admin.accounts.openaiQuotaReset.reset') }}
      </button>
    </div>

    <div
      v-if="autoResetState"
      class="flex flex-wrap items-center gap-1 text-[10px]"
      data-testid="auto-reset-credit-state"
    >
      <span
        class="inline-flex items-center rounded px-1.5 py-0.5 font-medium"
        :class="autoResetStateClass"
      >
        {{ autoResetStateLabel }}
        <span v-if="autoResetState.trigger_window" class="ml-1 tabular-nums">
          {{ autoResetState.trigger_window }}
        </span>
      </span>
      <span v-if="autoResetState.checked_at" class="text-gray-500 dark:text-gray-400">
        {{ formatResetCreditExpiry(autoResetState.checked_at, 'short') }}
      </span>
      <span
        v-if="autoResetState.error_code"
        class="max-w-full truncate text-red-600 dark:text-red-400"
        :title="autoResetState.error_code"
      >
        {{ autoResetState.error_code }}
      </span>
    </div>

    <div v-if="primaryResetCreditExpiry" class="space-y-1">
      <div class="flex flex-wrap items-center gap-1">
        <span
          class="inline-flex max-w-full items-center rounded bg-gray-100 px-1.5 py-0.5 text-[10px] leading-4 text-gray-600 tabular-nums dark:bg-dark-800 dark:text-gray-300"
          :title="t('admin.accounts.openaiQuotaReset.expiresAtFull', { time: formatResetCreditExpiry(primaryResetCreditExpiry, 'full') })"
        >
          {{ t('admin.accounts.openaiQuotaReset.expiresAt', { time: formatResetCreditExpiry(primaryResetCreditExpiry, 'short') }) }}
        </span>
        <button
          v-if="hiddenResetCreditCount > 0"
          type="button"
          data-testid="reset-credit-expiry-toggle"
          class="inline-flex items-center rounded-full bg-gray-100 px-1.5 py-0.5 text-[10px] font-medium leading-4 text-gray-600 transition-colors hover:bg-gray-200 dark:bg-dark-800 dark:text-gray-300 dark:hover:bg-dark-700"
          :aria-expanded="showResetCreditDetails"
          :aria-label="resetCreditDetailsToggleLabel"
          :title="resetCreditDetailsTitle"
          @click="toggleResetCreditDetails"
        >
          +{{ hiddenResetCreditCount }}
        </button>
      </div>

      <div
        v-if="showResetCreditDetails && resetCreditExpirations.length > 1"
        data-testid="reset-credit-expiry-details"
        class="inline-grid max-w-full gap-0.5 rounded border border-gray-200 bg-white px-1.5 py-1 text-[10px] leading-4 text-gray-600 shadow-sm dark:border-dark-700 dark:bg-dark-900 dark:text-gray-300"
      >
        <span class="sr-only">{{ t('admin.accounts.openaiQuotaReset.expirationDetails') }}</span>
        <span
          v-for="(expiresAt, index) in resetCreditExpirations"
          :key="`${expiresAt}-${index}`"
          class="flex min-w-0 items-center gap-1 tabular-nums"
          :title="t('admin.accounts.openaiQuotaReset.expiresAtFull', { time: formatResetCreditExpiry(expiresAt, 'full') })"
        >
          <span class="h-1 w-1 shrink-0 rounded-full bg-gray-400 dark:bg-dark-500" />
          <span class="truncate">{{ formatResetCreditExpiry(expiresAt, 'short') }}</span>
        </span>
      </div>
    </div>

    <!-- Error / success feedback -->
    <div
      v-if="error"
      class="text-[10px] text-red-600 dark:text-red-400"
      :title="error"
    >
      {{ truncatedError }}
    </div>
    <div
      v-else-if="resetWarning"
      class="text-[10px] text-amber-600 dark:text-amber-400"
    >
      {{ resetWarning }}
    </div>
    <div
      v-else-if="resetMessage"
      class="text-[10px] text-emerald-600 dark:text-emerald-400"
    >
      {{ resetMessage }}
    </div>

    <ConfirmDialog
      :show="showResetConfirm"
      :title="t('admin.accounts.openaiQuotaReset.confirmTitle')"
      :message="t('admin.accounts.openaiQuotaReset.confirmMessage', { count: availableResetCount })"
      :confirm-text="t('admin.accounts.openaiQuotaReset.reset')"
      :cancel-text="t('common.cancel')"
      danger
      @confirm="confirmReset"
      @cancel="showResetConfirm = false"
    />

    <BaseDialog
      :show="showScheduleDialog"
      :title="t('admin.accounts.openaiQuotaReset.scheduleTitle')"
      width="narrow"
      @close="showScheduleDialog = false"
    >
      <div v-if="currentTask" class="space-y-4 text-sm">
        <dl class="grid grid-cols-[auto_1fr] gap-x-4 gap-y-2 text-sm">
          <dt class="text-gray-500 dark:text-gray-400">{{ t('admin.accounts.openaiQuotaReset.taskStatus') }}</dt>
          <dd class="font-medium text-gray-900 dark:text-white">{{ taskStatusLabel(currentTask.status) }}</dd>
          <dt class="text-gray-500 dark:text-gray-400">{{ t('admin.accounts.openaiQuotaReset.creditExpiry') }}</dt>
          <dd class="tabular-nums text-gray-900 dark:text-white">{{ formatTaskTime(currentTask.credit_expires_at) }}</dd>
          <dt class="text-gray-500 dark:text-gray-400">{{ t('admin.accounts.openaiQuotaReset.executeAt') }}</dt>
          <dd class="tabular-nums text-gray-900 dark:text-white">{{ formatTaskTime(currentTask.run_at) }}</dd>
          <dt class="text-gray-500 dark:text-gray-400">{{ t('admin.accounts.openaiQuotaReset.dispatches') }}</dt>
          <dd class="tabular-nums text-gray-900 dark:text-white">{{ currentTask.dispatch_count }}</dd>
        </dl>
        <div
          v-if="currentTask.last_error_message"
          class="rounded border border-red-200 bg-red-50 p-2 text-xs text-red-700 dark:border-red-900/50 dark:bg-red-900/20 dark:text-red-300"
        >
          {{ currentTask.last_error_message }}
        </div>
        <div class="flex justify-end gap-2">
          <button
            type="button"
            class="rounded px-3 py-2 text-sm font-medium text-gray-600 hover:bg-gray-100 dark:text-gray-300 dark:hover:bg-dark-700"
            @click="showScheduleDialog = false"
          >
            {{ t('common.close') }}
          </button>
          <button
            v-if="currentTask.can_cancel"
            type="button"
            data-testid="cancel-quota-task"
            class="rounded bg-red-600 px-3 py-2 text-sm font-medium text-white hover:bg-red-700 disabled:opacity-50"
            :disabled="scheduling"
            @click="cancelCurrentTask"
          >
            {{ t('common.cancel') }}
          </button>
          <button
            v-if="currentTask.can_retry"
            type="button"
            data-testid="retry-quota-task"
            class="rounded bg-emerald-600 px-3 py-2 text-sm font-medium text-white hover:bg-emerald-700 disabled:opacity-50"
            :disabled="scheduling"
            @click="retryCurrentTask"
          >
            {{ t('admin.accounts.openaiQuotaReset.retry') }}
          </button>
        </div>
        <div v-if="canSchedule" class="space-y-4 border-t border-gray-200 pt-4 dark:border-dark-700">
          <dl class="grid grid-cols-[auto_1fr] gap-x-4 gap-y-2 text-sm">
            <dt class="text-gray-500 dark:text-gray-400">{{ t('admin.accounts.openaiQuotaReset.creditExpiry') }}</dt>
            <dd class="tabular-nums text-gray-900 dark:text-white">{{ formatTaskTime(primaryResetCreditExpiry) }}</dd>
            <dt class="text-gray-500 dark:text-gray-400">{{ t('admin.accounts.openaiQuotaReset.executeAt') }}</dt>
            <dd class="tabular-nums font-medium text-gray-900 dark:text-white">
              {{ scheduleRunsImmediately ? t('admin.accounts.openaiQuotaReset.executeImmediately') : formatTaskTime(scheduledExecutionTime) }}
            </dd>
          </dl>
          <div>
            <div class="mb-2 text-xs font-medium text-gray-500 dark:text-gray-400">
              {{ t('admin.accounts.openaiQuotaReset.leadTime') }}
            </div>
            <div class="grid grid-cols-3 gap-1 rounded bg-gray-100 p-1 dark:bg-dark-800">
              <button
                v-for="option in leadTimeOptions"
                :key="option"
                type="button"
                class="rounded px-2 py-1.5 text-xs font-medium transition-colors"
                :class="leadTimeMinutes === option ? 'bg-white text-gray-900 shadow-sm dark:bg-dark-600 dark:text-white' : 'text-gray-500 hover:text-gray-900 dark:text-gray-400 dark:hover:text-white'"
                @click="leadTimeMinutes = option"
              >
                {{ leadTimeLabel(option) }}
              </button>
            </div>
          </div>
          <div class="flex justify-end">
            <button
              type="button"
              data-testid="confirm-quota-schedule"
              class="rounded bg-emerald-600 px-3 py-2 text-sm font-medium text-white hover:bg-emerald-700 disabled:opacity-50"
              :disabled="scheduling || !canSchedule"
              @click="confirmSchedule"
            >
              {{ t('admin.accounts.openaiQuotaReset.createSchedule') }}
            </button>
          </div>
        </div>
      </div>

      <div v-else class="space-y-4">
        <dl class="grid grid-cols-[auto_1fr] gap-x-4 gap-y-2 text-sm">
          <dt class="text-gray-500 dark:text-gray-400">{{ t('admin.accounts.openaiQuotaReset.creditExpiry') }}</dt>
          <dd class="tabular-nums text-gray-900 dark:text-white">{{ formatTaskTime(primaryResetCreditExpiry) }}</dd>
          <dt class="text-gray-500 dark:text-gray-400">{{ t('admin.accounts.openaiQuotaReset.executeAt') }}</dt>
          <dd class="tabular-nums font-medium text-gray-900 dark:text-white">
            {{ scheduleRunsImmediately ? t('admin.accounts.openaiQuotaReset.executeImmediately') : formatTaskTime(scheduledExecutionTime) }}
          </dd>
        </dl>

        <div>
          <div class="mb-2 text-xs font-medium text-gray-500 dark:text-gray-400">
            {{ t('admin.accounts.openaiQuotaReset.leadTime') }}
          </div>
          <div class="grid grid-cols-3 gap-1 rounded bg-gray-100 p-1 dark:bg-dark-800">
            <button
              v-for="option in leadTimeOptions"
              :key="option"
              type="button"
              class="rounded px-2 py-1.5 text-xs font-medium transition-colors"
              :class="leadTimeMinutes === option ? 'bg-white text-gray-900 shadow-sm dark:bg-dark-600 dark:text-white' : 'text-gray-500 hover:text-gray-900 dark:text-gray-400 dark:hover:text-white'"
              @click="leadTimeMinutes = option"
            >
              {{ leadTimeLabel(option) }}
            </button>
          </div>
        </div>

        <div class="flex justify-end gap-2 pt-2">
          <button
            type="button"
            class="rounded px-3 py-2 text-sm font-medium text-gray-600 hover:bg-gray-100 dark:text-gray-300 dark:hover:bg-dark-700"
            @click="showScheduleDialog = false"
          >
            {{ t('common.cancel') }}
          </button>
          <button
            type="button"
            data-testid="confirm-quota-schedule"
            class="rounded bg-emerald-600 px-3 py-2 text-sm font-medium text-white hover:bg-emerald-700 disabled:opacity-50"
            :disabled="scheduling || !canSchedule"
            @click="confirmSchedule"
          >
            {{ t('admin.accounts.openaiQuotaReset.createSchedule') }}
          </button>
        </div>
      </div>
    </BaseDialog>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onUnmounted, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import type { Account } from '@/types'
import {
  refreshOpenAIQuota,
  resetOpenAIQuota,
  type OpenAIQuotaUsage,
  type OpenAIQuotaResetResult
} from '@/api/admin/accounts'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Icon from '@/components/icons/Icon.vue'
import {
  backgroundTasksAPI,
  type BackgroundTask,
  type BackgroundTaskStatus
} from '@/api/admin/backgroundTasks'

const props = defineProps<{
  account: Account
}>()

const emit = defineEmits<{
  'account-updated': [account: Account]
}>()

const { t } = useI18n()

// Visible only for OpenAI OAuth accounts.
const visible = computed(() => props.account.platform === 'openai' && props.account.type === 'oauth')

const loading = ref(false)
const resetting = ref(false)
const scheduling = ref(false)
const error = ref<string | null>(null)
const data = ref<OpenAIQuotaUsage | null>(null)
const cachedData = ref<OpenAIQuotaUsage | null>(null)
const resetMessage = ref<string | null>(null)
const resetWarning = ref<string | null>(null)
const showResetConfirm = ref(false)
const showScheduleDialog = ref(false)
const showResetCreditDetails = ref(false)
const currentTask = ref<BackgroundTask | null>(null)
const leadTimeMinutes = ref<10 | 30 | 60>(60)
const leadTimeOptions: Array<10 | 30 | 60> = [10, 30, 60]
const nowMs = ref(Date.now())
let accountGeneration = 0
let taskRequestGeneration = 0
let taskPollTimer: ReturnType<typeof setInterval> | null = null
let taskPollInFlight = false
let creditExpiryTimer: ReturnType<typeof setTimeout> | null = null

type AutoResetCreditState = NonNullable<NonNullable<Account['extra']>['codex_auto_reset_credit_state']>
const validAutoResetStatuses = new Set(['checking', 'available', 'resetting', 'success', 'no_credit', 'failed'])
const autoResetState = computed<AutoResetCreditState | null>(() => {
  if (props.account.extra?.auto_reset_credit_enabled !== true) return null
  const state = props.account.extra?.codex_auto_reset_credit_state
  if (!state || typeof state !== 'object' || !validAutoResetStatuses.has(String(state.status))) return null
  return state
})
const autoResetStateLabel = computed(() => {
  if (!autoResetState.value?.status) return ''
  const keyByStatus: Record<string, string> = {
    checking: 'checking',
    available: 'available',
    resetting: 'resetting',
    success: 'success',
    no_credit: 'noCredit',
    failed: 'failed'
  }
  return t(`admin.accounts.openaiQuotaReset.autoStatus.${keyByStatus[autoResetState.value.status]}`)
})
const autoResetStateClass = computed(() => {
  switch (autoResetState.value?.status) {
    case 'available':
      return 'bg-blue-50 text-blue-700 dark:bg-blue-900/30 dark:text-blue-300'
    case 'success':
      return 'bg-emerald-50 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300'
    case 'no_credit':
    case 'failed':
      return 'bg-red-50 text-red-700 dark:bg-red-900/30 dark:text-red-300'
    case 'resetting':
      return 'bg-orange-50 text-orange-700 dark:bg-orange-900/30 dark:text-orange-300'
    default:
      return 'bg-gray-100 text-gray-600 dark:bg-dark-800 dark:text-gray-300'
  }
})

// Rehydrate the card from the persisted snapshot. Credits that already expired
// are dropped and the count is clamped to what remains: the snapshot has no
// freshness signal, so an unfiltered read would offer to consume credits that no
// longer exist. A snapshot claiming credits with no usable expiration left is
// treated as absent, which keeps the reset button gated on a live query.
const readCachedResetCredits = (account: Account): OpenAIQuotaUsage | null => {
  const cached = account.extra?.codex_reset_credit_snapshot
  if (!cached || typeof cached !== 'object' || Array.isArray(cached)) return null

  const { available_count: count, credits: rawCredits } = cached as {
    available_count?: unknown
    credits?: unknown
  }
  if (typeof count !== 'number' || !Number.isFinite(count)) return null

  const now = Date.now()
  const credits: { expires_at?: string }[] = []
  if (Array.isArray(rawCredits)) {
    for (const credit of rawCredits) {
      if (!credit || typeof credit !== 'object') continue
      const expiresAt = (credit as { expires_at?: unknown }).expires_at
      if (typeof expiresAt !== 'string' || expiresAt.trim() === '') continue
      const expiryTime = new Date(expiresAt).getTime()
      // Unparsable timestamps are kept: they are already rendered verbatim and
      // dropping them would silently understate the available count.
      if (!Number.isNaN(expiryTime) && expiryTime <= now) continue
      credits.push({ expires_at: expiresAt })
    }
  }
  const availableCount = Math.min(Math.max(count, 0), credits.length)
  // A snapshot that claimed credits but has none left is no longer informative;
  // report "unknown" so the operator re-queries instead of trusting it.
  if (count > 0 && availableCount <= 0) return null
  return {
    fetched_at: 0,
    rate_limit_reset_credits: {
      available_count: availableCount,
      credits
    }
  }
}

cachedData.value = readCachedResetCredits(props.account)
data.value = cachedData.value

// 影子账号的额度查询会 resolve 到母账号,但影子本身不支持重置(后端返回 409);
// 重置必须在母账号上进行。前端据此禁用影子的重置入口(外审 F6)。
const isShadow = computed(() => props.account.parent_account_id != null)

const availableResetCount = computed(() => data.value?.rate_limit_reset_credits?.available_count ?? 0)
// Prefer the live payload and fall back to the persisted snapshot only when the
// live state is unknown, so the count and the expirations never come from two
// different generations of the same data.
const resetCreditExpirations = computed(() =>
  ((data.value ?? cachedData.value)?.rate_limit_reset_credits?.credits ?? [])
    .map((credit) => credit.expires_at?.trim() ?? '')
    .filter((expiresAt) => expiresAt.length > 0)
    .sort(compareResetCreditExpiry)
)
const primaryResetCreditExpiry = computed(() => resetCreditExpirations.value[0] ?? '')
const hiddenResetCreditCount = computed(() => Math.max(resetCreditExpirations.value.length - 1, 0))
const canReset = computed(() => availableResetCount.value > 0 && !isShadow.value)
const primaryResetCreditExpiryTime = computed(() => getResetCreditExpiryTime(primaryResetCreditExpiry.value))
const canSchedule = computed(() =>
  Boolean(data.value) &&
  availableResetCount.value > 0 &&
  !isShadow.value &&
  Number.isFinite(primaryResetCreditExpiryTime.value) &&
  primaryResetCreditExpiryTime.value > nowMs.value
)
const scheduledExecutionTime = computed(() => {
  if (!Number.isFinite(primaryResetCreditExpiryTime.value)) return ''
  return new Date(Math.max(nowMs.value, primaryResetCreditExpiryTime.value - leadTimeMinutes.value * 60_000)).toISOString()
})
const scheduleRunsImmediately = computed(() => {
  if (!scheduledExecutionTime.value) return false
  return new Date(scheduledExecutionTime.value).getTime() <= nowMs.value + 1000
})

const resetCreditDetailsTitle = computed(() =>
  resetCreditExpirations.value
    .map((expiresAt) => formatResetCreditExpiry(expiresAt, 'full'))
    .join('\n')
)

const resetCreditDetailsToggleLabel = computed(() => {
  if (showResetCreditDetails.value) {
    return t('admin.accounts.openaiQuotaReset.collapseExpirations')
  }
  return t('admin.accounts.openaiQuotaReset.expandExpirations', { count: hiddenResetCreditCount.value })
})

const resetButtonTitle = computed(() => {
  if (isShadow.value) return t('admin.accounts.openaiQuotaReset.resetTooltipShadow')
  if (!data.value) return t('admin.accounts.openaiQuotaReset.resetTooltipNeedQuery')
  if (!canReset.value) return t('admin.accounts.openaiQuotaReset.resetTooltipNoCredits')
  return t('admin.accounts.openaiQuotaReset.resetTooltipReady')
})

const scheduleButtonTitle = computed(() => {
  if (isShadow.value) return t('admin.accounts.openaiQuotaReset.scheduleTooltipShadow')
  if (currentTask.value) return t('admin.accounts.openaiQuotaReset.scheduleTooltipExisting')
  if (!data.value) return t('admin.accounts.openaiQuotaReset.scheduleTooltipNeedQuery')
  if (!primaryResetCreditExpiry.value) return t('admin.accounts.openaiQuotaReset.scheduleTooltipNoExpiry')
  if (!canSchedule.value) return t('admin.accounts.openaiQuotaReset.scheduleTooltipUnavailable')
  return t('admin.accounts.openaiQuotaReset.scheduleTooltipReady')
})

// "次数" button doubles as the upstream-query trigger and the count display.
// Tooltip differs between "click to load" (no data yet) and "click to refresh".
const countButtonTitle = computed(() => {
  if (!data.value) return t('admin.accounts.openaiQuotaReset.countTooltipLoad')
  return t('admin.accounts.openaiQuotaReset.countTooltipRefresh')
})

const truncatedError = computed(() => {
  if (!error.value) return ''
  return error.value.length > 80 ? `${error.value.slice(0, 80)}…` : error.value
})

const getResetCreditExpiryTime = (value: string): number => {
  const time = new Date(value).getTime()
  return Number.isNaN(time) ? Number.POSITIVE_INFINITY : time
}

const compareResetCreditExpiry = (a: string, b: string): number => {
  const diff = getResetCreditExpiryTime(a) - getResetCreditExpiryTime(b)
  if (diff !== 0) return diff
  return a.localeCompare(b)
}

const formatResetCreditExpiry = (value: string, style: 'short' | 'full'): string => {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value

  const options: Intl.DateTimeFormatOptions = {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit'
  }
  if (style === 'full') {
    options.year = 'numeric'
  }

  return new Intl.DateTimeFormat(undefined, options).format(date)
}

const formatTaskTime = (value?: string | null): string => {
  if (!value) return '-'
  return formatResetCreditExpiry(value, 'full')
}

const leadTimeLabel = (minutes: 10 | 30 | 60): string =>
  minutes === 60
    ? t('admin.accounts.openaiQuotaReset.oneHour')
    : t('admin.accounts.openaiQuotaReset.minutes', { count: minutes })

const taskStatusLabel = (status: BackgroundTaskStatus): string =>
  t(`admin.accounts.openaiQuotaReset.taskStatuses.${status}`)

const taskMatchesPrimaryCredit = (task: BackgroundTask): boolean => {
  if (!task.credit_expires_at || !primaryResetCreditExpiry.value) return false
  const taskExpiry = new Date(task.credit_expires_at).getTime()
  const primaryExpiry = primaryResetCreditExpiryTime.value
  if (Number.isFinite(taskExpiry) && Number.isFinite(primaryExpiry)) {
    return taskExpiry === primaryExpiry
  }
  return task.credit_expires_at === primaryResetCreditExpiry.value
}

const extractErrorMessage = (e: unknown): string => {
  // The project's axios response interceptor (api/client.ts) flattens server
  // errors into { status, code, message, reason, ... } and re-rejects them, so
  // the message lives at the top level rather than under .response.data. Fall
  // back to the raw axios shape for the cancellation/network branches that
  // bypass the flattening, and finally to the generic i18n string.
  const err = e as {
    message?: string
    reason?: string
    response?: { data?: { message?: string; error?: string } }
  }
  return (
    err?.message ||
    err?.reason ||
    err?.response?.data?.message ||
    err?.response?.data?.error ||
    t('common.error')
  )
}

const toggleResetCreditDetails = () => {
  if (hiddenResetCreditCount.value <= 0) return
  showResetCreditDetails.value = !showResetCreditDetails.value
}

const refreshNow = () => {
  nowMs.value = Date.now()
}

const clearCreditExpiryTimer = () => {
  if (creditExpiryTimer != null) {
    clearTimeout(creditExpiryTimer)
    creditExpiryTimer = null
  }
}

const scheduleCreditExpiryRefresh = () => {
  clearCreditExpiryTimer()
  const expiresAt = primaryResetCreditExpiryTime.value
  if (!Number.isFinite(expiresAt)) return
  const delay = expiresAt - Date.now()
  if (delay <= 0) {
    refreshNow()
    return
  }
  creditExpiryTimer = setTimeout(() => {
    refreshNow()
    scheduleCreditExpiryRefresh()
  }, Math.min(delay + 50, 2_147_483_647))
}

const handleQuery = async () => {
  if (loading.value) return
  const generation = accountGeneration
  const accountID = props.account.id
  loading.value = true
  error.value = null
  resetMessage.value = null
  resetWarning.value = null
  showResetCreditDetails.value = false
  try {
    const result = await refreshOpenAIQuota(accountID)
    if (generation !== accountGeneration || accountID !== props.account.id) return
    // The upstream read succeeded even when the snapshot write was rejected, so
    // the live count is always adopted. Only the persisted view is left alone,
    // which keeps the displayed expirations consistent with what is stored.
    data.value = result
    if (result.cache_persisted) {
      cachedData.value = result
    } else {
      resetWarning.value = t('admin.accounts.openaiQuotaReset.refreshCachePersistFailed')
    }
    refreshNow()
    scheduleCreditExpiryRefresh()
  } catch (e) {
    if (generation === accountGeneration && accountID === props.account.id) {
      error.value = extractErrorMessage(e)
    }
  } finally {
    if (generation === accountGeneration && accountID === props.account.id) {
      await loadCurrentTask(false)
    }
    if (generation === accountGeneration && accountID === props.account.id) {
      loading.value = false
    }
  }
}

const loadCurrentTask = async (preserveCurrent = true) => {
  if (!visible.value) {
    currentTask.value = null
    return
  }
  const generation = accountGeneration
  const accountID = props.account.id
  const requestGeneration = ++taskRequestGeneration
  const currentTaskID = preserveCurrent ? currentTask.value?.id : undefined
  try {
    const result = await backgroundTasksAPI.list({
      task_type: 'openai_quota_reset',
      resource_type: 'openai_account',
      resource_id: String(accountID),
      page: 1,
      page_size: 20
    })
    if (
      generation !== accountGeneration ||
      accountID !== props.account.id ||
      requestGeneration !== taskRequestGeneration
    ) return
    const actionableTasks = result.items.filter((task) =>
      ['pending', 'running', 'retry_wait', 'indeterminate'].includes(task.status)
    )
    const preservedTask = result.items.find((task) => task.id === currentTaskID)
    const matchingTask = actionableTasks.find(taskMatchesPrimaryCredit)
    currentTask.value = preservedTask ?? matchingTask ?? (
      !data.value || !canSchedule.value ? actionableTasks[0] ?? null : null
    )
  } catch (e) {
    // Task discovery is secondary to the quota cell; surface the error only
    // after an explicit task action.
    console.error('[OpenAIQuotaResetCell] Failed to load background task', e)
  }
}

const openScheduleDialog = async () => {
  if (scheduling.value || loading.value) return
  const generation = accountGeneration
  const accountID = props.account.id
  taskRequestGeneration++
  scheduling.value = true
  refreshNow()
  await loadCurrentTask(true)
  if (generation !== accountGeneration || accountID !== props.account.id) return
  scheduling.value = false
  if (!currentTask.value && !canSchedule.value) {
    error.value = scheduleButtonTitle.value
    return
  }
  leadTimeMinutes.value = 60
  showScheduleDialog.value = true
}

const confirmSchedule = async () => {
  refreshNow()
  if (scheduling.value || !canSchedule.value || !primaryResetCreditExpiry.value) return
  const generation = accountGeneration
  const accountID = props.account.id
  scheduling.value = true
  error.value = null
  try {
    const response = await backgroundTasksAPI.createOpenAIQuotaReset(accountID, {
      expected_expires_at: primaryResetCreditExpiry.value,
      lead_time_minutes: leadTimeMinutes.value
    })
    if (generation !== accountGeneration || accountID !== props.account.id) return
    currentTask.value = response.task
    resetMessage.value = response.created
      ? t('admin.accounts.openaiQuotaReset.scheduleCreated')
      : t('admin.accounts.openaiQuotaReset.scheduleAlreadyExists')
  } catch (e) {
    if (generation === accountGeneration && accountID === props.account.id) {
      error.value = extractErrorMessage(e)
      await loadCurrentTask(true)
    }
  } finally {
    if (generation === accountGeneration && accountID === props.account.id) {
      scheduling.value = false
    }
  }
}

const cancelCurrentTask = async () => {
  if (!currentTask.value?.can_cancel || scheduling.value) return
  const generation = accountGeneration
  const accountID = props.account.id
  const taskID = currentTask.value.id
  taskRequestGeneration++
  scheduling.value = true
  error.value = null
  try {
    await backgroundTasksAPI.cancel(taskID)
    if (generation !== accountGeneration || accountID !== props.account.id) return
    currentTask.value = null
    showScheduleDialog.value = false
    resetMessage.value = t('admin.accounts.openaiQuotaReset.scheduleCanceled')
  } catch (e) {
    if (generation === accountGeneration && accountID === props.account.id) {
      error.value = extractErrorMessage(e)
      await loadCurrentTask(true)
    }
  } finally {
    if (generation === accountGeneration && accountID === props.account.id) {
      scheduling.value = false
    }
  }
}

const retryCurrentTask = async () => {
  if (!currentTask.value?.can_retry || scheduling.value) return
  const generation = accountGeneration
  const accountID = props.account.id
  const taskID = currentTask.value.id
  taskRequestGeneration++
  scheduling.value = true
  error.value = null
  try {
    const task = await backgroundTasksAPI.retry(taskID)
    if (generation !== accountGeneration || accountID !== props.account.id) return
    currentTask.value = task
    resetMessage.value = t('admin.accounts.openaiQuotaReset.retryQueued')
  } catch (e) {
    if (generation === accountGeneration && accountID === props.account.id) {
      error.value = extractErrorMessage(e)
      await loadCurrentTask(true)
    }
  } finally {
    if (generation === accountGeneration && accountID === props.account.id) {
      scheduling.value = false
    }
  }
}

const openResetConfirm = () => {
  if (resetting.value || loading.value) return
  if (!canReset.value) {
    error.value = t('admin.accounts.openaiQuotaReset.noCreditsAvailable')
    return
  }
  showResetConfirm.value = true
}

const confirmReset = async () => {
  showResetConfirm.value = false
  if (resetting.value) return
  if (!canReset.value) {
    error.value = t('admin.accounts.openaiQuotaReset.noCreditsAvailable')
    return
  }
  resetting.value = true
  const generation = accountGeneration
  const accountID = props.account.id
  error.value = null
  resetMessage.value = null
  resetWarning.value = null
  try {
    const result: OpenAIQuotaResetResult = await resetOpenAIQuota(accountID)
    if (generation !== accountGeneration || accountID !== props.account.id) return
    showResetCreditDetails.value = false
    if (result.cache_refreshed && result.quota) {
      data.value = result.quota
      cachedData.value = result.quota
      refreshNow()
      scheduleCreditExpiryRefresh()
    } else {
      // A credit was consumed but the post-reset count could not be read back.
      // Whatever we still hold is one generation stale, so report the count as
      // unknown instead of letting a second consumption start from stale data.
      data.value = null
    }
    if (result.account) emit('account-updated', result.account)

    if (result.warning_code === 'reset_credit_cache_refresh_failed') {
      resetWarning.value = t('admin.accounts.openaiQuotaReset.resetCacheRefreshFailed')
    } else if (result.warning_code === 'account_state_recovery_failed') {
      resetWarning.value = t('admin.accounts.openaiQuotaReset.resetAccountRecoveryFailed')
    } else if (result.warning_code === 'account_state_refresh_failed') {
      resetWarning.value = t('admin.accounts.openaiQuotaReset.resetAccountRefreshFailed')
    } else {
      resetMessage.value = t('admin.accounts.openaiQuotaReset.resetSuccess', {
        windows: result.windows_reset
      })
    }
    await loadCurrentTask(false)
  } catch (e) {
    if (generation === accountGeneration && accountID === props.account.id) {
      error.value = extractErrorMessage(e)
    }
  } finally {
    if (generation === accountGeneration && accountID === props.account.id) {
      resetting.value = false
    }
  }
}

watch(
  () => props.account.id,
  () => {
    // Account row may be reused across paginated lists; reset local state.
    accountGeneration++
    taskRequestGeneration++
    taskPollInFlight = false
    clearCreditExpiryTimer()
    cachedData.value = readCachedResetCredits(props.account)
    data.value = cachedData.value
    error.value = null
    resetMessage.value = null
    resetWarning.value = null
    loading.value = false
    resetting.value = false
    scheduling.value = false
    showResetConfirm.value = false
    showScheduleDialog.value = false
    showResetCreditDetails.value = false
    currentTask.value = null
    leadTimeMinutes.value = 60
    refreshNow()
  }
)

watch(
  resetCreditExpirations,
  () => {
    if (hiddenResetCreditCount.value <= 0) {
      showResetCreditDetails.value = false
    }
    scheduleCreditExpiryRefresh()
  },
  { immediate: true }
)

watch(
  () => currentTask.value?.status,
  (status) => {
    if (taskPollTimer != null) {
      clearInterval(taskPollTimer)
      taskPollTimer = null
    }
    if (status === 'pending' || status === 'running' || status === 'retry_wait') {
      taskPollTimer = setInterval(() => {
        if (taskPollInFlight || scheduling.value || loading.value) return
        taskPollInFlight = true
        void loadCurrentTask(true).finally(() => {
          taskPollInFlight = false
        })
      }, 10_000)
    }
  }
)

onUnmounted(() => {
  accountGeneration++
  taskRequestGeneration++
  taskPollInFlight = false
  clearCreditExpiryTimer()
  if (taskPollTimer != null) clearInterval(taskPollTimer)
})
</script>
