<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { getMe, getTickets } from '../api'
import type { Booking, TicketResponse } from '../api'

interface GuideSeat {
  student_id?: string
  zone?: string
  row_no?: number
  col_no?: number
}

// Pan & Zoom state
const zoom = ref(1)
const panX = ref(0)
const panY = ref(0)
const isPanning = ref(false)
const panStart = ref({ x: 0, y: 0, panX: 0, panY: 0 })

const activePointers = new Map<number, { x: number; y: number }>()
const initialPinchDist = ref(0)
const initialPinchZoom = ref(1)
const initialPinchCenter = ref({ x: 0, y: 0, contentX: 0, contentY: 0 })

function startPan(e: PointerEvent) {
  activePointers.set(e.pointerId, { x: e.clientX, y: e.clientY })
  try { (e.currentTarget as HTMLElement).setPointerCapture(e.pointerId) } catch {}
  if (activePointers.size === 2) {
    const pts = Array.from(activePointers.values())
    const dx = pts[0].x - pts[1].x
    const dy = pts[0].y - pts[1].y
    initialPinchDist.value = Math.hypot(dx, dy)
    initialPinchZoom.value = zoom.value
    const cx = (pts[0].x + pts[1].x) / 2
    const cy = (pts[0].y + pts[1].y) / 2
    const rect = (e.currentTarget as HTMLElement).getBoundingClientRect()
    initialPinchCenter.value = {
      x: cx, y: cy,
      contentX: (cx - rect.left - panX.value) / zoom.value,
      contentY: (cy - rect.top - panY.value) / zoom.value
    }
    isPanning.value = false
  } else {
    isPanning.value = true
    panStart.value = { x: e.clientX, y: e.clientY, panX: panX.value, panY: panY.value }
  }
}

function movePan(e: PointerEvent) {
  if (!activePointers.has(e.pointerId)) return
  activePointers.set(e.pointerId, { x: e.clientX, y: e.clientY })
  if (activePointers.size === 2) {
    const pts = Array.from(activePointers.values())
    const dx = pts[0].x - pts[1].x
    const dy = pts[0].y - pts[1].y
    const dist = Math.hypot(dx, dy)
    const newZoom = initialPinchZoom.value * (dist / initialPinchDist.value)
    zoom.value = Math.max(0.4, Math.min(3, newZoom))
    const cx = (pts[0].x + pts[1].x) / 2
    const cy = (pts[0].y + pts[1].y) / 2
    const rect = (e.currentTarget as HTMLElement).getBoundingClientRect()
    panX.value = cx - rect.left - initialPinchCenter.value.contentX * zoom.value
    panY.value = cy - rect.top - initialPinchCenter.value.contentY * zoom.value
  } else if (isPanning.value) {
    panX.value = panStart.value.panX + (e.clientX - panStart.value.x)
    panY.value = panStart.value.panY + (e.clientY - panStart.value.y)
  }
}

function endPan(e: PointerEvent) {
  activePointers.delete(e.pointerId)
  if (activePointers.size < 2) initialPinchDist.value = 0
  if (activePointers.size === 0) isPanning.value = false
}

function zoomWheel(e: WheelEvent) {
  e.preventDefault()
  const delta = e.deltaY > 0 ? -0.1 : 0.1
  const newZoom = Math.max(0.4, Math.min(3, zoom.value + delta))
  const rect = (e.currentTarget as HTMLElement).getBoundingClientRect()
  const contentX = (e.clientX - rect.left - panX.value) / zoom.value
  const contentY = (e.clientY - rect.top - panY.value) / zoom.value
  zoom.value = newZoom
  panX.value = e.clientX - rect.left - contentX * zoom.value
  panY.value = e.clientY - rect.top - contentY * zoom.value
}

const route = useRoute()
const router = useRouter()
const loading = ref(true)
const error = ref('')
const seat = ref<GuideSeat | null>(null)

const ROWS = 21
const ROW_Y_START = 80
const ROW_PITCH = 32
const COL_PITCH = 28
const SEAT_SIZE = 20

const zones = [
  { id: 'left', label: '左區', cols: 4, startX: 100 },
  { id: 'middle', label: '中區', cols: 10, startX: 304 },
  { id: 'right', label: '右區', cols: 4, startX: 676 }
]

const activeZone = computed(() => seat.value?.zone || '')
const zoneInfo = computed(() => zones.find(z => z.id === activeZone.value))
const zoneText = computed(() => zoneInfo.value?.label || activeZone.value || '未取得區域')
const seatText = computed(() => {
  if (!seat.value?.row_no || !seat.value?.col_no) return '座位資訊待確認'
  return `${zoneText.value} ${seat.value.row_no}排 ${seat.value.col_no}號`
})

