<template>
  <div class="page">
    <div class="card">
      <div class="toolbar">
        <b>P-05 质检判定 / Inspection</b>
        <span class="muted">仅 RECEIVED 状态可质检；支持多选批量提交</span>
        <div class="spacer" />
        <el-select v-model="bulk" placeholder="批量判定" style="width: 170px">
          <el-option v-for="j in judgments" :key="j.value" :label="j.label" :value="j.value" />
        </el-select>
        <el-button type="primary" :disabled="!selected.length || !bulk" @click="doBatch">
          提交选中（{{ selected.length }}）
        </el-button>
      </div>

      <el-table :data="rows" v-loading="loading" border size="small" @selection-change="(v) => (selected = v)">
        <el-table-column type="selection" width="46" />
        <el-table-column prop="sn" :label="t('common.sn')" width="180" />
        <el-table-column prop="pn_code" :label="t('common.pn')" width="120" />
        <el-table-column prop="rma_no" :label="t('common.rma')" width="150" />
        <el-table-column prop="box_no" label="箱号" width="150" />
        <el-table-column prop="status" :label="t('common.status')" width="120" />
        <el-table-column prop="received_at" label="接收时间" min-width="170" />
        <el-table-column :label="t('common.action')" width="120" fixed="right">
          <template #default="{ row }">
            <el-button link type="primary" @click="openJudge(row)">判定</el-button>
          </template>
        </el-table-column>
      </el-table>
      <Pager :total="total" :page="page" :size="pageSize" @change="change" />
    </div>

    <el-dialog v-model="visible" title="质检判定录入" width="560px">
      <el-form label-width="110px" size="small">
        <el-form-item :label="t('common.sn')"><b class="mono">{{ current?.sn }}</b></el-form-item>
        <el-form-item label="判定结果" required>
          <el-select v-model="form.judgment_result" style="width: 100%">
            <el-option v-for="j in judgments" :key="j.value" :label="j.label" :value="j.value" />
          </el-select>
        </el-form-item>
        <template v-if="form.judgment_result === 'CID_RETURN'">
          <el-form-item label="责任标记" required>
            <el-checkbox v-model="form.cid_flag">客户责任（CID）</el-checkbox>
          </el-form-item>
          <el-form-item label="损坏佐证" required>
            <input ref="fileRef" type="file" accept="image/*" multiple style="display: none" @change="upload" />
            <el-button size="small" @click="fileRef.click()">上传照片</el-button>
            <span class="muted" style="margin-left: 8px">已上传 {{ form.damage_photo_ids.length }} 张</span>
          </el-form-item>
          <el-form-item label="项目确认" required>
            <el-input v-model="form.customer_confirmation" type="textarea" :rows="2" placeholder="项目人员与客户确认记录（必填）；客户回复佐证选填" />
          </el-form-item>
        </template>
        <div class="muted">还货方式由判定结果自动带出：REPAIR→原片修复；CID→CID；REPLACE/SCRAP/TRANSFER_RC→BUFFER 还货</div>
      </el-form>
      <template #footer>
        <el-button @click="visible = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" :loading="saving" @click="doJudge">{{ t('common.submit') }}</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { reactive, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { api, errMsg } from '../api'
import { useI18n } from '../i18n'
import { useList } from '../composables/useList'
import Pager from '../components/Pager.vue'

const { t } = useI18n()
const { rows, total, page, pageSize, loading, load, change } = useList(
  (p) => api.get('/inspections', { params: { ...p, status: 'RECEIVED' } })
)

const judgments = [
  { value: 'REPAIR', label: '维修 / Repair' },
  { value: 'REPLACE', label: '换新 / Replace' },
  { value: 'SCRAP', label: '报废 / Scrap' },
  { value: 'TRANSFER_RC', label: '转维修中心 / Transfer RC' },
  { value: 'CID_RETURN', label: 'CID 原片返还 / CID' },
]
const selected = ref([])
const bulk = ref('')

async function doBatch() {
  const items = selected.value.map((r) => ({ sn: r.sn, judgment_result: bulk.value }))
  try {
    await api.post('/inspections/batch', { items })
    ElMessage.success(`已提交 ${items.length} 条`)
    selected.value = []
    load()
  } catch (e) { ElMessage.error(errMsg(e)) }
}

const visible = ref(false)
const saving = ref(false)
const current = ref(null)
const fileRef = ref(null)
const form = reactive({ judgment_result: 'REPAIR', cid_flag: false, damage_photo_ids: [], customer_confirmation: '' })

function openJudge(row) {
  current.value = row
  form.judgment_result = 'REPAIR'
  form.cid_flag = false
  form.damage_photo_ids = []
  form.customer_confirmation = ''
  visible.value = true
}
async function upload(ev) {
  for (const f of [...(ev.target.files || [])]) {
    if (f.size > 10 * 1024 * 1024) { ElMessage.error(`${f.name} 超过 10MB`); continue }
    const fd = new FormData()
    fd.append('file', f)
    fd.append('biz_type', 'CID_DAMAGE_EVIDENCE')
    fd.append('biz_ref_no', current.value.sn)
    fd.append('sn', current.value.sn)
    try {
      const p = await api.post('/photos', fd, { headers: { 'Content-Type': 'multipart/form-data' } })
      form.damage_photo_ids.push(p.photo_id)
    } catch (e) { ElMessage.error(errMsg(e)) }
  }
  ev.target.value = ''
}
async function doJudge() {
  saving.value = true
  try {
    await api.post('/inspections', { sn: current.value.sn, ...form })
    ElMessage.success('判定成功')
    visible.value = false
    load()
  } catch (e) { ElMessage.error(errMsg(e)) } finally { saving.value = false }
}
</script>
