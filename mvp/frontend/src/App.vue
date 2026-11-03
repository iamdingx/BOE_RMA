<template>
  <div v-if="isLogin" class="login-wrap"><router-view /></div>
  <el-container v-else class="layout">
    <el-aside width="216px" class="aside">
      <div class="logo">
        <b>BOE</b> <span class="muted">RMA</span>
      </div>
      <el-menu :default-active="$route.path" router class="menu">
        <el-menu-item v-for="m in menus" :key="m.path" :index="m.path">
          <span>{{ t('menu.' + m.key) }}</span>
        </el-menu-item>
      </el-menu>
    </el-aside>
    <el-container>
      <el-header class="header">
        <div class="title">{{ t('app') }}</div>
        <div class="spacer" />
        <el-switch
          :model-value="locale === 'zh'" active-text="中" inactive-text="EN"
          @change="(v) => setLocale(v ? 'zh' : 'en')" style="margin-right: 12px"
        />
        <span class="muted" style="margin-right: 12px">{{ me?.username }} · {{ roleLabel }}</span>
        <el-button size="small" @click="logout">{{ t('logout') }}</el-button>
      </el-header>
      <el-main class="main">
        <div v-if="!online" class="feedback-err" style="margin: 0 16px 8px">
          {{ t('common.offline') }}
        </div>
        <router-view />
      </el-main>
    </el-container>
  </el-container>
</template>

<script setup>
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from './i18n'
import { user } from './api'

const { t, locale, setLocale } = useI18n()
const route = useRoute()
const router = useRouter()
const me = ref(user.get())
const online = ref(navigator.onLine)

const isLogin = computed(() => route.path === '/login')
const menus = [
  { path: '/forecast', key: 'forecast' },
  { path: '/receiving', key: 'receiving' },
  { path: '/kitting', key: 'kitting' },
  { path: '/discrepancy', key: 'discrepancy' },
  { path: '/inspection', key: 'inspection' },
  { path: '/outbound', key: 'outbound' },
  { path: '/packing', key: 'packing' },
  { path: '/return-board', key: 'returnBoard' },
  { path: '/buffer', key: 'buffer' },
  { path: '/reports', key: 'reports' },
  { path: '/master', key: 'master' },
  { path: '/admin', key: 'admin' },
  { path: '/mobile', key: 'mobile' },
]

const roleLabel = computed(() => {
  const map = {
    warehouse_operator: '仓库操作员', project_staff: '项目人员',
    inspector: '质检员/R&D', admin: '系统管理员', customer: '客户'
  }
  return map[me.value?.role] || me.value?.role || ''
})

function logout() {
  localStorage.clear()
  router.push('/login')
}
function onOffline() { online.value = false }
function onOnline() { online.value = true }
onMounted(() => {
  window.addEventListener('offline', onOffline)
  window.addEventListener('online', onOnline)
})
onUnmounted(() => {
  window.removeEventListener('offline', onOffline)
  window.removeEventListener('online', onOnline)
})
</script>

<style scoped>
.layout { height: 100%; }
.aside { background: #fff; border-right: 1px solid var(--border); }
.logo { height: 56px; display: flex; align-items: center; gap: 6px; padding: 0 16px; font-size: 16px; border-bottom: 1px solid var(--border); }
.menu { border-right: none; }
.header { background: #fff; border-bottom: 1px solid var(--border); display: flex; align-items: center; }
.header .title { font-weight: 600; }
.header .spacer { flex: 1; }
.main { padding: 0; background: var(--bg); overflow: auto; }
.login-wrap { height: 100%; display: flex; align-items: center; justify-content: center; background: #f0f2f5; }
</style>
