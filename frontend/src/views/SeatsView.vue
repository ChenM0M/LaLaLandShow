<script setup lang="ts">
import { ref, onMounted, onUnmounted, computed, nextTick } from 'vue'
import { useRouter } from 'vue-router'
import { useSeatsStore } from '../stores/seats'
import { useAuthStore } from '../stores/auth'
import type { Seat } from '../api'

const router = useRouter()
const seats = useSeatsStore()
const auth = useAuthStore()

const error = ref('')
const loading = ref(false)
const loadingLabel = ref('')
const rulesAccepted = ref(localStorage.getItem('seatRulesAccepted') === 'true')
const selectedSeatSlots = ref<Array<Seat | null>>([null, null, null])
const sheetExpanded = ref(true)
const proxyStudentIds = ref<Record<number, string>>({})
const managementMode = ref<'changeOwn' | 'replaceGroup'>('changeOwn')
const notice = ref<{
  tone: 'info' | 'success' | 'warning' | 'danger'
  title: string
  message: string
  primaryLabel: string
  secondaryLabel?: string
} | null>(null)

const zoom = ref(0.92)
const panX = ref(0)
const panY = ref(0)
const initialZoom = ref(0.92)
const initialPanX = ref(0)
const initialPanY = ref(0)
const isPanning = ref(false)
const hasDragged = ref(false)
const panStart = ref({ x: 0, y: 0, panX: 0, panY: 0 })
const activePointers = new Map<number, { x: number; y: number }>()
const activePointerCount = ref(0)
const initialPinchDist = ref(0)
const initialPinchZoom = ref(1)
const initialPinchCenter = ref({ screenX: 0, screenY: 0, contentX: 0, contentY: 0 })
const lastPointerType = ref('')
const mapMetrics = ref({
  left: 0,
  top: 0,
  width: 100,
  height: 100,
  mapWidth: 100,
  mapHeight: 100,
})
let centerTimer: number | undefined
let metricFrame: number | undefined
let transformFrame: number | undefined
let queuedTransform: { zoom: number; panX: number; panY: number } | null = null

const zoneLabels: Record<string, string> = {
  left: '左區',
  middle: '中區',
  right: '右區',
}

const zoneOrder = ['left', 'middle', 'right']

const zones = computed(() => {
  const byZone = new Map<string, Map<number, Seat[]>>()
  for (const seat of seats.seats) {
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
      count: Array.from(byZone.get(zone)!.values()).reduce((sum, row) => sum + row.length, 0),
      rows: Array.from(byZone.get(zone)!.entries())
        .sort(([a], [b]) => a - b)
        .map(([row, rowSeats]) => ({
          row,
          seats: [...rowSeats].sort((a, b) => a.col_no - b.col_no),
        })),
    }))
})

const seatStats = computed(() => {
  const total = seats.seats.length
  const free = seats.seats.filter(seat => seat.state === 'free').length
  const occupied = seats.seats.filter(seat => seat.state === 'others').length
  const proxy = seats.seats.filter(seat => seat.state === 'proxy').length
  const locked = seats.seats.filter(seat => seat.state === 'locked').length
  return { total, free, occupied, proxy, locked }
})

const myBooking = computed(() => seats.seats.find(seat => seat.state === 'mine'))
const proxyBookings = computed(() => seats.seats
  .filter(seat => seat.state === 'proxy')
  .sort((a, b) => a.row_no - b.row_no || a.col_no - b.col_no))
const reservedSeats = computed(() => [
  ...(myBooking.value ? [myBooking.value] : []),
  ...proxyBookings.value,
])
const hasManagedBookings = computed(() => reservedSeats.value.length > 0)
const isReplacingGroup = computed(() => hasManagedBookings.value && (!myBooking.value || managementMode.value === 'replaceGroup'))
const isChangingOwnSeat = computed(() => Boolean(myBooking.value) && !isReplacingGroup.value)
const selectedSeats = computed(() => selectedSeatSlots.value.filter((seat): seat is Seat => seat !== null))
const selectedSeatIds = computed(() => new Set(selectedSeats.value.map(seat => seat.seat_id)))
const showMiniMap = computed(() => zoom.value > initialZoom.value + 0.16)
const minimapViewportStyle = computed(() => ({
  left: `${Math.max(0, Math.min(92, (mapMetrics.value.left / mapMetrics.value.mapWidth) * 100))}%`,
  top: `${Math.max(0, Math.min(88, (mapMetrics.value.top / mapMetrics.value.mapHeight) * 100))}%`,
  width: `${Math.max(8, Math.min(100, (mapMetrics.value.width / mapMetrics.value.mapWidth) * 100))}%`,
  height: `${Math.max(12, Math.min(100, (mapMetrics.value.height / mapMetrics.value.mapHeight) * 100))}%`,
}))

const canSubmit = computed(() => {
  if (!auth.hasCompleteProfile) return false
  if (loading.value || seats.windowState !== 'open' || selectedSeats.value.length === 0) return false
  if (!selectedSeatSlots.value[0] || (selectedSeatSlots.value[2] && !selectedSeatSlots.value[1])) return false
  if (isChangingOwnSeat.value) return selectedSeats.value.length === 1
  if (selectedSeats.value.length === 1) return true
  return selectedSeatSlots.value
    .slice(1)
    .filter((seat): seat is Seat => seat !== null)
    .every(seat => /^[0-9]{7}$/.test(proxyStudentIds.value[seat.seat_id] || ''))
})

const windowBanner: Record<string, { label: string; detail: string; tone: string }> = {
  before_open: { label: '尚未開放', detail: '座位圖可預覽，暫不能選座', tone: 'muted' },
  open: { label: '可選座', detail: '點選白色座位加入清單', tone: 'live' },
  locked: { label: '已截止', detail: '已預約者仍可取消座位', tone: 'warn' },
  show_started: { label: '演出中', detail: '座位已凍結', tone: 'closed' },
}

const seatHint = computed(() => {
  if (seats.windowState === 'before_open') return '預約尚未開放，先熟悉座位區域與視角。'
  if (seats.windowState === 'locked') return '預約已截止；若已選座，可在上方取消目前座位。'
  if (seats.windowState === 'show_started') return '演出已開始，座位圖僅供查看。'
  if (isReplacingGroup.value && selectedSeats.value.length === 0) return '重選全部會替換本人與代約座位；先點好新座位，再確認。'
  if (isReplacingGroup.value) return '第 1 個座位為本人，第 2、3 個座位填代約學號。'
  if (myBooking.value && selectedSeats.value.length === 0) return '目前是換本人座位；點選新座位後再確認。'
  if (myBooking.value) return '確認後只更換本人座位，代約座位保持不變。'
  if (selectedSeats.value.length === 0) return '最多可選 3 座；第 2、3 座需填寫代約學號。'
  return '確認前仍可再次點擊座位取消選取。'
})

const selectionTitle = computed(() => {
  if (selectedSeats.value.length > 0) {
    if (isChangingOwnSeat.value) return '待確認換座'
    if (isReplacingGroup.value) return `重選 ${selectedSeats.value.length}/3`
    return `已選 ${selectedSeats.value.length}/3`
  }
  if (hasManagedBookings.value) return '目前座位'
  return '選座清單'
})

const submitLabel = computed(() => {
  if (loading.value) return loadingLabel.value || '處理中...'
  if (isChangingOwnSeat.value) return '確認換本人座位'
  if (isReplacingGroup.value) return '確認重選全部'
  return '確認預約'
})

const noticeIcon = computed(() => {
  if (!notice.value) return ''
  if (notice.value.tone === 'success') return '✓'
  if (notice.value.tone === 'warning') return '!'
  if (notice.value.tone === 'danger') return '!'
  return 'i'
})

function seatLabel(seat: Seat) {
  return `${zoneLabels[seat.zone] ?? seat.zone} ${seat.row_no}排 ${seat.col_no}號`
}

function seatStateLabel(seat: Seat) {
  if (selectedSeatIds.value.has(seat.seat_id)) return '已選'
  if (seat.state === 'mine') return '我的座位'
  if (seat.state === 'proxy') return `代約座位${seat.student_id ? ` · ${seat.student_id}` : ''}`
  if (seat.state === 'others') return '已被選'
  if (seat.state === 'locked') return '鎖定'
  return '可選'
}

