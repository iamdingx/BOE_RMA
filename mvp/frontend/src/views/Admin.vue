<template>
  <div class="page">
    <div class="card">
      <div class="toolbar">
        <b>P-08 用户与角色 / Users &amp; Roles</b>
        <span class="muted">5 角色固定；客户角色一期仅预留（不开放登录）</span>
        <div class="spacer" />
        <el-button type="primary" @click="visible = true">新增用户</el-button>
      </div>

      <el-table :data="users" border size="small">
        <el-table-column prop="username" :label="t('username')" width="150" />
        <el-table-column prop="display_name" label="姓名" width="180" />
        <el-table-column label="角色" width="180">
          <template #default="{ row }">
            <el-tag size="small" :type="row.role === 'customer' ? 'info' : ''">{{ roleLabel(row.role) }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="状态" width="110">
          <template #default="{ row }">
            <el-tag size="small" :type="row.enabled ? 'success' : 'danger'">{{ row.enabled ? '启用' : '停用' }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="created_at" label="创建时间" min-width="180" />
      </el-table>
    </div>

    <el-dialog v-model="visible" title="新增用户" width="460px">
      <el-form label-width="90px" size="small">
        <el-form-item :label="t('username')" required><el-input v-model="form.username" /></el-form-item>
        <el-form-item :label="t('password')" required><el-input v-model="form.password" type="password" /></el-form-item>
        <el-form-item label="姓名"><el-input v-model="form.display_name" /></el-form-item>
        <el-form-item label="角色" required>
          <el-select v-model="form.role" style="width:100%">
            <el-option v-for="r in roles" :key="r.value" :label="r.label" :value="r.value" />
          </el-select>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="visible = false">{{ t('common.cancel') }}</el-button>
        <el-button type="primary" @click="save">{{ t('common.save') }}</el-button>
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
const users = ref([])
const visible = ref(false)
const form = reactive({ username: '', password: '', display_name: '', role: 'warehouse_operator' })
const roles = [
  { value: 'warehouse_operator', label: '仓库操作员' },
  { value: 'project_staff', label: '项目人员' },
  { value: 'inspector', label: '质检员/R&D' },
  { value: 'customer', label: '客户/BOE US（一期不开放）' },
  { value: 'admin', label: '系统管理员' },
]
function roleLabel(r) { return roles.find((x) => x.value === r)?.label || r }

async function load() {
  try { users.value = (await api.get('/users')).items } catch (e) { ElMessage.error(errMsg(e)) }
}
onMounted(load)
async function save() {
  try {
    await api.post('/users', form)
    ElMessage.success(t('common.ok'))
    visible.value = false
    load()
  } catch (e) { ElMessage.error(errMsg(e)) }
}
</script>
