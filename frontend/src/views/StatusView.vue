<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { getConfig, getWindowState } from '../api'
import type { PublicConfig, WindowState } from '../api'
import { useAuthStore } from '../stores/auth'

const config = ref<PublicConfig>({})
const windowState = ref<WindowState['state']>('before_open')
const error = ref('')
const now = ref(Date.now())
const auth = useAuthStore()

const stateCopy: Record<WindowState['state'], { label: string; tone: string; detail: string }> = {
  before_open: { label: '預約未開放', tone: 'warn', detail: '現在可以瀏覽劇目資訊，預約入口會在開放時間後進入選座。' },
  open: { label: '預約開放中', tone: 'good', detail: '學生可以登入、選座、換座或釋放座位。' },
  locked: { label: '預約已鎖定', tone: 'warn', detail: '不能新增或換座，已預約學生仍可查看票根並釋放座位。' },
  show_started: { label: '演出已開始', tone: 'bad', detail: '預約操作已凍結，僅保留票根和驗票核驗。' },
}

const milestones = computed(() => [
  { label: '開放預約', value: config.value.booking_open_at },
  { label: '鎖定預約', value: config.value.booking_lock_at },
  { label: '演出開始', value: config.value.show_start_at || config.value.show_date },
])

const target = computed(() => {
  if (windowState.value === 'before_open') return config.value.booking_open_at
  if (windowState.value === 'open') return config.value.booking_lock_at
  if (windowState.value === 'locked') return config.value.show_start_at || config.value.show_date
  return ''
})

const countdown = computed(() => {
  if (!target.value) return '--:--:--:--'
  const diff = new Date(target.value).getTime() - now.value
  if (!Number.isFinite(diff) || diff <= 0) return '00:00:00:00'
  const d = Math.floor(diff / 86400000)
  const h = Math.floor((diff % 86400000) / 3600000)
  const m = Math.floor((diff % 3600000) / 60000)
  const s = Math.floor((diff % 60000) / 1000)
  return `${String(d).padStart(2, '0')}:${String(h).padStart(2, '0')}:${String(m).padStart(2, '0')}:${String(s).padStart(2, '0')}`
})

const reservationTarget = computed(() => (auth.token ? '/seats' : '/login'))
const reservationLabel = computed(() => (auth.token ? '選座管理' : '學生預約'))

function formatTime(value?: string) {
  if (!value) return '未設定'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return date.toLocaleString('zh-TW', { hour12: false })
}

function formatDate(value?: string) {
  if (!value) return 'TBA'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return `${date.getMonth() + 1}/${date.getDate()}/${date.getFullYear()}`
}

async function refresh() {
  error.value = ''
  try {
    const [cfg, state] = await Promise.all([getConfig(), getWindowState()])
    config.value = cfg
    windowState.value = state.state
  } catch (e: any) {
    error.value = e?.response?.data?.error || '狀態暫時不可用'
  }
}

onMounted(() => {
  refresh()
  setInterval(() => { now.value = Date.now() }, 1000)
  setInterval(refresh, 30000)
})
</script>

<template>
  <main class="cinematic-bg">
    <div class="spotlight"></div>
    <div class="content-wrapper">
      <p v-if="error" class="error-msg">{{ error }}</p>
      
      <!-- Hollywood Clapperboard -->
      <div class="clapperboard">
        <div class="clapper-stick top-stick">
          <div class="chevron-stripes"></div>
        </div>
        <div class="clapper-stick bottom-stick">
          <div class="chevron-stripes"></div>
        </div>
        
        <div class="clapper-body">
          <div class="board-header">
            <div class="field prod">
              <span class="label">PROD.</span>
              <span class="value">{{ config.play_name || 'La La Land' }}</span>
            </div>
          </div>
          
          <div class="board-grid">
            <div class="field scene">
              <span class="label">SCENE</span>
              <span class="value">{{ stateCopy[windowState].label }}</span>
            </div>
            <div class="field take">
              <span class="label">TAKE</span>
              <span class="value">01</span>
            </div>
            <div class="field date">
              <span class="label">DATE</span>
              <span class="value">{{ formatDate(config.show_date) }}</span>
            </div>
          </div>
          
          <div class="board-footer">
            <div class="field director">
              <span class="label">LOC.</span>
              <span class="value">{{ config.venue || 'TBA' }}</span>
            </div>
            <div class="field camera">
              <span class="label">NEXT</span>
              <span class="value">{{ target ? formatTime(target) : 'TBA' }}</span>
            </div>
          </div>
          
          <!-- Digital Timecode Countdown -->
          <div class="timecode-display">
            <div class="timecode-label">REMAINING TIME TO NEXT NODE</div>
            <div class="timecode">{{ countdown }}</div>
          </div>
        </div>
      </div>

      <!-- Cinematic Timeline (Film Strip Style) -->
      <div class="film-strip">
        <div v-for="item in milestones" :key="item.label" class="film-frame" :class="{'active': target === item.value}">
          <div class="sprocket-holes top"></div>
          <div class="frame-content">
            <span class="status-indicator" :class="stateCopy[windowState].tone"></span>
            <div class="frame-text">
              <span class="frame-label">{{ item.label }}</span>
              <strong class="frame-time">{{ formatTime(item.value) }}</strong>
            </div>
          </div>
          <div class="sprocket-holes bottom"></div>
        </div>
      </div>

      <!-- Cinematic Actions -->
      <div class="cinematic-actions">
        <router-link class="btn-ticket primary" :to="reservationTarget">
          <span class="ticket-stub"></span>
          {{ reservationLabel }}
        </router-link>
        <router-link class="btn-ticket secondary" to="/ticket">
          <span class="ticket-stub"></span>
          查看票券
        </router-link>
      </div>
    </div>
  </main>
