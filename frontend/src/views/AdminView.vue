<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import {
  adminAuditLogs,
  adminCheckinEvents,
  adminDashboard,
  adminLogin,
  adminSeats,
  adminStudentSeatConflicts,
  adminStudents,
  assignAdminSeat,
  exportBackup,
  getAdminConfig,
  lockAdminSeat,
  planRestore,
  restoreBackup,
  setAdminConfig,
  setAdminStudentBan,
} from '../api'
import type { AdminAuditList, AdminCheckinList, AdminSeat, AdminStudentList, AdminStudentSeatConflict, DashboardStats, Staff } from '../api'

const username = ref(localStorage.getItem('staffUser') || 'admin')
const password = ref('')
const staff = ref<Staff | null>(localStorage.getItem('staffToken') && localStorage.getItem('staffRole') === 'admin' ? {
  username: localStorage.getItem('staffUser') || 'admin',
  role: 'admin',
} : null)
const tab = ref<'dashboard' | 'seats' | 'students' | 'logs' | 'config' | 'backup'>('dashboard')
const dashboard = ref<DashboardStats | null>(null)
const seats = ref<AdminSeat[]>([])
const students = ref<AdminStudentList | null>(null)
const studentConflicts = ref<AdminStudentSeatConflict[]>([])
const audit = ref<AdminAuditList | null>(null)
const checkins = ref<AdminCheckinList | null>(null)
const selectedSeatId = ref<number | null>(null)
const assignValue = ref('')
const customLabel = ref('')
const configKey = ref('play_name')
const configValue = ref('"La La Land"')
const windowConfig = ref({
  countdown_target_at: '',
  booking_open_at: '',
  booking_lock_at: '',
  show_start_at: '',
})
const backupText = ref('')
const restoreToken = ref('')
const restorePlan = ref<any>(null)
const message = ref('')
const error = ref('')
const loading = ref(false)
const studentFilters = ref({
  grade_prefix: '',
  class_code: '',
  search: '',
  registration_status: '',
  booking_status: '',
  account_status: '',
  page: 1,
  page_size: 50,
})

const zoneLabels: Record<string, string> = {
  left: '左區',
  middle: '中區',
  right: '右區',
}

const windowStateLabels: Record<string, string> = {
  before_open: '尚未開放',
  open: '預約開放中',
  locked: '預約已停止',
  show_started: '演出已開始',
}

const zoneOrder = ['left', 'middle', 'right']
const gradeOptions = ['7', '8', '9', '10', '11', '12']
const classOptions = ['701', '702', '703', '704', '705', '801', '802', '803', '804', '805', '901', '902', '903', '904', '905', '1001', '1002', '1003', '1004', '1005', '1101', '1102', '1103', '1104', '1105', '1201', '1202', '1203', '1204', '1205']

const selectedSeat = computed(() => seats.value.find(s => s.seat_id === selectedSeatId.value) || seats.value[0])
const occupiedSeats = computed(() => seats.value.filter(s => s.booking).length)
const currentWindowStateLabel = computed(() => {
  const state = dashboard.value?.window.state
  return state ? windowStateLabels[state] ?? state : '-'
})
const adminZones = computed(() => {
  const byZone = new Map<string, Map<number, AdminSeat[]>>()
  for (const seat of seats.value) {
    if (!byZone.has(seat.zone)) byZone.set(seat.zone, new Map())
    const rows = byZone.get(seat.zone)!
    if (!rows.has(seat.row_no)) rows.set(seat.row_no, [])
    rows.get(seat.row_no)!.push(seat)
  }

  return zoneOrder
    .filter(zone => byZone.has(zone))
    .map(zone => ({
      zone,
      label: zoneLabels[zone] ?? zone,
      rows: Array.from(byZone.get(zone)!.entries())
        .sort(([a], [b]) => a - b)
        .map(([row, zoneSeats]) => ({
          row,
          seats: zoneSeats.sort((a, b) => a.col_no - b.col_no),
        })),
    }))
})

const normalizedStudentFilters = computed(() => ({
  grade_prefix: studentFilters.value.grade_prefix.trim(),
  class_code: studentFilters.value.class_code.trim(),
  search: studentFilters.value.search.trim(),
  registration_status: studentFilters.value.registration_status,
  booking_status: studentFilters.value.booking_status,
  account_status: studentFilters.value.account_status,
  page: studentFilters.value.page,
  page_size: studentFilters.value.page_size,
}))

const studentOverview = computed(() => {
  const rows = students.value?.students || []
  const registered = rows.filter(student => !student.need_register).length
  const banned = rows.filter(student => student.account_banned).length
  return {
    total: students.value?.total ?? 0,
    visible: rows.length,
    registered,
    needRegister: rows.length - registered,
    banned,
    conflicts: studentConflicts.value.length,
  }
})

const studentTotalPages = computed(() => {
  const total = students.value?.total ?? 0
  const pageSize = students.value?.page_size || studentFilters.value.page_size || 50
  return Math.max(1, Math.ceil(total / pageSize))
})

const availableClassOptions = computed(() => {
  const grade = studentFilters.value.grade_prefix
  if (!grade) return classOptions
  return classOptions.filter(classCode => classCode.startsWith(grade))
})

function seatLabel(seat: AdminSeat) {
  return `${zoneLabels[seat.zone] ?? seat.zone} ${seat.row_no}排 ${seat.col_no}號`
}

function bookingLabel(seat: AdminSeat) {
  if (!seat.booking) return seat.is_blocked ? '永久鎖定' : '空位'
  if (seat.booking.custom_label) return seat.booking.custom_label
  return seat.booking.display_name || seat.booking.student_id || '占用'
}

