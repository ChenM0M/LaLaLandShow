import axios from 'axios'

const api = axios.create({ baseURL: '/api' })

api.interceptors.request.use(config => {
  const url = config.url || ''
  const token = url.startsWith('/admin') || url.startsWith('/checker')
    ? localStorage.getItem('staffToken')
    : localStorage.getItem('token')
  if (token) config.headers.Authorization = `Bearer ${token}`
  return config
})

api.interceptors.response.use(
  response => response,
  error => {
    const url = error?.config?.url || ''
    if (error?.response?.status === 401 && !url.startsWith('/admin') && !url.startsWith('/checker')) {
      window.dispatchEvent(new CustomEvent('student-auth-expired'))
    }
    return Promise.reject(error)
  },
)

export interface Student {
  student_id: string
  class_code?: string | null
  seat_no_in_class?: number | null
  display_name?: string
  pending_notice?: unknown
}

export interface Booking {
  seat_id: number
  student_id: string
  zone?: string
  row_no?: number
  col_no?: number
  ticket_version?: number
  booked_at?: string
}

export interface Seat {
  seat_id: number
  zone: string
  row_no: number
  col_no: number
  is_blocked?: boolean
  state: 'free' | 'locked' | 'mine' | 'proxy' | 'others'
  student_id?: string | null
  booked_by_student_id?: string | null
}

export interface PublicConfig {
  play_name?: string
  venue?: string
  booking_open_at?: string
  booking_lock_at?: string
  show_start_at?: string
  countdown_target_at?: string
  show_date?: string
  cast_list?: string[]
  gallery_urls?: string[]
  story_background?: string
  story_summary?: string
}

export interface WindowState {
  state: 'before_open' | 'open' | 'locked' | 'show_started'
}

export interface LoginResponse {
  token: string
  student: Student
  need_register: boolean
}

export interface RegisterResponse {
  student: Student
  need_register: boolean
}

export interface MeResponse {
  student: Student
  booking: Booking | null
  proxy_bookings: Booking[]
}

export interface TicketResponse {
  play_name: string
  venue: string
  show_start_at?: string
  issued_at: string
  holder: {
    student_id: string
    class_code?: string | null
    seat_no_in_class?: number | null
    display_name?: string | null
  }
  seat: {
    seat_id: number
    zone: string
    row_no: number
    col_no: number
  }
  ticket_version: number
  qr_payload?: unknown
  qr_token?: string
  notice: string
  booked_by_student_id?: string | null
  owned_by_viewer: boolean
}

export interface TicketListResponse {
  tickets: TicketResponse[]
}

export interface Staff {
  username: string
  role: 'checker' | 'admin'
}

export interface StaffLoginResponse {
  token: string
  staff: Staff
}

export interface CheckerBooking {
  student_id: string
  class_code?: string | null
  seat_no_in_class?: number | null
  display_name?: string | null
  seat_id: number
  zone: string
  row_no: number
  col_no: number
  ticket_version: number
  booked_at: string
  last_checkin_at?: string
  checkin_count: number
  checked_in_before: boolean
}

export interface CheckerVerification {
  valid: boolean
  method: 'qr' | 'manual'
  booking: CheckerBooking
}

export interface CheckinResult {
  event: {
    id: number
    student_id: string
    seat_id?: number
    method: string
    result: string
    is_first_scan: boolean
    scanner_account: string
    scanned_at: string
  }
  booking?: CheckerBooking
}

export interface CheckerStats {
  checked_in: number
  total: number
  last_minute_rate: number
}

export interface AdminSeat {
  seat_id: number
  zone: string
  row_no: number
  col_no: number
  is_blocked: boolean
  block_reason?: string | null
  booking?: {
    student_id?: string | null
    custom_label?: string | null
    booked_by_student_id?: string | null
    class_code?: string | null
    seat_no_in_class?: number | null
    display_name?: string | null
    ticket_version: number
    booked_at: string
  } | null
}

export interface DashboardStats {
  generated_at: string
  window: WindowState
  seats: {
    total: number
    booked: number
    free: number
    blocked: number
    student_booked: number
    custom_occupied: number
  }
  students: {
    total: number
    registered: number
    need_register: number
    pending_notice: number
  }
  checkins: {
    approved: number
    rejected: number
    unique_students: number
    first_scans: number
    last_minute_rate: number
  }
  recent: {
    audit_logs: Array<{ id: number; actor_type: string; actor_id?: string; action: string; created_at: string }>
    checkin_events: Array<{ id: number; student_id?: string; method: string; result: string; is_first_scan: boolean; scanned_at: string }>
  }
}

