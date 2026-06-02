<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { BrowserQRCodeSvgWriter } from '@zxing/browser'
import { getMe, getTickets, getWindowState } from '../api'
import type { Booking, TicketResponse, WindowState } from '../api'
import { useAuthStore } from '../stores/auth'

type TicketStage = WindowState['state'] | 'loading'

const stage = ref<TicketStage>('loading')
const bookings = ref<Booking[]>([])
const tickets = ref<TicketResponse[]>([])
const error = ref('')
const loading = ref(true)
const qrWriter = new BrowserQRCodeSvgWriter()
const auth = useAuthStore()

const canShowQR = computed(() => stage.value === 'locked' || stage.value === 'show_started')
const hasBookings = computed(() => bookings.value.length > 0 || tickets.value.length > 0)

onMounted(loadTicketPage)

async function loadTicketPage() {
  loading.value = true
  error.value = ''
  try {
    const [windowState, me] = await Promise.all([getWindowState(), getMe()])
    stage.value = windowState.state
    bookings.value = [me.booking, ...(me.proxy_bookings || [])].filter(Boolean) as Booking[]

    if (canShowQR.value) {
      try {
        const result = await getTickets()
        tickets.value = result.tickets
      } catch (e: any) {
        if (e?.response?.status !== 404) throw e
        tickets.value = []
      }
    }
  } catch (e: any) {
    error.value = e?.response?.data?.error || e?.response?.data?.message || '票券載入失敗'
  } finally {
    loading.value = false
  }
}

function qrSvg(token: string) {
  const svg = qrWriter.write(token, 190, 190)
  svg.setAttribute('aria-label', '入場 QR Code')
  return svg.outerHTML
}

function zoneLabel(zone?: string) {
  if (zone === 'left') return '左區'
  if (zone === 'middle') return '中區'
  if (zone === 'right') return '右區'
  return zone || '-'
}

function bookingSeatText(booking: Booking) {
  if (booking.zone && booking.row_no && booking.col_no) {
    return `${zoneLabel(booking.zone)} ${booking.row_no}排 ${booking.col_no}號`
  }
  return `座位 #${booking.seat_id}`
}

function bookingRelationLabel(booking: Booking) {
  return booking.student_id === auth.student?.student_id ? '本人座位' : '代約座位'
}

function bookingHint(booking: Booking) {
  if (booking.student_id === auth.student?.student_id) return '可前往選座頁查看位置、換座或取消。'
  return '僅顯示座位資訊；入場 QR Code 須由該學號本人登入查看。'
}

function ticketSeatText(ticket: TicketResponse) {
  return `${zoneLabel(ticket.seat.zone)} ${ticket.seat.row_no}排 ${ticket.seat.col_no}號`
}

function holderName(ticket: TicketResponse) {
  return ticket.holder.display_name || ticket.holder.student_id
}

function holderMeta(ticket: TicketResponse) {
  const parts = [
    ticket.holder.student_id,
    ticket.holder.class_code ? `${ticket.holder.class_code}班` : '',
    ticket.holder.seat_no_in_class ? `${ticket.holder.seat_no_in_class}號` : '',
  ].filter(Boolean)
  return parts.join(' · ')
}

function showTime(ticket?: TicketResponse) {
  const value = ticket?.show_start_at
  if (!value) return '演出時間待公布'
  return new Date(value).toLocaleString('zh-TW', { hour12: false })
}

function relationLabel(ticket: TicketResponse) {
  return ticket.owned_by_viewer ? '本人票券' : '代訂座位'
}

function bookingGuideRoute(booking: Booking) {
  return {
    path: '/seat-guide',
    query: {
      zone: booking.zone || '',
      row: booking.row_no ? String(booking.row_no) : '',
      col: booking.col_no ? String(booking.col_no) : '',
      student: booking.student_id,
    },
  }
}

function ticketGuideRoute(ticket: TicketResponse) {
  return {
    path: '/seat-guide',
    query: {
      zone: ticket.seat.zone,
      row: String(ticket.seat.row_no),
      col: String(ticket.seat.col_no),
      student: ticket.holder.student_id,
    },
  }
}
</script>