function holderLabel(seat: Seat) {
  if (seat.state === 'mine') return `${seat.student_id || auth.student?.student_id || '本人'} 本人`
  if (seat.state === 'proxy') return `${seat.student_id || '代約學生'} 代約`
  return seat.student_id || '-'
}

function selectedSlotNumber(seat: Seat) {
  const index = selectedSeatSlots.value.findIndex(item => item?.seat_id === seat.seat_id)
  return index === -1 ? '' : String(index + 1)
}

function canSelectSeat(seat: Seat) {
  if (loading.value || seats.windowState !== 'open') return false
  if (isReplacingGroup.value) return seat.state === 'free' || seat.state === 'mine' || seat.state === 'proxy'
  return seat.state === 'free'
}

function acceptRules() {
  rulesAccepted.value = true
  localStorage.setItem('seatRulesAccepted', 'true')
  centerMap()
}

function resetSelection() {
  selectedSeatSlots.value = [null, null, null]
  proxyStudentIds.value = {}
  sheetExpanded.value = true
}

function setManagementMode(mode: 'changeOwn' | 'replaceGroup') {
  managementMode.value = mode
  resetSelection()
}

function showNotice(next: typeof notice.value) {
  notice.value = next
  if (next?.tone === 'danger' || next?.tone === 'warning') {
    error.value = next.message
  }
}

function closeNotice() {
  notice.value = null
}

async function refreshSeatState() {
  try {
    await seats.loadSnapshot()
    return true
  } catch {
    return false
  }
}

function bookingErrorNotice(e: any, fallback: string) {
  const status = e?.response?.status
  const serverMessage = e?.response?.data?.error || e?.response?.data?.message || ''
  const raw = String(serverMessage || fallback)

  if (!e?.response) {
    return {
      tone: 'danger' as const,
      title: '連線不穩，尚未確認成功',
      message: '這次操作沒有收到伺服器確認。請先重新整理座位圖，再確認目前座位狀態。',
      primaryLabel: '重新整理座位圖',
      secondaryLabel: '稍後再試',
    }
  }
  if (status === 401) {
    return {
      tone: 'danger' as const,
      title: '登入狀態已失效',
      message: '請重新登入後再操作，避免用過期身分送出預約。',
      primaryLabel: '重新登入',
      secondaryLabel: '知道了',
    }
  }
  if (status === 429) {
    return {
      tone: 'warning' as const,
      title: '操作太密集了',
      message: '系統正在保護搶座窗口。座位狀態已嘗試更新，請等幾秒後再送出一次。',
      primaryLabel: '重新整理座位圖',
      secondaryLabel: '知道了',
    }
  }
  if (raw.includes('already booked')) {
    return {
      tone: 'warning' as const,
      title: '座位剛被搶走了',
      message: '你選的座位或代約學號已在剛才被其他人成功預約。座位圖已嘗試刷新，請重新選擇可用座位。',
      primaryLabel: '重新整理座位圖',
      secondaryLabel: '知道了',
    }
  }
  if (raw.includes('not open')) {
    return {
      tone: 'info' as const,
      title: '預約尚未開放',
      message: '現在只能查看座位圖，正式開放後才可以送出預約。',
      primaryLabel: '重新整理座位圖',
      secondaryLabel: '知道了',
    }
  }
  if (raw.includes('booking is locked') || raw.includes('changes are locked')) {
    return {
      tone: 'warning' as const,
      title: '預約已截止',
      message: '目前不能新增、重選或換座；如已有座位，只能依規則取消。',
      primaryLabel: '重新整理座位圖',
      secondaryLabel: '知道了',
    }
  }
  if (raw.includes('frozen')) {
    return {
      tone: 'warning' as const,
      title: '座位已凍結',
      message: '演出已開始或座位已進入凍結狀態，不能再變更預約。',
      primaryLabel: '重新整理座位圖',
      secondaryLabel: '知道了',
    }
  }
  if (raw.includes('locked')) {
    return {
      tone: 'warning' as const,
      title: '座位暫不可選',
      message: '你選的座位剛被鎖定或暫停使用。座位圖已嘗試刷新，請改選其他座位。',
      primaryLabel: '重新整理座位圖',
      secondaryLabel: '知道了',
    }
  }
  if (raw.includes('incomplete')) {
    return {
      tone: 'warning' as const,
      title: '資料尚未完成',
      message: '本人資料必須完整才能預約；若是代約對象，請確認學號正確並請對方完成資料。',
      primaryLabel: '去補資料',
      secondaryLabel: '知道了',
    }
  }
  if (status === 404) {
    return {
      tone: 'warning' as const,
      title: '狀態已改變',
      message: '座位或預約紀錄已不存在。系統已嘗試更新座位圖，請依最新狀態操作。',
      primaryLabel: '重新整理座位圖',
      secondaryLabel: '知道了',
    }
  }
  return {
    tone: 'danger' as const,
    title: '操作沒有完成',
    message: raw || fallback,
    primaryLabel: '重新整理座位圖',
    secondaryLabel: '知道了',
  }
}

async function handleNoticePrimary() {
  const current = notice.value
  if (!current) return
  if (current.primaryLabel === '重新登入') {
    closeNotice()
    router.replace({ path: '/login', query: { redirect: '/seats' } })
    return
  }
  if (current.primaryLabel === '去補資料') {
    closeNotice()
    router.replace({ path: '/register', query: { redirect: '/seats' } })
    return
  }
  if (current.primaryLabel === '知道了') {
    closeNotice()
    return
  }
  closeNotice()
  error.value = ''
  loading.value = true
  loadingLabel.value = '刷新中...'
  const ok = await refreshSeatState()
  loading.value = false
  loadingLabel.value = ''
  if (!ok) {
    showNotice({
      tone: 'danger',
      title: '座位圖暫時無法更新',
      message: '目前連線不穩，請稍後再試。系統沒有收到新的成功預約確認。',
      primaryLabel: '重新整理座位圖',
      secondaryLabel: '知道了',
    })
  }
}

function setZoomAroundContent(next: number, contentX: number, contentY: number, screenX: number, screenY: number) {
  const nextZoom = Math.min(2.15, Math.max(0.36, Number(next.toFixed(2))))
  queueTransform(
    nextZoom,
    Math.round(screenX - contentX * nextZoom),
    Math.round(screenY - contentY * nextZoom),
  )
}

function updatePointerCount() {
  activePointerCount.value = activePointers.size
}

function applyTransform(nextZoom: number, nextPanX: number, nextPanY: number) {
  zoom.value = nextZoom
  panX.value = nextPanX
  panY.value = nextPanY
  clampPan()
  scheduleMapMetrics()
}

function queueTransform(nextZoom: number, nextPanX: number, nextPanY: number) {
  queuedTransform = { zoom: nextZoom, panX: nextPanX, panY: nextPanY }
  if (transformFrame) return
  transformFrame = window.requestAnimationFrame(() => {
    transformFrame = undefined
    if (!queuedTransform) return
    const next = queuedTransform
    queuedTransform = null
    applyTransform(next.zoom, next.panX, next.panY)
  })
}

function flushQueuedTransform() {
  if (!queuedTransform) return
  if (transformFrame) {
    window.cancelAnimationFrame(transformFrame)
    transformFrame = undefined
  }
  const next = queuedTransform
  queuedTransform = null
  applyTransform(next.zoom, next.panX, next.panY)
}

function visibleCenter() {
  const sheet = document.querySelector('.bottom-sheet') as HTMLElement | null
  const topPanel = document.querySelector('.seat-top-panel') as HTMLElement | null
  const sheetTop = sheet?.getBoundingClientRect().top ?? window.innerHeight
  const topBottom = topPanel?.getBoundingClientRect().bottom ?? 90
  return {
    x: window.innerWidth / 2,
    y: topBottom + Math.max(120, (sheetTop - topBottom) / 2),
  }
}

