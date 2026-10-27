// 管理 API 客户端：统一携带 Bearer Token、解析统一错误信封。
import type { Announcement, Maintenance, MaintenanceSource, PublishRecord, PublishResult } from './types'

const TOKEN_KEY = 'ap_admin_token'

export class ApiError extends Error {
  status: number
  code: string

  constructor(status: number, code: string, message: string) {
    super(message)
    this.status = status
    this.code = code
  }

  get isAuthError(): boolean {
    return this.status === 401
  }
}

export function getToken(): string {
  return localStorage.getItem(TOKEN_KEY) ?? ''
}

export function setToken(token: string): void {
  localStorage.setItem(TOKEN_KEY, token)
}

export function clearToken(): void {
  localStorage.removeItem(TOKEN_KEY)
}

// 401 时的全局回调：由应用层注册（清空登录态并提示）。
let authErrorHandler: (() => void) | null = null

export function setAuthErrorHandler(handler: () => void): void {
  authErrorHandler = handler
}

export async function request<T>(method: string, path: string, body?: unknown): Promise<T> {
  const headers: Record<string, string> = {}
  const token = getToken()
  if (token) {
    headers['Authorization'] = `Bearer ${token}`
  }
  if (body !== undefined) {
    headers['Content-Type'] = 'application/json'
  }

  let resp: Response
  try {
    resp = await fetch(path, {
      method,
      headers,
      body: body !== undefined ? JSON.stringify(body) : undefined,
    })
  } catch {
    throw new ApiError(0, 'NETWORK', '无法连接服务端，请检查网络')
  }

  if (resp.status === 401) {
    // 凭据失效：清除本地登录态并通知应用层。
    clearToken()
    authErrorHandler?.()
  }

  const text = await resp.text()
  let data: unknown = null
  if (text) {
    try {
      data = JSON.parse(text)
    } catch {
      data = null
    }
  }

  if (!resp.ok) {
    const err = (data as { error?: { code?: string; message?: string } } | null)?.error
    throw new ApiError(
      resp.status,
      err?.code ?? `HTTP_${resp.status}`,
      err?.message ?? (resp.statusText || '请求失败'),
    )
  }
  return data as T
}

export const api = {
  probe: () => request<{ publish_history: Record<string, PublishRecord> }>('GET', '/api/v1/admin/publish/status'),

  getAnnouncement: () => request<{ announcement: Announcement | null }>('GET', '/api/v1/admin/announcement'),
  publishAnnouncement: (payload: { title: string; content?: string; url?: string }) =>
    request<{ announcement: Announcement; publish: PublishResult }>('PUT', '/api/v1/admin/announcement', payload),
  withdrawAnnouncement: () =>
    request<{ publish: PublishResult }>('DELETE', '/api/v1/admin/announcement'),

  getUpdatePolicy: () => request<{ force_update: boolean }>('GET', '/api/v1/admin/update-policy'),
  setUpdatePolicy: (forceUpdate: boolean) =>
    request<{ force_update: boolean; publish: PublishResult }>('PUT', '/api/v1/admin/update-policy', {
      force_update: forceUpdate,
    }),

  listMaintenance: () => request<{ maintenances: Maintenance[] }>('GET', '/api/v1/admin/maintenance'),
  maintenanceSource: () => request<MaintenanceSource>('GET', '/api/v1/admin/maintenance/source'),
  syncMaintenance: () => request<MaintenanceSource>('POST', '/api/v1/admin/maintenance/source'),
  saveMaintenance: (server: string, payload: { name: string; starts_at: string; ends_at?: string }) =>
    request<{ saved: boolean }>('PUT', `/api/v1/admin/maintenance/${server}`, payload),
  deleteMaintenance: (server: string) =>
    request<{ deleted: boolean }>('DELETE', `/api/v1/admin/maintenance/${server}`),

  publishStatus: () =>
    request<{ publish_history: Record<string, PublishRecord> }>('GET', '/api/v1/admin/publish/status'),
}