<template>
  <section class="ticket-page">
    <div class="ticket-backdrop" aria-hidden="true"></div>

    <div class="ticket-shell">
      <header class="ticket-hero">
        <p class="caption">票券資訊</p>
        <h1>La La Land</h1>
        <p>你的票券與入場 QR Code</p>
        <div v-if="!loading && stage !== 'before_open'" class="ticket-actions">
          <router-link to="/seats" class="btn-primary">管理座位</router-link>
          <button class="btn-outline" type="button" @click="loadTicketPage">重新整理</button>
        </div>
      </header>

      <div v-if="loading" class="state-panel">
        <div class="spinner"></div>
        <p>載入票券資訊中...</p>
      </div>

      <div v-else-if="error" class="state-panel">
        <p class="state-title">票務暫時無法載入</p>
        <p class="error-msg">{{ error }}</p>
        <button class="btn-outline" @click="loadTicketPage">重新整理</button>
      </div>

      <div v-else-if="stage === 'before_open'" class="closed-stage">
        <div class="closed-card">
          <p class="state-kicker">預約狀態</p>
          <h2>預約尚未開放</h2>
        <p>預約開放後，這裡會顯示你的座位與代訂座位。</p>
        </div>
      </div>

      <div v-else-if="!hasBookings" class="state-panel">
        <p class="state-title">目前沒有預約座位</p>
        <p>先完成座位預約，預約截止後會在這裡產生入場 QR Code。</p>
        <router-link v-if="stage === 'open'" to="/seats" class="btn-primary">前往預約</router-link>
        <p v-else class="muted-text">預約已截止，若有問題請聯絡現場工作人員。</p>
      </div>

      <div v-else-if="!canShowQR" class="ticket-grid pending-grid">
        <article v-for="booking in bookings" :key="booking.seat_id" class="pending-ticket">
          <span class="stub-label">{{ bookingRelationLabel(booking) }}</span>
          <h2>{{ booking.student_id }}</h2>
          <p>{{ bookingSeatText(booking) }}</p>
          <small>{{ bookingHint(booking) }}</small>
          <router-link v-if="booking.student_id === auth.student?.student_id" to="/seats" class="manage-link">
            查看或換座
          </router-link>
          <router-link :to="bookingGuideRoute(booking)" class="manage-link">
            座位指引
          </router-link>
        </article>
      </div>

      <div v-else-if="tickets.length === 0" class="state-panel">
        <p class="state-title">票券尚未產生</p>
        <p>已查到預約紀錄，但入場票尚未生成。請稍後重新整理，或聯絡現場工作人員。</p>
        <button class="btn-outline" @click="loadTicketPage">重新整理</button>
      </div>

      <div v-else class="ticket-grid">
        <article v-for="ticket in tickets" :key="ticket.holder.student_id" class="ticket-card">
          <div class="ticket-card-main">
            <div>
              <span class="stub-label">{{ relationLabel(ticket) }}</span>
              <h2>{{ holderName(ticket) }}</h2>
              <p>{{ holderMeta(ticket) }}</p>
            </div>
            <div class="seat-block">
              <span>座位</span>
              <strong>{{ ticketSeatText(ticket) }}</strong>
              <router-link :to="ticketGuideRoute(ticket)" class="seat-guide-link">座位指引</router-link>
            </div>
          </div>

          <div class="ticket-cut"></div>

          <div class="ticket-card-bottom">
            <div class="ticket-details">
              <p><span>劇目</span>{{ ticket.play_name }}</p>
              <p><span>場館</span>{{ ticket.venue }}</p>
              <p><span>時間</span>{{ showTime(ticket) }}</p>
              <p><span>版本</span>#{{ ticket.ticket_version }}</p>
            </div>
            <div v-if="ticket.owned_by_viewer && ticket.qr_token" class="qr-box" v-html="qrSvg(ticket.qr_token)"></div>
            <div v-else class="qr-private">
              <span>QR 僅本人可見</span>
              <p>代約人無法查看 QR Code，請對方使用本人學號登入。</p>
            </div>
          </div>
        </article>
      </div>
    </div>
  </section>
</template>

<style scoped>
.ticket-page {
  min-height: 100vh;
  position: relative;
  padding: clamp(24px, 5vw, 64px) 18px 110px;
  overflow: hidden;
}

.ticket-backdrop {
  position: fixed;
  inset: 0;
  z-index: -1;
  background:
    linear-gradient(90deg, rgba(3, 9, 33, 0.82), rgba(7, 18, 56, 0.54) 52%, rgba(77, 36, 49, 0.5)),
    url('/assets/images/lalaland-clean-hero.jpg') center / cover;
  filter: saturate(0.9);
}

.ticket-shell {
  width: min(1080px, 100%);
  margin: 0 auto;
}

.ticket-hero {
  margin-bottom: 28px;
}

.ticket-hero h1 {
  margin: 6px 0;
  font-family: 'Libre Bodoni', 'Bodoni 72 Smallcaps', serif;
  font-size: clamp(48px, 10vw, 112px);
  font-weight: 400;
  line-height: 0.9;
  color: #fff4cf;
}

.ticket-hero p:last-child {
  color: rgba(255, 244, 207, 0.76);
}

.ticket-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 12px;
  margin-top: 18px;
}

.ticket-actions .btn-primary,
.ticket-actions .btn-outline {
  min-height: 42px;
  padding: 10px 18px;
}

.ticket-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(min(100%, 420px), 1fr));
  gap: 18px;
}

.ticket-card,
.pending-ticket,
.state-panel,
.closed-card {
  border: 1px solid rgba(255, 244, 207, 0.18);
  background: rgba(5, 12, 37, 0.78);
  box-shadow: 0 24px 80px rgba(0, 0, 0, 0.32);
  backdrop-filter: blur(18px);
  -webkit-backdrop-filter: blur(18px);
}

.ticket-card {
  border-radius: 8px;
  overflow: hidden;
}

.ticket-card-main,
.ticket-card-bottom {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  gap: 18px;
  align-items: center;
  padding: 24px;
}