function updateMapMetrics() {
  const viewport = document.querySelector('.seat-map-viewport') as HTMLElement | null
  const wrapper = document.querySelector('.grid-wrapper') as HTMLElement | null
  const sheet = document.querySelector('.bottom-sheet') as HTMLElement | null
  if (!viewport || !wrapper) return

  const visibleHeight = viewport.clientHeight - (sheet?.clientHeight || 0)
  const left = Math.max(0, (-panX.value / zoom.value) - wrapper.offsetLeft)
  const top = Math.max(0, (-panY.value / zoom.value) - wrapper.offsetTop)
  const width = Math.min(wrapper.scrollWidth, viewport.clientWidth / zoom.value)
  const height = Math.min(wrapper.scrollHeight, visibleHeight / zoom.value)
  mapMetrics.value = {
    left,
    top,
    width,
    height,
    mapWidth: Math.max(1, wrapper.scrollWidth),
    mapHeight: Math.max(1, wrapper.scrollHeight),
  }
}

function scheduleMapMetrics() {
  if (metricFrame) window.cancelAnimationFrame(metricFrame)
  metricFrame = window.requestAnimationFrame(updateMapMetrics)
}

function clampPan() {
  const viewport = document.querySelector('.seat-map-viewport') as HTMLElement | null
  const wrapper = document.querySelector('.grid-wrapper') as HTMLElement | null
  const sheet = document.querySelector('.bottom-sheet') as HTMLElement | null
  const topPanel = document.querySelector('.seat-top-panel') as HTMLElement | null
  if (!viewport || !wrapper) return

  const safeInset = 34
  const topLimit = (topPanel?.getBoundingClientRect().bottom ?? 90) + 18
  const bottomLimit = (sheet?.getBoundingClientRect().top ?? viewport.clientHeight) - 18

  const mapLeft = wrapper.offsetLeft * zoom.value
  const mapRight = (wrapper.offsetLeft + wrapper.scrollWidth) * zoom.value
  const minX = Math.min(safeInset - mapLeft, viewport.clientWidth - safeInset - mapRight)
  const maxX = Math.max(safeInset - mapLeft, viewport.clientWidth - safeInset - mapRight)
  panX.value = Math.min(maxX, Math.max(minX, panX.value))

  const mapTop = wrapper.offsetTop * zoom.value
  const mapBottom = (wrapper.offsetTop + wrapper.scrollHeight) * zoom.value
  if (mapBottom - mapTop <= bottomLimit - topLimit) {
    panY.value = Math.round(topLimit + (bottomLimit - topLimit - (mapBottom - mapTop)) / 2 - mapTop)
  } else {
    const minY = bottomLimit - mapBottom
    const maxY = topLimit - mapTop
    panY.value = Math.min(maxY, Math.max(minY, panY.value))
  }
}

async function centerMap() {
  await nextTick()
  const viewport = document.querySelector('.seat-map-viewport') as HTMLElement | null
  const wrapper = document.querySelector('.grid-wrapper') as HTMLElement | null
  const canvas = document.querySelector('.seat-canvas') as HTMLElement | null
  const sheet = document.querySelector('.bottom-sheet') as HTMLElement | null
  if (!viewport || !wrapper || !canvas) return

  const visibleHeight = viewport.clientHeight - (sheet?.clientHeight || 132)
  const targetByWidth = (viewport.clientWidth - 36) / Math.max(wrapper.scrollWidth, 1)
  const targetByHeight = (visibleHeight - 120) / Math.max(wrapper.scrollHeight + 92, 1)
  const nextZoom = Math.max(0.36, Math.min(1.08, targetByWidth, targetByHeight || targetByWidth))

  initialZoom.value = Number(nextZoom.toFixed(2))
  zoom.value = initialZoom.value

  const wrapperCenterX = wrapper.offsetLeft + wrapper.offsetWidth / 2
  const wrapperCenterY = wrapper.offsetTop + wrapper.offsetHeight / 2
  const viewportCenterX = viewport.clientWidth / 2
  const viewportCenterY = Math.max(170, visibleHeight / 2 + 34)

  initialPanX.value = Math.round(viewportCenterX - wrapperCenterX * initialZoom.value)
  initialPanY.value = Math.round(viewportCenterY - wrapperCenterY * initialZoom.value)
  panX.value = initialPanX.value
  panY.value = initialPanY.value
  updateMapMetrics()
}

function resetView() {
  zoom.value = initialZoom.value
  panX.value = initialPanX.value
  panY.value = initialPanY.value
  scheduleMapMetrics()
}

function zoomAroundViewport(delta: number) {
  const center = visibleCenter()
  const contentX = (center.x - panX.value) / zoom.value
  const contentY = (center.y - panY.value) / zoom.value
  focusContentPoint(contentX, contentY, zoom.value + delta)
}

function focusContentPoint(contentX: number, contentY: number, targetZoom = 1.36) {
  const nextZoom = Math.min(2.15, Math.max(Math.max(initialZoom.value, 1.24), Number(targetZoom.toFixed(2))))
  const center = visibleCenter()
  zoom.value = nextZoom
  panX.value = Math.round(center.x - contentX * nextZoom)
  panY.value = Math.round(center.y - contentY * nextZoom)
  clampPan()
  scheduleMapMetrics()
}

function focusElement(element: HTMLElement, targetZoom = 1.42) {
  if (lastPointerType.value === 'touch') return
  const rect = element.getBoundingClientRect()
  const contentX = (rect.left + rect.width / 2 - panX.value) / zoom.value
  const contentY = (rect.top + rect.height / 2 - panY.value) / zoom.value
  focusContentPoint(contentX, contentY, targetZoom)
}

function focusMapPoint(event: MouseEvent) {
  if (hasDragged.value) return
  if (lastPointerType.value === 'touch') return
  if ((event.target as HTMLElement).closest('.seat, .seat-topbar, .seat-top-panel, .map-zoom-controls, .bottom-sheet, .mini-map')) return
  const contentX = (event.clientX - panX.value) / zoom.value
  const contentY = (event.clientY - panY.value) / zoom.value
  focusContentPoint(contentX, contentY, zoom.value < 1.18 ? 1.26 : zoom.value + 0.18)
}

async function clickSeat(seat: Seat, event: MouseEvent) {
  if (hasDragged.value || loading.value) return
  if (seats.windowState !== 'open') return
  if (!canSelectSeat(seat)) return
  error.value = ''

  if (seat.state === 'mine' && !isReplacingGroup.value) {
    error.value = '如需取消目前座位，請使用上方「取消預約」。'
    return
  }

  if (selectedSeatIds.value.has(seat.seat_id)) {
    const index = selectedSeatSlots.value.findIndex(item => item?.seat_id === seat.seat_id)
    if (index !== -1) selectedSeatSlots.value[index] = null
    delete proxyStudentIds.value[seat.seat_id]
    return
  }

  if (isChangingOwnSeat.value) {
    selectedSeatSlots.value = [seat, null, null]
    proxyStudentIds.value = {}
    sheetExpanded.value = true
    focusElement(event.currentTarget as HTMLElement)
    return
  }

  if (selectedSeats.value.length >= 3) {
    error.value = '最多一次預約 3 個座位。'
    return
  }

  const emptySlot = selectedSeatSlots.value.findIndex(item => item === null)
  if (emptySlot === -1) return
  selectedSeatSlots.value[emptySlot] = seat
  if (isReplacingGroup.value && seat.state === 'proxy' && seat.student_id) {
    proxyStudentIds.value[seat.seat_id] = seat.student_id
  }
  sheetExpanded.value = true
  focusElement(event.currentTarget as HTMLElement)
}

async function cancelBooking() {
  if (reservedSeats.value.length === 0) return
  error.value = ''
  notice.value = null
  loading.value = true
  loadingLabel.value = '取消中...'
  try {
    await seats.release()
    resetSelection()
    showNotice({
      tone: 'success',
      title: '預約已取消',
      message: '你管理的座位已釋放，座位圖已更新。',
      primaryLabel: '知道了',
    })
  } catch (e: any) {
    const nextNotice = bookingErrorNotice(e, '取消失敗')
    showNotice(nextNotice)
    await refreshSeatState()
  } finally {
    loading.value = false
    loadingLabel.value = ''
  }
}