function seatStatusClass(seat: AdminSeat) {
  if (selectedSeatId.value === seat.seat_id) return 'selected'
  if (seat.is_blocked) return 'blocked'
  if (seat.booking?.custom_label) return 'custom'
  if (seat.booking) return 'booked'
  return 'free'
}

function formatDate(value?: string | null) {
  if (!value) return '-'
  return new Date(value).toLocaleString('zh-TW', { hour12: false })
}

function toDateTimeLocal(value: string) {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return ''
  const offset = date.getTimezoneOffset() * 60000
  return new Date(date.getTime() - offset).toISOString().slice(0, 16)
}

function fromDateTimeLocal(value: string) {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) throw new Error('invalid date')
  return date.toISOString()
}

function minutesFromNow(minutes: number) {
  return new Date(Date.now() + minutes * 60000).toISOString()
}

async function login() {
  loading.value = true
  error.value = ''
  try {
    const res = await adminLogin(username.value, password.value)
    localStorage.setItem('staffToken', res.token)
    localStorage.setItem('staffUser', res.staff.username)
    localStorage.setItem('staffRole', res.staff.role)
    staff.value = res.staff
    password.value = ''
    await refreshAll()
  } catch (e: any) {
    error.value = e?.response?.data?.error || '管理員登入失敗'
  } finally {
    loading.value = false
  }
}

function logout() {
  if (!window.confirm('確定要登出管理後台嗎？')) return
  localStorage.removeItem('staffToken')
  localStorage.removeItem('staffUser')
  localStorage.removeItem('staffRole')
  staff.value = null
}

async function refreshAll() {
  if (!staff.value) return
  error.value = ''
  const [dash, seatRows, studentRows, conflictRows, auditRows, checkinRows] = await Promise.all([
    adminDashboard(),
    adminSeats(),
    adminStudents(normalizedStudentFilters.value),
    adminStudentSeatConflicts(normalizedStudentFilters.value),
    adminAuditLogs(),
    adminCheckinEvents(),
  ])
  dashboard.value = dash
  seats.value = seatRows
  students.value = studentRows
  studentConflicts.value = conflictRows
  audit.value = auditRows
  checkins.value = checkinRows
  if (!selectedSeatId.value && seatRows.length) selectedSeatId.value = seatRows[0].seat_id
  await loadWindowConfig()
}

async function refreshStudents() {
  if (!staff.value) return
  loading.value = true
  error.value = ''
  try {
    const [studentRows, conflictRows] = await Promise.all([
      adminStudents(normalizedStudentFilters.value),
      adminStudentSeatConflicts(normalizedStudentFilters.value),
    ])
    students.value = studentRows
    studentConflicts.value = conflictRows
  } catch (e: any) {
    error.value = e?.response?.data?.error || '學生資料載入失敗'
  } finally {
    loading.value = false
  }
}

function resetStudentFilters() {
  studentFilters.value = { grade_prefix: '', class_code: '', search: '', registration_status: '', booking_status: '', account_status: '', page: 1, page_size: 50 }
  refreshStudents()
}

function updateStudentGradeFilter() {
  if (studentFilters.value.class_code && !studentFilters.value.class_code.startsWith(studentFilters.value.grade_prefix)) {
    studentFilters.value.class_code = ''
  }
  studentFilters.value.page = 1
  refreshStudents()
}

function applyStudentFilters() {
  studentFilters.value.page = 1
  refreshStudents()
}

function goStudentPage(page: number) {
  studentFilters.value.page = Math.min(Math.max(page, 1), studentTotalPages.value)
  refreshStudents()
}

async function toggleStudentBan(student: AdminStudentList['students'][number]) {
  const banned = !student.account_banned
  const reason = banned ? window.prompt('請輸入封禁原因', student.ban_reason || '資料異常') : ''
  if (banned && reason === null) return
  if (!banned && !window.confirm(`確定解除 ${student.student_id} 的封禁嗎？`)) return
  loading.value = true
  error.value = ''
  message.value = ''
  try {
    await setAdminStudentBan(student.student_id, { banned, reason: reason || '' })
    message.value = banned ? '帳號已封禁' : '帳號已解封'
    await refreshStudents()
    dashboard.value = await adminDashboard()
  } catch (e: any) {
    error.value = e?.response?.data?.error || '帳號狀態更新失敗'
  } finally {
    loading.value = false
  }
}

async function mutateSeat(kind: 'student' | 'custom' | 'release' | 'lock' | 'unlock') {
  if (!selectedSeat.value) return
  loading.value = true
  error.value = ''
  message.value = ''
  try {
    if (kind === 'student') {
      await assignAdminSeat(selectedSeat.value.seat_id, { student_id: assignValue.value, force_release_existing: true })
    } else if (kind === 'custom') {
      await assignAdminSeat(selectedSeat.value.seat_id, { custom_label: customLabel.value, force_release_existing: true })
    } else if (kind === 'release') {
      await assignAdminSeat(selectedSeat.value.seat_id, { release: true, force_release_existing: true })
    } else {
      await lockAdminSeat(selectedSeat.value.seat_id, { lock: kind === 'lock', block_reason: kind === 'lock' ? 'admin' : null })
    }
    message.value = '座位已更新'
    await refreshAll()
  } catch (e: any) {
    error.value = e?.response?.data?.error || '座位操作失敗'
  } finally {
    loading.value = false
  }
}

