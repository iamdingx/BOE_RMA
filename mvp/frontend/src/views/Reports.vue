<template>
  <div class="page">
    <div class="card">
      <div class="toolbar">
        <b>P-07 查询与报表 / Reports</b>
        <span class="muted">TAT 仅列表标红，不做图表（二期）</span>
      </div>

      <el-tabs v-model="tab" @tab-change="loadAll">
        <el-tab-pane label="RMA 汇总" name="rma" />
        <el-tab-pane label="SKU 统计" name="sku" />
        <el-tab-pane label="周期趋势" name="trend" />
        <el-tab-pane label="差异与出库分布" name="disc" />
        <el-tab-pane label="TAT 报表" name="tat" />
        <el-tab-pane label="TAT 预警" name="alerts" />
        <el-tab-pane label="明细报表" name="detail" />
      </el-tabs>

      <!-- RMA 汇总 -->
      <template v-if="tab === 'rma'">
        <el-table :data="rmaRows" v-loading="loading" border size="small">
          <el-table-column prop="rma_no" :label="t('common.rma')" width="150" />
          <el-table-column prop="customer" :label="t('common.customer')" width="140" />
          <el-table-column prop="sn_total" label="SN 总数" width="100" align="center" />
          <el-table-column prop="judged" label="已判定" width="100" align="center" />
          <el-table-column prop="shipped" label="已还货" width="100" align="center" />
          <el-table-column prop="rma_status" label="RMA 状态" width="150" />
          <el-table-column prop="created_at" label="创建时间" min-width="170" />
        </el-table>
        <Pager :total="rmaTotal" :page="rmaPage" :size="rmaSize" @change="onRmaPage" />
      </template>

      <!-- SKU -->
      <el-table v-if="tab === 'sku'" :data="skuRows" border size="small">
        <el-table-column prop="pn_code" :label="t('common.pn')" width="160" />
        <el-table-column prop="model_code" label="机型" width="160" />
        <el-table-column prop="sn_count" label="SN 数量" width="130" align="center" />
      </el-table>

      <!-- 趋势 -->
      <el-table v-if="tab === 'trend'" :data="trendRows" border size="small">
        <el-table-column prop="period" label="周期" width="160" />
        <el-table-column prop="count" label="退货量" width="130" align="center" />
      </el-table>

      <!-- 差异 -->
      <div v-if="tab === 'disc'">
        <div class="stat-row">
          <div class="stat"><div class="num">{{ disc.total || 0 }}</div><div class="lbl">出库单总数</div></div>
          <div class="stat"><div class="num">{{ (disc.cid_ratio * 100 || 0).toFixed(1) }}%</div><div class="lbl">CID 占比</div></div>
        </div>
        <el-table :data="disc.by_type || []" border size="small" style="margin-bottom: 12px">
          <el-table-column prop="discrepancy_type" label="差异类型" width="220" />
          <el-table-column prop="count" label="数量" width="120" />
        </el-table>
        <el-table :data="disc.by_outbound || []" border size="small">
          <el-table-column prop="outbound_type" label="出库类型" width="220" />
          <el-table-column prop="count" label="单数" width="120" />
          <el-table-column prop="cid_count" label="其中 CID" width="120" />
        </el-table>
      </div>

      <!-- TAT -->
      <div v-if="tab === 'tat'">
        <div class="stat-row">
          <div class="stat"><div class="num">{{ tat.closed_count || 0 }}</div><div class="lbl">已关闭 RMA</div></div>
          <div class="stat"><div class="num">{{ (tat.avg_days || 0).toFixed(2) }}</div><div class="lbl">平均 TAT（天）</div></div>
          <div class="stat"><div class="num">{{ tat.max_days || 0 }}</div><div class="lbl">最长 TAT（天）</div></div>
          <div class="stat"><div class="num tag-red">{{ ((tat.overdue_rate || 0) * 100).toFixed(1) }}%</div><div class="lbl">超期率（&gt;10 天）</div></div>
        </div>
        <el-table :data="tat.buckets || []" border size="small">
          <el-table-column prop="label" label="TAT 区间" width="200" />
          <el-table-column prop="count" label="RMA 数" width="140" />
        </el-table>
      </div>

      <!-- 预警 -->
      <el-table v-if="tab === 'alerts'" :data="alertRows" border size="small">
        <el-table-column prop="rma_no" :label="t('common.rma')" width="150" />
        <el-table-column prop="customer" :label="t('common.customer')" width="140" />
        <el-table-column prop="rma_status" label="状态" width="150" />
        <el-table-column prop="elapsed_days" label="已用天数" width="110" align="center" />
        <el-table-column label="预警" width="140">
          <template #default="{ row }">
            <span :class="row.overdue ? 'tag-red' : 'tag-warn'">{{ row.overdue ? '超期标红' : '第8天预警' }}</span>
          </template>
        </el-table-column>
      </el-table>

      <!-- 明细 -->
      <template v-if="tab === 'detail'">
        <el-table :data="detailRows" border size="small">
          <el-table-column prop="sn" :label="t('common.sn')" width="170" />
          <el-table-column prop="pn_code" :label="t('common.pn')" width="120" />
          <el-table-column prop="rma_no" :label="t('common.rma')" width="140" />
          <el-table-column prop="status" :label="t('common.status')" width="130" />
          <el-table-column prop="inbound_at" label="入库时间" min-width="170" />
          <el-table-column prop="outbound_at" label="出库时间" min-width="170" />
          <el-table-column prop="judgment_result" label="判定" width="120" />
          <el-table-column prop="owner" label="责任人" width="110" />
        </el-table>
        <Pager :total="detailTotal" :page="detailPage" :size="detailSize" @change="onDetailPage" />
      </template>
    </div>
  </div>
