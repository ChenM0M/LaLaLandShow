<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { BrowserQRCodeReader } from '@zxing/browser'
import type { IScannerControls } from '@zxing/browser'
import api, { checkerLogin, checkerStats, checkIn, lookupStudent, verifyQR } from '../api'
import type { CheckerBooking, CheckerStats, CheckerVerification, CheckinResult, Staff } from '../api'

const router = useRouter()
const username = ref(localStorage.getItem('staffUser') || 'checker')
const password = ref('')
const staff = ref<Staff | null>(localStorage.getItem('staffToken') ? {
  username: localStorage.getItem('staffUser') || 'checker',
  role: (localStorage.getItem('staffRole') as Staff['role']) || 'checker',
} : null)
const stats = ref<CheckerStats | null>(null)
const qrToken = ref('')
const studentId = ref('')
const verification = ref<CheckerVerification | null>(null)
const lastEvent = ref<CheckinResult | null>(null)
const loading = ref(false)
const error = ref('')
const videoEl = ref<HTMLVideoElement | null>(null)
const scannerActive = ref(false)
const scannerReady = ref(false)
const autoApprove = ref(true)
const scanMessage = ref('')
const lastScannedToken = ref('')
const scannerSupported = ref(typeof navigator !== 'undefined' && !!navigator.mediaDevices?.getUserMedia)
let scannerControls: IScannerControls | null = null
let codeReader: BrowserQRCodeReader | null = null
let statsTimer: ReturnType<typeof window.setInterval> | null = null

const canForce = computed(() => /^\d{7}$/.test(studentId.value))
const lastEventGuideRoute = computed(() => {
  const booking = lastEvent.value?.booking
  if (!booking) return null
  return {
    path: '/seat-guide',
    query: {
      zone: booking.zone,
      row: String(booking.row_no),
      col: String(booking.col_no),
      student: booking.student_id,
    },
  }
})

function seatText(booking?: CheckerBooking) {
  if (!booking) return '無預約座位'
  return `${booking.zone} 區 ${booking.row_no} 排 ${booking.col_no} 號`
}

async function login() {
  loading.value = true
  error.value = ''
  try {
    const res = await checkerLogin(username.value, password.value)
    localStorage.setItem('staffToken', res.token)
    localStorage.setItem('staffUser', res.staff.username)
    localStorage.setItem('staffRole', res.staff.role)
    staff.value = res.staff
    password.value = ''
    await refreshStats()
  } catch (e: any) {
    error.value = e?.response?.data?.error || '驗票登入失敗'
  } finally {
    loading.value = false
  }
}

function logout() {
  if (!window.confirm('確定要登出驗票後台嗎？')) return
  stopScanner()
  localStorage.removeItem('staffToken')
  localStorage.removeItem('staffUser')
  localStorage.removeItem('staffRole')
  staff.value = null
  verification.value = null
  lastEvent.value = null
}

async function startScanner() {
  error.value = ''
  scanMessage.value = ''
  if (!scannerSupported.value) {
    error.value = '瀏覽器不支援網頁掃描，請改用手動輸入或貼上 QR 內容。'
    return
  }
  try {
    if (!videoEl.value) return
    codeReader = new BrowserQRCodeReader(undefined, {
      delayBetweenScanAttempts: 120,
      delayBetweenScanSuccess: 700,
    })
    scannerActive.value = true
    scannerReady.value = false
    scanMessage.value = autoApprove.value ? '自動核准模式：對準票券' : '手動核准模式：對準票券'
    scannerControls = await codeReader.decodeFromVideoDevice(undefined, videoEl.value, (result) => {
      scannerReady.value = true
      const value = result?.getText()?.trim()
      if (value && value !== lastScannedToken.value && !loading.value) {
        handleScannedQR(value)
      }
    })
  } catch (e: any) {
    scannerReady.value = false
    scannerActive.value = false
    error.value = e?.name === 'NotAllowedError'
      ? '相機權限被拒絕，請允許使用相機。'
      : '無法啟動相機，請確認裝置有可用鏡頭。'
  }
}

