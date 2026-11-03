<template>
  <div class="page">
    <div class="card">
      <div class="toolbar">
        <b>P-11 Kitting 扫码（Web + 扫码枪） / M-01</b>
        <span class="muted">扫码枪=键盘输入，扫入即校验（Enter 自动触发）</span>
        <div class="spacer" />
        <el-select v-model="batchNo" placeholder="选择允收批次" style="width: 210px" @change="loadBoxes">
          <el-option v-for="b in batches" :key="b.batch_no" :label="`${b.batch_no} · ${b.forecast_no}`" :value="b.batch_no" />
        </el-select>
        <el-button type="primary" :disabled="!batchNo" @click="createBox">新建箱号</el-button>
      </div>

      <div class="stat-row">
        <div class="stat"><div class="num">{{ progress.total }}</div><div class="lbl">RMA SN 总量 / Total</div></div>
        <div class="stat"><div class="num" style="color:var(--ok)">{{ progress.scanned }}</div><div class="lbl">已扫 / Scanned</div></div>
        <div class="stat"><div class="num" style="color:var(--warn)">{{ progress.pending }}</div><div class="lbl">待扫 / Pending</div></div>
        <div class="stat"><div class="num">{{ box?.box_no || '—' }}</div><div class="lbl">当前箱号 / Box</div></div>
      </div>

      <div style="display:flex; gap:14px">
        <div style="flex:1">
          <div class="scan-box" style="margin-bottom:10px">
            <el-input
              ref="scanRef" v-model="scanSN" size="large" clearable
              placeholder="扫码 / 输入 SN 后回车" :disabled="!box || box.status === 'SEALED'"
              @keyup.enter="doScan"
            >
              <template #prepend>SN</template>
            </el-input>
          </div>
          <div v-if="feedback" :class="feedback.cls" style="margin-bottom:10px">{{ feedback.msg }}</div>

          <el-table :data="boxItems" border size="small" max-height="320">
            <el-table-column prop="sn" :label="t('common.sn')" min-width="180" />
            <el-table-column label="状态" width="140">
              <template #default="{ row }">
                <el-tag size="small" :type="row._status === 'RECEIVED' ? 'success' : 'warning'">
                  {{ row._status || '已扫' }}
                </el-tag>
              </template>
            </el-table-column>
          </el-table>

          <div style="margin-top:10px; display:flex; gap:8px">
            <el-button type="primary" :disabled="!box || box.status === 'SEALED'" @click="sealBox">封箱并打印箱标</el-button>
            <el-button :disabled="!boxNoSealed" @click="printLabel">重新打印箱标</el-button>
          </div>
        </div>

        <div style="width:300px">
          <div class="card" style="padding:12px">
            <div style="font-weight:600; margin-bottom:8px">批次箱列表 / Boxes</div>
            <div v-for="b in boxes" :key="b.box_no" class="box-row" :class="{ active: b.box_no === boxNo }" @click="selectBox(b)">
              <span class="mono">{{ b.box_no }}</span>
              <el-tag size="small" :type="b.status === 'SEALED' ? 'success' : 'info'">{{ b.status }}</el-tag>
            </div>
            <div v-if="!boxes.length" class="muted">暂无箱号，请先新建</div>
          </div>
        </div>
      </div>
    </div>
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
const batches = ref([])
const batchNo = ref(route.query.batch || '')
const boxes = ref([])
const boxNo = ref('')
const box = ref(null)
const boxItems = ref([])
const scanSN = ref('')
const scanRef = ref(null)
const feedback = ref(null)
const progress = reactive({ total: 0, scanned: 0, pending: 0 })
const boxNoSealed = ref(false)

onMounted(async () => {
  try { batches.value = (await api.get('/receiving/batches', { params: { size: 200 } })).items } catch { /* ignore */ }
  if (batchNo.value) loadBoxes()
})

async function loadBoxes() {
  const res = await api.get('/boxes', { params: { batch_no: batchNo.value, size: 200 } })
  boxes.value = res.items
  const b = batches.value.find((x) => x.batch_no === batchNo.value)
  if (b) await loadProgress(b.forecast_no)
  if (boxes.value.length) selectBox(boxes.value[0])
}

async function loadProgress(forecastNo) {
  try {
    const d = await api.get('/forecasts/' + forecastNo)
    const sns = d.sn_status || []
    progress.total = d.forecast?.forecast_qty || sns.length
    progress.scanned = sns.filter((s) => s.status !== 'FORECASTED' && s.status !== 'CLOSED_SHORT').length
    progress.pending = Math.max(0, progress.total - progress.scanned)
  } catch { /* ignore */ }
}

async function createBox() {
  try {
    const b = await api.post('/boxes', { batch_no: batchNo.value })
    ElMessage.success('已生成箱号 ' + b.box_no)
    await loadBoxes()
    selectBox(b)
  } catch (e) { ElMessage.error(errMsg(e)) }
}

async function selectBox(b) {
  boxNo.value = b.box_no
  const d = await api.get('/boxes/' + b.box_no)
  box.value = d.box
  // 附 SN 当前状态
  const items = []
  for (const it of d.sn_items) {
    items.push({ ...it })
  }
  boxItems.value = items
  boxNoSealed.value = b.status === 'SEALED'
  await nextTick()
  scanRef.value?.focus()
}

async function doScan() {
  const sn = scanSN.value.trim()
  if (!sn) return
  scanSN.value = ''
  try {
    const res = await api.post('/boxes/scan', { box_no: boxNo.value, sn })
    if (res.result === 'OK') {
      feedback.value = { cls: 'feedback-ok', msg: `✓ 校验通过：${sn} 已接收（本箱 ${res.box_sn_count} 片）` }
    } else {
      feedback.value = { cls: 'feedback-warn', msg: `⚠ ${res.discrepancy_type}：${res.message}` }
    }
    await selectBox({ box_no: boxNo.value, status: box.value.status })
  } catch (e) {
    feedback.value = { cls: 'feedback-err', msg: '✗ ' + errMsg(e) }
  }
  await nextTick()
  scanRef.value?.focus()
}

async function sealBox() {
  try {
    await api.post(`/boxes/${boxNo.value}/seal`, {})
    ElMessage.success('已封箱')
    openLabel(`/api/v1/boxes/${boxNo.value}/label`)
    await loadBoxes()
  } catch (e) { ElMessage.error(errMsg(e)) }
}

function printLabel() {
  openLabel(`/api/v1/boxes/${boxNo.value}/label`)
}
</script>

<style scoped>
.box-row {
  display: flex; justify-content: space-between; align-items: center;
  padding: 8px; border: 1px solid var(--border); border-radius: 6px; margin-bottom: 6px; cursor: pointer;
}
.box-row.active { border-color: var(--brand); background: #f0f6ff; }
</style>