async function saveConfig() {
  loading.value = true
  error.value = ''
  message.value = ''
  try {
    const parsed = JSON.parse(configValue.value)
    await setAdminConfig(configKey.value, parsed)
    message.value = '設定已儲存'
  } catch (e: any) {
    error.value = e instanceof SyntaxError ? '設定值必須是合法 JSON，例如 \"文字\" 或 [\"A\"]' : (e?.response?.data?.error || '儲存設定失敗')
  } finally {
    loading.value = false
  }
}

async function loadWindowConfig() {
  const keys = ['countdown_target_at', 'booking_open_at', 'booking_lock_at', 'show_start_at'] as const
  const results = await Promise.allSettled(keys.map(key => getAdminConfig(key)))
  results.forEach((result, index) => {
    if (result.status !== 'fulfilled') return
    const value = result.value?.value
    if (typeof value === 'string') {
      windowConfig.value[keys[index]] = toDateTimeLocal(value)
    }
  })
  if (!windowConfig.value.countdown_target_at && windowConfig.value.booking_open_at) {
    windowConfig.value.countdown_target_at = windowConfig.value.booking_open_at
  }
}

async function saveWindowConfig(next?: Partial<Record<keyof typeof windowConfig.value, string>>) {
  loading.value = true
  error.value = ''
  message.value = ''
  try {
    const values = {
      countdown_target_at: next?.countdown_target_at ?? (windowConfig.value.countdown_target_at
        ? fromDateTimeLocal(windowConfig.value.countdown_target_at)
        : fromDateTimeLocal(windowConfig.value.booking_open_at)),
      booking_open_at: next?.booking_open_at ?? fromDateTimeLocal(windowConfig.value.booking_open_at),
      booking_lock_at: next?.booking_lock_at ?? fromDateTimeLocal(windowConfig.value.booking_lock_at),
      show_start_at: next?.show_start_at ?? fromDateTimeLocal(windowConfig.value.show_start_at),
    }
    await Promise.all([
      setAdminConfig('countdown_target_at', values.countdown_target_at),
      setAdminConfig('booking_open_at', values.booking_open_at),
      setAdminConfig('booking_lock_at', values.booking_lock_at),
      setAdminConfig('show_start_at', values.show_start_at),
    ])
    message.value = '階段已更新'
    await refreshAll()
  } catch (e: any) {
    error.value = e?.message === 'invalid date' ? '請填寫完整且合法的時間' : (e?.response?.data?.error || '更新階段失敗')
  } finally {
    loading.value = false
  }
}

function applyStage(stage: 'before_open' | 'open' | 'locked' | 'show_started') {
  const soon = minutesFromNow(60)
  const openNow = minutesFromNow(-1)
  const openYesterday = minutesFromNow(-60 * 24)
  const lockedNow = minutesFromNow(-1)
  const tomorrow = minutesFromNow(60 * 24)
  const presets = {
    before_open: {
      countdown_target_at: soon,
      booking_open_at: soon,
      booking_lock_at: minutesFromNow(60 * 24 * 30),
      show_start_at: minutesFromNow(60 * 24 * 31),
    },
    open: {
      countdown_target_at: openNow,
      booking_open_at: openNow,
      booking_lock_at: minutesFromNow(60 * 24 * 30),
      show_start_at: minutesFromNow(60 * 24 * 31),
    },
    locked: {
      countdown_target_at: openYesterday,
      booking_open_at: openYesterday,
      booking_lock_at: lockedNow,
      show_start_at: tomorrow,
    },
    show_started: {
      countdown_target_at: minutesFromNow(-60 * 24 * 2),
      booking_open_at: minutesFromNow(-60 * 24 * 2),
      booking_lock_at: minutesFromNow(-60 * 24),
      show_start_at: minutesFromNow(-1),
    },
  }
  saveWindowConfig(presets[stage])
}

async function downloadBackup() {
  loading.value = true
  error.value = ''
  try {
    const snapshot = await exportBackup()
    backupText.value = JSON.stringify(snapshot, null, 2)
    const blob = new Blob([backupText.value], { type: 'application/json' })
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = 'lalaland-backup.json'
    a.click()
    URL.revokeObjectURL(url)
  } catch (e: any) {
    error.value = e?.response?.data?.error || '匯出備份失敗'
  } finally {
    loading.value = false
  }
}

async function makeRestorePlan() {
  loading.value = true
  error.value = ''
  restorePlan.value = null
  try {
    const snapshot = JSON.parse(backupText.value)
    restorePlan.value = await planRestore(snapshot)
    restoreToken.value = restorePlan.value.confirmation_token
  } catch (e: any) {
    error.value = e instanceof SyntaxError ? '備份 JSON 無法解析' : (e?.response?.data?.error || '產生復原計畫失敗')
  } finally {
    loading.value = false
  }
}

async function doRestore() {
  loading.value = true
  error.value = ''
  message.value = ''
  try {
    const snapshot = JSON.parse(backupText.value)
    await restoreBackup(snapshot, restoreToken.value)
    message.value = '復原完成'
    await refreshAll()
  } catch (e: any) {
    error.value = e?.response?.data?.error || '復原失敗'
  } finally {
    loading.value = false
  }
}

onMounted(() => {
  if (staff.value) refreshAll().catch(() => {})
})
</script>