const isLeftAisleRoute = computed(() => {
  if (activeZone.value === 'left') return true
  if (activeZone.value === 'middle' && (seat.value?.col_no || 1) <= 5) return true
  return false
})

const aisleHint = computed(() => isLeftAisleRoute.value ? '左側過道' : '右側過道')

const sideHint = computed(() => {
  if (activeZone.value === 'left') return '入口在右下，舞台在前方。沿前方過道到左區，再往上找。'
  if (activeZone.value === 'right') return '入口在右下，舞台在前方。沿右側過道往上找。'
  if (activeZone.value === 'middle') return '入口在右下，舞台在前方。沿前方過道到中區，再往上找。'
  return '到入口出示票券，現場人員會帶你找位置。'
})

function getSeatY(row: number) {
  return ROW_Y_START + (ROWS - row) * ROW_PITCH
}

const guidePath = computed(() => {
  if (!seat.value?.row_no || !seat.value?.col_no || !activeZone.value) return ''
  const r = seat.value.row_no
  const c = seat.value.col_no
  const zConfig = zones.find(x => x.id === activeZone.value)
  if (!zConfig) return ''

  const targetX = zConfig.startX + (c - 1) * COL_PITCH + (SEAT_SIZE / 2)
  const targetY = getSeatY(r) + (SEAT_SIZE / 2)

  const aisleX = isLeftAisleRoute.value ? 254 : 626
  
  // Start at Entrance (830, 750), go to Aisle bottom (aisleX, 750), up to row (aisleX, targetY), to seat (targetX, targetY)
  return `M 830 750 L ${aisleX} 750 L ${aisleX} ${targetY} L ${targetX} ${targetY}`
})

function numberParam(value: unknown) {
  const next = Number(value)
  return Number.isFinite(next) && next > 0 ? next : undefined
}

function querySeat(): GuideSeat | null {
  const zone = typeof route.query.zone === 'string' ? route.query.zone : ''
  const rowNo = numberParam(route.query.row)
  const colNo = numberParam(route.query.col)
  if (!zone || !rowNo || !colNo) return null
  return {
    student_id: typeof route.query.student === 'string' ? route.query.student : undefined,
    zone,
    row_no: rowNo,
    col_no: colNo,
  }
}

function fromTicket(ticket: TicketResponse): GuideSeat {
  return {
    student_id: ticket.holder.student_id,
    zone: ticket.seat.zone,
    row_no: ticket.seat.row_no,
    col_no: ticket.seat.col_no,
  }
}

function fromBooking(booking: Booking): GuideSeat {
  return {
    student_id: booking.student_id,
    zone: booking.zone,
    row_no: booking.row_no,
    col_no: booking.col_no,
  }
}

function goBackToTickets() {
  router.push('/ticket')
}

function isTargetSeat(z: string, r: number, c: number) {
  return activeZone.value === z && seat.value?.row_no === r && seat.value?.col_no === c
}

function isDefaultLocked(z: string, r: number) {
  return (z === 'middle' && r <= 2) || (z === 'left' && r === 1)
}

function isRouteSeat(z: string, r: number, c: number) {
  if (z !== activeZone.value || r !== seat.value?.row_no) return false
  if (c === seat.value?.col_no) return false
  if (isLeftAisleRoute.value) {
    return c < (seat.value?.col_no || 1)
  } else {
    return c > (seat.value?.col_no || 1)
  }
}

function seatClass(z: string, r: number, c: number) {
  return {
    'target-seat': isTargetSeat(z, r, c),
    'route-seat': isRouteSeat(z, r, c),
    'locked-seat': isDefaultLocked(z, r),
    'ghost-seat': activeZone.value && activeZone.value !== z,
  }
}

onMounted(async () => {
  loading.value = true
  error.value = ''
  const fromQuery = querySeat()
  if (fromQuery) {
    seat.value = fromQuery
    loading.value = false
    return
  }

  try {
    try {
      const tickets = await getTickets()
      const ownTicket = tickets.tickets.find((item) => item.owned_by_viewer) || tickets.tickets[0]
      if (ownTicket) {
        seat.value = fromTicket(ownTicket)
        return
      }
    } catch {}

    const me = await getMe()
    const ownBooking = me.booking || me.proxy_bookings?.[0]
    if (ownBooking) {
      seat.value = fromBooking(ownBooking)
      return
    }
    error.value = '目前沒有可顯示的座位。'
  } catch (e: any) {
    error.value = e?.response?.data?.error || '座位指引暫時無法載入。'
  } finally {
    loading.value = false
  }
})
</script>

