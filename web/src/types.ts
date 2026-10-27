// 管理后台 API 类型定义（与 Go 服务端的 JSON 结构一一对应）。

export interface Announcement {
  id: string
  title: string
  content: string | null
  url: string | null
  active: boolean
  updated_at: string
}

export interface PublishResult {
  success: boolean
  skipped: boolean
  error: string
}

export interface Maintenance {
  server: string
  name: string
  starts_at: string
  ends_at: string | null
  updated_at: string
  source?: 'manual' | 'official'
  source_url?: string
}

export interface MaintenanceSource {
  server: string
  title: string
  url: string
  plan: Maintenance | null
  checked_at: string | null
  synced_at: string | null
  error: string
  enabled: boolean
  interval_seconds: number
  running: boolean
}

export interface PublishRecord {
  worker: string
  content_hash: string
  status: 'success' | 'failed' | 'skipped'
  error?: string
  created_at: string
}

export const MAINTENANCE_SERVERS = ['cn', 'en', 'jp', 'tw'] as const
export type MaintenanceServer = (typeof MAINTENANCE_SERVERS)[number]

export const SERVER_LABELS: Record<MaintenanceServer, string> = {
  cn: '国服',
  en: '国际服',
  jp: '日服',
  tw: '台服',
}