async function submitSelection() {
  if (!auth.hasCompleteProfile) {
    router.replace({ path: '/register', query: { redirect: '/seats' } })
    return
  }
  if (!canSubmit.value || !auth.student?.student_id) return
  error.value = ''
  notice.value = null
  loading.value = true
  loadingLabel.value = isChangingOwnSeat.value ? '換座中...' : (isReplacingGroup.value ? '重選中...' : '預約中...')
  try {
    if (isChangingOwnSeat.value) {
      await seats.changeSeat(selectedSeats.value[0].seat_id)
    } else {
      const bookings = selectedSeatSlots.value
        .map((seat, index) => seat ? {
          seat_id: seat.seat_id,
          student_id: index === 0 ? auth.student!.student_id : proxyStudentIds.value[seat.seat_id],
        } : null)
        .filter((booking): booking is { seat_id: number; student_id: string } => booking !== null)
      if (isReplacingGroup.value) {
        await seats.rebook(bookings)
      } else {
        await seats.book(bookings)
      }
    }
    resetSelection()
    router.push('/ticket')
  } catch (e: any) {
    const nextNotice = bookingErrorNotice(e, isChangingOwnSeat.value ? '換座失敗，請重新選擇' : '預約失敗，請重新選擇')
    showNotice(nextNotice)
    if ([401, 429].includes(e?.response?.status) || e?.response?.status >= 500 || !e?.response) {
      await refreshSeatState()
    } else {
      resetSelection()
      await refreshSeatState()
    }
  } finally {
    loading.value = false
    loadingLabel.value = ''
  }
}

function beginPinch() {
  flushQueuedTransform()
  const points = Array.from(activePointers.values())
  if (points.length < 2) return
  const dx = points[0].x - points[1].x
  const dy = points[0].y - points[1].y
  const centerX = (points[0].x + points[1].x) / 2
  const centerY = (points[0].y + points[1].y) / 2
  initialPinchDist.value = Math.hypot(dx, dy)
  initialPinchZoom.value = zoom.value
  initialPinchCenter.value = {
    screenX: centerX,
    screenY: centerY,
    contentX: (centerX - panX.value) / zoom.value,
    contentY: (centerY - panY.value) / zoom.value,
  }
  isPanning.value = false
  hasDragged.value = true
}

function registerPointer(event: PointerEvent, allowSingleFingerPan: boolean) {
  lastPointerType.value = event.pointerType
  activePointers.set(event.pointerId, { x: event.clientX, y: event.clientY })
  updatePointerCount()

  try {
    ;(event.currentTarget as HTMLElement).setPointerCapture(event.pointerId)
  } catch {
    // Some browsers may reject capture for synthetic or already-captured pointers.
  }

  if (activePointers.size >= 2) {
    beginPinch()
    return
  }

  if (!allowSingleFingerPan) {
    isPanning.value = false
    hasDragged.value = false
    return
  }

  isPanning.value = true
  hasDragged.value = false
  panStart.value = { x: event.clientX, y: event.clientY, panX: panX.value, panY: panY.value }
}

function startPan(event: PointerEvent) {
  if ((event.target as HTMLElement).closest('.seat, .seat-topbar, .seat-top-panel, .map-zoom-controls, .bottom-sheet, .rules-modal-overlay, .seat-notice-overlay, .mini-map')) return
  registerPointer(event, true)
}

function startSeatPointer(event: PointerEvent) {
  registerPointer(event, true)
}

function movePan(event: PointerEvent) {
  if (!activePointers.has(event.pointerId)) return
  activePointers.set(event.pointerId, { x: event.clientX, y: event.clientY })

  if (activePointers.size === 2) {
    const points = Array.from(activePointers.values())
    const dx = points[0].x - points[1].x
    const dy = points[0].y - points[1].y
    const dist = Math.hypot(dx, dy)
    if (initialPinchDist.value > 0) {
      const centerX = (points[0].x + points[1].x) / 2
      const centerY = (points[0].y + points[1].y) / 2
      setZoomAroundContent(
        initialPinchZoom.value * (dist / initialPinchDist.value),
        initialPinchCenter.value.contentX,
        initialPinchCenter.value.contentY,
        centerX,
        centerY,
      )
    }
    return
  }

  if (!isPanning.value) return
  const dx = event.clientX - panStart.value.x
  const dy = event.clientY - panStart.value.y
  const dragThreshold = event.pointerType === 'touch' ? 9 : 5
  if (Math.abs(dx) > dragThreshold || Math.abs(dy) > dragThreshold) hasDragged.value = true
  queueTransform(zoom.value, panStart.value.panX + dx, panStart.value.panY + dy)
}

function endPan(event: PointerEvent) {
  activePointers.delete(event.pointerId)
  updatePointerCount()
  if (activePointers.size < 2) initialPinchDist.value = 0
  if (activePointers.size === 0) {
    flushQueuedTransform()
    isPanning.value = false
    try {
      ;(event.currentTarget as HTMLElement).releasePointerCapture(event.pointerId)
    } catch {
      // Pointer capture may already be released by the browser.
    }
    setTimeout(() => {
      hasDragged.value = false
    }, lastPointerType.value === 'touch' ? 140 : 60)
  }
}

function zoomWheel(event: WheelEvent) {
  event.preventDefault()
  zoomAroundViewport(event.deltaY > 0 ? -0.08 : 0.08)
}

onMounted(async () => {
  try {
    if (!auth.student) await auth.fetchMe()
    if (!auth.hasCompleteProfile) {
      router.replace({ path: '/register', query: { redirect: '/seats' } })
      return
    }
    await seats.loadSnapshot()
    seats.connectStream()
  } catch {
    showNotice({
      tone: 'danger',
      title: '座位圖載入失敗',
      message: '目前無法取得最新座位狀態。請重新整理後再操作，避免用過期座位圖送出預約。',
      primaryLabel: '重新整理座位圖',
      secondaryLabel: '稍後再試',
    })
  }
  if (rulesAccepted.value) await centerMap()
  centerTimer = window.setTimeout(() => centerMap(), 280)
  window.addEventListener('resize', centerMap)
})

onUnmounted(() => {
  if (centerTimer) window.clearTimeout(centerTimer)
  if (metricFrame) window.cancelAnimationFrame(metricFrame)
  if (transformFrame) window.cancelAnimationFrame(transformFrame)
  window.removeEventListener('resize', centerMap)
  seats.disconnectStream()
})
</script>

