import { createRouter, createWebHashHistory } from 'vue-router'
import { token } from './api'

const routes = [
  { path: '/login', component: () => import('./views/Login.vue'), meta: { public: true } },
  { path: '/', redirect: '/forecast' },
  { path: '/forecast', component: () => import('./views/Forecast.vue') },
  { path: '/receiving', component: () => import('./views/Receiving.vue') },
  { path: '/kitting', component: () => import('./views/Kitting.vue') },
  { path: '/discrepancy', component: () => import('./views/Discrepancy.vue') },
  { path: '/inspection', component: () => import('./views/Inspection.vue') },
  { path: '/outbound', component: () => import('./views/Outbound.vue') },
  { path: '/packing', component: () => import('./views/Packing.vue') },
  { path: '/return-board', component: () => import('./views/ReturnBoard.vue') },
  { path: '/buffer', component: () => import('./views/Buffer.vue') },
  { path: '/reports', component: () => import('./views/Reports.vue') },
  { path: '/master', component: () => import('./views/Master.vue') },
  { path: '/admin', component: () => import('./views/Admin.vue') },
  { path: '/mobile', component: () => import('./views/Mobile.vue') },
]

const router = createRouter({ history: createWebHashHistory(), routes })

router.beforeEach((to) => {
  if (!to.meta.public && !token.get()) return { path: '/login' }
  return true
})

export default router
