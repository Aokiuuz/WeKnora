<template>
  <SettingDrawer
    v-model:visible="drawerVisible"
    :title="t('modelSettings.usage.title')"
    :description="t('modelSettings.usage.description')"
    icon="chart-analytics"
    width="920px"
    :min-width="720"
    :max-width="1280"
    storage-key="setting-drawer:width:model-usage"
    hide-footer
  >
    <div class="usage-toolbar">
      <t-radio-group v-model="windowDays" variant="default-filled" @change="loadUsage">
        <t-radio-button :value="7">7 {{ t('modelSettings.usage.days') }}</t-radio-button>
        <t-radio-button :value="30">30 {{ t('modelSettings.usage.days') }}</t-radio-button>
        <t-radio-button :value="90">90 {{ t('modelSettings.usage.days') }}</t-radio-button>
      </t-radio-group>
      <t-button variant="outline" :loading="loading" @click="loadUsage">
        <template #icon><t-icon name="refresh" /></template>
        {{ t('common.refresh') }}
      </t-button>
    </div>

    <t-loading :loading="loading" size="small">
      <div v-if="!loading && rows.length === 0" class="usage-empty">
        <t-empty :description="t('modelSettings.usage.empty')" />
      </div>
      <div v-else class="usage-list">
        <article v-for="row in rows" :key="row.model_id" class="usage-card">
          <header class="usage-card__header">
            <div>
              <h4>{{ modelName(row.model_id) }}</h4>
              <span>{{ modelType(row.model_id) }}</span>
            </div>
            <div class="usage-card__cost">{{ formatCosts(row.costs) }}</div>
          </header>
          <div class="usage-metrics">
            <div><span>{{ t('modelSettings.usage.calls') }}</span><strong>{{ integer(row.call_count) }}</strong></div>
            <div><span>{{ t('modelSettings.usage.tokens') }}</span><strong>{{ integer(row.total_tokens) }}</strong></div>
            <div><span>{{ t('modelSettings.usage.latency') }}</span><strong>{{ decimal(row.average_duration_ms) }} ms</strong></div>
            <div><span>{{ t('modelSettings.usage.unpriced') }}</span><strong>{{ integer(row.unpriced_calls) }}</strong></div>
          </div>
          <div class="cache-grid">
            <section>
              <h5>{{ t('modelSettings.usage.providerCache') }}</h5>
              <strong>{{ percent(row.provider_cache.hit_rate) }}</strong>
              <p>{{ t('modelSettings.usage.providerDenominator', { value: integer(row.provider_cache.observed_tokens) }) }}</p>
            </section>
            <section>
              <h5>{{ t('modelSettings.usage.applicationCache') }}</h5>
              <strong>{{ percent(row.application_cache.hit_rate) }}</strong>
              <p>{{ t('modelSettings.usage.applicationDenominator', { value: integer(row.application_cache.observed_items), bypass: integer(row.application_cache.bypass_items) }) }}</p>
            </section>
          </div>
        </article>
      </div>
    </t-loading>

    <section v-if="canEditPricing" class="pricing-section">
      <div class="pricing-section__heading">
        <div>
          <h4>{{ t('modelSettings.usage.pricingTitle') }}</h4>
          <p>{{ t('modelSettings.usage.pricingDescription') }}</p>
        </div>
      </div>
      <div class="pricing-form">
        <t-select v-model="priceModelId" :placeholder="t('modelSettings.usage.selectModel')" @change="loadPrices">
          <t-option v-for="model in models" :key="model.id" :value="model.id!" :label="modelLabel(model)" />
        </t-select>
        <t-input-number v-model="inputPrice" :min="0" :decimal-places="6" :placeholder="t('modelSettings.usage.inputPrice')" />
        <t-input-number v-model="outputPrice" :min="0" :decimal-places="6" :placeholder="t('modelSettings.usage.outputPrice')" />
        <t-input v-model="currency" maxlength="3" :placeholder="t('modelSettings.usage.currency')" />
        <t-input v-model="validFrom" type="datetime-local" />
        <t-input v-model="validTo" type="datetime-local" :placeholder="t('modelSettings.usage.validTo')" />
        <t-button theme="primary" :disabled="!canSavePrice" :loading="savingPrice" @click="savePrice">
          {{ t('modelSettings.usage.addPrice') }}
        </t-button>
      </div>
      <div v-if="prices.length" class="price-history">
        <div v-for="price in prices" :key="price.id" class="price-version">
          <span>{{ new Date(price.valid_from).toLocaleString() }}</span>
          <strong>{{ price.currency }} {{ micros(price.input_microunits_per_million) }} / {{ micros(price.output_microunits_per_million) }}</strong>
        </div>
      </div>
    </section>
  </SettingDrawer>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { MessagePlugin } from 'tdesign-vue-next'
import { useI18n } from 'vue-i18n'
import SettingDrawer from '@/components/settings/SettingDrawer.vue'
import {
  listModelPrices,
  listModelUsage,
  putModelPrice,
  type ModelConfig,
  type ModelCostTotal,
  type ModelPriceVersion,
  type ModelUsageStatistics,
} from '@/api/model'

const props = defineProps<{ visible: boolean; models: ModelConfig[]; canEditPricing: boolean }>()
const emit = defineEmits<{ (e: 'update:visible', value: boolean): void }>()
const { t } = useI18n()