<template>
  <main class="page-shell">
    <section v-if="!staff" class="page-center" style="min-height:70vh">
      <div class="card" style="width:100%;max-width:420px">
        <p class="caption">ADMIN</p>
        <h1 class="h1" style="margin-bottom:24px">後台登入</h1>
        <form @submit.prevent="login">
          <div class="form-group">
            <label>帳號</label>
            <input v-model="username" autocomplete="username" />
          </div>
          <div class="form-group">
            <label>密碼</label>
            <input v-model="password" type="password" autocomplete="current-password" />
          </div>
          <p v-if="error" class="error-msg" style="margin-bottom:14px">{{ error }}</p>
          <button class="btn-primary" style="width:100%" :disabled="loading">{{ loading ? '登入中...' : '進入後台' }}</button>
        </form>
      </div>
    </section>

    <section v-else class="stack">
      <div class="toolbar admin-toolbar">
        <div>
          <p class="caption">ADMIN CONSOLE</p>
          <h1 class="h1">管理後台</h1>
        </div>
        <div class="actions">
          <router-link class="btn-outline" to="/check">進入驗票</router-link>
          <button class="btn-outline" @click="refreshAll">重新整理</button>
          <button class="btn-outline" @click="logout">登出</button>
        </div>
      </div>

      <div class="glass-tabs">
        <button v-for="item in ['dashboard','seats','students','logs','config','backup']" :key="item" class="glass-tab" :class="{ active: tab === item }" @click="tab = item as any">
          {{ ({ dashboard:'Dashboard', seats:'座位', students:'學生', logs:'紀錄', config:'設定', backup:'備份' } as any)[item] }}
        </button>
      </div>

      <p v-if="message" class="status-pill good">{{ message }}</p>
      <p v-if="error" class="error-msg">{{ error }}</p>

      <section v-if="tab === 'dashboard'" class="stack">
        <div class="panel-grid">
          <div class="metric"><span class="caption">座位總數</span><strong class="metric-value">{{ dashboard?.seats.total ?? '-' }}</strong></div>
          <div class="metric"><span class="caption">已占用</span><strong class="metric-value">{{ dashboard?.seats.booked ?? occupiedSeats }}</strong></div>
          <div class="metric"><span class="caption">已入場</span><strong class="metric-value">{{ dashboard?.checkins.approved ?? '-' }}</strong></div>
          <div class="metric"><span class="caption">窗口</span><strong class="metric-value">{{ dashboard?.window.state ?? '-' }}</strong></div>
        </div>
        <div class="split">
          <div class="card">
            <h2 class="h2" style="margin-bottom:16px">最近稽核</h2>
            <div class="table-wrap">
              <table>
                <thead><tr><th>ID</th><th>動作</th><th>操作者</th><th>時間</th></tr></thead>
                <tbody>
                  <tr v-for="log in dashboard?.recent.audit_logs || []" :key="log.id">
                    <td>{{ log.id }}</td><td>{{ log.action }}</td><td>{{ log.actor_id || log.actor_type }}</td><td>{{ new Date(log.created_at).toLocaleString('zh-TW', { hour12:false }) }}</td>
                  </tr>
                </tbody>
              </table>
            </div>
          </div>
          <div class="card">
            <h2 class="h2" style="margin-bottom:16px">入場動態</h2>
            <div v-for="event in dashboard?.recent.checkin_events || []" :key="event.id" style="padding:10px 0;border-bottom:1px solid var(--border)">
              <span class="status-pill" :class="event.is_first_scan ? 'good' : 'warn'">{{ event.method }}</span>
              <p style="margin-top:8px">{{ event.student_id || '-' }} · {{ new Date(event.scanned_at).toLocaleString('zh-TW', { hour12:false }) }}</p>
            </div>
          </div>
        </div>
      </section>

      <section v-if="tab === 'seats'" class="split">
        <div class="card">
          <div class="seat-map-head">
            <div>
              <p class="caption">SEAT MAP</p>
              <h2 class="h2">座位圖</h2>
            </div>
            <div class="admin-legend">
              <span><i class="legend-dot free"></i>空位</span>
              <span><i class="legend-dot booked"></i>已訂</span>
              <span><i class="legend-dot blocked"></i>鎖定</span>
            </div>
          </div>
          <div class="admin-stage">
            <span>舞台</span>
          </div>
          <div class="admin-seat-map" aria-label="管理後台左中右三區座位圖">
            <section v-for="zone in adminZones" :key="zone.zone" class="admin-zone" :class="`admin-zone--${zone.zone}`">
              <header class="admin-zone-header">
                <span>{{ zone.label }}</span>
              </header>
              <div class="admin-zone-rows">
                <div v-for="{ row, seats: rowSeats } in zone.rows" :key="`${zone.zone}-${row}`" class="admin-row">
                  <span class="admin-row-label">{{ row }}</span>
                  <div class="admin-seat-line">
                    <button
                      v-for="seat in rowSeats"
                      :key="seat.seat_id"
                      type="button"
                      class="admin-seat"
                      :class="seatStatusClass(seat)"
                      :title="`${seatLabel(seat)} · ${bookingLabel(seat)}`"
                      :aria-label="`${seatLabel(seat)} · ${bookingLabel(seat)}`"
                      @click="selectedSeatId = seat.seat_id"
                    >
                      <span v-if="selectedSeatId === seat.seat_id"></span>
                    </button>
                  </div>
                </div>
              </div>
            </section>
          </div>
        </div>
        <div class="card stack" v-if="selectedSeat">
          <div>
            <p class="caption">SEAT #{{ selectedSeat.seat_id }}</p>
            <h2 class="h2">{{ seatLabel(selectedSeat) }}</h2>
            <p class="muted">{{ bookingLabel(selectedSeat) }}</p>
          </div>
          <div class="seat-detail-list">
            <div>
              <span class="caption">狀態</span>
              <strong>{{ selectedSeat.is_blocked ? '永久鎖定' : (selectedSeat.booking ? '已占用' : '空位') }}</strong>
            </div>
            <div v-if="selectedSeat.booking?.display_name">
              <span class="caption">姓名</span>
              <strong>{{ selectedSeat.booking.display_name }}</strong>
            </div>
            <div v-if="selectedSeat.booking?.student_id">
              <span class="caption">學號</span>
              <strong>{{ selectedSeat.booking.student_id }}</strong>
            </div>
            <div v-if="selectedSeat.booking?.class_code">
              <span class="caption">班級</span>
              <strong>{{ selectedSeat.booking.class_code }}</strong>
            </div>
            <div v-if="selectedSeat.booking?.seat_no_in_class">
              <span class="caption">座號</span>
              <strong>{{ selectedSeat.booking.seat_no_in_class }}</strong>
            </div>
            <div v-if="selectedSeat.booking?.booked_by_student_id && selectedSeat.booking.booked_by_student_id !== selectedSeat.booking.student_id">
              <span class="caption">代約人</span>
              <strong>{{ selectedSeat.booking.booked_by_student_id }}</strong>
            </div>
            <div v-if="selectedSeat.booking?.custom_label">
              <span class="caption">自訂標籤</span>
              <strong>{{ selectedSeat.booking.custom_label }}</strong>
            </div>
            <div v-if="selectedSeat.booking?.booked_at">
              <span class="caption">預約時間</span>
              <strong>{{ formatDate(selectedSeat.booking.booked_at) }}</strong>
            </div>
          </div>
          <input v-model="assignValue" inputmode="numeric" maxlength="7" pattern="11[0-5][1-4][0-9]{3}" placeholder="綁定學號 1121001" />
          <button class="btn-primary" :disabled="loading" @click="mutateSeat('student')">強制綁定學號</button>
          <input v-model="customLabel" placeholder="自訂占用標籤" />
          <button class="btn-outline" :disabled="loading" @click="mutateSeat('custom')">自訂占用</button>
          <div class="actions">
            <button class="btn-outline" :disabled="loading" @click="mutateSeat('release')">釋放座位</button>
            <button class="btn-outline" :disabled="loading" @click="mutateSeat(selectedSeat.is_blocked ? 'unlock' : 'lock')">
              {{ selectedSeat.is_blocked ? '解除鎖定' : '永久鎖定' }}
            </button>
          </div>
        </div>
      </section>

      <section v-if="tab === 'students'" class="card stack">
        <div class="toolbar admin-toolbar">
          <div>
            <p class="caption">STUDENT DIRECTORY</p>
            <h2 class="h2">學生名單</h2>
          </div>
          <button class="btn-outline" :disabled="loading" @click="refreshStudents">重新整理</button>
        </div>
        <div class="student-filter-bar">
          <label>
            <span>年級</span>
            <select v-model="studentFilters.grade_prefix" @change="updateStudentGradeFilter">
              <option value="">全部</option>
              <option v-for="grade in gradeOptions" :key="grade" :value="grade">{{ grade }}</option>
            </select>
          </label>
          <label>
            <span>班級</span>
            <select v-model="studentFilters.class_code" @change="refreshStudents">
              <option value="">全部</option>
              <option v-for="classCode in availableClassOptions" :key="classCode" :value="classCode">{{ classCode }}</option>
            </select>
          </label>
          <label>
            <span>搜尋</span>
            <input v-model="studentFilters.search" placeholder="學號或姓名" @keyup.enter="refreshStudents" />
          </label>
          <label>
            <span>註冊狀態</span>
            <select v-model="studentFilters.registration_status" @change="applyStudentFilters">
              <option value="">全部</option>
              <option value="registered">已註冊</option>
              <option value="need_register">待補全</option>
            </select>
          </label>
          <label>
            <span>訂座狀態</span>
            <select v-model="studentFilters.booking_status" @change="applyStudentFilters">
              <option value="">全部</option>
              <option value="booked">已訂座</option>
              <option value="unbooked">未訂座</option>
            </select>
          </label>
          <label>
            <span>帳號狀態</span>
            <select v-model="studentFilters.account_status" @change="applyStudentFilters">
              <option value="">全部</option>
              <option value="active">正常</option>
              <option value="banned">已封禁</option>
            </select>
          </label>
          <label>
            <span>每頁</span>
            <select v-model.number="studentFilters.page_size" @change="applyStudentFilters">
              <option :value="50">50</option>
              <option :value="100">100</option>
            </select>
          </label>
          <div class="filter-actions">
            <button class="btn-primary" :disabled="loading" @click="applyStudentFilters">篩選</button>
            <button class="btn-outline" :disabled="loading" @click="resetStudentFilters">清除</button>
          </div>
        </div>
        <div class="panel-grid student-overview">
          <div class="metric"><span class="caption">符合筆數</span><strong class="metric-value">{{ studentOverview.total }}</strong></div>
          <div class="metric"><span class="caption">本頁顯示</span><strong class="metric-value">{{ studentOverview.visible }}</strong></div>
          <div class="metric"><span class="caption">已註冊</span><strong class="metric-value">{{ studentOverview.registered }}</strong></div>
          <div class="metric"><span class="caption">本頁封禁</span><strong class="metric-value">{{ studentOverview.banned }}</strong></div>
          <div class="metric"><span class="caption">異常組</span><strong class="metric-value">{{ studentOverview.conflicts }}</strong></div>
        </div>
        <div class="conflict-panel">
          <div class="toolbar admin-toolbar">
            <div>
              <p class="caption">DUPLICATE CHECK</p>
              <h3>班級座號重複</h3>
            </div>
            <span class="status-pill" :class="studentConflicts.length ? 'warn' : 'good'">
              {{ studentConflicts.length ? `${studentConflicts.length} 組異常` : '無異常' }}
            </span>
          </div>
          <div v-if="studentConflicts.length" class="table-wrap">
            <table>
              <thead><tr><th>班級</th><th>座號</th><th>帳號 ID / 學生證號</th></tr></thead>
              <tbody>
                <tr v-for="conflict in studentConflicts" :key="`${conflict.class_code}-${conflict.seat_no_in_class}`">
                  <td>{{ conflict.class_code }}</td>
                  <td>{{ conflict.seat_no_in_class }}</td>
                  <td>{{ conflict.student_ids.join('、') }}</td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>
        <div class="table-wrap">
          <table>
            <thead><tr><th>學號</th><th>班級</th><th>座號</th><th>註冊</th><th>帳號</th><th>座位</th><th>操作</th></tr></thead>
            <tbody>
              <tr v-for="student in students?.students || []" :key="student.student_id">
                <td>{{ student.student_id }}</td>
                <td>{{ student.class_code || '-' }}</td>
                <td>{{ student.seat_no_in_class || '-' }}</td>
                <td><span class="status-pill" :class="student.need_register ? 'warn' : 'good'">{{ student.need_register ? '待補全' : '已註冊' }}</span></td>
                <td>
                  <span class="status-pill" :class="student.account_banned ? 'warn' : 'good'">
                    {{ student.account_banned ? '已封禁' : '正常' }}
                  </span>
                  <small v-if="student.ban_reason" class="ban-reason">{{ student.ban_reason }}</small>
                </td>
                <td>{{ student.seat_id || '-' }}</td>
                <td>
                  <button class="table-action" :class="{ danger: !student.account_banned }" :disabled="loading" @click="toggleStudentBan(student)">
                    {{ student.account_banned ? '解封' : '封禁' }}
                  </button>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
        <div class="student-pagination">
          <button class="btn-outline" :disabled="loading || studentFilters.page <= 1" @click="goStudentPage(studentFilters.page - 1)">上一頁</button>
          <span>第 {{ students?.page || studentFilters.page }} / {{ studentTotalPages }} 頁</span>
          <button class="btn-outline" :disabled="loading || studentFilters.page >= studentTotalPages" @click="goStudentPage(studentFilters.page + 1)">下一頁</button>
        </div>
      </section>

      <section v-if="tab === 'logs'" class="split">
        <div class="card">
          <h2 class="h2" style="margin-bottom:16px">稽核紀錄</h2>
          <div class="table-wrap">
            <table>
              <thead><tr><th>ID</th><th>動作</th><th>目標</th><th>時間</th></tr></thead>
              <tbody>
                <tr v-for="log in audit?.logs || []" :key="log.id">
                  <td>{{ log.id }}</td><td>{{ log.action }}</td><td>{{ log.target_student || log.target_seat_id || '-' }}</td><td>{{ new Date(log.created_at).toLocaleString('zh-TW', { hour12:false }) }}</td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>
        <div class="card">
          <h2 class="h2" style="margin-bottom:16px">入場事件</h2>
          <div class="table-wrap">
            <table>
              <thead><tr><th>ID</th><th>學號</th><th>方式</th><th>首次</th><th>時間</th></tr></thead>
              <tbody>
                <tr v-for="event in checkins?.events || []" :key="event.id">
                  <td>{{ event.id }}</td><td>{{ event.student_id || '-' }}</td><td>{{ event.method }}</td><td>{{ event.is_first_scan ? '是' : '否' }}</td><td>{{ new Date(event.scanned_at).toLocaleString('zh-TW', { hour12:false }) }}</td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>
      </section>

      <section v-if="tab === 'config'" class="card stack">
        <h2 class="h2">系統設定</h2>
        <div class="stage-control">
          <div>
            <p class="caption">STAGE CONTROL</p>
            <h3>階段調整</h3>
            <p class="muted">目前階段：{{ currentWindowStateLabel }}</p>
          </div>
          <div class="stage-buttons">
            <button class="btn-outline" :disabled="loading" @click="applyStage('before_open')">尚未開放</button>
            <button class="btn-primary" :disabled="loading" @click="applyStage('open')">立即開放預約</button>
            <button class="btn-outline" :disabled="loading" @click="applyStage('locked')">停止預約</button>
            <button class="btn-outline" :disabled="loading" @click="applyStage('show_started')">演出開始</button>
          </div>
        </div>
        <div class="split">
          <div>
            <label>首頁倒數目標</label>
            <input v-model="windowConfig.countdown_target_at" type="datetime-local" />
          </div>
          <div>
            <label>開放預約時間</label>
            <input v-model="windowConfig.booking_open_at" type="datetime-local" />
          </div>
        </div>
        <div class="split">
          <div>
            <label>停止預約 / 改位時間</label>
            <input v-model="windowConfig.booking_lock_at" type="datetime-local" />
          </div>
        </div>
        <div class="split">
          <div>
            <label>演出開始時間</label>
            <input v-model="windowConfig.show_start_at" type="datetime-local" />
          </div>
          <div class="config-action">
            <button class="btn-primary" :disabled="loading" @click="saveWindowConfig()">儲存階段時間</button>
          </div>
        </div>
        <hr class="soft-divider" />
        <div class="split">
          <div>
            <label>設定 Key</label>
            <input v-model="configKey" placeholder="play_name" />
          </div>
          <div>
            <label>設定值 JSON</label>
            <input v-model="configValue" placeholder="&quot;La La Land&quot;" />
          </div>
        </div>
        <button class="btn-primary" style="width:max-content" :disabled="loading" @click="saveConfig">儲存設定</button>
      </section>

      <section v-if="tab === 'backup'" class="card stack">
        <div class="toolbar admin-toolbar">
          <div>
            <h2 class="h2">備份 / 復原</h2>
            <p class="muted">復原會覆蓋現有資料，先產生計畫確認 token 再執行。</p>
          </div>
          <button class="btn-primary" :disabled="loading" @click="downloadBackup">匯出備份</button>
        </div>
        <textarea v-model="backupText" placeholder="貼上 lalaland-backup.json 內容，或點擊匯出備份後自動填入"></textarea>
        <div class="actions">
          <button class="btn-outline" :disabled="loading || !backupText" @click="makeRestorePlan">產生復原計畫</button>
          <button class="btn-outline" :disabled="loading || !restoreToken" @click="doRestore">執行復原</button>
        </div>
        <div v-if="restorePlan" class="metric">
          <span class="caption">CONFIRM TOKEN</span>
          <strong class="metric-value" style="font-size:18px;word-break:break-all">{{ restorePlan.confirmation_token }}</strong>
        </div>
      </section>
    </section>
  </main>