function stopScanner() {
  scannerActive.value = false
  scannerReady.value = false
  scannerControls?.stop()
  scannerControls = null
  codeReader = null
  if (videoEl.value) videoEl.value.srcObject = null
}

async function handleScannedQR(value: string) {
  lastScannedToken.value = value
  qrToken.value = value
  scanMessage.value = '正在核驗...'
  await verifyByQR()
  
  if (verification.value && autoApprove.value) {
    scanMessage.value = '票券有效，記錄中...'
    await approve('qr')
    scanMessage.value = lastEvent.value?.event?.is_first_scan ? '✅ 首次入場' : '⚠️ 重複掃描'
    window.setTimeout(() => {
      verification.value = null
      error.value = ''
      lastScannedToken.value = ''
      if (scannerActive.value) scanMessage.value = '等待下一張票'
    }, 1800)
  } else if (verification.value) {
    scanMessage.value = '請手動確認並核准。'
  } else {
    scanMessage.value = '❌ 無效票券'
    window.setTimeout(() => {
      error.value = ''
      lastScannedToken.value = ''
      if (scannerActive.value) scanMessage.value = '等待下一張票'
    }, 1800)
  }
}

async function refreshStats() {
  if (!localStorage.getItem('staffToken')) return
  try { stats.value = await checkerStats() } catch {}
}

async function verifyByQR() {
  if (!qrToken.value.trim()) return
  loading.value = true
  error.value = ''
  lastEvent.value = null
  try {
    verification.value = await verifyQR(qrToken.value.trim())
    studentId.value = verification.value.booking.student_id
  } catch (e: any) {
    verification.value = null
    error.value = e?.response?.data?.error || 'QR 碼無效或已過期'
  } finally {
    loading.value = false
  }
}

async function lookup() {
  const normalizedStudentId = studentId.value.trim()
  if (!/^11[0-5][1-4]\d{3}$/.test(normalizedStudentId)) {
    error.value = '學號格式需為 110-115 + 1-4 + 後三位數字'
    return
  }
  loading.value = true
  error.value = ''
  lastEvent.value = null
  try {
    verification.value = await lookupStudent(normalizedStudentId)
  } catch (e: any) {
    verification.value = null
    error.value = e?.response?.status === 404 ? '該學號無預約，仍可強制核准入場' : (e?.response?.data?.error || '查詢失敗')
  } finally {
    loading.value = false
  }
}

async function approve(method: 'qr' | 'manual' | 'force_no_booking') {
  loading.value = true
  error.value = ''
  try {
    const payload = method === 'qr'
      ? { method, qr_token: qrToken.value.trim() }
      : { method, student_id: studentId.value.trim() }
    lastEvent.value = await checkIn(payload)
    await refreshStats()
  } catch (e: any) {
    error.value = e?.response?.data?.error || '核准失敗'
  } finally {
    loading.value = false
  }
}

async function downloadRoster() {
  error.value = ''
  try {
    const res = await api.get('/checker/export-roster.csv', { responseType: 'blob' })
    const url = URL.createObjectURL(res.data)
    const a = document.createElement('a')
    a.href = url
    a.download = 'lalaland-checkin-roster.csv'
    a.click()
    URL.revokeObjectURL(url)
  } catch (e: any) {
    error.value = e?.response?.data?.error || '下載失敗'
  }
}

onMounted(() => {
  refreshStats()
  statsTimer = window.setInterval(refreshStats, 30000)
})

onUnmounted(() => {
  stopScanner()
  if (statsTimer) {
    window.clearInterval(statsTimer)
    statsTimer = null
  }
})
</script>

