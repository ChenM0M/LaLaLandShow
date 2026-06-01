import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import * as api from '../api'
import type { Student } from '../api'

export const useAuthStore = defineStore('auth', () => {
  const token = ref<string | null>(localStorage.getItem('token'))
  const student = ref<Student | null>(null)
  const needRegister = ref(false)
  const hasCompleteProfile = computed(() => {
    const classCode = student.value?.class_code?.trim()
    const seatNo = student.value?.seat_no_in_class
    return Boolean(classCode && seatNo && seatNo > 0)
  })

  async function login(student_id: string) {
    const res = await api.login(student_id)
    token.value = res.token
    student.value = res.student
    needRegister.value = res.need_register
    localStorage.setItem('token', res.token)
  }

  async function register(data: { class_code: string; seat_no_in_class: number; display_name?: string }) {
    const res = await api.register(data)
    student.value = res.student
    needRegister.value = res.need_register
  }

  async function fetchMe() {
    const res = await api.getMe()
    student.value = res.student
    needRegister.value = !hasCompleteProfile.value
  }

  function logout() {
    token.value = null
    student.value = null
    needRegister.value = false
    localStorage.removeItem('token')
  }

  if (typeof window !== 'undefined') {
    window.addEventListener('storage', (event) => {
      if (event.key === 'token' && !event.newValue) logout()
    })
  }

  return { token, student, needRegister, hasCompleteProfile, login, register, fetchMe, logout }
})