<template>
  <main class="seats-page-fullscreen">
    <Transition name="fade">
      <div v-if="!rulesAccepted" class="rules-modal-overlay">
        <section class="rules-modal">
          <span class="caption">Seat Guide</span>
          <h1>選座前請確認規則</h1>
          <div class="rules-grid">
            <span>最多一次預約 3 座</span>
            <span>第 2、3 座須填寫代約學號</span>
            <span>白色座位可選，灰色座位不可選</span>
            <span>鎖定後只能釋放，不能新增或換座</span>
          </div>
          <button class="btn-primary" type="button" @click="acceptRules">進入選座</button>
        </section>
      </div>
    </Transition>

    <Transition name="fade">
      <div v-if="notice" class="seat-notice-overlay" @click.self="closeNotice">
        <section class="seat-notice" :class="`seat-notice--${notice.tone}`" role="alertdialog" aria-modal="true">
          <span class="notice-icon">{{ noticeIcon }}</span>
          <div class="notice-copy">
            <h2>{{ notice.title }}</h2>
            <p>{{ notice.message }}</p>
          </div>
          <div class="notice-actions">
            <button v-if="notice.secondaryLabel" class="notice-secondary" type="button" @click="closeNotice">
              {{ notice.secondaryLabel }}
            </button>
            <button class="notice-primary" type="button" :disabled="loading" @click="handleNoticePrimary">
              {{ loading ? '處理中...' : notice.primaryLabel }}
            </button>
          </div>
        </section>
      </div>
    </Transition>

    <header v-if="rulesAccepted" class="seat-topbar">
      <button class="back-btn" type="button" aria-label="返回首頁" @click="router.push('/')">‹</button>
      <div class="top-title">
        <span class="caption">La La Land</span>
        <strong>選座</strong>
      </div>
      <div class="window-chip" :class="windowBanner[seats.windowState].tone">
        <span></span>
        {{ windowBanner[seats.windowState].label }}
      </div>
    </header>

    <section v-if="rulesAccepted" class="seat-top-panel">
      <div class="seat-status-strip">
        <span><i class="legend-dot free-dot"></i>可選 {{ seatStats.free }}</span>
        <span><i class="legend-dot selected-dot"></i>已選 {{ selectedSeats.length }}</span>
        <span><i class="legend-dot mine-dot"></i>我的 {{ myBooking ? 1 : 0 }}</span>
        <span><i class="legend-dot proxy-dot"></i>代約 {{ proxyBookings.length }}</span>
        <span><i class="legend-dot occupied-dot"></i>不可選 {{ seatStats.occupied + seatStats.locked }}</span>
      </div>
      <div class="zone-strip">
        <span v-for="zone in zones" :key="zone.zone">{{ zone.label }} {{ zone.count }} 席</span>
      </div>
    </section>

    <section
      class="seat-map-viewport"
      :class="{ panning: isPanning, interacting: activePointerCount > 0 }"
      @pointerdown="startPan"
      @pointermove="movePan"
      @pointerup="endPan"
      @pointercancel="endPan"
      @wheel="zoomWheel"
      @click="focusMapPoint"
    >
      <div
        class="seat-canvas"
        aria-label="左中右三區座位圖"
        :style="{ transform: `translate3d(${panX}px, ${panY}px, 0) scale(${zoom})` }"
      >
        <div class="stage">
          <span>舞台</span>
        </div>

        <div class="grid-wrapper">
          <section v-for="zone in zones" :key="zone.zone" class="seat-zone" :class="`seat-zone--${zone.zone}`">
            <header class="zone-header">
              <strong>{{ zone.label }}</strong>
              <span>{{ zone.count }} 席</span>
            </header>
            <div class="zone-rows">
              <div v-for="{ row, seats: rowSeats } in zone.rows" :key="`${zone.zone}-${row}`" class="row">
                <span class="row-label">{{ row }}</span>
                <div class="seat-line">
                  <button
                    v-for="seat in rowSeats"
                    :key="seat.seat_id"
                    type="button"
                    class="seat"
                    :class="{
                      mine: seat.state === 'mine',
                      proxy: seat.state === 'proxy',
                      free: seat.state === 'free',
                      selected: selectedSeatIds.has(seat.seat_id),
                      occupied: seat.state === 'others',
                      locked: seat.state === 'locked',
                      clickable: canSelectSeat(seat)
                    }"
                    :title="`${seatLabel(seat)} · ${seatStateLabel(seat)}`"
                    :aria-label="`${seatLabel(seat)} · ${seatStateLabel(seat)}`"
                    :aria-disabled="!canSelectSeat(seat)"
                    :tabindex="canSelectSeat(seat) ? 0 : -1"
                    @pointerdown.stop="startSeatPointer"
                    @pointermove.stop="movePan"
                    @pointerup.stop="endPan"
                    @pointercancel.stop="endPan"
                    @click.stop="clickSeat(seat, $event)"
                  >
                    <span v-if="selectedSeatIds.has(seat.seat_id)" class="seat-mark">
                      {{ selectedSlotNumber(seat) }}
                    </span>
                    <span v-else-if="seat.state === 'mine'" class="seat-mark">✓</span>
                    <span v-else-if="seat.state === 'proxy'" class="seat-mark">代</span>
                  </button>
                </div>
              </div>
            </div>
          </section>
        </div>
      </div>
    </section>

    <aside v-if="rulesAccepted && showMiniMap" class="mini-map" aria-hidden="true">
      <div class="mini-stage"></div>
      <div class="mini-grid">
        <section v-for="zone in zones" :key="`mini-${zone.zone}`" class="mini-zone" :class="`mini-zone--${zone.zone}`">
          <div v-for="{ row, seats: rowSeats } in zone.rows" :key="`mini-${zone.zone}-${row}`" class="mini-row">
            <i
              v-for="seat in rowSeats"
              :key="`mini-${seat.seat_id}`"
              :class="{
                selected: selectedSeatIds.has(seat.seat_id),
                mine: seat.state === 'mine',
                proxy: seat.state === 'proxy',
                unavailable: seat.state === 'others' || seat.state === 'locked'
              }"
            ></i>
          </div>
        </section>
        <span class="mini-viewport" :style="minimapViewportStyle"></span>
      </div>
    </aside>

    <aside v-if="rulesAccepted" class="map-zoom-controls">
      <div class="zoom-controls">
        <button type="button" aria-label="縮小" @click="zoomAroundViewport(-0.1)">−</button>
        <button type="button" aria-label="回正視角" @click="resetView">⌖</button>
        <button type="button" aria-label="放大" @click="zoomAroundViewport(0.1)">＋</button>
      </div>
    </aside>

    <Transition name="slide-up">
      <section v-if="rulesAccepted" class="bottom-sheet" :class="{ expanded: selectedSeats.length > 0 && sheetExpanded }">
        <div class="sheet-grabber" aria-hidden="true"></div>
        <div class="sheet-head">
          <button class="sheet-title" type="button" @click="sheetExpanded = !sheetExpanded">
            <span class="caption">{{ selectionTitle }}</span>
            <strong>{{ seatHint }}</strong>
          </button>
          <button
            v-if="selectedSeats.length > 0"
            class="mini-action"
            type="button"
            :disabled="loading"
            @click="resetSelection"
          >
            清空
          </button>
        </div>

        <div v-if="hasManagedBookings && seats.windowState === 'open'" class="mode-switch" role="group" aria-label="座位管理模式">
          <button
            v-if="myBooking"
            type="button"
            :class="{ active: isChangingOwnSeat }"
            @click="setManagementMode('changeOwn')"
          >
            換本人座位
          </button>
          <button
            type="button"
            :class="{ active: isReplacingGroup }"
            @click="setManagementMode('replaceGroup')"
          >
            重選全部
          </button>
        </div>

        <p v-if="error" class="error-msg">{{ error }}</p>

        <div v-if="reservedSeats.length > 0 && selectedSeats.length === 0" class="reserved-list">
          <article
            v-for="(seat, index) in reservedSeats"
            :key="seat.seat_id"
            class="reserved-row"
            :class="{ own: seat.state === 'mine', proxy: seat.state === 'proxy' }"
          >
            <span class="reserved-badge">{{ seat.state === 'mine' ? '本' : '代' }}</span>
            <div>
              <strong>{{ seatLabel(seat) }}</strong>
              <small>{{ holderLabel(seat) }}</small>
            </div>
            <button
              v-if="seat.state === 'mine'"
              class="mini-action danger"
              type="button"
              :disabled="loading"
              @click="cancelBooking"
            >
              {{ loading ? '處理中' : (proxyBookings.length > 0 ? '取消全部' : '取消') }}
            </button>
            <button
              v-else-if="!myBooking && index === 0"
              class="mini-action danger"
              type="button"
              :disabled="loading"
              @click="cancelBooking"
            >
              {{ loading ? '處理中' : '取消代約' }}
            </button>
          </article>
        </div>

        <div v-else v-show="sheetExpanded" class="selection-list">
          <template v-for="(seat, index) in selectedSeatSlots" :key="seat?.seat_id || `empty-${index}`">
            <label v-if="seat" class="selection-row">
              <span class="selection-index">{{ index + 1 }}</span>
              <strong>{{ seatLabel(seat) }}</strong>
              <input
                v-if="!isChangingOwnSeat && index > 0"
                v-model="proxyStudentIds[seat.seat_id]"
                inputmode="numeric"
                maxlength="7"
                placeholder="代約學號"
              />
              <small v-else>{{ isChangingOwnSeat ? '新座位' : '本人' }}</small>
            </label>
          </template>
        </div>

        <div class="sheet-footer">
          <div class="seat-counts">
            <span>可選 {{ seatStats.free }}</span>
            <span>已占 {{ seatStats.occupied + seatStats.proxy }}</span>
            <span>鎖定 {{ seatStats.locked }}</span>
          </div>
          <button
            class="submit-btn"
            type="button"
            :disabled="!canSubmit"
            @click="submitSelection"
          >
            {{ submitLabel }}
          </button>
        </div>
      </section>
    </Transition>
  </main>
</template>

