<template>
  <div class="page">
    <div class="card">
      <div class="toolbar">
        <b>P-09 RMA 还货看板 / Return Board</b>
        <span class="muted">TAT：完整收货 → 完整发货完成 ≤10 自然日；第 8 天预警、超 10 天标红</span>
        <div class="spacer" />
        <el-select v-model="status" clearable placeholder="RMA 状态" style="width: 160px" @change="reload">
          <el-option v-for="s in statuses" :key="s" :label="s" :value="s" />
        </el-select>
        <el-button @click="load">刷新</el-button>
      </div>

      <el-table :data="rows" v-loading="loading" border size="small">
        <el-table-column prop="rma_no" :label="t('common.rma')" width="150" />
        <el-table-column prop="forecast_no" label="预报单" width="140" />
        <el-table-column label="RMA 状态" width="150">
          <template #default="{ row }"><el-tag size="small">{{ row.rma_status }}</el-tag></template>
        </el-table-column>
        <el-table-column prop="sn_total" label="SN 总数" width="90" align="center" />
        <el-table-column prop="returned" label="已还货" width="90" align="center" />
        <el-table-column prop="pending" label="待还货" width="90" align="center" />
        <el-table-column label="还货方式分布" min-width="200">
          <template #default="{ row }">
            <span v-for="(v, k) in row.method_dist" :key="k" style="margin-right: 8px">
              <el-tag size="small" type="info">{{ k }}:{{ v }}</el-tag>
            </span>
          </template>
        </el-table-column>
        <el-table-column label="TAT" width="130">
          <template #default="{ row }">
            <span :class="row.tat_overdue ? 'tag-red' : (row.tat_warning ? 'tag-warn' : '')">
              {{ row.tat_days != null ? row.tat_days + ' 天' : row.elapsed_days + ' 天（进行中）' }}
              {{ row.tat_overdue ? ' · 超期' : (row.tat_warning ? ' · 预警' : '') }}
            </span>
          </template>
        </el-table-column>
        <el-table-column :label="t('common.action')" width="200" fixed="right">
          <template #default="{ row }">
            <el-button link type="primary" @click="showDetail(row)">{{ t('common.detail') }}</el-button>
            <el-button link type="warning" @click="openDispose(row)">RETAINED 处置</el-button>
          </template>
        </el-table-column>
      </el-table>
      <Pager :total="total" :page="page" :size="pageSize" @change="change" />
    </div>

    <el-dialog v-model="detailVisible" title="还货进度 / Progress" width="520px">
      <div class="stat-row" v-if="detail.forecast_no">
        <div class="stat"><div class="num">{{ detail.sn_total }}</div><div class="lbl">SN 总数</div></div>
        <div class="stat"><div class="num" style="color:var(--ok)">{{ detail.returned }}</div><div class="lbl">已还货</div></div>
        <div class="stat"><div class="num" style="color:var(--warn)">{{ detail.pending }}</div><div class="lbl">待还货</div></div>
      </div>
      <p class="muted">RETAINED 原片留库不影响 RMA 关闭；「100% 还货」按 return_record 计数。</p>
    </el-dialog>

    <el-dialog v-model="disposeVisible" title="RETAINED 原片处置" width="520px">
      <el-form label-width="110px" size="small">
        <el-form-item :label="t('common.sn')" required>
          <el-input v-model="dispose.sn" placeholder="留库待定的原片 SN" />
        </el-form-item>
        <el-form-item label="处置路径" required>
          <el-select v-model="dispose.retained_disposition" style="width: 100%">
            <el-option label="报废处置 SCRAPPED" value="SCRAPPED" />
            <el-option label="退供应商 RETURN_VENDOR" value="RETURN_VENDOR" />
            <el-option label="修好入 BUFFER REPAIR_TO_BUFFER" value="REPAIR_TO_BUFFER" />
            <el-option label="长期留库 LONG_RETAIN（不强制收尾）" value="LONG_RETAIN" />
          </el-select>
        </el-form-item>
        <el-form-item label="说明"><el-input v-model="dispose.note" type="textarea" :rows="2" /></el-form-item>
        <div class="muted">报废/退供应商将生成处置出库单（报废需项目审批）；修好入 BUFFER 直接转为可用备件（同 SN 双身份）。</div>
      </el-form>
      <template #footer>
        <el-button @click="disposeVisible = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" @click="doDispose">{{ t('common.submit') }}</el-button>
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
const status = ref('')
const statuses = ['OPEN', 'RECEIVING', 'PROCESSING', 'PENDING_RETURN', 'RETURNED', 'CLOSED']
const { rows, total, page, pageSize, loading, load, change } = useList(
  (p) => api.get('/return-board', { params: { ...p, status: status.value } })
)
function reload() { change(1, pageSize.value) }

const detailVisible = ref(false)
const detail = ref({})
async function showDetail(row) {
  detail.value = await api.get(`/rma/${row.forecast_no}/return-progress`)
  detailVisible.value = true
}

const disposeVisible = ref(false)
const dispose = reactive({ sn: '', retained_disposition: 'SCRAPPED', note: '' })
function openDispose() { dispose.sn = ''; dispose.retained_disposition = 'SCRAPPED'; dispose.note = ''; disposeVisible.value = true }
async function doDispose() {
  if (!dispose.sn) return ElMessage.warning('SN 必填')
  try {
    const res = await api.post(`/retained/${dispose.sn}/dispose`, {
      retained_disposition: dispose.retained_disposition, note: dispose.note
    })
    ElMessage.success(res.result || '处置已记录')
    disposeVisible.value = false
    load()
  } catch (e) { ElMessage.error(errMsg(e)) }
}
</script>
