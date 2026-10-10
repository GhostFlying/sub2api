import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createI18n } from 'vue-i18n'
import AccountModelUsageDialog from '../AccountModelUsageDialog.vue'
import type { AccountModelWindowStats, UsageWindow } from '@/types'

const { getUsageModelStats } = vi.hoisted(() => ({ getUsageModelStats: vi.fn() }))
vi.mock('@/api/admin', () => ({ adminAPI: { accounts: { getUsageModelStats } } }))
function result(window: UsageWindow = '5h'): AccountModelWindowStats {
  const tokens = { input_tokens: 1000, cache_read_tokens: 2000, cache_creation_tokens: 3000, output_tokens: 4000, total_tokens: 10000 }
  return { window, period: window === '5h' ? 'last_5_hours' : 'cycle', start_at: '2026-10-10T01:00:00Z', end_at: '2026-10-10T06:00:00Z', model_source: 'upstream', models: [{ model: 'full-upstream-model-version', requests: 1, ...tokens }], totals: { ...tokens, requests: 1, tokens: 10000, cost: 1 } }
}
function deferred<T>() {
  let resolve!: (value: T) => void
  const promise = new Promise<T>(r => { resolve = r })
  return { promise, resolve }
}
const wrappers: ReturnType<typeof mount>[] = []
function render(show = true, initialWindow: UsageWindow = '5h') {
  const i18n = createI18n({ legacy: false, locale: 'en', missingWarn: false, fallbackWarn: false, messages: { en: {} } })
  const wrapper = mount(AccountModelUsageDialog, { props: { show, accountId: 42, accountName: 'test account', initialWindow }, global: { plugins: [i18n], stubs: { BaseDialog: { props: ['show', 'title'], template: '<div v-if="show"><h1>{{ title }}</h1><slot /></div>' } } } })
  wrappers.push(wrapper)
  return wrapper
}
beforeEach(() => { getUsageModelStats.mockReset(); getUsageModelStats.mockImplementation((_id, window) => Promise.resolve(result(window))) })
afterEach(() => { wrappers.splice(0).forEach(w => w.unmount()) })
describe('AccountModelUsageDialog', () => {
  it('loads only when opened, formats buckets and renders desktop/mobile totals', async () => {
    const wrapper = render(false)
    expect(getUsageModelStats).not.toHaveBeenCalled()
    await wrapper.setProps({ show: true }); await flushPromises()
    expect(getUsageModelStats).toHaveBeenCalledWith(42, '5h')
    expect(wrapper.get('[data-test="model-table"]').text()).toContain('1,000')
    expect(wrapper.get('[data-test="model-table"]').text()).toContain('10,000')
    expect(wrapper.get('[data-test="model-cards"]').text()).toContain('full-upstream-model-version')
    expect(wrapper.get('[data-test="window-period"]').text()).toContain('last5h')
    expect(wrapper.emitted('loaded')?.[0]).toEqual([result()])
  })
  it('selects clicked 7d, loads 5h on demand and reuses loaded tabs', async () => {
    const wrapper = render(true, '7d'); await flushPromises()
    expect(getUsageModelStats).toHaveBeenCalledWith(42, '7d')
    const tab = (text: string) => wrapper.findAll('button').find(b => b.text() === text)!
    await tab('5h').trigger('click'); await flushPromises()
    await tab('7d').trigger('click'); await flushPromises()
    expect(getUsageModelStats).toHaveBeenCalledTimes(2)
  })
  it('shows an empty window with zero totals', async () => {
    getUsageModelStats.mockResolvedValue({ ...result(), models: [], totals: { requests: 0, tokens: 0, cost: 0, input_tokens: 0, cache_read_tokens: 0, cache_creation_tokens: 0, output_tokens: 0, total_tokens: 0 } })
    const wrapper = render(); await flushPromises()
    expect(wrapper.text()).toContain('usageWindow.empty')
    expect(wrapper.get('[data-test="model-table"]').text()).toContain('0')
  })
  it('shows failure instead of zeros and retries', async () => {
    getUsageModelStats.mockRejectedValueOnce(new Error('db unavailable'))
    const wrapper = render(); await flushPromises()
    expect(wrapper.find('[role="alert"]').exists()).toBe(true)
    expect(wrapper.find('[data-test="model-table"]').exists()).toBe(false)
    await wrapper.get('[role="alert"] button').trigger('click'); await flushPromises()
    expect(wrapper.find('[role="alert"]').exists()).toBe(false)
    expect(getUsageModelStats).toHaveBeenCalledTimes(2)
  })
  it('ignores responses after switching window', async () => {
    const slow = deferred<AccountModelWindowStats>()
    getUsageModelStats.mockReturnValueOnce(slow.promise)
    const wrapper = render(); await flushPromises()
    await wrapper.findAll('button').find(b => b.text() === '7d')!.trigger('click'); await flushPromises()
    slow.resolve(result()); await flushPromises()
    expect(wrapper.emitted('loaded')).toEqual([[result('7d')]])
  })
  it('ignores closed responses and reloads for new account', async () => {
    const slow = deferred<AccountModelWindowStats>()
    getUsageModelStats.mockReturnValueOnce(slow.promise)
    const wrapper = render(); await wrapper.setProps({ show: false })
    slow.resolve(result()); await flushPromises()
    expect(wrapper.emitted('loaded')).toBeUndefined()
    await wrapper.setProps({ accountId: 43, show: true }); await flushPromises()
    expect(getUsageModelStats).toHaveBeenLastCalledWith(43, '5h')
    expect(wrapper.emitted('loaded')).toHaveLength(1)
  })
})
