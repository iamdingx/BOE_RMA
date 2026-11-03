<template>
  <div class="page">
    <div class="card">
      <div class="toolbar">
        <b>P-10 BUFFER 缓冲备件 / Buffer Stock</b>
        <span class="muted">备件独立表，不进 sn_item；跨 PN 替代需项目人员审批</span>
        <div class="spacer" />
        <el-button @click="inboundVisible = true">备件入库</el-button>
        <el-button @click="loadStock">刷新库存</el-button>
      </div>

      <el-table :data="stock" v-loading="stockLoading" border size="small">
        <el-table-column prop="pn_code" :label="t('common.pn')" width="140" />
        <el-table-column prop="available" label="可用 AVAILABLE" width="140" align="center" />
        <el-table-column prop="reserved" label="预占 RESERVED" width="140" align="center" />
        <el-table-column prop="out" label="已出库 OUT" width="130" align="center" />
        <el-table-column prop="scrapped" label="已报废" width="110" align="center" />
        <el-table-column :label="t('common.action')" min-width="200">
          <template #default="{ row }">
            <el-button link type="primary" @click="viewSN(row.pn_code, 'AVAILABLE')">可用明细</el-button>
            <el-button link @click="viewSN(row.pn_code, 'OUT')">已出库明细</el-button>
          </template>
        </el-table-column>
      </el-table>
    </div>

    <div class="card">
      <div class="toolbar">
        <b>明细 SN</b>
        <span class="muted">按 SN 报废走单独流程，不进退货 SN 状态机</span>
      </div>
      <el-table :data="snRows" border size="small">
        <el-table-column prop="sn" :label="t('common.sn')" width="180" />
        <el-table-column prop="pn_code" :label="t('common.pn')" width="130" />
        <el-table-column prop="source" label="来源" width="160" />
        <el-table-column prop="stock_status" :label="t('common.status')" width="130" />
        <el-table-column prop="inbound_at" label="入库时间" min-width="170" />
        <el-table-column :label="t('common.action')" width="120">
          <template #default="{ row }">
            <el-button v-if="row.stock_status === 'AVAILABLE'" link type="danger" @click="scrapSN(row)">报废</el-button>
          </template>
        </el-table-column>
      </el-table>
      <Pager :total="snTotal" :page="snPage" :size="snSize" @change="onPage" />
    </div>

    <el-dialog v-model="inboundVisible" title="缓冲备件入库" width="500px">
      <el-form label-width="100px" size="small">
        <el-form-item :label="t('common.sn')" required><el-input v-model="inbound.sn" /></el-form-item>
        <el-form-item :label="t('common.pn')" required>
          <el-select v-model="inbound.pn_code" filterable allow-create style="width: 100%">
            <el-option v-for="p in pns" :key="p.pn_code" :label="p.pn_code" :value="p.pn_code" />
          </el-select>
        </el-form-item>
        <el-form-item label="来源" required>
          <el-select v-model="inbound.source" style="width: 100%">
            <el-option label="新品采购 NEW_PURCHASE" value="NEW_PURCHASE" />
            <el-option label="维修良品 REPAIRED_GOOD" value="REPAIRED_GOOD" />
          </el-select>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="inboundVisible = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" @click="doInbound">{{ t('common.submit') }}</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { onMounted, reactive, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { api, errMsg } from '../api'
import { useI18n } from '../i18n'
import Pager from '../components/Pager.vue'

const { t } = useI18n()
const stock = ref([])
const stockLoading = ref(false)
const pns = ref([])
const snRows = ref([])
const snTotal = ref(0)
const snPage = ref(1)
const snSize = ref(20)
const inboundVisible = ref(false)
const inbound = reactive({ sn: '', pn_code: 'PN-A100', source: 'NEW_PURCHASE' })
let curPN = ''
let curStatus = 'AVAILABLE'

onMounted(async () => {
  await loadStock()
  try { pns.value = (await api.get('/master/pns')).items } catch { /* ignore */ }
})

async function loadStock() {
  stockLoading.value = true
  try { stock.value = (await api.get('/buffer/stock')).items } catch (e) { ElMessage.error(errMsg(e)) }
  finally { stockLoading.value = false }
}
async function viewSN(pn, status) {
  curPN = pn
  curStatus = status
  snPage.value = 1
  await loadSN()
}
async function loadSN() {
  const res = await api.get('/buffer/sns', { params: { pn_code: curPN, status: curStatus, page: snPage.value, size: snSize.value } })
  snRows.value = res.items
  snTotal.value = res.total
}
function onPage(p, s) { snPage.value = p; snSize.value = s; loadSN() }

async function doInbound() {
  try {
    await api.post('/buffer/inbound', inbound)
    ElMessage.success('入库成功')
    inboundVisible.value = false
    inbound.sn = ''
    loadStock()
  } catch (e) { ElMessage.error(errMsg(e)) }
}
async function scrapSN(row) {
  try {
    const { value } = await ElMessageBox.prompt('备件报废原因', '备件报废（单独流程）')
    await api.post(`/buffer/${row.sn}/scrap`, { reason: value })
    ElMessage.success('已报废')
    loadStock(); loadSN()
  } catch (e) { if (e !== 'cancel') ElMessage.error(errMsg(e)) }
}
</script>
