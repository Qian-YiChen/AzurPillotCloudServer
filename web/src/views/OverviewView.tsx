import { api } from '../api'
import type { ReactNode } from 'react'
import type { Announcement, Maintenance, PublishRecord } from '../types'
import { SERVER_LABELS, MAINTENANCE_SERVERS } from '../types'
import { Card, useLoad } from '../ui'
import { formatTime } from './AnnouncementView'

function StatCard({ title, children }: { title: string; children: ReactNode }) {
  return (
    <section className="card stat">
      <p className="stat-title">{title}</p>
      <div className="stat-body">{children}</div>
    </section>
  )
}

// 概览页：登录后的落地页，一屏看清公告 / 更新策略 / 维护 / 发布状态。
export function OverviewView({ onNavigate }: { onNavigate: (section: string) => void }) {
  const announcement = useLoad(() => api.getAnnouncement(), [])
  const policy = useLoad(() => api.getUpdatePolicy(), [])
  const maintenance = useLoad(() => api.listMaintenance(), [])
  const publish = useLoad(() => api.publishStatus(), [])

  const a: Announcement | null = announcement.data?.announcement ?? null
  const force = policy.data?.force_update ?? false
  const plans = (maintenance.data?.maintenances ?? []).filter((m: Maintenance) =>
    MAINTENANCE_SERVERS.includes(m.server as (typeof MAINTENANCE_SERVERS)[number]) && (!m.ends_at || new Date(m.ends_at) >= new Date()),
  )
  const history = publish.data?.publish_history ?? {}
  const records: PublishRecord[] = Object.values(history).sort(
    (x, y) => new Date(y.created_at).getTime() - new Date(x.created_at).getTime(),
  )

  return (
    <>
      <div className="stat-grid">
        <StatCard title="公告">
          {a ? (
            <>
              <p className="stat-main">{a.title}</p>
              <p className="muted">{a.id} · {formatTime(a.updated_at)}</p>
            </>
          ) : (
            <>
              <p className="stat-main dim">无有效公告</p>
              <p className="muted">客户端显示 announcement: null</p>
            </>
          )}
        </StatCard>
        <StatCard title="更新策略">
          <p className="stat-main">
            <span className={force ? 'pill pill-err' : 'pill pill-ok'}>{force ? '强制更新开启' : '正常（不强更）'}</span>
          </p>
          <p className="muted">force_update = {String(force)}</p>
        </StatCard>
        <StatCard title="维护计划">
          {plans.length > 0 ? (
            <>
              <p className="stat-main">{plans.length} 个区服有安排</p>
              <p className="muted">
                {plans.map((m) => SERVER_LABELS[m.server as keyof typeof SERVER_LABELS] ?? m.server).join('、')}
              </p>
            </>
          ) : (
            <>
              <p className="stat-main dim">无维护安排</p>
              <p className="muted">cn / en / jp / tw 均为空</p>
            </>
          )}
        </StatCard>
      </div>

      <Card
        title="最近发布"
        extra={
          <button type="button" className="ghost" onClick={() => onNavigate('publish')}>
            查看发布状态
          </button>
        }
      >
        {records.length > 0 ? (
          <table className="detail">
            <thead>
              <tr>
                <th>Worker</th>
                <th>状态</th>
                <th>内容摘要</th>
                <th>时间</th>
              </tr>
            </thead>
            <tbody>
              {records.map((r) => (
                <tr key={r.worker}>
                  <td>{r.worker}</td>
                  <td>
                    <span className={r.status === 'success' ? 'pill pill-ok' : r.status === 'failed' ? 'pill pill-err' : 'pill pill-warn'}>
                      {r.status === 'success' ? '成功' : r.status === 'failed' ? '失败' : '仅本地'}
                    </span>
                  </td>
                  <td>
                    <code>{r.content_hash || '—'}</code>
                  </td>
                  <td>{formatTime(r.created_at)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        ) : (
          <p className="muted">还没有发布记录。</p>
        )}
      </Card>
    </>
  )
}
