import { defineStore } from 'pinia'
import { ref } from 'vue'
import * as api from '../api'
import type { Seat, WindowState } from '../api'

export const useSeatsStore = defineStore('seats', () => {
  const seats = ref<Seat[]>([])
  const windowState = ref<WindowState['state']>('before_open')
  let es: EventSource | null = null
  let reconnectTimer: number | undefined
  let snapshotRefreshTimer: number | undefined

  async function loadSnapshot() {
    seats.value = await api.getSnapshot()
    const ws = await api.getWindowState()
    windowState.value = ws.state
  }

  function scheduleSnapshotRefresh() {
    if (snapshotRefreshTimer) window.clearTimeout(snapshotRefreshTimer)
    snapshotRefreshTimer = window.setTimeout(() => {
      snapshotRefreshTimer = undefined
      void loadSnapshot()
    }, 250)
  }

  function connectStream() {
    if (es) es.close()
    if (reconnectTimer) window.clearTimeout(reconnectTimer)
    const token = localStorage.getItem('token')
    es = new EventSource(`/api/seats/stream${token ? `?token=${token}` : ''}`)

    es.addEventListener('ready', () => {})

    es.addEventListener('seat_changed', (e: MessageEvent) => {
      const updated: { seat_id?: number; state?: Seat['state'] } = JSON.parse(e.data)
      const idx = seats.value.findIndex(s => s.seat_id === updated.seat_id)
      if (idx !== -1 && updated.state) {
        const clearedIdentity = updated.state === 'free' || updated.state === 'others'
          ? { student_id: null, booked_by_student_id: null }
          : {}
        seats.value[idx] = { ...seats.value[idx], ...clearedIdentity, state: updated.state }
      }
      scheduleSnapshotRefresh()
    })

    es.onerror = () => {
      es?.close()
      es = null
      reconnectTimer = window.setTimeout(() => connectStream(), 3000)
    }
  }

  function disconnectStream() {
    if (reconnectTimer) window.clearTimeout(reconnectTimer)
    if (snapshotRefreshTimer) window.clearTimeout(snapshotRefreshTimer)
    reconnectTimer = undefined
    snapshotRefreshTimer = undefined
    es?.close()
    es = null
  }

  async function book(seatIds: { seat_id: number; student_id: string }[]) {
    await api.bookSeats(seatIds)
    await loadSnapshot()
  }

  async function rebook(seatIds: { seat_id: number; student_id: string }[]) {
    await api.rebookSeats(seatIds)
    await loadSnapshot()
  }

  async function release() {
    await api.releaseSeats()
    await loadSnapshot()
  }

  async function changeSeat(seat_id: number) {
    await api.changeSeat(seat_id)
    await loadSnapshot()
  }

  return { seats, windowState, loadSnapshot, connectStream, disconnectStream, book, rebook, release, changeSeat }
})