</template>

<style scoped>
.glass-tabs {
  display: flex;
  background: rgba(0, 0, 0, 0.2);
  border-radius: 999px;
  padding: 6px;
  gap: 4px;
  margin-bottom: 24px;
  overflow-x: auto;
  box-shadow: inset 0 2px 10px rgba(0, 0, 0, 0.2);
}
.glass-tab {
  flex: 1;
  min-width: 80px;
  padding: 10px 16px;
  border-radius: 999px;
  border: none;
  background: transparent;
  color: var(--muted);
  font-weight: 600;
  font-size: 14px;
  cursor: pointer;
  transition: all 0.2s ease;
}
.glass-tab:hover {
  color: var(--cream);
  background: rgba(255, 255, 255, 0.05);
}
.glass-tab.active {
  background: var(--surface-glass);
  color: var(--gold);
  box-shadow: 0 4px 15px rgba(0, 0, 0, 0.2), inset 0 1px 0 rgba(255, 255, 255, 0.1);
}

.admin-toolbar {
  justify-content: space-between;
}

.seat-map-head {
  display: flex;
  align-items: end;
  justify-content: space-between;
  gap: 16px;
  margin-bottom: 18px;
}

.admin-legend {
  display: flex;
  flex-wrap: wrap;
  justify-content: flex-end;
  gap: 10px;
  color: var(--muted);
  font-size: 12px;
}

