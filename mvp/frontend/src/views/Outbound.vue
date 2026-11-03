<template>
  <div class="page">
    <div class="card">
      <div class="toolbar">
        <b>P-06 出库管理 / Outbound</b>
        <el-input v-model="kw" placeholder="出库单 / RMA" style="width: 190px" clearable @keyup.enter="reload" />
        <el-button @click="reload">{{ t('common.search') }}</el-button>
        <div class="spacer" />
        <el-button @click="openCreate">创建出库单</el-button>
      </div>

      <el-tabs v-model="obType" @tab-change="reload">
        <el-tab-pane label="发客户" name="SHIP_TO_CUSTOMER" />
        <el-tab-pane label="发维修中心" name="SHIP_TO_RC" />
        <el-tab-pane label="报废处置" name="SCRAP_DISPOSAL" />
        <el-tab-pane label="调拨转仓" name="TRANSFER_WAREHOUSE" />
      </el-tabs>

      <el-table :data="rows" v-loading="loading" border size="small">
        <el-table-column prop="outbound_no" label="出库单" width="170" />
        <el-table-column prop="rma_no" :label="t('common.rma')" width="150" />
        <el-table-column prop="sn_count" label="SN 数" width="80" align="center" />
        <el-table-column label="状态" width="150">
          <template #default="{ row }">
            <el-tag size="small" :type="stType(row.status)">{{ row.status }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="carrier" label="物流公司" width="120" />
        <el-table-column prop="logistics_no" label="物流单号" min-width="150" />
        <el-table-column prop="created_at" label="创建时间" min-width="170" />
        <el-table-column :label="t('common.action')" width="300" fixed="right">
          <template #default="{ row }">
            <el-button link type="primary" @click="showDetail(row)">{{ t('common.detail') }}</el-button>
            <el-button v-if="row.status === 'PENDING_APPROVAL'" link type="success" @click="approveScrap(row)">报废审批</el-button>
            <el-button v-if="row.status === 'PENDING_APPROVAL'" link type="danger" @click="rejectScrap(row)">驳回</el-button>
            <el-button v-if="row.status === 'REJECTED'" link type="warning" @click="resubmit(row)">重提交</el-button>
            <el-button v-if="row.status === 'CREATED' && row.outbound_type === 'TRANSFER_WAREHOUSE'" link type="primary" @click="countConfirm(row)">清点确认</el-button>
            <el-button v-if="row.status === 'CREATED' && row.outbound_type !== 'TRANSFER_WAREHOUSE'" link type="primary" @click="openShip(row)">发运</el-button>
            <el-button v-if="row.status === 'CREATED'" link @click="$router.push('/packing?outbound=' + row.outbound_no)">打包</el-button>
          </template>
        </el-table-column>
      </el-table>
      <Pager :total="total" :page="page" :size="pageSize" @change="change" />
    </div>

    <el-dialog v-model="createVisible" title="创建出库单" width="560px">
      <el-form label-width="110px" size="small">
        <el-form-item label="出库类型" required>
          <el-select v-model="form.outbound_type" style="width: 100%">
            <el-option v-for="o in outTypes" :key="o.value" :label="o.label" :value="o.value" />
          </el-select>
        </el-form-item>
        <el-form-item :label="t('common.rma')"><el-input v-model="form.rma_no" /></el-form-item>
        <el-form-item label="SN 清单" required>
          <el-input v-model="snText" type="textarea" :rows="4" placeholder="每行一个 SN" />
        </el-form-item>
        <el-form-item v-if="form.outbound_type === 'SHIP_TO_CUSTOMER'" label="CID 子类">
          <el-checkbox v-model="form.cid_flag">CID 原片返还（需已录入责任标记与佐证）</el-checkbox>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="createVisible = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" @click="doCreate">{{ t('common.submit') }}</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="shipVisible" title="发运提交（物流公司 + 物流单号必填）" width="480px">
      <el-form label-width="100px" size="small">
        <el-form-item label="物流公司" required><el-input v-model="ship.carrier" /></el-form-item>
        <el-form-item label="物流单号" required><el-input v-model="ship.tracking_no" /></el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="shipVisible = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" @click="doShip">{{ t('common.submit') }}</el-button>
      </template>
    </el-dialog>

    <el-drawer v-model="detailVisible" title="出库单详情" size="640px">
      <div v-if="detail.outbound">
        <p><b>{{ detail.outbound.outbound_no }}</b> · {{ detail.outbound.outbound_type }} · {{ detail.outbound.status }}</p>
        <p class="muted">物流：{{ detail.outbound.carrier || '—' }} / {{ detail.outbound.logistics_no || '—' }}</p>
        <div v-if="detail.outbound.scrap_approval" class="feedback-warn" style="margin-bottom: 10px">
          报废审批留痕：{{ JSON.stringify(detail.outbound.scrap_approval) }}
        </div>
        <el-table :data="detail.sn_items" border size="small" max-height="300">
          <el-table-column prop="sn" :label="t('common.sn')" />
        </el-table>
        <h4 v-if="detail.shipment_boxes?.length">发货箱</h4>
        <el-table :data="detail.shipment_boxes" border size="small">
          <el-table-column prop="shipment_box_no" label="箱号" width="150" />
          <el-table-column label="SN 数" width="80"><template #default="{ row }">{{ row.sn_list?.length || 0 }}</template></el-table-column>
          <el-table-column prop="weight" label="重量" width="90" />
          <el-table-column prop="volume" label="材积" width="100" />
          <el-table-column label="打印" width="90">
            <template #default="{ row }"><el-button link type="primary" @click="printBox(row)">箱标</el-button></template>
          </el-table-column>
        </el-table>
      </div>
    </el-drawer>
  </div>
</template>

<script setup>
import { reactive, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { api, errMsg, openLabel } from '../api'
import { useI18n } from '../i18n'
import { useList } from '../composables/useList'
import Pager from '../components/Pager.vue'

const { t } = useI18n()
const obType = ref('SHIP_TO_CUSTOMER')
const kw = ref('')
const outTypes = [
  { value: 'SHIP_TO_CUSTOMER', label: '发回客户' },
  { value: 'SHIP_TO_RC', label: '发维修中心' },
  { value: 'SCRAP_DISPOSAL', label: '报废处置' },
  { value: 'TRANSFER_WAREHOUSE', label: '调拨转仓' },
]
const { rows, total, page, pageSize, loading, load, change } = useList(
  (p) => api.get('/outbounds', { params: { ...p, outbound_type: obType.value, keyword: kw.value } })
)
function reload() { change(1, pageSize.value) }
function stType(s) {
  return { CREATED: 'info', PENDING_APPROVAL: 'warning', REJECTED: 'danger', SHIPPED: 'success' }[s] || ''
}

const createVisible = ref(false)
const snText = ref('')
const form = reactive({ outbound_type: 'SHIP_TO_CUSTOMER', rma_no: '', cid_flag: false })
function openCreate() {
  form.outbound_type = obType.value
  form.rma_no = ''
  form.cid_flag = false
  snText.value = ''
  createVisible.value = true
}
async function doCreate() {
  const sns = snText.value.split(/\r?\n/).map((s) => s.trim()).filter(Boolean)
  if (!sns.length) return ElMessage.warning('SN 清单不能为空')
  try {
    await api.post('/outbounds', { ...form, sns })
    ElMessage.success(t('common.ok'))
    createVisible.value = false
    load()
  } catch (e) { ElMessage.error(errMsg(e)) }
}

const shipVisible = ref(false)
const ship = reactive({ carrier: '', tracking_no: '' })
const current = ref(null)
function openShip(row) { current.value = row; ship.carrier = ''; ship.tracking_no = ''; shipVisible.value = true }
async function doShip() {
  if (!ship.carrier || !ship.tracking_no) return ElMessage.warning('物流公司与单号必填')
  try {
    await api.post(`/outbounds/${current.value.outbound_no}/ship`, ship)
    ElMessage.success('已发运')
    shipVisible.value = false
    load()
  } catch (e) { ElMessage.error(errMsg(e)) }
}

async function approveScrap(row) {
  try {
    const { value } = await ElMessageBox.prompt('报废审批意见（必填）', '报废处置审批', { inputPlaceholder: '同意报废，原因…' })
    await api.post(`/outbounds/${row.outbound_no}/scrap-approve`, { opinion: value })
    ElMessage.success('审批通过')
    load()
  } catch (e) { if (e !== 'cancel') ElMessage.error(errMsg(e)) }
}
async function rejectScrap(row) {
  try {
    const { value } = await ElMessageBox.prompt('驳回原因（必填，可重新提交）', '报废驳回', { inputPlaceholder: '原因…' })
    await api.post(`/outbounds/${row.outbound_no}/scrap-reject`, { reason: value })
    ElMessage.success('已驳回')
    load()
  } catch (e) { if (e !== 'cancel') ElMessage.error(errMsg(e)) }
}
async function resubmit(row) {
  try {
    await api.post(`/outbounds/${row.outbound_no}/scrap-resubmit`, {})
    ElMessage.success('已重新提交审批')
    load()
  } catch (e) { ElMessage.error(errMsg(e)) }
}
async function countConfirm(row) {
  try {
    await api.post(`/outbounds/${row.outbound_no}/count-confirm`, {})
    ElMessage.success('清点确认完成，SN 已置 COMPLETED')
    load()
  } catch (e) { ElMessage.error(errMsg(e)) }
}

const detailVisible = ref(false)
const detail = ref({})
async function showDetail(row) {
  detail.value = await api.get('/outbounds/' + row.outbound_no)
  detailVisible.value = true
}
function printBox(b) { openLabel(`/api/v1/shipment-boxes/${b.shipment_box_no}/label`) }
</script>