<template>
  <main class="page-shell check-page">
    <section v-if="!staff" class="page-center">
      <div class="card login-card">
        <div class="glow-bg"></div>
        <p class="caption">SYSTEM TERMINAL</p>
        <h1 class="h2">驗票登入</h1>
        <form @submit.prevent="login">
          <div class="form-group">
            <label>Staff ID</label>
            <input v-model="username" autocomplete="username" placeholder="Enter ID" />
          </div>
          <div class="form-group">
            <label>Password</label>
            <input v-model="password" type="password" autocomplete="current-password" placeholder="••••••••" />
          </div>
          <p v-if="error" class="error-msg">{{ error }}</p>
          <button class="btn-primary full-width" :disabled="loading">{{ loading ? 'AUTHENTICATING...' : 'ACCESS TERMINAL' }}</button>
        </form>
      </div>
    </section>

    <section v-else class="check-container">
      <header class="check-header">
        <div>
          <p class="caption">ENTRY TERMINAL</p>
          <h1 class="h2">入場核驗</h1>
        </div>
        <div class="header-actions">
          <button class="icon-btn" @click="downloadRoster" title="匯出名單">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"></path><polyline points="7 10 12 15 17 10"></polyline><line x1="12" y1="15" x2="12" y2="3"></line></svg>
          </button>
          <button class="icon-btn" @click="logout" title="登出">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4"></path><polyline points="16 17 21 12 16 7"></polyline><line x1="21" y1="12" x2="9" y2="12"></line></svg>
          </button>
        </div>
      </header>

      <div class="hud-metrics">
        <div class="hud-metric">
          <span class="hud-label">已入場 (IN)</span>
          <strong class="hud-value">{{ stats?.checked_in ?? '-' }}</strong>
        </div>
        <div class="hud-metric">
          <span class="hud-label">總票數 (TOTAL)</span>
          <strong class="hud-value">{{ stats?.total ?? '-' }}</strong>
        </div>
        <div class="hud-metric highlight-metric">
          <span class="hud-label">效率 (RATE/MIN)</span>
          <strong class="hud-value">{{ stats?.last_minute_rate ?? '-' }}</strong>
        </div>
      </div>

      <div class="check-grid">
        <div class="scanner-section card">
          <div class="scanner-frame" :class="{ active: scannerActive, ready: scannerReady }">
            <video ref="videoEl" playsinline muted></video>
            
            <div v-if="!scannerActive" class="scanner-placeholder">
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5"><rect x="3" y="3" width="18" height="18" rx="2" ry="2"></rect><circle cx="8.5" cy="8.5" r="1.5"></circle><polyline points="21 15 16 10 5 21"></polyline></svg>
              <span>{{ scannerSupported ? '相機待機中' : '不支援網頁相機' }}</span>
            </div>

            <div class="scan-ui" v-if="scannerActive">
              <div class="scan-reticle"></div>
              <div class="scan-laser" v-if="scannerReady && !verification && !error"></div>
            </div>

            <transition name="hud-pop">
              <div v-if="verification" class="hud-result" :class="verification.booking.checked_in_before ? 'result-warn' : 'result-good'">
                <div class="result-icon">{{ verification.booking.checked_in_before ? '⚠️' : '✅' }}</div>
                <div class="result-title">{{ verification.booking.checked_in_before ? '重複入場' : '首次入場' }}</div>
                <div class="result-seat">{{ seatText(verification.booking) }}</div>
                <div class="result-id">{{ verification.booking.student_id }}</div>
              </div>
            </transition>
            
            <transition name="hud-pop">
              <div v-if="!verification && error && scannerActive" class="hud-result result-bad">
                 <div class="result-icon">❌</div>
                 <div class="result-title">無效票券</div>
                 <div class="result-msg">{{ error }}</div>
              </div>
            </transition>
          </div>
          
          <div class="scanner-controls">
            <button v-if="!scannerActive" class="btn-primary full-width" :disabled="loading" @click="startScanner">
              啟動掃描器
            </button>
            <button v-else class="btn-outline full-width" @click="stopScanner">
              暫停掃描
            </button>
            <div class="control-row">
              <label class="scan-toggle">
                <input v-model="autoApprove" type="checkbox" />
                <span>自動核准有效票券</span>
              </label>
              <span class="scan-message" :class="{ 'has-msg': !!scanMessage }">{{ scanMessage || 'READY' }}</span>
            </div>
            <div class="manual-approve-actions" v-if="verification && !autoApprove">
              <button class="btn-outline" @click="verification = null; qrToken = ''">取消</button>
              <button class="btn-primary success" @click="approve('qr')">確認核准</button>
            </div>
          </div>
        </div>

        <div class="side-panel">
          <div class="card stack">
            <div class="card-header">
              <p class="caption">MANUAL ENTRY</p>
              <h3 class="h3">學號快查</h3>
            </div>
            
            <div class="input-row">
              <input v-model="studentId" class="large-input" maxlength="7" placeholder="1121001" inputmode="numeric" @keyup.enter="lookup" />
              <button class="btn-primary" :disabled="loading || studentId.length !== 7" @click="lookup">查詢</button>
            </div>
            
            <transition name="fade">
              <div v-if="verification && !scannerActive" class="manual-result">
                <div class="m-status" :class="verification.booking.checked_in_before ? 'warn' : 'good'">
                  {{ verification.booking.checked_in_before ? '已掃描過' : '可入場' }}
                </div>
                <div class="m-info">
                  <strong>{{ seatText(verification.booking) }}</strong>
                  <span>班級: {{ verification.booking.class_code || '-' }} | 座號: {{ verification.booking.seat_no_in_class || '-' }}</span>
                </div>
                <button class="btn-primary full-width success" :disabled="loading" @click="approve('manual')">核准入場</button>
              </div>
            </transition>
            
            <transition name="fade">
              <div class="manual-actions" v-if="!verification && error && !scannerActive && canForce">
                 <p class="error-msg" style="margin-bottom: 12px; font-size: 14px;">{{ error }}</p>
                 <button class="btn-outline full-width warn" :disabled="loading" @click="approve('force_no_booking')">強制臨場核准 (無預約)</button>
              </div>
            </transition>
          </div>
          
          <div v-if="lastEvent" class="card mini-log">
             <div class="log-indicator"></div>
             <div class="log-details">
               <strong>{{ lastEvent.event.is_first_scan ? '已記錄：首次入場' : '已記錄：重複掃描' }}</strong>
               <span class="muted">{{ new Date(lastEvent.event.scanned_at).toLocaleTimeString('zh-TW', { hour12: false }) }} · ID: #{{ lastEvent.event.id }}</span>
             </div>
             <button
               v-if="lastEventGuideRoute"
               class="guide-mini-btn"
               type="button"
               @click="router.push(lastEventGuideRoute)"
             >
               座位指引
             </button>
          </div>
          
          <div class="card stack fallback-card" v-if="!scannerSupported || (!scannerActive && !verification && !studentId)">
             <p class="caption">TOKEN FALLBACK</p>
             <textarea v-model="qrToken" placeholder="貼上票根 qr_token..."></textarea>
             <button class="btn-outline full-width" :disabled="loading || !qrToken.trim()" @click="verifyByQR">手動驗證 Token</button>
          </div>
        </div>
      </div>
    </section>
  </main>
