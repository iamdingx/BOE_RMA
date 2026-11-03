<template>
  <div class="page">
    <div class="card">
      <div class="toolbar">
        <b>P-01 主数据 / Master Data</b>
        <span class="muted">机型与料号维护（kitting 四维校验基准）</span>
        <div class="spacer" />
        <input ref="fileRef" type="file" accept=".xlsx" style="display: none" @change="importFile" />
        <el-button @click="fileRef.click()">Excel 导入</el-button>
        <el-button type="primary" @click="modelVisible = true">新增机型</el-button>
        <el-button type="primary" @click="pnVisible = true">新增料号</el-button>
      </div>

      <el-row :gutter="12">
        <el-col :span="11">
          <div style="font-weight:600;margin-bottom:8px">产品机型 / Models</div>
          <el-table :data="models" border size="small">
            <el-table-column prop="model_code" label="机型编码" width="140" />
            <el-table-column prop="model_name" label="名称" />
          </el-table>
        </el-col>
        <el-col :span="13">
          <div style="font-weight:600;margin-bottom:8px">料号 / Part Numbers</div>
          <el-table :data="pns" border size="small">
            <el-table-column prop="pn_code" :label="t('common.pn')" width="140" />
            <el-table-column prop="model_code" label="机型" width="120" />
            <el-table-column prop="description" label="描述" />
          </el-table>
        </el-col>
      </el-row>
      <div v-if="importErrors.length" class="feedback-err" style="margin-top:12px">
        <div v-for="(e, i) in importErrors" :key="i">{{ e }}</div>
      </div>
    </div>

    <el-dialog v-model="modelVisible" title="新增机型" width="440px">
      <el-form label-width="90px" size="small">
        <el-form-item label="机型编码" required><el-input v-model="modelForm.model_code" /></el-form-item>
        <el-form-item label="名称" required><el-input v-model="modelForm.model_name" /></el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="modelVisible = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" @click="saveModel">{{ t('common.save') }}</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="pnVisible" title="新增料号" width="440px">
      <el-form label-width="90px" size="small">
        <el-form-item :label="t('common.pn')" required><el-input v-model="pnForm.pn_code" /></el-form-item>
        <el-form-item label="机型" required>
          <el-select v-model="pnForm.model_code" style="width:100%">
            <el-option v-for="m in models" :key="m.model_code" :label="m.model_code" :value="m.model_code" />
          </el-select>
        </el-form-item>
        <el-form-item label="描述"><el-input v-model="pnForm.description" /></el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="pnVisible = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" @click="savePN">{{ t('common.save') }}</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { onMounted, reactive, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { api, errMsg } from '../api'
import { useI18n } from '../i18n'

const { t } = useI18n()
const models = ref([])
const pns = ref([])
const modelVisible = ref(false)
const pnVisible = ref(false)
const fileRef = ref(null)
const importErrors = ref([])
const modelForm = reactive({ model_code: '', model_name: '' })
const pnForm = reactive({ pn_code: '', model_code: '', description: '' })

async function load() {
  models.value = (await api.get('/master/models')).items
  pns.value = (await api.get('/master/pns')).items
}
onMounted(load)

async function saveModel() {
  try {
    await api.post('/master/models', modelForm)
    ElMessage.success(t('common.ok'))
    modelVisible.value = false
    modelForm.model_code = ''
    modelForm.model_name = ''
    load()
  } catch (e) { ElMessage.error(errMsg(e)) }
}
async function savePN() {
  try {
    await api.post('/master/pns', pnForm)
    ElMessage.success(t('common.ok'))
    pnVisible.value = false
    pnForm.pn_code = ''
    load()
  } catch (e) { ElMessage.error(errMsg(e)) }
}
async function importFile(ev) {
  const f = ev.target.files?.[0]
  if (!f) return
  importErrors.value = []
  const fd = new FormData()
  fd.append('file', f)
  try {
    const res = await api.post('/master/import', fd, { headers: { 'Content-Type': 'multipart/form-data' } })
    ElMessage.success(`导入成功：机型 ${res.models_inserted}，料号 ${res.pns_inserted}`)
    load()
  } catch (e) {
    const detail = e?.response?.data?.detail
    if (detail?.errors) importErrors.value = detail.errors
    ElMessage.error(errMsg(e))
  }
  ev.target.value = ''
}
</script>