<template>
  <main class="guide-page">
    <div class="guide-backdrop" aria-hidden="true"></div>
    <section class="guide-shell">
      <nav class="guide-nav">
        <button class="back-button" type="button" @click="goBackToTickets">
          <svg viewBox="0 0 24 24" width="18" height="18" stroke="currentColor" stroke-width="2" fill="none" stroke-linecap="round" stroke-linejoin="round"><line x1="19" y1="12" x2="5" y2="12"></line><polyline points="12 19 5 12 12 5"></polyline></svg>
          返回票券
        </button>
      </nav>

      <div v-if="loading" class="guide-panel">
        <div class="spinner"></div>
        <p class="caption">SEAT GUIDE</p>
        <h1>正在讀取座位</h1>
      </div>

      <div v-else-if="error" class="guide-panel error-panel">
        <p class="caption">SEAT GUIDE</p>
        <h1>座位指引</h1>
        <p>{{ error }}</p>
        <router-link to="/ticket" class="guide-link">查看票券</router-link>
      </div>

      <template v-else>
        <header class="guide-hero">
          <p class="caption">SEAT LOCATOR</p>
          <h1>{{ seatText }}</h1>
          <p>{{ sideHint }} <br class="mobile-only"/>系統已為你繪製專屬引導線 (白線)，預計經由<strong>{{ aisleHint }}</strong>抵達第 {{ seat?.row_no }} 排。</p>
        </header>

        <section class="map-container" aria-label="座位指引圖"
          @pointerdown="startPan"
          @pointermove="movePan"
          @pointerup="endPan"
          @pointerleave="endPan"
          @pointercancel="endPan"
          @wheel="zoomWheel">
          <div class="zoom-hint-badge">支援雙指縮放與拖曳</div>
          <svg viewBox="0 0 880 920" class="seat-map-svg" preserveAspectRatio="xMidYMid meet">
            <g :transform="`translate(${panX}, ${panY}) scale(${zoom})`" class="zoom-layer" transform-origin="center">
              <defs>
              <filter id="glow" x="-20%" y="-20%" width="140%" height="140%">
                <feGaussianBlur stdDeviation="6" result="blur" />
                <feComposite in="SourceGraphic" in2="blur" operator="over" />
              </filter>
              <linearGradient id="stageGrad" x1="0" y1="0" x2="0" y2="1">
                <stop offset="0%" stop-color="#fff4cf" stop-opacity="0.8" />
                <stop offset="100%" stop-color="#f6b23f" stop-opacity="0.2" />
              </linearGradient>
            </defs>

              <!-- Stage -->
              <path d="M 60 840 Q 440 800 820 840 L 820 880 L 60 880 Z" fill="url(#stageGrad)" class="stage-shape"/>
              <text x="440" y="865" class="stage-text" text-anchor="middle">STAGE / 舞台</text>

            <!-- Zone Headers -->
            <g v-for="z in zones" :key="`header-${z.id}`">
              <text :x="z.startX + (z.cols * COL_PITCH) / 2" y="30" class="zone-label" text-anchor="middle">{{ z.label }}</text>
            </g>

            <!-- Row Labels (Left & Right) -->
            <g v-for="row in ROWS" :key="`label-${row}`">
              <text x="70" :y="getSeatY(row) + 14" class="row-label" text-anchor="end">{{ row }}</text>
              <text x="810" :y="getSeatY(row) + 14" class="row-label" text-anchor="start">{{ row }}</text>
            </g>

            <!-- Seats Grid -->
            <g v-for="z in zones" :key="`grid-${z.id}`">
              <g v-for="row in ROWS" :key="`row-${row}`">
                <rect v-for="col in z.cols" :key="`seat-${col}`"
                      :x="z.startX + (col - 1) * COL_PITCH"
                      :y="getSeatY(row)"
                      :width="SEAT_SIZE"
                      :height="SEAT_SIZE"
                      rx="4"
                      class="guide-seat"
                      :class="seatClass(z.id, row, col)" />
              </g>
            </g>

            <!-- Entrance Marker -->
            <g class="entry-marker" transform="translate(830, 750)">
              <rect x="-26" y="-14" width="52" height="28" rx="6" class="entry-box" />
              <text x="0" y="4" class="entry-label" text-anchor="middle">入口</text>
            </g>

            <!-- Route Path -->
            <path :d="guidePath" id="guideRoutePath" class="route-line" pathLength="100" stroke-linejoin="round" />

            <!-- Moving Arrow -->
            <path v-if="guidePath" d="M -8 -6 L 6 0 L -8 6 Z" class="route-arrow">
              <animateMotion dur="2.5s" repeatCount="indefinite" rotate="auto">
                <mpath href="#guideRoutePath" />
              </animateMotion>
            </path>
            
            </g>
          </svg>
          
          <div v-if="activeZone" class="target-floating-badge">
            <span class="badge-label">YOUR SEAT</span>
            <strong>{{ seat?.row_no }}排 {{ seat?.col_no }}號</strong>
          </div>
        </section>
      </template>
    </section>
  </main>