</template>

<style scoped>
.check-page {
  padding: 16px;
  max-width: 1000px;
  min-height: 100vh;
}

.login-card {
  width: 100%;
  max-width: 400px;
  position: relative;
  overflow: hidden;
}

.glow-bg {
  position: absolute;
  top: -50%;
  left: -50%;
  width: 200%;
  height: 200%;
  background: radial-gradient(circle at center, rgba(39, 215, 223, 0.1) 0%, transparent 50%);
  pointer-events: none;
}

.check-container {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.check-header {
  display: flex;
  justify-content: space-between;
  align-items: flex-end;
  padding-bottom: 12px;
  border-bottom: 1px solid var(--border);
}

.header-actions {
  display: flex;
  gap: 8px;
}

.icon-btn {
  background: transparent;
  border: 1px solid var(--border);
  color: var(--cream);
  width: 40px;
  height: 40px;
  border-radius: var(--radius-md);
  display: grid;
  place-items: center;
  cursor: pointer;
  transition: all 0.2s;
}

.icon-btn:hover {
  background: rgba(255, 255, 255, 0.1);
  border-color: var(--gold);
}

.icon-btn svg {
  width: 20px;
  height: 20px;
}

/* HUD Metrics */
.hud-metrics {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: 12px;
}

.hud-metric {
  background: rgba(10, 14, 39, 0.6);
  border: 1px solid rgba(255, 255, 255, 0.1);
  border-radius: var(--radius-md);
  padding: 12px 16px;
  display: flex;
  flex-direction: column;
  justify-content: center;
  backdrop-filter: blur(8px);
}

.highlight-metric {
  border-color: rgba(246, 178, 63, 0.3);
  background: linear-gradient(180deg, rgba(246, 178, 63, 0.05) 0%, transparent 100%);
}

.hud-label {
  font-size: 11px;
  color: rgba(255, 255, 255, 0.5);
  text-transform: uppercase;
  letter-spacing: 0.1em;
  margin-bottom: 4px;
}

.hud-value {
  font-family: 'Inter', monospace;
  font-size: 28px;
  color: var(--cream);
  font-weight: 600;
  line-height: 1;
}

.highlight-metric .hud-value {
  color: var(--gold);
}

/* Layout Grid */
.check-grid {
  display: grid;
  grid-template-columns: 1.2fr 1fr;
  gap: 20px;
  align-items: start;
}

/* Scanner */
.scanner-section {
  display: flex;
  flex-direction: column;
  gap: 16px;
  padding: 16px;
}

.scanner-frame {
  position: relative;
  width: 100%;
  aspect-ratio: 4 / 3;
  background: #050510;
  border-radius: var(--radius-lg);
  overflow: hidden;
  border: 1px solid rgba(255, 255, 255, 0.1);
  box-shadow: inset 0 0 40px rgba(0, 0, 0, 0.8);
}

.scanner-frame video {
  width: 100%;
  height: 100%;
  object-fit: cover;
  opacity: 0;
  transition: opacity 0.3s;
}

.scanner-frame.ready video {
  opacity: 1;
}

.scanner-placeholder {
  position: absolute;
  inset: 0;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 12px;
  color: rgba(255, 255, 255, 0.3);
}

.scanner-placeholder svg {
  width: 48px;
  height: 48px;
  opacity: 0.5;
}

.scan-ui {
  position: absolute;
  inset: 0;
  pointer-events: none;
}

/* Corner Brackets */
.scan-reticle {
  position: absolute;
  inset: 15%;
  background: 
    linear-gradient(to right, var(--cyan) 3px, transparent 3px) 0 0,
    linear-gradient(to bottom, var(--cyan) 3px, transparent 3px) 0 0,
    linear-gradient(to left, var(--cyan) 3px, transparent 3px) 100% 0,
    linear-gradient(to bottom, var(--cyan) 3px, transparent 3px) 100% 0,
    linear-gradient(to right, var(--cyan) 3px, transparent 3px) 0 100%,
    linear-gradient(to top, var(--cyan) 3px, transparent 3px) 0 100%,
    linear-gradient(to left, var(--cyan) 3px, transparent 3px) 100% 100%,
    linear-gradient(to top, var(--cyan) 3px, transparent 3px) 100% 100%;
  background-repeat: no-repeat;
  background-size: 24px 24px;
  opacity: 0.8;
}

/* Scanning Laser */
.scan-laser {
  position: absolute;
  left: 15%;
  right: 15%;
  height: 2px;
  background: var(--cyan);
  box-shadow: 0 0 10px var(--cyan), 0 0 20px var(--cyan);
  animation: scan 2s linear infinite;
  opacity: 0.7;
}

@keyframes scan {
  0% { top: 15%; }
  50% { top: 85%; }
  100% { top: 15%; }
}

/* HUD Overlay Results */
.hud-result {
  position: absolute;
  inset: 16px;
  background: rgba(10, 14, 39, 0.9);
  backdrop-filter: blur(12px);
  border-radius: var(--radius-md);
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  text-align: center;
  padding: 24px;
  border: 2px solid;
  z-index: 10;
  gap: 8px;
}

.result-good {
  border-color: #4ade80;
  color: #4ade80;
  box-shadow: inset 0 0 60px rgba(74, 222, 128, 0.15);
}

.result-warn {
  border-color: #fbbf24;
  color: #fbbf24;
  box-shadow: inset 0 0 60px rgba(251, 191, 36, 0.15);
}

.result-bad {
  border-color: #f87171;
  color: #f87171;
  box-shadow: inset 0 0 60px rgba(248, 113, 113, 0.15);
}

.result-icon {
  font-size: 48px;
  line-height: 1;
  margin-bottom: 8px;
}

.result-title {
  font-size: 24px;
  font-weight: 700;
  letter-spacing: 0.1em;
}

.result-seat {
  font-size: 20px;
  color: var(--cream);
  background: rgba(255,255,255,0.1);
  padding: 4px 16px;
  border-radius: 999px;
  margin-top: 4px;
}

.result-id {
  font-family: monospace;
  font-size: 16px;
  opacity: 0.8;
  margin-top: 8px;
}

.result-msg {
  color: var(--cream);
  font-size: 16px;
}

/* Animations */
.hud-pop-enter-active { animation: pop-in 0.3s cubic-bezier(0.175, 0.885, 0.32, 1.275); }
.hud-pop-leave-active { animation: pop-out 0.2s ease-in; }

@keyframes pop-in {
  0% { transform: scale(0.9); opacity: 0; }
  100% { transform: scale(1); opacity: 1; }
}
@keyframes pop-out {
  0% { transform: scale(1); opacity: 1; }
  100% { transform: scale(1.05); opacity: 0; }
}

/* Controls */
.scanner-controls {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.control-row {
  display: flex;
  justify-content: space-between;
  align-items: center;
  background: rgba(255,255,255,0.03);
  padding: 10px 14px;
  border-radius: var(--radius-md);
  border: 1px solid rgba(255,255,255,0.05);
}

.scan-message {
  font-size: 12px;
  color: var(--muted);
  font-family: monospace;
}
.scan-message.has-msg {
  color: var(--cyan);
}

.manual-approve-actions {
  display: flex;
  gap: 12px;
}
.manual-approve-actions button {
  flex: 1;
}

/* Side Panel */
.side-panel {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.input-row {
  display: flex;
  gap: 8px;
}

.large-input {
  flex: 1;
  font-size: 20px;
  letter-spacing: 0.1em;
  padding: 12px 16px;
  background: rgba(0,0,0,0.2);
  border: 1px solid var(--border);
  border-radius: var(--radius-md);
  color: var(--gold);
}
.large-input:focus {
  border-color: var(--gold);
}

.manual-result {
  background: rgba(255,255,255,0.03);
  border-radius: var(--radius-md);
  padding: 16px;
  display: flex;
  flex-direction: column;
  gap: 12px;
  border: 1px solid rgba(255,255,255,0.05);
}

.m-status {
  display: inline-block;
  padding: 4px 10px;
  border-radius: var(--radius-sm);
  font-size: 12px;
  font-weight: 600;
  align-self: flex-start;
}
.m-status.good { background: rgba(74, 222, 128, 0.15); color: #4ade80; border: 1px solid #4ade80; }
.m-status.warn { background: rgba(251, 191, 36, 0.15); color: #fbbf24; border: 1px solid #fbbf24; }

.m-info {
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.m-info strong {
  font-size: 18px;
  color: var(--cream);
}
.m-info span {
  font-size: 13px;
  color: var(--muted);
}

.mini-log {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 12px 16px;
}

.log-indicator {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: #4ade80;
  box-shadow: 0 0 8px #4ade80;
}

.log-details {
  display: flex;
  flex-direction: column;
  flex: 1;
  min-width: 0;
}
.log-details strong {
  font-size: 14px;
  color: var(--cream);
}
.log-details span {
  font-size: 12px;
  font-family: monospace;
}

.fallback-card textarea {
  min-height: 80px;
  font-size: 12px;
  font-family: monospace;
}

.guide-mini-btn {
  min-height: 34px;
  flex: 0 0 auto;
  padding: 0 12px;
  border: 1px solid rgba(246, 178, 63, 0.34);
  border-radius: var(--radius-md);
  background: rgba(246, 178, 63, 0.1);
  color: var(--gold);
  font-size: 12px;
  font-weight: 800;
  cursor: pointer;
}

.guide-mini-btn:hover {
  border-color: var(--gold);
  background: rgba(246, 178, 63, 0.18);
}

.full-width {
  width: 100%;
}

button.success {
  background: rgba(74, 222, 128, 0.1);
  color: #4ade80;
  border-color: rgba(74, 222, 128, 0.4);
}
button.success:hover:not(:disabled) {
  background: rgba(74, 222, 128, 0.2);
  border-color: #4ade80;
}
button.warn {
  background: rgba(248, 113, 113, 0.1);
  color: #f87171;
  border-color: rgba(248, 113, 113, 0.4);
}
button.warn:hover:not(:disabled) {
  background: rgba(248, 113, 113, 0.2);
  border-color: #f87171;
}

/* Mobile Adaptation (RWD) */
@media (max-width: 860px) {
  .check-page {
    padding: 12px;
  }
  
  .check-grid {
    grid-template-columns: 1fr;
    gap: 16px;
  }

  .hud-metrics {
    gap: 8px;
  }

  .hud-metric {
    padding: 8px 12px;
  }

  .hud-value {
    font-size: 22px;
  }

  .scanner-frame {
    aspect-ratio: 4 / 3;
    max-height: 50vh;
  }

  .scan-reticle {
    inset: 10%;
  }

  .result-icon {
    font-size: 40px;
  }

  .result-title {
    font-size: 20px;
  }

  .result-seat {
    font-size: 18px;
  }
  
  .control-row {
    flex-direction: column;
    align-items: flex-start;
    gap: 8px;
  }

  .check-header {
    flex-wrap: wrap;
    gap: 12px;
  }
}

@media (max-width: 480px) {
  .check-page {
    padding: 10px 8px;
  }

  .hud-metrics {
    grid-template-columns: repeat(2, 1fr);
    gap: 6px;
  }

  .hud-metric:last-child {
    grid-column: 1 / -1;
  }

  .hud-metric {
    padding: 8px 10px;
  }

  .hud-label {
    font-size: 10px;
    letter-spacing: 0.06em;
  }

  .hud-value {
    font-size: 20px;
  }

  .highlight-metric .hud-value {
    font-size: 22px;
  }

  .check-header {
    padding-bottom: 10px;
  }

  .scanner-frame {
    aspect-ratio: 1 / 1;
    max-height: 55vh;
  }

  .result-icon {
    font-size: 36px;
  }

  .result-title {
    font-size: 18px;
  }

  .result-seat {
    font-size: 16px;
    padding: 3px 12px;
  }

  .result-id {
    font-size: 14px;
  }

  .large-input {
    font-size: 18px;
    padding: 10px 12px;
  }

  .input-row {
    flex-direction: column;
    gap: 10px;
  }

  .input-row .btn-primary {
    width: 100%;
  }

  .scan-toggle {
    min-height: 44px;
    display: flex;
    align-items: center;
    gap: 10px;
  }

  .scan-toggle input[type="checkbox"] {
    width: 20px;
    height: 20px;
    min-width: 20px;
  }

  .manual-approve-actions {
    flex-direction: column;
    gap: 10px;
  }

  .mini-log {
    padding: 10px 12px;
    gap: 10px;
  }

  .log-details strong {
    font-size: 13px;
  }

  .log-details span {
    font-size: 11px;
  }

  .fallback-card textarea {
    min-height: 60px;
  }

  .scanner-section {
    padding: 12px;
  }

  .scanner-controls {
    gap: 10px;
  }
}
</style>
