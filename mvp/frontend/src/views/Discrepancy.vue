<template>
  <div class="page">
    <div class="card">
      <div class="toolbar">
        <b>P-04 差异审核工作台 / Discrepancy</b>
        <el-input v-model="kw" placeholder="RMA / 差异号 / SN" style="width: 200px" clearable @keyup.enter="reload" />
        <el-button @click="reload">{{ t('common.search') }}</el-button>
        <div class="spacer" />
        <span class="muted">按 RMA / 订单号为索引；双异常记录永久保留，任何角色不可删除</span>
      </div>

      <el-tabs v-model="tab" @tab-change="reload">
        <el-tab-pane label="待审批" name="pending" />
        <el-tab-pane label="已处理" name="done" />
        <el-tab-pane label="双异常记录查询" name="anomaly" />
      </el-tabs>

      <el-table :data="rows" v-loading="loading" border size="small">
        <el-table-column prop="rma_no" :label="t('common.rma')" width="140" />
        <el-table-column prop="forecast_no" label="预报单" width="140" />
        <el-table-column prop="record_no" label="差异号" width="150" />
        <el-table-column label="来源" width="110">
          <template #default="{ row }">
            <el-tag size="small" :type="row.source === 'RECEIVING' ? 'info' : 'warning'">{{ row.source }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="discrepancy_type" label="差异类型" width="160" />
        <el-table-column prop="sn" :label="t('common.sn')" width="150" />
        <el-table-column label="数量差" width="90" align="center">
          <template #default="{ row }">
            <span :class="row.qty_diff > 0 ? 'tag-warn' : (row.qty_diff < 0 ? 'tag-red' : '')">{{ row.qty_diff }}</span>
          </template>
        </el-table-column>
        <el-table-column label="照片" width="80" align="center">
          <template #default="{ row }">{{ row.photo_ids?.length || 0 }}</template>
        </el-table-column>
        <el-table-column label="状态" width="110">
          <template #default="{ row }">
            <el-tag size="small" :type="statusType(row.approval_status)">{{ row.approval_status }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column :label="t('common.action')" width="230" fixed="right">
          <template #default="{ row }">
            <el-button v-if="!row.locked && row.approval_status === 'PENDING'" link type="primary" @click="openSubmit(row)">
              补照片提交
            </el-button>
            <template v-if="row.approval_status === 'PENDING' && row.locked">
              <el-button link type="success" @click="openApprove(row)">{{ t('common.approve') }}</el-button>
              <el-button link type="danger" @click="openReject(row)">{{ t('common.reject') }}</el-button>
            </template>
          </template>
        </el-table-column>
      </el-table>
      <Pager :total="total" :page="page" :size="pageSize" @change="change" />
    </div>

    <!-- 提交 -->
    <el-dialog v-model="submitVisible" title="补佐证照片并提交" width="520px">
      <div class="muted" style="margin-bottom: 8px">
        多收需 ≥3 张（箱内物 + 箱标含数量 + 实物 PN/SN）；少收需 ≥2 张（箱内物 + 箱标含数量）
      </div>
      <input ref="fileRef" type="file" accept="image/*" multiple style="display: none" @change="upload" />
      <el-button size="small" @click="fileRef.click()">选择照片</el-button>
      <span class="muted" style="margin-left: 8px">已选 {{ photos.length }} 张</span>
      <template #footer>
        <el-button @click="submitVisible = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" @click="doSubmit">{{ t('common.submit') }}</el-button>
      </template>
    </el-dialog>

    <!-- 审批 -->
    <el-dialog v-model="approveVisible" title="差异审批" width="520px">
      <el-form label-width="90px" size="small">
        <el-form-item label="审批说明">
          <el-input v-model="note" type="textarea" :rows="3" placeholder="与客户厘清后的实际情况说明（必填）" />
        </el-form-item>
        <div class="muted">
          注意：KITTING 差异审批通过 → 预报修正为 REVISED；RECEIVING 差异审批通过 → 仅放行不改预报。
        </div>
      </el-form>
      <template #footer>
        <el-button @click="approveVisible = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" @click="doApprove">{{ t('common.approve') }}</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="rejectVisible" title="差异驳回" width="480px">
      <el-input v-model="reason" type="textarea" :rows="3" placeholder="驳回原因（必填，驳回后自动解锁允许重扫）" />
      <template #footer>
        <el-button @click="rejectVisible = false">{{ t('common.cancel') }}</el-button>
        <el-button type="danger" @click="doReject">{{ t('common.reject') }}</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref } from 'vue'
import { ElMessage } from 'element-plus'
import { api, errMsg } from '../api'
import { useI18n } from '../i18n'
import { useList } from '../composables/useList'
import Pager from '../components/Pager.vue'

const { t } = useI18n()
const tab = ref('pending')
const kw = ref('')
const { rows, total, page, pageSize, loading, load, change } = useList(
  (p) => api.get('/discrepancies', { params: { ...p, tab: tab.value, keyword: kw.value } })
)
function reload() { change(1, pageSize.value) }
function statusType(s) {
  return { PENDING: 'warning', APPROVED: 'success', REJECTED: 'danger' }[s] || ''
}

const submitVisible = ref(false)
const fileRef = ref(null)
const photos = ref([])
const current = ref(null)
function openSubmit(row) { current.value = row; photos.value = []; submitVisible.value = true }
async function upload(ev) {
  for (const f of [...(ev.target.files || [])]) {
    if (f.size > 10 * 1024 * 1024) { ElMessage.error(`${f.name} 超过 10MB`); continue }
    const fd = new FormData()
    fd.append('file', f)
    fd.append('biz_type', 'DISCREPANCY_EVIDENCE')
    fd.append('biz_ref_no', current.value.record_no)
    fd.append('sn', current.value.sn || '')
    try {
      const p = await api.post('/photos', fd, { headers: { 'Content-Type': 'multipart/form-data' } })
      photos.value.push(p.photo_id)
    } catch (e) { ElMessage.error(errMsg(e)) }
  }
  ev.target.value = ''
}
async function doSubmit() {
  try {
    await api.post(`/discrepancies/${current.value.record_no}/submit`, { photo_ids: photos.value })
    ElMessage.success('已提交')
    submitVisible.value = false
    load()
  } catch (e) { ElMessage.error(errMsg(e)) }
}

const approveVisible = ref(false)
const note = ref('')
function openApprove(row) { current.value = row; note.value = ''; approveVisible.value = true }
async function doApprove() {
  if (!note.value) return ElMessage.warning('审批说明必填')
  try {
    await api.post(`/discrepancies/${current.value.record_no}/approve`, { note: note.value })
    ElMessage.success('审批通过')
    approveVisible.value = false
    load()
  } catch (e) { ElMessage.error(errMsg(e)) }
}

const rejectVisible = ref(false)
const reason = ref('')
function openReject(row) { current.value = row; reason.value = ''; rejectVisible.value = true }
async function doReject() {
  if (!reason.value) return ElMessage.warning('驳回原因必填')
  try {
    await api.post(`/discrepancies/${current.value.record_no}/reject`, { reason: reason.value })
    ElMessage.success('已驳回并解锁')
    rejectVisible.value = false
    load()
  } catch (e) { ElMessage.error(errMsg(e)) }
}
</script>
