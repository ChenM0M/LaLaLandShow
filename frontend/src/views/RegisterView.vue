<script setup lang="ts">
import { computed, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useAuthStore } from '../stores/auth'

const router = useRouter()
const route = useRoute()
const auth = useAuthStore()
const classCode = ref('')
const seatNo = ref<number | null>(null)
const displayName = ref('')
const error = ref('')
const loading = ref(false)

const studentId = computed(() => auth.student?.student_id || 'STUDENT')
const classOptions = ['701', '702', '703', '704', '705', '801', '802', '803', '804', '805', '901', '902', '903', '904', '905', '1001', '1002', '1003', '1004', '1005', '1101', '1102', '1103', '1104', '1105', '1201', '1202', '1203', '1204', '1205']

async function submit() {
  if (!classOptions.includes(classCode.value.trim())) {
    error.value = '班級需為 701-705 到 1201-1205'
    return
  }
  if (!seatNo.value || seatNo.value < 1 || seatNo.value > 70) {
    error.value = '班級座號需介於 1 到 70'
    return
  }
  loading.value = true
  error.value = ''
  try {
    await auth.register({
      class_code: classCode.value.trim(),
      seat_no_in_class: seatNo.value,
      display_name: displayName.value.trim() || undefined,
    })
    const redirect = typeof route.query.redirect === 'string' ? route.query.redirect : '/seats'
    router.push(redirect)
  } catch (e: any) {
    error.value = e?.response?.data?.error || e?.response?.data?.message || '註冊失敗'
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <section class="register-page">
    <div class="poster-panel" aria-hidden="true"></div>

    <div class="register-shell">
      <div class="register-copy">
        <p class="caption">STUDENT PROFILE</p>
        <h1>入場資料</h1>
        <p>確認班級與座號後，即可進入選座；如未依實際資訊填寫，將無法正常預約。请正确登记班级座号否则无法正常开公假单，后果自负。</p>
      </div>

      <div class="mobile-hero-scene" aria-hidden="true"></div>

      <form class="profile-ticket" @submit.prevent="submit">
        <div class="ticket-ribbon">
          <span>VERIFY</span>
          <strong>{{ studentId }}</strong>
        </div>

        <div class="field-grid">
          <div class="form-group">
            <label for="class-code">班級代碼</label>
            <select id="class-code" v-model="classCode" autofocus>
              <option value="" disabled>選擇班級</option>
              <option v-for="option in classOptions" :key="option" :value="option">{{ option }}</option>
            </select>
          </div>

          <div class="form-group">
            <label for="seat-no">班級座號</label>
            <input id="seat-no" v-model.number="seatNo" type="number" min="1" max="70" placeholder="12" />
          </div>
        </div>

        <div class="form-group">
          <label for="display-name">顯示名稱</label>
          <input id="display-name" v-model="displayName" type="text" placeholder="可留空，預設使用學號" />
        </div>

        <p class="profile-note">請依本人實際班級、座號填寫；資料不符者可能無法完成預約或入場核驗。请正确登记班级座号否则无法正常开公假单，后果自负。</p>

        <p v-if="error" class="error-msg">{{ error }}</p>

        <button type="submit" class="btn-primary register-button" :disabled="loading">
          {{ loading ? '提交中...' : '完成資料' }}
        </button>
      </form>
    </div>
  </section>
</template>

<style scoped>
.register-page {
  min-height: 100vh;
  position: relative;
  display: grid;
  place-items: center;
  padding: clamp(22px, 5vw, 64px);
  overflow: hidden;
}

.poster-panel {
  position: fixed;
  inset: 0;
  z-index: -1;
  background:
    linear-gradient(90deg, rgba(3, 9, 34, 0.86) 0%, rgba(5, 17, 55, 0.62) 48%, rgba(53, 32, 54, 0.24) 100%),
    url('/assets/images/lalaland-clean-hero.jpg') center / cover;
}

.poster-panel::after {
  content: '';
  position: absolute;
  inset: 0;
  background:
    linear-gradient(180deg, rgba(0, 0, 0, 0.08), rgba(3, 9, 34, 0.5)),
    radial-gradient(circle at 18% 22%, rgba(244, 196, 111, 0.16), transparent 28rem);
}

.register-shell {
  width: min(1040px, 100%);
  display: grid;
  grid-template-columns: minmax(0, 1fr) minmax(360px, 460px);
  gap: clamp(28px, 6vw, 80px);
  align-items: end;
}

.register-copy {
  padding-bottom: 18px;
}

.register-copy h1 {
  margin: 8px 0 18px;
  font-family: 'Libre Bodoni', 'Noto Serif TC', serif;
  font-size: clamp(52px, 9vw, 112px);
  font-weight: 400;
  line-height: 0.95;
  color: #fff4cf;
}

.register-copy p:last-child {
  width: min(460px, 100%);
  color: rgba(255, 244, 207, 0.76);
  font-size: 17px;
}

.mobile-hero-scene {
  display: none;
}

.profile-ticket {
  position: relative;
  padding: 26px 26px 28px;
  border: 1px solid rgba(255, 244, 207, 0.22);
  border-radius: 8px;
  background: rgba(4, 11, 35, 0.78);
  box-shadow: 0 28px 90px rgba(0, 0, 0, 0.36);
  backdrop-filter: blur(18px);
  -webkit-backdrop-filter: blur(18px);
  overflow: hidden;
}

.profile-ticket::before,
.profile-ticket::after {
  content: '';
  position: absolute;
  top: 86px;
  width: 22px;
  height: 22px;
  border-radius: 50%;
  background: rgba(2, 7, 26, 0.96);
  border: 1px solid rgba(255, 244, 207, 0.16);
}

.profile-ticket::before { left: -12px; }
.profile-ticket::after { right: -12px; }

.ticket-ribbon {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: 16px;
  margin: -26px -26px 28px;
  padding: 18px 24px;
  background: rgba(255, 244, 207, 0.92);
  color: #081331;
}

.ticket-ribbon span {
  font-size: 12px;
  letter-spacing: 0.18em;
}

.ticket-ribbon strong {
  min-width: 0;
  font-family: 'Libre Bodoni', serif;
  font-size: clamp(26px, 5vw, 36px);
  font-weight: 400;
  line-height: 1;
  overflow-wrap: anywhere;
}

.field-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 18px;
}

.profile-ticket label {
  color: rgba(255, 244, 207, 0.62);
}

.profile-note {
  margin: 2px 0 14px;
  color: rgba(255, 244, 207, 0.68);
  font-size: 13px;
  line-height: 1.6;
}

.profile-ticket input,
.profile-ticket select {
  border-bottom-color: rgba(255, 244, 207, 0.34);
  color: #fff4cf;
  font-size: 26px;
  line-height: 1.15;
}

.profile-ticket input::placeholder {
  color: rgba(255, 244, 207, 0.22);
}

.register-button {
  width: 100%;
  margin-top: 6px;
  border-color: #fff4cf;
  background: #fff4cf;
  color: #081331;
}

@media (max-width: 760px) {
  .register-page {
    align-items: start;
    min-height: 100svh;
    padding: 18px 14px 132px;
  }

  .poster-panel {
    background:
      linear-gradient(180deg, rgba(3, 9, 34, 0.2), rgba(3, 9, 34, 0.74) 52%, rgba(3, 9, 34, 0.98)),
      url('/assets/images/lalaland-clean-hero-mobile.jpg') center top / cover;
  }

  .register-shell {
    grid-template-columns: 1fr;
    gap: 16px;
  }

  .register-copy {
    padding-top: clamp(78px, 18svh, 150px);
    padding-bottom: 0;
    position: relative;
    z-index: 2;
  }

  .register-copy h1 {
    margin-bottom: 12px;
    font-size: clamp(48px, 15vw, 68px);
  }

  .register-copy p:last-child {
    font-size: 15px;
  }

  .mobile-hero-scene {
    display: block;
    position: relative;
    z-index: 1;
    height: clamp(150px, 25svh, 220px);
    margin: -4px -14px 2px;
    background:
      linear-gradient(180deg, rgba(3, 9, 34, 0), rgba(3, 9, 34, 0.08) 42%, rgba(3, 9, 34, 0.74) 100%),
      url('/assets/images/lalaland-clean-hero-mobile.jpg') center 86% / 100% auto no-repeat;
    mask-image: linear-gradient(180deg, transparent 0%, #000 13%, #000 82%, transparent 100%);
    -webkit-mask-image: linear-gradient(180deg, transparent 0%, #000 13%, #000 82%, transparent 100%);
    pointer-events: none;
  }

  .profile-ticket {
    position: relative;
    z-index: 3;
    padding: 22px 20px 24px;
    background: rgba(4, 11, 35, 0.84);
  }

  .ticket-ribbon {
    margin: -22px -20px 24px;
  }

  .field-grid {
    gap: 14px;
  }

  .profile-ticket input,
  .profile-ticket select {
    font-size: 24px;
  }
}

@media (max-width: 430px) {
  .field-grid {
    grid-template-columns: 1fr;
    gap: 0;
  }
}

@media (max-width: 760px) and (max-height: 740px) {
  .register-page {
    padding-top: 14px;
  }

  .register-copy {
    padding-top: 0;
  }

  .register-copy h1 {
    font-size: clamp(46px, 14vw, 58px);
  }

  .mobile-hero-scene {
    height: 168px;
    background-position: center 84%;
  }
}
</style>