</template>

<script setup>
import { onMounted, ref } from 'vue'
import { ElMessage } from 'element-plus'
import { api, errMsg } from '../api'
import { useI18n } from '../i18n'
import Pager from '../components/Pager.vue'

const { t } = useI18n()
const tab = ref('rma')
const loading = ref(false)

const rmaRows = ref([]); const rmaTotal = ref(0); const rmaPage = ref(1); const rmaSize = ref(20)
const skuRows = ref([]); const trendRows = ref([]); const disc = ref({}); const tat = ref({})
const alertRows = ref([])
const detailRows = ref([]); const detailTotal = ref(0); const detailPage = ref(1); const detailSize = ref(20)

async function loadRMA() { const r = await api.get('/reports/rma', { params: { page: rmaPage.value, size: rmaSize.value } }); rmaRows.value = r.items; rmaTotal.value = r.total }
function onRmaPage(p, s) { rmaPage.value = p; rmaSize.value = s; loadRMA() }
async function loadDetail() { const r = await api.get('/reports/detail', { params: { page: detailPage.value, size: detailSize.value } }); detailRows.value = r.items; detailTotal.value = r.total }
function onDetailPage(p, s) { detailPage.value = p; detailSize.value = s; loadDetail() }

async function loadAll() {
  loading.value = true
  try {
    if (tab.value === 'rma') await loadRMA()
    if (tab.value === 'sku') skuRows.value = (await api.get('/reports/sku')).items
    if (tab.value === 'trend') trendRows.value = (await api.get('/reports/trend')).items
    if (tab.value === 'disc') disc.value = await api.get('/reports/discrepancy')
    if (tab.value === 'tat') tat.value = await api.get('/reports/tat')
    if (tab.value === 'alerts') alertRows.value = (await api.get('/reports/tat-alerts')).items
    if (tab.value === 'detail') await loadDetail()
  } catch (e) { ElMessage.error(errMsg(e)) } finally { loading.value = false }
}
onMounted(loadAll)
</script>