.ticket-card h2,
.pending-ticket h2,
.state-title,
.closed-card h2 {
  margin: 6px 0 4px;
  font-family: 'Libre Bodoni', 'Noto Serif TC', serif;
  font-size: clamp(28px, 5vw, 44px);
  font-weight: 400;
  line-height: 1;
  color: #fff4cf;
}

.ticket-card p,
.pending-ticket p,
.state-panel p,
.closed-card p {
  color: rgba(255, 244, 207, 0.74);
}

.stub-label,
.state-kicker {
  display: inline-flex;
  color: #f4c46f;
  font-size: 12px;
  letter-spacing: 0.16em;
  text-transform: uppercase;
}

.seat-block {
  min-width: 150px;
  padding: 14px 16px;
  border-left: 1px solid rgba(255, 244, 207, 0.18);
}

.seat-block span,
.ticket-details span {
  display: block;
  color: rgba(255, 244, 207, 0.52);
  font-size: 12px;
  letter-spacing: 0.12em;
  text-transform: uppercase;
}

.seat-block strong {
  display: block;
  margin-top: 4px;
  color: #fff4cf;
  font-size: 20px;
}

.seat-guide-link {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-height: 32px;
  margin-top: 10px;
  padding: 0 12px;
  border: 1px solid rgba(255, 244, 207, 0.2);
  border-radius: 999px;
  color: #fff4cf;
  background: rgba(255, 244, 207, 0.06);
  text-decoration: none;
  font-size: 12px;
}

.ticket-cut {
  height: 1px;
  margin: 0 18px;
  background: repeating-linear-gradient(90deg, rgba(255, 244, 207, 0.4) 0 8px, transparent 8px 17px);
}

.ticket-details {
  display: grid;
  gap: 12px;
}

.ticket-details p {
  margin: 0;
  overflow-wrap: anywhere;
}

.qr-box {
  width: 214px;
  height: 214px;
  display: grid;
  place-items: center;
  padding: 12px;
  border-radius: 6px;
  background: #fff;
}

.qr-box :deep(svg) {
  width: 190px;
  height: 190px;
  display: block;
}

.qr-private {
  width: 214px;
  min-height: 214px;
  display: grid;
  align-content: center;
  gap: 10px;
  padding: 18px;
  border-radius: 6px;
  border: 1px dashed rgba(255, 244, 207, 0.28);
  background: rgba(255, 244, 207, 0.06);
  text-align: center;
}

.qr-private span {
  color: #f4c46f;
  font-size: 13px;
  letter-spacing: 0.12em;
}

.qr-private p {
  margin: 0;
  font-size: 14px;
  color: rgba(255, 244, 207, 0.64);
}

.state-panel,
.closed-card {
  width: min(560px, 100%);
  min-height: 260px;
  display: grid;
  align-content: center;
  gap: 18px;
  padding: 34px;
  border-radius: 8px;
}

.closed-stage {
  min-height: 420px;
  display: grid;
  place-items: center;
  background: rgba(0, 0, 0, 0.42);
  filter: grayscale(0.1);
  border-radius: 8px;
}

.closed-card {
  text-align: center;
  background: rgba(10, 12, 20, 0.78);
}

.pending-ticket {
  position: relative;
  padding: 24px;
  border-radius: 8px;
  overflow: hidden;
}

.pending-ticket::after {
  content: '';
  position: absolute;
  inset: 0;
  background: linear-gradient(135deg, transparent, rgba(255, 244, 207, 0.08));
  pointer-events: none;
}

.pending-ticket small,
.muted-text {
  color: rgba(255, 244, 207, 0.56);
}

.manage-link {
  position: relative;
  z-index: 1;
  width: max-content;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-height: 38px;
  margin-top: 16px;
  padding: 0 14px;
  border: 1px solid rgba(255, 244, 207, 0.22);
  border-radius: 999px;
  color: #fff4cf;
  background: rgba(255, 244, 207, 0.06);
  text-decoration: none;
  font-size: 13px;
  letter-spacing: 0.04em;
}

.manage-link:hover {
  border-color: rgba(255, 214, 107, 0.5);
  background: rgba(255, 214, 107, 0.12);
}

.manage-link.compact {
  justify-self: center;
  margin-top: 4px;
}

@media (max-width: 680px) {
  .ticket-page {
    padding: 22px 12px 100px;
  }

  .ticket-backdrop {
    background:
      linear-gradient(180deg, rgba(3, 9, 33, 0.64), rgba(3, 9, 33, 0.9)),
      url('/assets/images/lalaland-clean-hero-mobile.jpg') center top / cover;
  }

  .ticket-card-main,
  .ticket-card-bottom {
    grid-template-columns: 1fr;
    padding: 20px;
  }

  .seat-block {
    min-width: 0;
    border-left: 0;
    border-top: 1px solid rgba(255, 244, 207, 0.18);
    padding: 14px 0 0;
  }

  .qr-box {
    width: min(100%, 246px);
    height: auto;
    aspect-ratio: 1;
    justify-self: center;
  }

  .qr-private {
    width: min(100%, 246px);
    min-height: 168px;
    justify-self: center;
  }

  .state-panel,
  .closed-card {
    padding: 26px 20px;
  }
}
</style>