</template>

<style scoped>
.guide-page {
  min-height: 100svh;
  position: relative;
  padding: clamp(20px, 5vw, 40px) 16px max(34px, env(safe-area-inset-bottom));
  color: #fff4cf;
}

.guide-backdrop {
  position: fixed;
  inset: 0;
  z-index: -1;
  background:
    radial-gradient(circle at 50% -10%, rgba(39, 62, 126, 0.4), transparent 60%),
    linear-gradient(180deg, rgba(3, 7, 24, 0.8), rgba(6, 10, 32, 0.98)),
    url('/assets/images/lalaland-clean-hero.jpg') center / cover;
  filter: saturate(0.9);
}

.guide-shell {
  width: min(880px, 100%);
  margin: 0 auto;
}

.guide-nav {
  margin-bottom: 24px;
}

.back-button {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  min-height: 42px;
  padding: 0 16px 0 12px;
  border: 1px solid rgba(255, 244, 207, 0.15);
  border-radius: 999px;
  background: rgba(255, 255, 255, 0.03);
  color: #fff4cf;
  font-weight: 600;
  font-size: 14px;
  cursor: pointer;
  transition: all 0.2s;
  backdrop-filter: blur(12px);
  -webkit-backdrop-filter: blur(12px);
}

.back-button:hover {
  background: rgba(255, 255, 255, 0.08);
  border-color: rgba(255, 244, 207, 0.3);
}

.guide-hero {
  margin-bottom: 32px;
}

.caption {
  margin: 0;
  color: #f4c46f;
  font-size: 11px;
  letter-spacing: 0.2em;
  text-transform: uppercase;
  opacity: 0.8;
}

.guide-hero h1 {
  margin: 8px 0;
  font-family: 'Libre Bodoni', 'Noto Serif TC', serif;
  font-size: clamp(32px, 8vw, 64px);
  font-weight: 400;
  line-height: 1.1;
  color: #fff;
}

.guide-hero p {
  max-width: 38rem;
  color: rgba(255, 255, 255, 0.6);
  line-height: 1.6;
  font-size: clamp(14px, 4vw, 16px);
  margin-top: 12px;
}

.guide-hero p strong {
  color: #fff4cf;
  font-weight: 600;
}

.mobile-only {
  display: none;
}

.guide-panel {
  min-height: 340px;
  display: flex;
  flex-direction: column;
  justify-content: center;
  align-items: flex-start;
  gap: 12px;
  padding: 40px;
  border: 1px solid rgba(255, 255, 255, 0.08);
  border-radius: 16px;
  background: rgba(10, 14, 39, 0.5);
  backdrop-filter: blur(16px);
  -webkit-backdrop-filter: blur(16px);
}

.error-panel h1 {
  font-family: 'Libre Bodoni', serif;
  font-size: 32px;
  margin: 0;
}

.guide-link {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-height: 44px;
  padding: 0 20px;
  border: 1px solid transparent;
  border-radius: 999px;
  background: #f6b23f;
  color: #110803;
  text-decoration: none;
  font-weight: 700;
  font-size: 14px;
  margin-top: 8px;
  transition: opacity 0.2s;
}

.guide-link:hover {
  opacity: 0.9;
}

.guide-link.subtle {
  background: rgba(255, 255, 255, 0.05);
  border: 1px solid rgba(255, 255, 255, 0.1);
  color: #fff4cf;
}

.guide-link.subtle:hover {
  background: rgba(255, 255, 255, 0.1);
}

/* SVG Map Architecture */
.map-container {
  position: relative;
  background: rgba(4, 7, 20, 0.6);
  border: 1px solid rgba(255, 255, 255, 0.06);
  border-radius: 20px;
  padding: 30px;
  box-shadow: 0 30px 60px rgba(0, 0, 0, 0.5);
  backdrop-filter: blur(16px);
  -webkit-backdrop-filter: blur(16px);
  overflow: hidden;
  touch-action: none;
  cursor: grab;
}

.map-container:active {
  cursor: grabbing;
}

