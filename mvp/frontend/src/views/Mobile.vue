<template>
  <div class="page" style="max-width: 520px; margin: 0 auto">
    <div class="card">
      <b>移动端扫码 / Mobile Scan（H5）</b>
      <div class="muted" style="margin: 4px 0 12px">点击逐个扫（DZ-02）；断网禁止提交，不做本地缓存</div>

      <el-tabs v-model="tab">
        <!-- M-01 Kitting -->
        <el-tab-pane label="M-01 Kitting" name="kitting">
          <el-input v-model="batchNo" placeholder="允收批次号" style="margin-bottom: 8px" />
          <el-button size="small" style="margin-bottom: 8px" @click="createBox">新建箱号</el-button>
          <div v-if="boxNo" class="muted" style="margin-bottom: 8px">当前箱：<span class="mono">{{ boxNo }}</span></div>
          <el-input v-model="scanSN" size="large" placeholder="扫 SN 后回车" @keyup.enter="doScan" style="margin-bottom: 8px" />
          <div v-if="feedback" :class="feedback.cls" style="margin-bottom: 8px">{{ feedback.msg }}</div>
          <el-button type="primary" style="width:100%" :disabled="!boxNo" @click="seal">封箱并打印箱标</el-button>
        </el-tab-pane>

        <!-- M-02 出库打包 -->
        <el-tab-pane label="M-02 打包" name="packing">
          <el-input v-model="outboundNo" placeholder="出库单号" style="margin-bottom: 8px" />
          <el-button size="small" style="margin-bottom: 8px" @click="createShipBox">新建发货箱</el-button>
          <div v-if="shipBoxNo" class="muted" style="margin-bottom: 8px">当前箱：<span class="mono">{{ shipBoxNo }}</span></div>
          <el-input v-model="scanSN2" size="large" placeholder="扫 SN 后回车入箱" @keyup.enter="addItem" style="margin-bottom: 8px" />
          <el-input-number v-model="weight" :min="0" :precision="3" style="margin-bottom: 8px" />
          <el-input-number v-model="volume" :min="0" :precision="4" style="margin-bottom: 8px" />
          <div v-if="feedback2" :class="feedback2.cls" style="margin-bottom: 8px">{{ feedback2.msg }}</div>
          <el-button type="primary" style="width:100%" :disabled="!shipBoxNo" @click="sealShipBox">封箱并打印箱标</el-button>
        </el-tab-pane>

        <!-- M-03 差异拍照 -->
        <el-tab-pane label="M-03 差异拍照" name="photo">
          <el-input v-model="discNo" placeholder="差异记录号（扫描后可查）" style="margin-bottom: 8px" />
          <input ref="fRef" type="file" accept="image/*" capture="environment" style="display: none" @change="uploadPhoto" />
          <el-button style="width:100%; margin-bottom:8px" @click="fRef.click()">拍照上传</el-button>
          <div class="muted">已上传 {{ photos.length }} 张（多收需 ≥3，少收需 ≥2）</div>
          <el-button type="primary" style="width:100%; margin-top:8px" @click="submitDisc">提交差异</el-button>
        </el-tab-pane>
      </el-tabs>
    </div>
  </div>
</template>

<script setup>
import { ref } from 'vue'
import { ElMessage } from 'element-plus'
import { api, errMsg, openLabel } from '../api'

const tab = ref('kitting')
const batchNo = ref('')
const boxNo = ref('')
const scanSN = ref('')
const feedback = ref(null)
const outboundNo = ref('')
const shipBoxNo = ref('')
const scanSN2 = ref('')
const feedback2 = ref(null)
const weight = ref(1)
const volume = ref(0.01)
const discNo = ref('')
const photos = ref([])
const fRef = ref(null)

async function createBox() {
  if (!batchNo.value) return ElMessage.warning('请输入批次号')
  try {
    const b = await api.post('/boxes', { batch_no: batchNo.value })
    boxNo.value = b.box_no
    ElMessage.success('箱号 ' + b.box_no)
  } catch (e) { ElMessage.error(errMsg(e)) }
}
async function doScan() {
  const sn = scanSN.value.trim()
  if (!sn || !boxNo.value) return
  scanSN.value = ''
  try {
    const r = await api.post('/boxes/scan', { box_no: boxNo.value, sn })
    feedback.value = r.result === 'OK'
      ? { cls: 'feedback-ok', msg: `✓ ${sn} 校验通过` }
      : { cls: 'feedback-warn', msg: `⚠ ${r.message}` }
  } catch (e) { feedback.value = { cls: 'feedback-err', msg: '✗ ' + errMsg(e) } }
}
async function seal() {
  try {
    await api.post(`/boxes/${boxNo.value}/seal`, {})
    openLabel(`/api/v1/boxes/${boxNo.value}/label`)
    ElMessage.success('已封箱')
  } catch (e) { ElMessage.error(errMsg(e)) }
}

async function createShipBox() {
  if (!outboundNo.value) return ElMessage.warning('请输入出库单号')
  try {
    const b = await api.post('/shipment-boxes', { outbound_no: outboundNo.value })
    shipBoxNo.value = b.shipment_box_no
    ElMessage.success('发货箱 ' + b.shipment_box_no)
  } catch (e) { ElMessage.error(errMsg(e)) }
}
async function addItem() {
  const sn = scanSN2.value.trim()
  if (!sn || !shipBoxNo.value) return
  scanSN2.value = ''
  try {
    const b = await api.post(`/shipment-boxes/${shipBoxNo.value}/items`, { sn })
    feedback2.value = { cls: 'feedback-ok', msg: `✓ ${sn} 已入箱（${b.sn_list.length} 片）` }
  } catch (e) { feedback2.value = { cls: 'feedback-err', msg: '✗ ' + errMsg(e) } }
}
async function sealShipBox() {
  try {
    await api.post(`/shipment-boxes/${shipBoxNo.value}/seal`, { weight: weight.value, volume: volume.value })
    openLabel(`/api/v1/shipment-boxes/${shipBoxNo.value}/label`)
    ElMessage.success('已封箱')
  } catch (e) { ElMessage.error(errMsg(e)) }
}

async function uploadPhoto(ev) {
  const f = ev.target.files?.[0]
  if (!f) return
  if (f.size > 10 * 1024 * 1024) { ElMessage.error('单张不得超过 10MB'); return }
  const fd = new FormData()
  fd.append('file', f)
  fd.append('biz_type', 'DISCREPANCY_EVIDENCE')
  fd.append('biz_ref_no', discNo.value || 'TEMP')
  try {
    const p = await api.post('/photos', fd, { headers: { 'Content-Type': 'multipart/form-data' } })
    photos.value.push(p.photo_id)
    ElMessage.success('已上传')
  } catch (e) { ElMessage.error(errMsg(e)) }
  ev.target.value = ''
}
async function submitDisc() {
  if (!discNo.value) return ElMessage.warning('请填写差异记录号')
  try {
    await api.post(`/discrepancies/${discNo.value}/submit`, { photo_ids: photos.value })
    ElMessage.success('已提交审核')
    photos.value = []
  } catch (e) { ElMessage.error(errMsg(e)) }
}
</script>