.admin-legend span {
  display: inline-flex;
  align-items: center;
  gap: 6px;
}

.legend-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  display: inline-block;
}

.legend-dot.free { background: rgba(255, 255, 255, 0.86); }
.legend-dot.booked { background: var(--gold); }
.legend-dot.blocked { background: rgba(252, 165, 165, 0.72); }

.admin-stage {
  width: min(360px, 64%);
  margin: 0 auto 30px;
  padding: 8px 16px;
  text-align: center;
  color: rgba(255, 243, 215, 0.82);
  border-bottom: 2px solid rgba(255, 216, 95, 0.38);
  background: linear-gradient(180deg, transparent, rgba(255, 216, 95, 0.08));
  border-radius: 50% 50% 0 0 / 100% 100% 0 0;
  font-size: 12px;
  letter-spacing: 0.2em;
}

.admin-seat-map {
  display: grid;
  grid-template-columns: max-content max-content max-content;
  justify-content: center;
  align-items: start;
  gap: clamp(12px, 2vw, 32px);
  max-height: 70vh;
  overflow: auto;
  padding: 8px 12px 20px;
}

.admin-zone-header {
  height: 24px;
  margin-bottom: 12px;
  text-align: center;
  color: var(--muted);
  font-size: 12px;
  letter-spacing: 0.12em;
}

