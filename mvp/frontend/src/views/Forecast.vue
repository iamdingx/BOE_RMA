<template>
  <div class="page">
    <div class="card">
      <div class="toolbar">
        <b>P-02 预报管理 / Forecast</b>
        <el-input v-model="kw" :placeholder="t('common.rma')" style="width: 180px" clearable @keyup.enter="reload" />
        <el-select v-model="status" clearable placeholder="RMA 状态" style="width: 160px" @change="reload">
          <el-option v-for="s in rmaStatuses" :key="s" :label="s" :value="s" />
        </el-select>
        <el-button @click="reload">{{ t('common.search') }}</el-button>
        <div class="spacer" />
        <el-button type="primary" @click="openCreate">{{ t('common.add') }}</el-button>
      </div>

      <el-table :data="rows" v-loading="loading" border size="small">
        <el-table-column prop="rma_no" :label="t('common.rma')" width="150" />
        <el-table-column prop="forecast_no" label="预报单号" width="140" />
        <el-table-column prop="customer" :label="t('common.customer')" min-width="120" />
        <el-table-column prop="forecast_qty" :label="t('common.qty')" width="80" align="center" />
        <el-table-column prop="label_qty_total" label="外箱标识数" width="100" align="center" />
        <el-table-column prop="status" label="预报状态" width="100" />
        <el-table-column label="RMA 状态" width="140">
          <template #default="{ row }">
            <el-tag size="small" :type="rmaTagType(row.rma_status)">{{ row.rma_status }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="TAT" width="110">
          <template #default="{ row }">
            <span v-if="row.closed_at">{{ row.tat_days }} 天</span>
            <span v-else-if="row.full_received_at" :class="row.tat_overdue ? 'tag-red' : (row.tat_warning ? 'tag-warn' : '')">
              进行中{{ row.tat_overdue ? ' · 超期' : (row.tat_warning ? ' · 预警' : '') }}
            </span>
            <span v-else class="muted">—</span>
          </template>
        </el-table-column>
        <el-table-column :label="t('common.action')" width="220" fixed="right">
          <template #default="{ row }">
            <el-button link type="primary" @click="showDetail(row)">{{ t('common.detail') }}</el-button>
            <el-button link type="primary" @click="exportForecast(row)">{{ t('common.export') }}</el-button>
            <el-button link type="primary" @click="$router.push('/receiving?forecast=' + row.forecast_no)">允收</el-button>
          </template>
        </el-table-column>
      </el-table>
      <Pager :total="total" :page="page" :size="pageSize" @change="change" />
    </div>

    <!-- 新建预报 -->
    <el-dialog v-model="createVisible" title="预报录入 / New Forecast" width="720px">
      <el-form :model="form" label-width="110px" size="small">
        <el-form-item :label="t('common.rma')" required>
          <el-input v-model="form.rma_no" placeholder="RMA-9001" />
        </el-form-item>
        <el-form-item :label="t('common.pn')" required>
          <el-select v-model="form.pn_code" filterable allow-create style="width: 100%" placeholder="PN-A100">
            <el-option v-for="p in pns" :key="p.pn_code" :label="`${p.pn_code} (${p.model_code})`" :value="p.pn_code" />
          </el-select>
        </el-form-item>
        <el-form-item :label="t('common.customer')" required>
          <el-input v-model="form.customer" placeholder="BOE US" />
        </el-form-item>
        <el-form-item :label="t('common.qty')">
          <el-input-number v-model="form.forecast_qty" :min="0" />
          <span class="muted" style="margin-left: 8px">留空则按 SN 清单条数</span>
        </el-form-item>
        <el-form-item label="SN 清单" required>
          <el-input v-model="snText" type="textarea" :rows="4" placeholder="每行一个 SN（支持扫码枪逐行扫入）" />
          <div style="margin-top: 6px; display: flex; gap: 8px; align-items: center">
            <input ref="fileRef" type="file" accept=".xlsx" style="display: none" @change="importSN" />
            <el-button size="small" @click="fileRef.click()">Excel 导入 SN</el-button>
            <el-button size="small" @click="downloadTemplate">下载模板</el-button>
            <span class="muted">已录 {{ snList.length }} 条</span>
          </div>
        </el-form-item>
        <el-form-item label="二次返修确认">
          <el-checkbox v-model="form.confirm_secondary_return">该清单含历史 SN，确认按二次返修处理</el-checkbox>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="createVisible = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" :loading="saving" @click="doCreate">{{ t('common.submit') }}</el-button>
      </template>
    </el-dialog>

    <!-- 详情 -->
    <el-dialog v-model="detailVisible" title="预报详情 / Forecast Detail" width="860px">
      <div class="stat-row" v-if="detail.forecast">
        <div class="stat"><div class="num">{{ detail.forecast.forecast_qty }}</div><div class="lbl">预报数量</div></div>
        <div class="stat"><div class="num">{{ detail.sn_status?.length || 0 }}</div><div class="lbl">SN 记录</div></div>
        <div class="stat"><div class="num">{{ detail.forecast.rma_status }}</div><div class="lbl">RMA 状态</div></div>
      </div>
      <el-table :data="detail.sn_status || []" border size="small" max-height="380">
        <el-table-column prop="sn" :label="t('common.sn')" width="160" />
        <el-table-column prop="pn_code" :label="t('common.pn')" width="110" />
        <el-table-column prop="status" :label="t('common.status')" width="120" />
        <el-table-column prop="judgment_result" label="判定" width="110" />
        <el-table-column prop="return_method" label="还货方式" width="140" />
        <el-table-column prop="box_no" label="箱号" width="140" />
      </el-table>
    </el-dialog>
  </div>
</template>

<script setup>
import { computed, onMounted, reactive, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { api, errMsg, openLabel } from '../api'
import { useI18n } from '../i18n'
import { useList } from '../composables/useList'
import Pager from '../components/Pager.vue'

const { t } = useI18n()
const kw = ref('')
const status = ref('')
const rmaStatuses = ['OPEN', 'RECEIVING', 'PROCESSING', 'PENDING_RETURN', 'RETURNED', 'CLOSED']

const { rows, total, page, pageSize, loading, load, change } = useList(
  (p) => api.get('/forecasts', { params: { ...p, keyword: kw.value, status: status.value } })
)
function reload() { change(1, pageSize.value) }
function rmaTagType(s) {
  return { OPEN: 'info', RECEIVING: 'warning', PROCESSING: '', PENDING_RETURN: 'warning', RETURNED: 'success', CLOSED: 'success' }[s] || ''
}

const pns = ref([])
onMounted(async () => {
  try { pns.value = (await api.get('/master/pns')).items } catch { /* ignore */ }
})

const createVisible = ref(false)
const saving = ref(false)
const snText = ref('')
const fileRef = ref(null)
const form = reactive({ rma_no: '', pn_code: 'PN-A100', customer: 'BOE US', forecast_qty: 0, confirm_secondary_return: false })
const snList = computed(() => snText.value.split(/\r?\n/).map((s) => s.trim()).filter(Boolean))

function openCreate() {
  createVisible.value = true
  form.rma_no = ''
  form.forecast_qty = 0
  form.confirm_secondary_return = false
  snText.value = ''
}

async function doCreate() {
  if (!form.rma_no) return ElMessage.warning('RMA 号必填')
  if (!snList.value.length) return ElMessage.warning('SN 清单不能为空')
  saving.value = true
  try {
    await api.post('/forecasts', {
      ...form,
      items: snList.value.map((sn) => ({ sn, pn_code: form.pn_code }))
    })
    ElMessage.success(t('common.ok'))
    createVisible.value = false
    reload()
  } catch (e) { ElMessage.error(errMsg(e)) } finally { saving.value = false }
}

async function importSN(ev) {
  const f = ev.target.files?.[0]
  if (!f) return
  const fd = new FormData()
  fd.append('file', f)
  try {
    const res = await api.post('/forecasts/import-sn', fd, { headers: { 'Content-Type': 'multipart/form-data' } })
    snText.value = res.items.map((i) => i.sn).join('\n')
    ElMessage.success(`导入 ${res.count} 条`)
  } catch (e) { ElMessage.error(errMsg(e)) }
  ev.target.value = ''
}

async function downloadTemplate() {
  const t0 = localStorage.getItem('token')
  const r = await fetch('/api/v1/forecasts/template', { headers: { Authorization: `Bearer ${t0}` } })
  const b = await r.blob()
  const a = document.createElement('a')
  a.href = URL.createObjectURL(b)
  a.download = 'forecast_sn_template.xlsx'
  a.click()
}

const detailVisible = ref(false)
const detail = ref({})
async function showDetail(row) {
  try {
    detail.value = await api.get('/forecasts/' + row.forecast_no)
    detailVisible.value = true
  } catch (e) { ElMessage.error(errMsg(e)) }
}
async function exportForecast(row) {
  const tk = localStorage.getItem('token')
  const r = await fetch(`/api/v1/forecasts/${row.forecast_no}/export`, { headers: { Authorization: `Bearer ${tk}` } })
  const b = await r.blob()
  const a = document.createElement('a')
  a.href = URL.createObjectURL(b)
  a.download = `forecast_${row.forecast_no}.xlsx`
  a.click()
}
</script>
