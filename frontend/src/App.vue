<script setup lang="ts">
import { watch } from 'vue'
import { useRoute } from 'vue-router'
import { useRouter } from 'vue-router'
import { useAuthStore } from './stores/auth'

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()

watch(
  () => [auth.token, auth.hasCompleteProfile, route.fullPath],
  () => {
    if (route.meta.requiresAuth && !auth.token) {
      router.replace('/login')
      return
    }
    if (route.meta.requiresCompleteProfile && auth.token && !auth.hasCompleteProfile) {
      router.replace({ path: '/register', query: { redirect: route.fullPath } })
      return
    }
    if (route.path === '/register' && auth.token && auth.hasCompleteProfile) {
      const redirect = typeof route.query.redirect === 'string' ? route.query.redirect : '/seats'
      router.replace(redirect)
    }
  }
)

function confirmLogout() {
  if (!window.confirm('確定要登出目前學生帳號嗎？')) return
  auth.logout()
}
</script>

<template>
  <div class="app-layout">
    <!-- Router view contains the main page content -->
    <main class="app-content">
      <router-view v-slot="{ Component }">
        <transition name="page" mode="out-in">
          <component :is="Component" :key="route.path" />
        </transition>
      </router-view>
    </main>

    <!-- Global Navigation Dock -->
    <nav v-show="!['/seats', '/register', '/admin', '/check'].includes(route.path)" class="floating-dock glass-panel">
      <div class="dock-inner">
        <router-link to="/" class="nav-item">
          <span class="icon">★</span>
          <span class="label">首頁</span>
        </router-link>
        <router-link to="/status" class="nav-item">
          <span class="icon">◎</span>
          <span class="label">狀態</span>
        </router-link>
        <template v-if="auth.token">
          <router-link to="/ticket" class="nav-item">
            <span class="icon">▤</span>
            <span class="label">票券</span>
          </router-link>
          <button @click="confirmLogout" class="nav-item text-btn">
            <span class="icon">⏏</span>
            <span class="label">登出</span>
          </button>
        </template>
        <template v-else>
          <router-link to="/login" class="nav-item">
            <span class="icon">⚿</span>
            <span class="label">登入</span>
          </router-link>
        </template>
        <!-- Hidden but accessible for admins/checkers unless linked elsewhere -->
        <router-link to="/check" class="nav-item" v-if="auth.token && false">驗票</router-link>
        <router-link to="/admin" class="nav-item" v-if="auth.token && false">後台</router-link>
      </div>
    </nav>
  </div>
</template>

<style>
.app-layout {
  display: flex;
  flex-direction: column;
  min-height: 100vh;
}
.app-content {
  flex: 1;
}

/* Floating Dock */
.floating-dock.glass-panel {
  position: fixed;
  bottom: clamp(16px, 3vw, 24px);
  left: 50%;
  transform: translateX(-50%);
  z-index: 1000;
  border-radius: 999px;
  padding: 7px 14px;
  border: 1px solid rgba(247, 242, 223, 0.16);
  background: rgba(2, 6, 31, 0.62);
  backdrop-filter: blur(22px);
  -webkit-backdrop-filter: blur(22px);
  box-shadow: 0 1rem 2.6rem rgba(0, 0, 0, 0.3), inset 0 1px 0 rgba(255, 255, 255, 0.06);
  transition: opacity 0.3s ease, transform 0.3s ease;
}

.dock-inner {
  display: flex;
  align-items: center;
  gap: clamp(16px, 4vw, 32px);
}

.nav-item, .text-btn {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 4px;
  color: rgba(247, 242, 223, 0.62);
  text-decoration: none;
  font-family: 'Nunito Sans', -apple-system, BlinkMacSystemFont, sans-serif;
  transition: all 0.3s ease;
  background: none;
  border: none;
  cursor: pointer;
  padding: 6px;
}

.nav-item .icon {
  font-size: 1.2rem;
  line-height: 1;
}

.nav-item .label {
  font-size: 0.65rem;
  text-transform: uppercase;
  letter-spacing: 0.1em;
  font-weight: 600;
  white-space: nowrap;
}

.nav-item:hover, .text-btn:hover, .nav-item.router-link-active {
  color: #f7f2df;
  transform: translateY(-2px);
}
</style>
