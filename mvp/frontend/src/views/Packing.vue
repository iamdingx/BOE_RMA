<template>
  <div class="page">
    <div class="card">
      <div class="toolbar">
        <b>P-12 出库打包 / M-02（逐箱打包 · 整单发运）</b>
        <div class="spacer" />
        <el-select v-model="outboundNo" placeholder="选择出库单" style="width: 240px" @change="loadBoxes">
          <el-option v-for="o in outbounds" :key="o.outbound_no" :label="`${o.outbound_no} · ${o.rma_no || ''}`" :value="o.outbound_no" />
        </el-select>
        <el-button type="primary" :disabled="!outboundNo" @click="createBox">新建发货箱</el-button>
      </div>

      <div style="display:flex; gap:14px">
        <div style="flex:1">
          <div class="scan-box" style="margin-bottom:10px">
            <el-input ref="scanRef" v-model="scanSN" size="large" clearable
              placeholder="扫码 / 输入 SN 后回车装入当前箱" :disabled="!box || box.sealed" @keyup.enter="addSN">
              <template #prepend>SN</template>
            </el-input>
          </div>
          <div v-if="feedback" :class="feedback.cls" style="margin-bottom:10px">{{ feedback.msg }}</div>

          <el-form :inline="true" size="small">
            <el-form-item :label="t('common.weight')">
              <el-input-number v-model="weight" :min="0" :precision="3" :step="0.1" />
            </el-form-item>
            <el-form-item :label="t('common.volume')">
              <el-input-number v-model="volume" :min="0" :precision="4" :step="0.01" />
            </el-form-item>
            <el-form-item>
              <el-button type="primary" :disabled="!box || box.sealed" @click="seal">封箱并打印箱标</el-button>
            </el-form-item>
          </el-form>

          <el-table :data="box?.sn_list || []" border size="small" max-height="260">
            <el-table-column label="SN"><template #default="{ row }">{{ row }}</template></el-table-column>
          </el-table>

          <div style="margin-top:12px">
            <el-button type="success" :disabled="!outboundNo" @click="openShip">整单发运（物流公司 + 单号）</el-button>
            <span class="muted" style="margin-left: 8px">一个出库单可多箱，整单共用一个物流单号</span>
          </div>
        </div>

        <div style="width: 320px">
          <div class="card" style="padding:12px">
            <div style="font-weight:600;margin-bottom:8px">发货箱 / Shipment Boxes</div>
            <div v-for="b in boxes" :key="b.shipment_box_no" class="box-row"
              :class="{ active: b.shipment_box_no === boxNo }" @click="selectBox(b)">
              <span class="mono">{{ b.shipment_box_no }}</span>
              <span class="muted">{{ b.sn_list?.length || 0 }} 片 · {{ b.sealed ? '已封箱' : '打包中' }}</span>
            </div>
            <div v-if="!boxes.length" class="muted">暂无发货箱</div>
          </div>
        </div>
      </div>
    </div>

    <el-dialog v-model="shipVisible" title="整单发运" width="460px">
      <el-form label-width="100px" size="small">
        <el-form-item label="物流公司" required><el-input v-model="ship.carrier" /></el-form-item>
        <el-form-item label="物流单号" required><el-input v-model="ship.tracking_no" /></el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="shipVisible = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" @click="doShip">{{ t('common.submit') }}</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { nextTick, onMounted, reactive, ref } from 'vue'
import { useRoute } from 'vue-router'
import { ElMessage } from 'element-plus'
import { api, errMsg, openLabel } from '../api'
import { useI18n } from '../i18n'

const { t } = useI18n()
const route = useRoute()
const outbounds = ref([])
const outboundNo = ref(route.query.outbound || '')
const boxes = ref([])
const boxNo = ref('')
const box = ref(null)
const scanSN = ref('')
const scanRef = ref(null)
const feedback = ref(null)
const weight = ref(1)
const volume = ref(0.01)

onMounted(async () => {
  try {
    outbounds.value = (await api.get('/outbounds', { params: { size: 200, status: 'CREATED' } })).items
  } catch { /* ignore */ }
  if (outboundNo.value) loadBoxes()
})

async function loadBoxes() {
  const res = await api.get('/shipment-boxes', { params: { outbound_no: outboundNo.value, size: 200 } })
  boxes.value = res.items
  if (boxes.value.length) selectBox(boxes.value[0])
}
async function createBox() {
  try {
    const b = await api.post('/shipment-boxes', { outbound_no: outboundNo.value })
    ElMessage.success('已生成发货箱 ' + b.shipment_box_no)
    await loadBoxes()
    selectBox(b)
  } catch (e) { ElMessage.error(errMsg(e)) }
}
async function selectBox(b) {
  boxNo.value = b.shipment_box_no
  box.value = b
  if (b.weight) weight.value = b.weight
  if (b.volume) volume.value = b.volume
  await nextTick()
  scanRef.value?.focus()
}
async function addSN() {
  const sn = scanSN.value.trim()
  if (!sn) return
  scanSN.value = ''
  try {
    const b = await api.post(`/shipment-boxes/${boxNo.value}/items`, { sn })
    box.value = b
    feedback.value = { cls: 'feedback-ok', msg: `✓ ${sn} 已入箱（${b.sn_list.length} 片）` }
    await loadBoxes()
  } catch (e) {
    feedback.value = { cls: 'feedback-err', msg: '✗ ' + errMsg(e) }
  }
  await nextTick()
  scanRef.value?.focus()
}
async function seal() {
  try {
    await api.post(`/shipment-boxes/${boxNo.value}/seal`, { weight: weight.value, volume: volume.value })
    ElMessage.success('已封箱')
    openLabel(`/api/v1/shipment-boxes/${boxNo.value}/label`)
    await loadBoxes()
  } catch (e) { ElMessage.error(errMsg(e)) }
}

const shipVisible = ref(false)
const ship = reactive({ carrier: '', tracking_no: '' })
function openShip() { ship.carrier = ''; ship.tracking_no = ''; shipVisible.value = true }
async function doShip() {
  if (!ship.carrier || !ship.tracking_no) return ElMessage.warning('物流公司与单号必填')
  try {
    await api.post(`/outbounds/${outboundNo.value}/ship`, ship)
    ElMessage.success('已发运')
    shipVisible.value = false
    outbounds.value = (await api.get('/outbounds', { params: { size: 200, status: 'CREATED' } })).items
    await loadBoxes()
  } catch (e) { ElMessage.error(errMsg(e)) }
}
</script>

<style scoped>
.box-row { display:flex; justify-content:space-between; align-items:center; padding:8px; border:1px solid var(--border); border-radius:6px; margin-bottom:6px; cursor:pointer; font-size:12px; }
.box-row.active { border-color: var(--brand); background:#f0f6ff; }
</style>
