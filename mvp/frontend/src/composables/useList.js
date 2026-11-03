import { ref } from 'vue'
import { ElMessage } from 'element-plus'
import { errMsg } from '../api'

// 通用列表逻辑：分页固定 10/20/50/200（PRD V1.22）
export function useList(loader, { size = 20, immediate = true, onData } = {}) {
  const rows = ref([])
  const total = ref(0)
  const page = ref(1)
  const pageSize = ref(size)
  const loading = ref(false)

  async function load() {
    loading.value = true
    try {
      const res = await loader({ page: page.value, size: pageSize.value })
      rows.value = res.items || []
      total.value = res.total || 0
      if (onData) onData(res)
    } catch (e) {
      ElMessage.error(errMsg(e))
    } finally {
      loading.value = false
    }
  }

  function change(p, s) {
    page.value = p
    pageSize.value = s
    load()
  }

  if (immediate) load()
  return { rows, total, page, pageSize, loading, load, change }
}