.admin-zone-rows {
  display: flex;
  flex-direction: column;
  gap: 7px;
}

.admin-row {
  display: flex;
  align-items: center;
  gap: 6px;
}

.admin-row-label {
  width: 20px;
  text-align: right;
  color: rgba(255, 255, 255, 0.36);
  font-size: 11px;
}

.admin-seat-line {
  display: flex;
  gap: 6px;
}

.admin-zone--left .admin-seat-line { justify-content: flex-end; }
.admin-zone--middle .admin-seat-line { justify-content: center; }
.admin-zone--right .admin-seat-line { justify-content: flex-start; }

.admin-seat {
  width: 20px;
  height: 20px;
  border: 0;
  border-radius: 50%;
  cursor: pointer;
  transition: transform 160ms ease, box-shadow 160ms ease, background 160ms ease;
}

.admin-seat.free {
  background: rgba(255, 255, 255, 0.84);
  box-shadow: 0 3px 8px rgba(0, 0, 0, 0.22);
}

.admin-seat.booked {
  background: var(--gold);
  box-shadow: 0 0 0 1px rgba(255, 255, 255, 0.28), 0 0 12px rgba(246, 178, 63, 0.3);
}

.admin-seat.custom {
  background: #b06cff;
  box-shadow: 0 0 0 1px rgba(255, 255, 255, 0.24), 0 0 12px rgba(176, 108, 255, 0.25);
}

.admin-seat.blocked {
  background: rgba(252, 165, 165, 0.72);
  box-shadow: inset 0 0 0 1px rgba(255, 255, 255, 0.14);
}

.admin-seat.selected {
  background: var(--cream);
  transform: scale(1.42);
  box-shadow: 0 0 0 2px var(--gold), 0 0 18px rgba(255, 216, 95, 0.65);
}

.admin-seat:hover {
  transform: scale(1.24);
}

