import { ref, computed } from 'vue'

// 中英双语（DZ-04），字典化，禁止硬编码文案散落
const dict = {
  zh: {
    app: 'BOE US RMA 收发管理系统',
    login: '登录', logout: '退出', username: '账号', password: '密码',
    menu: {
      forecast: '预报管理', receiving: '物流允收', kitting: 'Kitting 扫码',
      discrepancy: '差异审核', inspection: '质检判定', outbound: '出库管理',
      packing: '打包发运', returnBoard: '还货看板', buffer: 'BUFFER 库存',
      reports: '查询报表', master: '主数据', admin: '用户与角色', mobile: '移动端扫码'
    },
    common: {
      search: '查询', reset: '重置', add: '新增', submit: '提交', cancel: '取消',
      confirm: '确认', reject: '驳回', approve: '通过', detail: '详情', action: '操作',
      total: '共 {n} 条', status: '状态', sn: 'SN', pn: '料号', rma: 'RMA 号', customer: '客户',
      qty: '数量', createdAt: '创建时间', save: '保存', print: '打印', ok: '操作成功',
      loading: '加载中', noData: '暂无数据', offline: '网络已断开，禁止提交',
      export: '导出', import: '导入', weight: '重量(kg)', volume: '材积(m³)'
    }
  },
  en: {
    app: 'BOE US RMA Management System',
    login: 'Sign in', logout: 'Sign out', username: 'Username', password: 'Password',
    menu: {
      forecast: 'Forecast', receiving: 'Receiving', kitting: 'Kitting Scan',
      discrepancy: 'Discrepancy', inspection: 'Inspection', outbound: 'Outbound',
      packing: 'Packing & Ship', returnBoard: 'Return Board', buffer: 'Buffer Stock',
      reports: 'Reports', master: 'Master Data', admin: 'Users & Roles', mobile: 'Mobile Scan'
    },
    common: {
      search: 'Search', reset: 'Reset', add: 'Add', submit: 'Submit', cancel: 'Cancel',
      confirm: 'Confirm', reject: 'Reject', approve: 'Approve', detail: 'Detail', action: 'Action',
      total: '{n} items', status: 'Status', sn: 'SN', pn: 'PN', rma: 'RMA No.', customer: 'Customer',
      qty: 'Qty', createdAt: 'Created', save: 'Save', print: 'Print', ok: 'Done',
      loading: 'Loading', noData: 'No data', offline: 'Network offline, submission blocked',
      export: 'Export', import: 'Import', weight: 'Weight(kg)', volume: 'Volume(m³)'
    }
  }
}

const locale = ref(localStorage.getItem('locale') || 'zh')

export function setLocale(v) {
  locale.value = v
  localStorage.setItem('locale', v)
}

export function useI18n() {
  const t = (path, params) => {
    const seg = path.split('.')
    let cur = dict[locale.value]
    for (const s of seg) { cur = cur?.[s] }
    if (typeof cur !== 'string') return path
    return params ? cur.replace(/\{(\w+)\}/g, (_, k) => params[k] ?? '') : cur
  }
  return { t, locale, setLocale, isZh: computed(() => locale.value === 'zh') }
}
