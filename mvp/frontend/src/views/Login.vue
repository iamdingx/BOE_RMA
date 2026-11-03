<template>
  <div class="page">
    <div class="card" style="max-width: 420px; margin: 0 auto">
      <h2 style="margin: 0 0 4px">{{ t('app') }}</h2>
      <div class="muted" style="margin-bottom: 16px">BOE US RMA · MVP v1.0</div>
      <el-form :model="form" label-position="top" @submit.prevent="doLogin">
        <el-form-item :label="t('username')">
          <el-input v-model="form.username" autofocus placeholder="project / warehouse / inspector / admin" />
        </el-form-item>
        <el-form-item :label="t('password')">
          <el-input v-model="form.password" type="password" show-password @keyup.enter="doLogin" />
        </el-form-item>
        <el-button type="primary" style="width: 100%" :loading="loading" @click="doLogin">
          {{ t('login') }}
        </el-button>
      </el-form>
      <div class="muted" style="margin-top: 12px; line-height: 1.7">
        演示账号：project / Proj@123 · warehouse / Wh@123 · inspector / Insp@123 · admin / Admin@123<br />
        客户账号一期不开放（登录返回 403）。
      </div>
    </div>
  </div>
</template>

<script setup>
import { reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { api, errMsg, token, user } from '../api'
import { useI18n } from '../i18n'

const { t } = useI18n()
const router = useRouter()
const loading = ref(false)
const form = reactive({ username: 'project', password: 'Proj@123' })

async function doLogin() {
  loading.value = true
  try {
    const res = await api.post('/auth/login', form)
    token.set(res.token)
    user.set(res)
    ElMessage.success('登录成功')
    router.push('/forecast')
  } catch (e) {
    ElMessage.error(errMsg(e))
  } finally {
    loading.value = false
  }
}
</script>
