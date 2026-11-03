<template>
  <div class="page">
    <div class="card">
      <div class="toolbar">
        <b>P-03 物流允收 / Receiving</b>
        <span class="muted">只确认外箱标识数量 + POD，不开箱、不点 SN、不生成箱号</span>
        <div class="spacer" />
        <el-button type="primary" @click="openCreate">登记允收</el-button>
        <el-button @click="$router.push('/kitting')">去 Kitting 开箱</el-button>
      </div>

      <el-table :data="rows" v-loading="loading" border size="small">
        <el-table-column prop="batch_no" label="批次号" width="150" />
        <el-table-column prop="forecast_no" label="预报单号" width="140" />
        <el-table-column prop="label_qty_total" label="外箱标识数量" width="120" align="center" />
        <el-table-column prop="qty_diff" label="差异" width="90" align="center">
          <template #default="{ row }">
            <span :class="row.qty_diff === 0 ? '' : (row.qty_diff > 0 ? 'tag-warn' : 'tag-red')">
              {{ row.qty_diff === 0 ? '0' : (row.qty_diff > 0 ? '+' + row.qty_diff : row.qty_diff) }}
            </span>
          </template>
        </el-table-column>
        <el-table-column label="POD" width="140">
          <template #default="{ row }">
            <el-tag size="small" :type="row.pod_photo_ids?.length ? 'success' : 'danger'">
              {{ row.pod_photo_ids?.length ? `${row.pod_photo_ids.length} 张` : '未上传' }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="received_by" label="允收人" width="120" />
        <el-table-column prop="received_at" label="允收时间" min-width="170" />
        <el-table-column :label="t('common.action')" width="180" fixed="right">
          <template #default="{ row }">
            <el-button link type="primary" @click="viewPOD(row)">查看 POD</el-button>
            <el-button link type="primary" @click="$router.push('/kitting?batch=' + row.batch_no)">开箱扫码</el-button>
          </template>
        </el-table-column>
      </el-table>
      <Pager :total="total" :page="page" :size="pageSize" @change="change" />
    </div>

    <el-dialog v-model="visible" title="登记物流允收 / New Receiving Batch" width="560px">
      <el-form :model="form" label-width="120px" size="small">
        <el-form-item label="预报单" required>
          <el-select v-model="form.forecast_no" filterable style="width: 100%" @change="onForecast">
            <el-option v-for="f in forecasts" :key="f.forecast_no" :label="`${f.rma_no} · ${f.forecast_no}`" :value="f.forecast_no" />
          </el-select>
        </el-form-item>
        <el-form-item label="预报数量">
          <b>{{ current?.forecast_qty ?? '—' }}</b>
        </el-form-item>
        <el-form-item label="外箱标识数量" required>
          <el-input-number v-model="form.label_qty_total" :min="0" />
          <span class="muted" style="margin-left: 8px">按外箱标签累计数量</span>
        </el-form-item>
        <el-form-item label="POD 照片" required>
          <input ref="podRef" type="file" accept="image/*" multiple style="display: none" @change="uploadPOD" />
          <el-button size="small" @click="podRef.click()">选择照片</el-button>
          <span class="muted" style="margin-left: 8px">已上传 {{ form.pod_photo_ids.length }} 张（单张 ≤10MB）</span>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="visible = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" :loading="saving" @click="doCreate">{{ t('common.submit') }}</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { computed, onMounted, reactive, ref } from 'vue'
import { useRoute } from 'vue-router'
import { ElMessage } from 'element-plus'
import { api, errMsg } from '../api'
import { useI18n } from '../i18n'
import { useList } from '../composables/useList'
import Pager from '../components/Pager.vue'

const { t } = useI18n()
const route = useRoute()
const { rows, total, page, pageSize, loading, load, change } = useList(
  (p) => api.get('/receiving/batches', { params: p })
)

const forecasts = ref([])
const visible = ref(false)
const saving = ref(false)
const podRef = ref(null)
const form = reactive({ forecast_no: '', label_qty_total: 0, pod_photo_ids: [] })
const current = computed(() => forecasts.value.find((f) => f.forecast_no === form.forecast_no))

onMounted(async () => {
  try { forecasts.value = (await api.get('/forecasts', { params: { size: 200 } })).items } catch { /* ignore */ }
  if (route.query.forecast) openCreate(route.query.forecast)
})
function onForecast() {
  form.label_qty_total = current.value?.forecast_qty || 0
}
function openCreate(fno) {
  visible.value = true
  form.forecast_no = fno || ''
  form.pod_photo_ids = []
  onForecast()
}
async function uploadPOD(ev) {
  const files = [...(ev.target.files || [])]
  for (const f of files) {
    if (f.size > 10 * 1024 * 1024) { ElMessage.error(`${f.name} 超过 10MB`); continue }
    const fd = new FormData()
    fd.append('file', f)
    fd.append('biz_type', 'POD')
    fd.append('biz_ref_no', form.forecast_no || 'TEMP')
    try {
      const p = await api.post('/photos', fd, { headers: { 'Content-Type': 'multipart/form-data' } })
      form.pod_photo_ids.push(p.photo_id)
    } catch (e) { ElMessage.error(errMsg(e)) }
  }
  ev.target.value = ''
}
async function doCreate() {
  if (!form.forecast_no) return ElMessage.warning('请选择预报单')
  if (!form.pod_photo_ids.length) return ElMessage.warning('必须上传 POD 照片')
  saving.value = true
  try {
    await api.post('/receiving/batches', form)
    ElMessage.success(t('common.ok'))
    visible.value = false
    load()
  } catch (e) { ElMessage.error(errMsg(e)) } finally { saving.value = false }
}
async function viewPOD(row) {
  const ids = row.pod_photo_ids || []
  if (!ids.length) return ElMessage.info('该批次未上传 POD')
  ElMessage.info(`POD 照片 ${ids.length} 张：${ids.join(', ')}`)
}
</script>