export interface AdminStudentList {
  students: Array<Student & { need_register: boolean; seat_id?: number | null; booked_at?: string | null }>
  total: number
  page: number
  page_size: number
}

export interface AdminAuditList {
  logs: Array<{ id: number; actor_type: string; actor_id?: string | null; action: string; target_seat_id?: number | null; target_student?: string | null; created_at: string }>
  total: number
  page: number
  page_size: number
}

export interface AdminCheckinList {
  events: Array<{ id: number; student_id?: string | null; seat_id?: number | null; method: string; result: string; is_first_scan: boolean; scanner_account?: string | null; scanned_at: string }>
  total: number
  page: number
  page_size: number
}

// Public
export const getConfig = () => api.get<PublicConfig>('/public/config').then(r => r.data)
export const getWindowState = () => api.get<WindowState>('/public/window-state').then(r => r.data)

// Auth
export const login = (student_id: string) =>
  api.post<LoginResponse>('/student/login', { student_id }).then(r => r.data)

export const register = (data: { class_code: string; seat_no_in_class: number; display_name?: string }) =>
  api.post<RegisterResponse>('/student/register', data).then(r => r.data)

export const getMe = () => api.get<MeResponse>('/student/me').then(r => r.data)

// Seats
export const getSnapshot = () => api.get<{ seats: Seat[] }>('/seats/snapshot').then(r => r.data.seats)

export const bookSeats = (seats: { seat_id: number; student_id: string }[]) =>
  api.post('/seats/book', seats).then(r => r.data)

export const rebookSeats = (seats: { seat_id: number; student_id: string }[]) =>
  api.post('/seats/rebook', seats).then(r => r.data)

export const releaseSeats = () => api.post('/seats/release').then(r => r.data)

export const changeSeat = (seat_id: number) =>
  api.post('/seats/change', { seat_id }).then(r => r.data)

// Ticket
export const getTicket = () => api.get<TicketResponse>('/ticket/me').then(r => r.data)
export const getTickets = () => api.get<TicketListResponse>('/ticket/mine').then(r => r.data)

// Checker
export const checkerLogin = (username: string, password: string) =>
  api.post<StaffLoginResponse>('/checker/login', { username, password }).then(r => r.data)

export const verifyQR = (qr_token: string) =>
  api.post<CheckerVerification>('/checker/verify-qr', { qr_token }).then(r => r.data)

export const lookupStudent = (student_id: string) =>
  api.post<CheckerVerification>('/checker/lookup', { student_id }).then(r => r.data)

export const checkIn = (data: { method: 'qr' | 'manual' | 'force_no_booking'; qr_token?: string; student_id?: string }) =>
  api.post<CheckinResult>('/checker/checkin', data).then(r => r.data)

export const checkerStats = () => api.get<CheckerStats>('/checker/stats').then(r => r.data)

// Admin
export const adminLogin = (username: string, password: string) =>
  api.post<StaffLoginResponse>('/admin/login', { username, password }).then(r => r.data)

export const adminDashboard = () => api.get<DashboardStats>('/admin/dashboard').then(r => r.data)
export const adminSeats = () => api.get<{ seats: AdminSeat[] }>('/admin/seats').then(r => r.data.seats)
export const adminStudents = () => api.get<AdminStudentList>('/admin/students').then(r => r.data)
export const adminAuditLogs = () => api.get<AdminAuditList>('/admin/audit-logs').then(r => r.data)
export const adminCheckinEvents = () => api.get<AdminCheckinList>('/admin/checkin-events').then(r => r.data)

export const assignAdminSeat = (
  seat_id: number,
  data: { student_id?: string; custom_label?: string; release?: boolean; force_release_existing?: boolean },
) => api.post<{ seat: AdminSeat }>(`/admin/seats/${seat_id}/assign`, data).then(r => r.data.seat)

export const lockAdminSeat = (seat_id: number, data: { lock: boolean; block_reason?: string | null }) =>
  api.post<{ seat: AdminSeat }>(`/admin/seats/${seat_id}/lock`, data).then(r => r.data.seat)

export const getAdminConfig = (key: string) => api.get(`/admin/config/${key}`).then(r => r.data)
export const setAdminConfig = (key: string, value: unknown) =>
  api.put(`/admin/config/${key}`, { value }).then(r => r.data)

export const exportBackup = () => api.get('/admin/backup/export').then(r => r.data)
export const planRestore = (snapshot: unknown) =>
  api.post('/admin/backup/plan', { snapshot }).then(r => r.data)
export const restoreBackup = (snapshot: unknown, confirmation_token: string) =>
  api.post('/admin/backup/restore', { snapshot, confirmation_token }).then(r => r.data)

export default api