const drawerVisible = computed({ get: () => props.visible, set: value => emit('update:visible', value) })
const loading = ref(false)
const windowDays = ref(30)
const rows = ref<ModelUsageStatistics[]>([])
const priceModelId = ref('')
const prices = ref<ModelPriceVersion[]>([])
const inputPrice = ref<number | undefined>()
const outputPrice = ref<number | undefined>()
const currency = ref('USD')
const validFrom = ref(new Date().toISOString().slice(0, 16))
const validTo = ref('')
const savingPrice = ref(false)

const canSavePrice = computed(() => Boolean(
  priceModelId.value && inputPrice.value != null && outputPrice.value != null &&
  currency.value.trim().length === 3 && validFrom.value,
))

watch(() => props.visible, visible => {
  if (visible) {
    if (!priceModelId.value && props.models[0]?.id) priceModelId.value = props.models[0].id
    void Promise.all([loadUsage(), loadPrices()])
  }
})

async function loadUsage() {
  loading.value = true
  try {
    const to = new Date()
    const from = new Date(to.getTime() - windowDays.value * 86400000)
    rows.value = (await listModelUsage({ from: from.toISOString(), to: to.toISOString() })).items
  } catch (error: any) {
    MessagePlugin.error(error?.message || t('modelSettings.usage.loadFailed'))
  } finally {
    loading.value = false
  }
}

async function loadPrices() {
  if (!priceModelId.value) {
    prices.value = []
    return
  }
  try {
    prices.value = await listModelPrices(priceModelId.value)
  } catch (error: any) {
    MessagePlugin.error(error?.message || t('modelSettings.usage.priceLoadFailed'))
  }
}

async function savePrice() {
  if (!canSavePrice.value) return
  savingPrice.value = true
  try {
    await putModelPrice(priceModelId.value, {
      valid_from: new Date(validFrom.value).toISOString(),
      valid_to: validTo.value ? new Date(validTo.value).toISOString() : undefined,
      input_microunits_per_million: Math.round((inputPrice.value || 0) * 1_000_000),
      output_microunits_per_million: Math.round((outputPrice.value || 0) * 1_000_000),
      currency: currency.value.trim().toUpperCase(),
    })
    MessagePlugin.success(t('modelSettings.usage.priceSaved'))
    await loadPrices()
  } catch (error: any) {
    MessagePlugin.error(error?.message || t('modelSettings.usage.priceSaveFailed'))
  } finally {
    savingPrice.value = false
  }
}

const modelFor = (id: string) => props.models.find(model => model.id === id)
const modelLabel = (model: ModelConfig) => model.display_name || model.name
const modelName = (id: string) => modelFor(id) ? modelLabel(modelFor(id)!) : id
const modelType = (id: string) => modelFor(id)?.type || ''
const integer = (value: number) => new Intl.NumberFormat().format(value || 0)
const decimal = (value: number) => new Intl.NumberFormat(undefined, { maximumFractionDigits: 1 }).format(value || 0)
const percent = (value: number | null) => value == null ? '—' : `${(value * 100).toFixed(1)}%`
const micros = (value: number) => (value / 1_000_000).toFixed(6).replace(/0+$/, '').replace(/\.$/, '')
const formatCosts = (costs: ModelCostTotal[]) => costs?.length
  ? costs.map(item => `${item.currency} ${micros(item.cost_microunits)}`).join(' · ')
  : t('modelSettings.usage.noPricedCost')
</script>

<style scoped lang="less">
.usage-toolbar, .pricing-section__heading { display: flex; align-items: center; justify-content: space-between; gap: 16px; }
.usage-toolbar { margin-bottom: 18px; }
.usage-empty { padding: 44px 0; }
.usage-list { display: grid; gap: 14px; }
.usage-card { border: 1px solid var(--td-component-border); border-radius: 12px; padding: 16px; background: var(--td-bg-color-container); }
.usage-card__header { display: flex; justify-content: space-between; gap: 16px; align-items: flex-start; }
.usage-card__header h4, .pricing-section h4 { margin: 0; font-size: 15px; }
.usage-card__header span, .pricing-section p, .cache-grid p { color: var(--td-text-color-secondary); font-size: 12px; margin: 4px 0 0; }
.usage-card__cost { font-weight: 600; color: var(--td-brand-color); }
.usage-metrics { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 10px; margin-top: 14px; }
.usage-metrics div, .cache-grid section { padding: 10px 12px; border-radius: 8px; background: var(--td-bg-color-secondarycontainer); }
.usage-metrics span { display: block; color: var(--td-text-color-secondary); font-size: 12px; }
.usage-metrics strong { display: block; margin-top: 5px; font-size: 15px; }
.cache-grid { display: grid; grid-template-columns: 1fr 1fr; gap: 10px; margin-top: 10px; }
.cache-grid h5 { margin: 0 0 6px; font-size: 12px; color: var(--td-text-color-secondary); }
.cache-grid strong { font-size: 20px; }
.pricing-section { margin-top: 24px; padding-top: 20px; border-top: 1px solid var(--td-component-stroke); }
.pricing-form { display: grid; grid-template-columns: 1.4fr 1fr 1fr .65fr 1.15fr 1.15fr auto; gap: 8px; margin-top: 14px; }
.price-history { margin-top: 12px; border-top: 1px solid var(--td-component-stroke); }
.price-version { display: flex; justify-content: space-between; gap: 12px; padding: 9px 0; font-size: 12px; border-bottom: 1px solid var(--td-component-stroke); }
@media (max-width: 840px) { .usage-metrics { grid-template-columns: 1fr 1fr; } .pricing-form { grid-template-columns: 1fr 1fr; } }
</style>