</template>

<style scoped>
@import url('https://fonts.googleapis.com/css2?family=Share+Tech+Mono&display=swap');

.cinematic-bg {
  min-height: 100vh;
  background: radial-gradient(circle at center, #11133f 0%, #040827 100%);
  position: relative;
  overflow: hidden;
  display: flex;
  justify-content: center;
  align-items: center;
  padding: 40px 16px;
  font-family: 'Inter', sans-serif;
}

.spotlight {
  position: absolute;
  top: -20vh;
  left: 50%;
  transform: translateX(-50%);
  width: 100vw;
  height: 100vh;
  background: radial-gradient(ellipse at top, rgba(255, 216, 95, 0.1) 0%, transparent 60%);
  pointer-events: none;
}

.content-wrapper {
  position: relative;
  z-index: 10;
  width: 100%;
  max-width: 720px;
  display: flex;
  flex-direction: column;
  gap: 40px;
}

/* --- Clapperboard --- */
.clapperboard {
  transform: rotate(-1deg);
  transition: transform 0.3s ease;
  filter: drop-shadow(0 20px 30px rgba(0,0,0,0.6));
}
.clapperboard:hover {
  transform: rotate(0deg) scale(1.02);
}

.clapper-stick {
  height: 32px;
  background: #fff;
  position: relative;
  overflow: hidden;
}
.top-stick {
  transform-origin: left bottom;
  transform: rotate(-5deg);
  margin-bottom: 2px;
  z-index: 2;
  border-radius: 12px 12px 6px 6px;
}
.bottom-stick {
  border-bottom: 2px solid #333;
  border-radius: 6px 6px 0 0;
}

.chevron-stripes {
  width: 150%;
  height: 100%;
  background: repeating-linear-gradient(
    -45deg,
    #111,
    #111 20px,
    #fff 20px,
    #fff 40px
  );
}

.clapper-body {
  padding: 24px;
  background: #1a1a1a;
  background-image: url('data:image/svg+xml;utf8,<svg viewBox="0 0 200 200" xmlns="http://www.w3.org/2000/svg"><filter id="noise"><feTurbulence type="fractalNoise" baseFrequency="0.65" numOctaves="3" stitchTiles="stitch"/></filter><rect width="100%" height="100%" filter="url(%23noise)" opacity="0.05"/></svg>');
  color: #fff;
  border-radius: 0 0 12px 12px;
  box-shadow: inset 0 0 0 2px #333;
}

.board-header, .board-footer {
  display: flex;
  border-bottom: 2px solid rgba(255,255,255,0.1);
  margin-bottom: 16px;
  padding-bottom: 16px;
}
.board-footer {
  border-bottom: none;
  border-top: 2px solid rgba(255,255,255,0.1);
  margin-top: 16px;
  padding-top: 16px;
  margin-bottom: 0;
}

.board-grid {
  display: grid;
  grid-template-columns: 2fr 1fr 1.5fr;
  gap: 16px;
}

.field {
  display: flex;
  flex-direction: column;
}
.board-header .field { width: 100%; }
.board-footer .field { flex: 1; }
.board-footer .field:not(:last-child) { border-right: 2px solid rgba(255,255,255,0.1); padding-right: 16px; margin-right: 16px; }

.board-grid .field:not(:last-child) {
  border-right: 2px solid rgba(255,255,255,0.1);
  padding-right: 16px;
}

.label {
  font-size: 11px;
  font-weight: 700;
  letter-spacing: 0.15em;
  color: rgba(255,255,255,0.5);
  margin-bottom: 4px;
}

.value {
  font-size: 20px;
  font-weight: 600;
  letter-spacing: 0.05em;
  font-family: 'Courier New', Courier, monospace;
}
.prod .value { font-size: 28px; text-transform: uppercase; font-family: 'Inter', sans-serif; font-weight: 800; }

/* Digital Timecode */
.timecode-display {
  margin-top: 24px;
  background: #000;
  border-radius: 8px;
  padding: 16px;
  text-align: center;
  border: 2px inset #333;
  box-shadow: inset 0 0 20px rgba(0,0,0,0.8);
}
.timecode-label {
  font-size: 10px;
  color: #ff3333;
  letter-spacing: 0.2em;
  margin-bottom: 8px;
  opacity: 0.8;
}
.timecode {
  font-family: 'Share Tech Mono', monospace;
  font-size: 42px;
  color: #ff3333;
  text-shadow: 0 0 10px rgba(255, 51, 51, 0.6), 0 0 20px rgba(255, 51, 51, 0.4);
  letter-spacing: 0.1em;
  line-height: 1;
}

/* --- Film Strip Timeline --- */
.film-strip {
  display: flex;
  flex-direction: column;
  gap: 2px;
  background: #000;
  padding: 4px;
  border-radius: 8px;
}
.film-frame {
  background: rgba(255,255,255,0.05);
  display: flex;
  align-items: stretch;
  position: relative;
}
.film-frame.active {
  background: rgba(255, 216, 95, 0.1);
}
.sprocket-holes {
  width: 24px;
  background-image: radial-gradient(circle, #000 40%, transparent 45%);
  background-size: 100% 16px;
  background-position: center;
}
.frame-content {
  flex: 1;
  padding: 16px;
  display: flex;
  align-items: center;
  gap: 16px;
  border-left: 1px solid rgba(255,255,255,0.1);
  border-right: 1px solid rgba(255,255,255,0.1);
}
.status-indicator {
  width: 12px;
  height: 12px;
  border-radius: 50%;
  background: var(--muted);
}
.status-indicator.good { background: var(--green); box-shadow: 0 0 10px var(--green); }
.status-indicator.warn { background: var(--gold); box-shadow: 0 0 10px var(--gold); }
.status-indicator.bad { background: var(--red); box-shadow: 0 0 10px var(--red); }

.frame-text {
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.frame-label {
  font-size: 13px;
  color: var(--muted);
  letter-spacing: 0.1em;
}
.frame-time {
  font-size: 15px;
  color: var(--cream);
  font-family: 'Share Tech Mono', monospace;
}

/* --- Cinematic Actions --- */
.cinematic-actions {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 16px;
}
.btn-ticket {
  position: relative;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 16px;
  border-radius: 8px;
  font-size: 14px;
  font-weight: 700;
  letter-spacing: 0.1em;
  text-decoration: none;
  overflow: hidden;
  transition: all 0.3s;
}
.btn-ticket::before, .btn-ticket::after {
  content: '';
  position: absolute;
  top: 50%;
  width: 20px;
  height: 20px;
  background: #040827; /* match body bg */
  border-radius: 50%;
  transform: translateY(-50%);
}
.btn-ticket::before { left: -10px; }
.btn-ticket::after { right: -10px; }

.btn-ticket.primary {
  background: var(--gold);
  color: #000;
}
.btn-ticket.primary:hover {
  background: #fff;
  transform: translateY(-2px);
  box-shadow: 0 10px 20px rgba(255, 216, 95, 0.3);
}

.btn-ticket.secondary {
  background: rgba(255,255,255,0.1);
  color: var(--cream);
  border: 1px solid rgba(255,255,255,0.2);
}
.btn-ticket.secondary:hover {
  background: rgba(255,255,255,0.15);
  transform: translateY(-2px);
}

@media (max-width: 640px) {
  .cinematic-bg { padding: 20px 12px calc(100px + env(safe-area-inset-bottom)) 12px; }
  .content-wrapper { gap: 24px; }
  
  .clapper-body { padding: 16px; }
  .board-header, .board-footer { margin-bottom: 12px; padding-bottom: 12px; }
  .board-footer { margin-top: 12px; padding-top: 12px; }
  
  .board-grid { grid-template-columns: 1fr 1fr; gap: 12px; }
  .take { display: none; }
  
  .value { font-size: 16px; }
  .prod .value { font-size: 20px; }
  .label { font-size: 10px; }
  
  .timecode-display { margin-top: 16px; padding: 12px; }
  .timecode { font-size: 28px; }
  
  .cinematic-actions { grid-template-columns: 1fr; gap: 12px; }
  .btn-ticket { padding: 12px; }
}
</style>
