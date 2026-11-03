import axios from 'axios'

export const api = axios.create({ baseURL: '/api/v1', timeout: 30000 })

// 断网禁止提交（DZ-08：不做本地缓存/离线补传）
api.interceptors.request.use((cfg) => {
  if (!navigator.onLine && cfg.method && cfg.method !== 'get') {
    return Promise.reject({ offline: true, message: 'OFFLINE' })
  }
  const t = localStorage.getItem('token')
  if (t) cfg.headers.Authorization = `Bearer ${t}`
  return cfg
})

api.interceptors.response.use(
  (r) => r.data,
  (e) => {
    if (e?.response?.status === 401) {
      localStorage.removeItem('token')
      localStorage.removeItem('user')
      if (location.hash !== '#/login') location.hash = '#/login'
    }
    return Promise.reject(e)
  }
)

export function errMsg(e) {
  if (e?.offline) return '网络已断开，禁止提交（请恢复网络后重试）'
  return e?.response?.data?.message || e?.message || '请求失败'
}

export const token = {
  get: () => localStorage.getItem('token'),
  set: (v) => localStorage.setItem('token', v),
  clear: () => { localStorage.removeItem('token'); localStorage.removeItem('user') }
}

export const user = {
  get() {
    try { return JSON.parse(localStorage.getItem('user') || 'null') } catch { return null }
  },
  set(u) { localStorage.setItem('user', JSON.stringify(u)) }
}

// 打印标签（浏览器原生打印）
export function openLabel(path) {
  const t = token.get()
  fetch(path, { headers: { Authorization: `Bearer ${t}` } })
    .then((r) => r.text())
    .then((html) => {
      const w = window.open('', '_blank', 'width=420,height=640')
      if (w) { w.document.write(html); w.document.close() }
    })
}
