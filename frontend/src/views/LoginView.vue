<script setup lang="ts">
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { useAuthStore } from '../stores/auth'

const router = useRouter()
const auth = useAuthStore()
const studentId = ref('')
const error = ref('')
const loading = ref(false)

async function submit() {
  if (!/^\d{7}$/.test(studentId.value)) {
    error.value = '請輸入 7 位學號'
    return
  }
  loading.value = true
  error.value = ''
  try {
    await auth.login(studentId.value)
    router.push(auth.needRegister ? '/register' : '/seats')
  } catch (e: any) {
    error.value = e?.response?.data?.error || e?.response?.data?.message || '登入失敗，請檢查學號'
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <section class="login-page">
    <div class="poster-panel" aria-hidden="true"></div>

    <div class="login-stage">
      <div class="login-copy">
        <p class="caption">STUDENT ACCESS</p>
        <h1>La La Land</h1>
        <p>輸入學號即可預約座位與查看票券。</p>
      </div>

      <div class="mobile-hero-scene" aria-hidden="true"></div>

      <form class="admit-form" @submit.prevent="submit">
        <div class="admit-top">
          <span>ADMIT</span>
          <strong>ONE</strong>
        </div>

        <label for="student-id">學號</label>
        <input
          id="student-id"
          v-model="studentId"
          inputmode="numeric"
          pattern="[0-9]*"
          type="text"
          maxlength="7"
          placeholder="1234567"
          autofocus
        />
        <p v-if="error" class="error-msg">{{ error }}</p>

        <button type="submit" class="btn-primary login-button" :disabled="loading">
          {{ loading ? '登入中...' : '進入票務' }}
        </button>
      </form>
    </div>
  </section>
</template>

<style scoped>
.login-page {
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
    linear-gradient(90deg, rgba(3, 9, 34, 0.84) 0%, rgba(5, 17, 55, 0.58) 48%, rgba(53, 32, 54, 0.2) 100%),
    url('/assets/images/lalaland-clean-hero.jpg') center / cover;
}

.poster-panel::after {
  content: '';
  position: absolute;
  inset: 0;
  background:
    linear-gradient(180deg, rgba(0, 0, 0, 0.1), rgba(3, 9, 34, 0.42)),
    radial-gradient(circle at 20% 20%, rgba(244, 196, 111, 0.16), transparent 28rem);
}

.login-stage {
  width: min(1040px, 100%);
  display: grid;
  grid-template-columns: minmax(0, 1fr) minmax(320px, 420px);
  gap: clamp(28px, 6vw, 80px);
  align-items: end;
}

.login-copy {
  padding-bottom: 18px;
}

.login-copy h1 {
  margin: 8px 0 18px;
  font-family: 'Libre Bodoni', 'Bodoni 72 Smallcaps', serif;
  font-size: clamp(58px, 12vw, 138px);
  font-weight: 400;
  line-height: 0.88;
  color: #fff4cf;
}

.login-copy p:last-child {
  width: min(460px, 100%);
  color: rgba(255, 244, 207, 0.76);
  font-size: 17px;
}

.mobile-hero-scene {
  display: none;
}

.admit-form {
  position: relative;
  padding: 26px 26px 28px;
  border: 1px solid rgba(255, 244, 207, 0.22);
  border-radius: 8px;
  background: rgba(4, 11, 35, 0.76);
  box-shadow: 0 28px 90px rgba(0, 0, 0, 0.36);
  backdrop-filter: blur(18px);
  -webkit-backdrop-filter: blur(18px);
  overflow: hidden;
}

.admit-form::before,
.admit-form::after {
  content: '';
  position: absolute;
  top: 82px;
  width: 22px;
  height: 22px;
  border-radius: 50%;
  background: rgba(2, 7, 26, 0.96);
  border: 1px solid rgba(255, 244, 207, 0.16);
}

.admit-form::before { left: -12px; }
.admit-form::after { right: -12px; }

.admit-top {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: 16px;
  margin: -26px -26px 28px;
  padding: 18px 24px;
  background: rgba(255, 244, 207, 0.92);
  color: #081331;
}

.admit-top span {
  font-size: 12px;
  letter-spacing: 0.18em;
}

.admit-top strong {
  font-family: 'Libre Bodoni', serif;
  font-size: 34px;
  font-weight: 400;
  line-height: 1;
}

.admit-form label {
  color: rgba(255, 244, 207, 0.62);
}

.admit-form input {
  margin-bottom: 10px;
  border-bottom-color: rgba(255, 244, 207, 0.34);
  color: #fff4cf;
  font-size: clamp(28px, 8vw, 48px);
  line-height: 1.1;
  letter-spacing: 0.08em;
}

.admit-form input::placeholder {
  color: rgba(255, 244, 207, 0.2);
}

.login-button {
  width: 100%;
  margin-top: 20px;
  border-color: #fff4cf;
  background: #fff4cf;
  color: #081331;
}

@media (max-width: 760px) {
  .login-page {
    align-items: start;
    min-height: 100svh;
    padding: 18px 14px 132px;
  }

  .poster-panel {
    background:
      linear-gradient(180deg, rgba(3, 9, 34, 0.2), rgba(3, 9, 34, 0.72) 52%, rgba(3, 9, 34, 0.98)),
      url('/assets/images/lalaland-clean-hero-mobile.jpg') center top / cover;
  }

  .login-stage {
    grid-template-columns: 1fr;
    gap: 16px;
  }

  .login-copy {
    padding-top: clamp(78px, 18svh, 150px);
    padding-bottom: 0;
    position: relative;
    z-index: 2;
  }

  .login-copy h1 {
    margin-bottom: 12px;
    font-size: clamp(54px, 17vw, 72px);
  }

  .login-copy p:last-child {
    font-size: 15px;
  }

  .mobile-hero-scene {
    display: block;
    position: relative;
    z-index: 1;
    height: clamp(170px, 28svh, 240px);
    margin: -4px -14px 2px;
    background:
      linear-gradient(180deg, rgba(3, 9, 34, 0), rgba(3, 9, 34, 0.08) 42%, rgba(3, 9, 34, 0.72) 100%),
      url('/assets/images/lalaland-clean-hero-mobile.jpg') center 86% / 100% auto no-repeat;
    mask-image: linear-gradient(180deg, transparent 0%, #000 13%, #000 82%, transparent 100%);
    -webkit-mask-image: linear-gradient(180deg, transparent 0%, #000 13%, #000 82%, transparent 100%);
    pointer-events: none;
  }

  .admit-form {
    position: relative;
    z-index: 3;
    padding: 22px 20px 24px;
    background: rgba(4, 11, 35, 0.84);
  }

  .admit-top {
    margin: -22px -20px 24px;
  }
}

@media (max-width: 760px) and (max-height: 740px) {
  .login-page {
    padding-top: 14px;
  }

  .login-copy {
    padding-top: 0;
  }

  .login-copy h1 {
    font-size: clamp(50px, 16vw, 62px);
  }

  .mobile-hero-scene {
    height: 188px;
    background-position: center 84%;
  }
}
</style>