<style scoped>
.seats-page-fullscreen {
  position: fixed;
  inset: 0;
  overflow: hidden;
  color: var(--cream);
  background:
    linear-gradient(180deg, rgba(7, 8, 26, 0.42), rgba(7, 8, 26, 0.88)),
    radial-gradient(circle at 50% 6%, rgba(246, 178, 63, 0.26), transparent 23rem),
    radial-gradient(circle at 12% 84%, rgba(39, 215, 223, 0.12), transparent 18rem),
    linear-gradient(150deg, #11183b 0%, #24144e 46%, #4b193c 100%);
  z-index: 2000;
  user-select: none;
  -webkit-user-select: none;
  -webkit-touch-callout: none;
  overscroll-behavior: none;
}

.seats-page-fullscreen * {
  user-select: none;
  -webkit-user-select: none;
  -webkit-touch-callout: none;
}

.seat-topbar {
  position: absolute;
  top: calc(10px + env(safe-area-inset-top));
  left: 16px;
  right: 16px;
  z-index: 30;
  display: grid;
  grid-template-columns: 44px minmax(0, 1fr) auto;
  align-items: center;
  gap: 12px;
  pointer-events: none;
}

.seat-topbar > * {
  pointer-events: auto;
}

.back-btn,
.zoom-controls button {
  width: 44px;
  height: 44px;
  border: 1px solid rgba(255, 243, 215, 0.16);
  border-radius: 50%;
  background: rgba(8, 10, 34, 0.76);
  color: var(--cream);
  backdrop-filter: blur(18px);
  -webkit-backdrop-filter: blur(18px);
  box-shadow: 0 12px 32px rgba(0, 0, 0, 0.24);
  cursor: pointer;
}

.back-btn {
  font-size: 30px;
  line-height: 1;
}

.top-title {
  min-width: 0;
  display: flex;
  align-items: baseline;
  gap: 10px;
}

.top-title strong {
  font-size: 22px;
  line-height: 1;
}

.window-chip {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  min-height: 38px;
  padding: 0 14px;
  border-radius: 999px;
  background: rgba(8, 10, 34, 0.76);
  border: 1px solid rgba(255, 243, 215, 0.14);
  color: rgba(255, 243, 215, 0.82);
  font-size: 13px;
  font-weight: 700;
  white-space: nowrap;
  backdrop-filter: blur(18px);
}

.window-chip span {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: currentColor;
  box-shadow: 0 0 14px currentColor;
}

.window-chip.live { color: #79f4cd; }
.window-chip.warn { color: var(--gold-light); }
.window-chip.closed { color: #ff8dae; }

.seat-top-panel {
  position: absolute;
  top: calc(62px + env(safe-area-inset-top));
  left: 50%;
  z-index: 29;
  width: min(760px, calc(100vw - 28px));
  display: grid;
  gap: 8px;
  transform: translateX(-50%);
  pointer-events: none;
}

.seat-status-strip,
.zone-strip {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 10px;
  min-height: 34px;
  padding: 0 12px;
  border: 1px solid rgba(255, 243, 215, 0.12);
  border-radius: 999px;
  background: rgba(8, 10, 34, 0.58);
  color: rgba(255, 243, 215, 0.76);
  font-size: 11px;
  backdrop-filter: blur(18px);
  -webkit-backdrop-filter: blur(18px);
  box-shadow: 0 10px 32px rgba(0, 0, 0, 0.18);
  overflow-x: auto;
  white-space: nowrap;
  pointer-events: auto;
}

.seat-status-strip span,
.zone-strip span {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  flex: 0 0 auto;
}

.zone-strip {
  justify-self: center;
  min-height: 28px;
  color: rgba(255, 243, 215, 0.56);
  font-size: 10px;
}

.seat-map-viewport {
  width: 100%;
  height: 100%;
  overflow: hidden;
  touch-action: none;
  cursor: grab;
  user-select: none;
  -webkit-user-select: none;
}

.seat-map-viewport.panning {
  cursor: grabbing;
}

.seat-canvas {
  position: absolute;
  left: 0;
  top: 0;
  width: max-content;
  min-width: 100%;
  padding: 142px clamp(20px, 4vw, 56px) 300px;
  transform-origin: top left;
  transition: transform 160ms ease-out;
  will-change: transform;
}

.seat-map-viewport.interacting .seat-canvas {
  transition: none;
}

.stage {
  width: min(620px, 70vw);
  height: 52px;
  margin: 0 auto 28px;
  display: flex;
  align-items: center;
  justify-content: center;
  color: #24110b;
  font-weight: 900;
  letter-spacing: 0.26em;
  text-indent: 0.26em;
  border-radius: 0 0 50% 50% / 0 0 100% 100%;
  background:
    linear-gradient(180deg, rgba(255, 246, 208, 0.94), rgba(246, 178, 63, 0.78)),
    repeating-linear-gradient(90deg, transparent, transparent 18px, rgba(88, 30, 14, 0.16) 18px, rgba(88, 30, 14, 0.16) 20px);
  box-shadow: 0 24px 70px rgba(246, 178, 63, 0.24);
}

.grid-wrapper {
  display: grid;
  grid-template-columns: max-content max-content max-content;
  align-items: start;
  justify-content: center;
  gap: clamp(18px, 3vw, 46px);
}

.seat-zone {
  min-width: 0;
}

.seat-zone--middle {
  padding-inline: clamp(8px, 1.4vw, 18px);
  border-inline: 1px dashed rgba(255, 243, 215, 0.16);
}

.zone-header {
  height: 34px;
  margin-bottom: 10px;
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  color: rgba(255, 243, 215, 0.78);
  font-size: 12px;
}

.zone-header strong {
  color: var(--cream);
  font-size: 15px;
}

.zone-header span {
  color: rgba(255, 243, 215, 0.48);
}

.zone-rows {
  display: flex;
  flex-direction: column;
  gap: 7px;
}

.row {
  display: flex;
  align-items: center;
  gap: 7px;
}

.row-label {
  width: 22px;
  min-height: 22px;
  text-align: right;
  color: rgba(255, 243, 215, 0.52);
  font-size: 11px;
  font-variant-numeric: tabular-nums;
  padding: 2px 4px 0 0;
  border-radius: 999px;
  background: linear-gradient(90deg, rgba(8, 10, 34, 0), rgba(8, 10, 34, 0.54));
  pointer-events: none;
}

.seat-line {
  display: flex;
  gap: 7px;
  min-height: 25px;
}

.seat-zone--left .seat-line {
  justify-content: flex-end;
}

.seat-zone--middle .seat-line {
  justify-content: center;
}

.seat-zone--right .seat-line {
  justify-content: flex-start;
}

.seat {
  position: relative;
  width: 25px;
  height: 25px;
  flex: 0 0 25px;
  border: 0;
  border-radius: 8px 8px 11px 11px;
  background: rgba(255, 255, 255, 0.12);
  box-shadow:
    inset 0 -4px 0 rgba(0, 0, 0, 0.14),
    inset 0 0 0 1px rgba(255, 255, 255, 0.12);
  color: transparent;
  cursor: default;
  transition: transform 140ms ease, background 140ms ease, box-shadow 140ms ease;
  user-select: none;
  -webkit-user-select: none;
  -webkit-tap-highlight-color: transparent;
}

.seat::before {
  content: '';
  position: absolute;
  left: 4px;
  right: 4px;
  top: 3px;
  height: 4px;
  border-radius: 999px;
  background: rgba(0, 0, 0, 0.14);
}

.seat.free {
  background: #fff8e7;
  box-shadow:
    0 5px 13px rgba(3, 6, 24, 0.26),
    inset 0 -4px 0 rgba(190, 165, 122, 0.5);
}

.seat.clickable {
  cursor: pointer;
}

.seat.clickable:hover {
  transform: translateY(-3px) scale(1.12);
  box-shadow:
    0 9px 22px rgba(246, 178, 63, 0.32),
    inset 0 -4px 0 rgba(190, 165, 122, 0.44);
}

.seat.selected,
.seat.mine,
.seat.proxy {
  color: #1a0f09;
  background: linear-gradient(180deg, #ffe58a, #f6b23f);
  box-shadow:
    0 0 0 3px rgba(246, 178, 63, 0.2),
    0 10px 24px rgba(246, 178, 63, 0.36),
    inset 0 -4px 0 rgba(128, 82, 18, 0.32);
  transform: translateY(-2px);
}

.seat.proxy {
  background: linear-gradient(180deg, #bde8ff, #64b5ff);
  box-shadow:
    0 0 0 3px rgba(100, 181, 255, 0.18),
    0 10px 24px rgba(100, 181, 255, 0.28),
    inset 0 -4px 0 rgba(27, 91, 145, 0.28);
}

.seat.occupied {
  background: rgba(197, 207, 220, 0.24);
}

.seat.locked {
  background: rgba(2, 4, 16, 0.54);
  box-shadow: inset 0 0 0 1px rgba(255, 255, 255, 0.08);
}

.seat[aria-disabled="true"] {
  opacity: 1;
}

.seat-mark {
  position: relative;
  z-index: 1;
  color: #1b0f05;
  font-size: 12px;
  font-weight: 900;
  line-height: 1;
}

.map-zoom-controls {
  position: absolute;
  right: 18px;
  bottom: calc(184px + env(safe-area-inset-bottom));
  z-index: 28;
  display: flex;
  align-items: center;
  gap: 12px;
  pointer-events: none;
}

.map-zoom-controls > * {
  pointer-events: auto;
}

.legend-dot {
  width: 10px;
  height: 10px;
  border-radius: 4px;
  display: inline-block;
}

.free-dot { background: #fff8e7; }
.selected-dot { background: var(--gold); }
.mine-dot { background: #79f4cd; }
.proxy-dot { background: #64b5ff; }
.occupied-dot { background: rgba(197, 207, 220, 0.35); }

.zoom-controls {
  display: grid;
  gap: 8px;
}

.zoom-controls button {
  font-size: 18px;
  font-weight: 800;
}

.mini-map {
  position: absolute;
  left: 18px;
  top: calc(138px + env(safe-area-inset-top));
  z-index: 27;
  width: 132px;
  padding: 9px;
  border: 1px solid rgba(255, 243, 215, 0.13);
  border-radius: 10px;
  background: rgba(8, 10, 34, 0.48);
  box-shadow: 0 12px 36px rgba(0, 0, 0, 0.26);
  backdrop-filter: blur(16px);
  -webkit-backdrop-filter: blur(16px);
}

.mini-stage {
  width: 58%;
  height: 5px;
  margin: 0 auto 8px;
  border-radius: 0 0 999px 999px;
  background: rgba(246, 178, 63, 0.8);
}

.mini-grid {
  position: relative;
  display: grid;
  grid-template-columns: max-content max-content max-content;
  justify-content: center;
  gap: 5px;
}

.mini-zone {
  display: grid;
  gap: 1.5px;
}

.mini-zone--middle {
  padding-inline: 3px;
  border-inline: 1px solid rgba(255, 243, 215, 0.12);
}

.mini-row {
  display: flex;
  gap: 1.5px;
}

.mini-row i {
  width: 2.5px;
  height: 2.5px;
  border-radius: 50%;
  background: rgba(255, 248, 231, 0.76);
}

.mini-row i.unavailable {
  background: rgba(197, 207, 220, 0.28);
}

.mini-row i.selected,
.mini-row i.mine {
  background: var(--gold);
}

.mini-row i.proxy {
  background: #64b5ff;
}

.mini-viewport {
  position: absolute;
  min-width: 14px;
  min-height: 16px;
  border: 1px solid rgba(255, 214, 107, 0.94);
  border-radius: 3px;
  background: rgba(255, 214, 107, 0.12);
  box-shadow: 0 0 12px rgba(255, 214, 107, 0.3);
  pointer-events: none;
}

.bottom-sheet {
  position: absolute;
  left: 0;
  right: 0;
  bottom: 0;
  z-index: 40;
  padding: 10px clamp(16px, 4vw, 34px) calc(14px + env(safe-area-inset-bottom));
  border-radius: 22px 22px 0 0;
  background:
    linear-gradient(180deg, rgba(21, 24, 55, 0.96), rgba(12, 13, 34, 0.98));
  border-top: 1px solid rgba(255, 243, 215, 0.16);
  box-shadow: 0 -18px 60px rgba(0, 0, 0, 0.45);
  backdrop-filter: blur(20px);
}

.sheet-grabber {
  width: 42px;
  height: 4px;
  margin: 0 auto 12px;
  border-radius: 999px;
  background: rgba(255, 243, 215, 0.28);
}

.sheet-head,
.sheet-footer,
.current-booking,
.selection-row {
  display: flex;
  align-items: center;
}

.sheet-head {
  justify-content: space-between;
  gap: 12px;
}

.sheet-title {
  min-width: 0;
  border: 0;
  background: transparent;
  color: inherit;
  text-align: left;
  cursor: pointer;
}

.sheet-title strong {
  display: block;
  margin-top: 2px;
  color: var(--cream);
  font-size: 15px;
  line-height: 1.25;
}

.mini-action {
  flex: 0 0 auto;
  min-height: 34px;
  padding: 0 12px;
  border: 1px solid rgba(255, 243, 215, 0.18);
  border-radius: 999px;
  background: rgba(255, 255, 255, 0.06);
  color: var(--cream);
  font-size: 12px;
  cursor: pointer;
}

.mini-action.danger {
  color: #ffb8c8;
  border-color: rgba(255, 141, 174, 0.28);
}

.mode-switch {
  display: inline-flex;
  gap: 6px;
  margin-top: 12px;
  padding: 4px;
  border: 1px solid rgba(255, 243, 215, 0.14);
  border-radius: 999px;
  background: rgba(255, 255, 255, 0.06);
}

.mode-switch button {
  min-height: 34px;
  padding: 0 13px;
  border: 0;
  border-radius: 999px;
  background: transparent;
  color: rgba(255, 243, 215, 0.68);
  font-size: 13px;
  cursor: pointer;
}

.mode-switch button.active {
  background: rgba(244, 196, 111, 0.22);
  color: var(--cream);
  box-shadow: inset 0 0 0 1px rgba(244, 196, 111, 0.28);
}

.reserved-list {
  display: grid;
  gap: 8px;
  margin-top: 12px;
  max-height: 24vh;
  overflow-y: auto;
}

.reserved-row {
  margin-top: 12px;
  min-height: 58px;
  gap: 12px;
  padding: 10px 12px;
  border-radius: 8px;
  background: rgba(121, 244, 205, 0.08);
  border: 1px solid rgba(121, 244, 205, 0.18);
  display: flex;
  align-items: center;
}

.reserved-row + .reserved-row {
  margin-top: 0;
}

.reserved-row.proxy {
  background: rgba(100, 181, 255, 0.08);
  border-color: rgba(100, 181, 255, 0.2);
}

.reserved-badge,
.selection-index {
  width: 26px;
  height: 26px;
  flex: 0 0 26px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  border-radius: 50%;
  background: var(--gold);
  color: #170e07;
  font-size: 12px;
  font-weight: 900;
}

.reserved-row.proxy .reserved-badge {
  background: #64b5ff;
}

.reserved-row div {
  flex: 1;
  min-width: 0;
}

.reserved-row strong,
.selection-row strong {
  display: block;
  color: var(--cream);
  font-size: 14px;
}

.reserved-row small,
.selection-row small {
  display: block;
  color: var(--muted);
  font-size: 12px;
}

.sheet-hint {
  margin-top: 10px;
  color: rgba(255, 243, 215, 0.76);
  font-size: 13px;
  line-height: 1.45;
}

.selection-list {
  display: grid;
  gap: 8px;
  max-height: 30vh;
  margin-top: 12px;
  overflow-y: auto;
}

.selection-row {
  gap: 10px;
  min-height: 48px;
  padding: 8px 10px;
  border-radius: 8px;
  background: rgba(255, 255, 255, 0.06);
}

.selection-row strong {
  flex: 1;
  min-width: 96px;
}

.selection-row input {
  width: 126px;
  height: 34px;
  padding: 0 10px;
  border: 1px solid rgba(255, 243, 215, 0.18);
  border-radius: 6px;
  background: rgba(0, 0, 0, 0.2);
  color: var(--cream);
  text-align: center;
  font-size: 14px;
}

.sheet-footer {
  justify-content: space-between;
  gap: 14px;
  margin-top: 12px;
}

.seat-counts {
  display: flex;
  flex-wrap: wrap;
  gap: 8px 12px;
  color: rgba(255, 243, 215, 0.58);
  font-size: 12px;
}

.submit-btn {
  min-width: 150px;
  height: 44px;
  border: 0;
  border-radius: 999px;
  background: linear-gradient(135deg, #fff4cf, #f6b23f 62%, #ff8dae);
  color: #1c0d05;
  font-size: 14px;
  font-weight: 900;
  letter-spacing: 0.08em;
  cursor: pointer;
  box-shadow: 0 12px 30px rgba(246, 178, 63, 0.22);
}

.submit-btn:disabled,
.mini-action:disabled {
  cursor: not-allowed;
  opacity: 0.46;
}

	.error-msg {
	  margin: 10px 0 0;
	  color: #ffb8c8;
	  font-size: 13px;
	}

	.seat-notice-overlay {
	  position: fixed;
	  inset: 0;
	  z-index: 90;
	  display: flex;
	  align-items: center;
	  justify-content: center;
	  padding: 20px;
	  background: rgba(7, 8, 26, 0.68);
	  backdrop-filter: blur(12px);
	  -webkit-backdrop-filter: blur(12px);
	}

	.seat-notice {
	  width: min(430px, 100%);
	  display: grid;
	  grid-template-columns: 42px minmax(0, 1fr);
	  gap: 14px;
	  padding: 22px;
	  border: 1px solid rgba(255, 243, 215, 0.16);
	  border-radius: 10px;
	  background:
	    linear-gradient(180deg, rgba(24, 27, 62, 0.98), rgba(13, 15, 39, 0.98));
	  box-shadow: 0 28px 90px rgba(0, 0, 0, 0.48);
	}

	.notice-icon {
	  width: 42px;
	  height: 42px;
	  display: inline-flex;
	  align-items: center;
	  justify-content: center;
	  border-radius: 50%;
	  background: rgba(255, 243, 215, 0.12);
	  color: var(--cream);
	  font-weight: 900;
	}

	.seat-notice--success .notice-icon {
	  background: rgba(121, 244, 205, 0.2);
	  color: #79f4cd;
	}

	.seat-notice--warning .notice-icon {
	  background: rgba(246, 178, 63, 0.2);
	  color: var(--gold-light);
	}

	.seat-notice--danger .notice-icon {
	  background: rgba(255, 141, 174, 0.2);
	  color: #ff8dae;
	}

	.notice-copy h2 {
	  margin: 1px 0 8px;
	  color: var(--cream);
	  font-size: 18px;
	  line-height: 1.25;
	}

	.notice-copy p {
	  margin: 0;
	  color: rgba(255, 243, 215, 0.76);
	  font-size: 14px;
	  line-height: 1.55;
	}

	.notice-actions {
	  grid-column: 1 / -1;
	  display: flex;
	  justify-content: flex-end;
	  gap: 10px;
	  margin-top: 6px;
	}

	.notice-primary,
	.notice-secondary {
	  min-height: 40px;
	  padding: 0 16px;
	  border-radius: 999px;
	  font-size: 13px;
	  font-weight: 800;
	  cursor: pointer;
	}

	.notice-primary {
	  border: 0;
	  background: linear-gradient(135deg, #fff4cf, #f6b23f 70%);
	  color: #1c0d05;
	}

	.notice-primary:disabled {
	  cursor: not-allowed;
	  opacity: 0.56;
	}

	.notice-secondary {
	  border: 1px solid rgba(255, 243, 215, 0.18);
	  background: rgba(255, 255, 255, 0.06);
	  color: rgba(255, 243, 215, 0.84);
	}

	.rules-modal-overlay {
	  position: fixed;
  inset: 0;
  z-index: 80;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 20px;
  background: rgba(7, 8, 26, 0.84);
  backdrop-filter: blur(14px);
}

.rules-modal {
  width: min(460px, 100%);
  padding: 30px;
  border-radius: 8px;
  background: rgba(19, 21, 54, 0.94);
  border: 1px solid rgba(255, 243, 215, 0.16);
  box-shadow: 0 28px 90px rgba(0, 0, 0, 0.44);
  text-align: center;
}

.rules-modal h1 {
  margin: 8px 0 20px;
  font-size: 24px;
  line-height: 1.2;
}

.rules-grid {
  display: grid;
  gap: 10px;
  margin-bottom: 24px;
}

.rules-grid span {
  padding: 12px 14px;
  border-radius: 6px;
  background: rgba(255, 255, 255, 0.06);
  color: rgba(255, 243, 215, 0.86);
  font-size: 14px;
}

.fade-enter-active,
.fade-leave-active,
.slide-up-enter-active,
.slide-up-leave-active {
  transition: opacity 180ms ease, transform 220ms ease;
}

.fade-enter-from,
.fade-leave-to {
  opacity: 0;
}

.slide-up-enter-from,
.slide-up-leave-to {
  opacity: 0;
  transform: translateY(20px);
}

@media (min-width: 900px) {
  .bottom-sheet {
    left: 50%;
    right: auto;
    bottom: 22px;
    width: min(840px, calc(100vw - 48px));
    transform: translateX(-50%);
    border: 1px solid rgba(255, 243, 215, 0.16);
    border-radius: 14px;
    padding: 12px 18px;
  }

  .sheet-grabber {
    display: none;
  }

  .map-zoom-controls {
    bottom: 274px;
  }
}

@media (max-width: 760px) {
  .seat-topbar {
    left: 12px;
    right: 12px;
    grid-template-columns: 40px minmax(0, 1fr) auto;
  }

  .back-btn,
  .zoom-controls button {
    width: 40px;
    height: 40px;
  }

  .top-title {
    gap: 7px;
  }

  .top-title strong {
    font-size: 18px;
  }

  .window-chip {
    min-height: 34px;
    padding: 0 10px;
    font-size: 12px;
  }

  .seat-canvas {
    padding: 132px 18px 285px;
  }

  .stage {
    height: 44px;
    margin-bottom: 22px;
    font-size: 13px;
  }

  .grid-wrapper {
    gap: 12px;
  }

  .seat-zone--middle {
    padding-inline: 7px;
  }

  .zone-header {
    height: 30px;
    margin-bottom: 8px;
    font-size: 10px;
  }

  .zone-header strong {
    font-size: 12px;
  }

  .zone-rows {
    gap: 5px;
  }

  .row {
    gap: 4px;
  }

  .row-label {
    width: 16px;
    font-size: 9px;
  }

  .seat-line {
    gap: 4px;
    min-height: 21px;
  }

  .seat {
    width: 21px;
    height: 21px;
    flex-basis: 21px;
    border-radius: 7px 7px 9px 9px;
  }

  .seat-top-panel {
    top: calc(58px + env(safe-area-inset-top));
    width: calc(100vw - 24px);
  }

  .seat-status-strip {
    justify-content: flex-start;
  }

  .zone-strip {
    display: none;
  }

  .map-zoom-controls {
    right: 12px;
    bottom: calc(258px + env(safe-area-inset-bottom));
  }

  .zoom-controls {
    gap: 6px;
  }

  .mini-map {
    left: 12px;
    top: calc(102px + env(safe-area-inset-top));
    width: 110px;
    padding: 8px;
  }

  .sheet-footer {
    align-items: stretch;
  }

  .seat-counts {
    max-width: 46%;
    align-content: center;
  }

  .submit-btn {
    min-width: 132px;
  }

  .selection-row {
    display: grid;
    grid-template-columns: 26px minmax(92px, 1fr) minmax(92px, 126px);
  }

  .selection-row input {
    width: 100%;
  }

  .seat-notice {
    grid-template-columns: 36px minmax(0, 1fr);
    gap: 12px;
    padding: 18px;
  }

  .notice-icon {
    width: 36px;
    height: 36px;
  }

  .notice-actions {
    flex-direction: column-reverse;
  }

  .notice-primary,
  .notice-secondary {
    width: 100%;
  }
}

@media (max-width: 420px) {
  .top-title .caption {
    display: none;
  }

  .window-chip {
    max-width: 92px;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .map-zoom-controls {
    bottom: calc(258px + env(safe-area-inset-bottom));
  }

  .seat-counts {
    display: none;
  }

  .submit-btn {
    width: 100%;
  }
}
</style>