.seat-detail-list {
  display: grid;
  gap: 10px;
  padding: 12px;
  background: rgba(0, 0, 0, 0.16);
  border-radius: var(--radius-md);
  box-shadow: inset 0 0 0 1px rgba(255, 255, 255, 0.06);
}

.seat-detail-list div {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 14px;
  min-height: 28px;
}

.seat-detail-list strong {
  color: var(--cream);
  text-align: right;
}

.student-filter-bar {
  display: grid;
  grid-template-columns: repeat(4, minmax(120px, 1fr)) auto;
  gap: 12px;
  align-items: end;
  padding: 14px;
  border-radius: var(--radius-md);
  background: rgba(0, 0, 0, 0.14);
  box-shadow: inset 0 0 0 1px rgba(255, 255, 255, 0.06);
}

.student-filter-bar label {
  display: grid;
  gap: 6px;
  margin: 0;
}

.student-filter-bar span {
  color: var(--muted);
  font-size: 12px;
}

.student-filter-bar select {
  width: 100%;
  min-height: 42px;
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
  background: rgba(8, 16, 39, 0.78);
  color: var(--cream);
  padding: 0 12px;
}

.filter-actions {
  display: flex;
  gap: 10px;
}

.ban-reason {
  display: block;
  margin-top: 6px;
  color: var(--muted);
  font-size: 12px;
}

.table-action {
  min-width: 58px;
  min-height: 34px;
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
  background: rgba(255, 255, 255, 0.08);
  color: var(--cream);
  cursor: pointer;
}

.table-action.danger {
  border-color: rgba(252, 165, 165, 0.5);
  color: #fecaca;
}

.student-pagination {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 12px;
  color: var(--muted);
}

.student-overview {
  margin: 0;
}

.conflict-panel {
  padding: 14px;
  border-radius: var(--radius-md);
  background: rgba(0, 0, 0, 0.14);
  box-shadow: inset 0 0 0 1px rgba(255, 255, 255, 0.06);
}

.conflict-panel h3 {
  margin: 4px 0 0;
  color: var(--cream);
  font-size: 18px;
}

.stage-control {
  display: flex;
  align-items: flex-end;
  justify-content: space-between;
  gap: 18px;
  padding: 16px;
  border-radius: var(--radius-md);
  background: rgba(0, 0, 0, 0.14);
  box-shadow: inset 0 0 0 1px rgba(255, 255, 255, 0.06);
}

.stage-control h3 {
  margin: 4px 0 8px;
  color: var(--cream);
  font-size: 20px;
}

.stage-buttons {
  display: flex;
  flex-wrap: wrap;
  justify-content: flex-end;
  gap: 10px;
}

.config-action {
  display: flex;
  align-items: end;
}

.soft-divider {
  width: 100%;
  border: 0;
  border-top: 1px solid var(--border);
}

@media (max-width: 860px) {
  .glass-tabs {
    overflow-x: auto;
    -webkit-overflow-scrolling: touch;
    scrollbar-width: none;
  }
  .glass-tabs::-webkit-scrollbar { display: none; }
  .glass-tab {
    flex: 0 0 auto;
    min-width: 72px;
    padding: 9px 14px;
    font-size: 13px;
  }

  .seat-map-head {
    align-items: flex-start;
    flex-direction: column;
  }

  .admin-legend {
    justify-content: flex-start;
  }

  .admin-seat-map {
    justify-content: start;
    gap: 12px;
  }

  .admin-seat {
    width: 18px;
    height: 18px;
  }

  .admin-seat-line {
    gap: 4px;
  }

  .admin-stage {
    width: min(280px, 80%);
    margin-bottom: 20px;
  }

  .stage-control {
    align-items: flex-start;
    flex-direction: column;
  }

  .stage-buttons {
    justify-content: flex-start;
  }

  .student-filter-bar {
    grid-template-columns: 1fr 1fr;
  }

  .filter-actions {
    grid-column: 1 / -1;
  }

  .student-pagination {
    justify-content: center;
  }
}

@media (max-width: 640px) {
  .glass-tab {
    min-width: 64px;
    padding: 8px 12px;
    font-size: 12px;
  }

  .admin-seat-map {
    grid-template-columns: 1fr;
    gap: 20px;
    max-height: none;
    padding: 4px 8px 16px;
  }

  .admin-zone-header {
    text-align: left;
    margin-bottom: 8px;
    font-size: 13px;
    font-weight: 600;
    color: var(--gold-light);
    letter-spacing: 0.15em;
  }

  .admin-seat {
    width: 22px;
    height: 22px;
  }

  .admin-seat-line {
    gap: 5px;
    flex-wrap: wrap;
  }

  .admin-stage {
    width: 100%;
    border-radius: 0;
    margin-bottom: 16px;
  }

  .seat-detail-list div {
    flex-direction: column;
    align-items: flex-start;
    gap: 4px;
  }

  .seat-detail-list strong {
    text-align: left;
  }

  .student-filter-bar {
    grid-template-columns: 1fr;
  }

  .filter-actions {
    flex-direction: column;
  }
}

@media (max-width: 480px) {
  .glass-tabs {
    border-radius: var(--radius-lg);
    padding: 4px;
    gap: 2px;
  }

  .glass-tab {
    min-width: 56px;
    padding: 7px 10px;
    font-size: 11px;
    font-weight: 700;
  }

  .admin-seat {
    width: 20px;
    height: 20px;
  }

  .admin-row-label {
    width: 16px;
    font-size: 10px;
  }

  .admin-zone-rows {
    gap: 5px;
  }
}

</style>