.zoom-hint-badge {
  position: absolute;
  top: 12px;
  left: 50%;
  transform: translateX(-50%);
  background: rgba(255, 255, 255, 0.1);
  color: rgba(255, 255, 255, 0.7);
  padding: 6px 12px;
  border-radius: 999px;
  font-size: 11px;
  font-weight: 600;
  letter-spacing: 0.1em;
  pointer-events: none;
  z-index: 10;
}

.seat-map-svg {
  width: 100%;
  height: auto;
  display: block;
}

.zoom-layer {
  transition: transform 0.1s cubic-bezier(0.2, 0, 0, 1);
  will-change: transform;
}

.zone-label {
  fill: rgba(255, 255, 255, 0.4);
  font-size: 16px;
  font-weight: 600;
  letter-spacing: 0.1em;
}

.row-label {
  fill: rgba(255, 255, 255, 0.25);
  font-size: 13px;
  font-variant-numeric: tabular-nums;
}

.stage-text {
  fill: #f6b23f;
  font-size: 20px;
  font-weight: 800;
  letter-spacing: 0.3em;
  opacity: 0.9;
}

.guide-seat {
  fill: rgba(255, 255, 255, 0.12);
  transition: all 0.4s ease;
}

.guide-seat.ghost-seat {
  fill: rgba(255, 255, 255, 0.04);
}

.guide-seat.locked-seat {
  fill: rgba(0, 0, 0, 0.4);
  stroke: rgba(255, 255, 255, 0.08);
  stroke-width: 1;
}

.guide-seat.target-seat {
  fill: #f6b23f;
  filter: drop-shadow(0 0 8px rgba(246, 178, 63, 0.6));
  animation: targetSeatPop 0.8s 1.2s cubic-bezier(0.175, 0.885, 0.32, 1.275) both;
}

.guide-seat.route-seat {
  fill: rgba(246, 178, 63, 0.4);
  stroke: rgba(246, 178, 63, 0.8);
  stroke-width: 1.5;
}



.entry-box {
  fill: rgba(255, 255, 255, 0.1);
  stroke: rgba(255, 255, 255, 0.4);
  stroke-width: 1.5;
}

.entry-label {
  fill: #fff;
  font-size: 13px;
  font-weight: 700;
  letter-spacing: 0.1em;
}

.route-line {
  fill: none;
  stroke: #fff;
  stroke-width: 4;
  filter: url(#glow);
  stroke-dasharray: 100;
  stroke-dashoffset: 100;
  animation: drawLine 1.5s 0.3s cubic-bezier(0.4, 0, 0.2, 1) forwards;
}

.route-arrow {
  fill: #f6b23f;
  filter: drop-shadow(0 0 4px #f6b23f);
}

.target-floating-badge {
  position: absolute;
  right: 30px;
  bottom: 30px;
  background: rgba(246, 178, 63, 0.15);
  border: 1px solid rgba(246, 178, 63, 0.4);
  padding: 14px 20px;
  border-radius: 12px;
  backdrop-filter: blur(12px);
  -webkit-backdrop-filter: blur(12px);
  display: flex;
  flex-direction: column;
  gap: 4px;
  box-shadow: 0 10px 30px rgba(0, 0, 0, 0.3);
  animation: badgePop 0.6s 1.6s cubic-bezier(0.175, 0.885, 0.32, 1.275) both;
}

.badge-label {
  font-size: 10px;
  color: #f4c46f;
  letter-spacing: 0.15em;
  font-weight: 700;
}

.target-floating-badge strong {
  font-size: 18px;
  color: #fff;
  white-space: nowrap;
}

.guide-actions {
  display: flex;
  margin-top: 24px;
}

/* Animations */
@keyframes drawLine {
  to { stroke-dashoffset: 0; }
}

@keyframes targetSeatPop {
  0% { transform: scale(0.5); opacity: 0; }
  100% { transform: scale(1); opacity: 1; }
}

@keyframes badgePop {
  0% { transform: translateY(20px); opacity: 0; }
  100% { transform: translateY(0); opacity: 1; }
}

@keyframes ringPulse {
  0% { transform: scale(0.8); opacity: 1; }
  100% { transform: scale(2); opacity: 0; }
}

@media (max-width: 680px) {
  .guide-page {
    padding: 16px 12px max(24px, env(safe-area-inset-bottom));
  }
  
  .guide-hero h1 {
    font-size: clamp(28px, 9vw, 40px);
  }

  .mobile-only {
    display: inline;
  }

  .map-container {
    padding: 16px 8px;
    border-radius: 16px;
  }

  .target-floating-badge {
    right: 16px;
    bottom: 16px;
    padding: 12px 16px;
  }
  
  .target-floating-badge strong {
    font-size: 16px;
  }
}
</style>
