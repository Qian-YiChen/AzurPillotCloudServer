import { useEffect } from 'react'
import type { ReactNode } from 'react'
import { api } from '../api'
import type { PublishRecord } from '../types'
import { Card, useLoad } from '../ui'
import { formatTime } from './AnnouncementView'

const WORKERS = [
  { key: 'ap-notice', label: '公告（ap-notice）' },
  { key: 'ap-update-policy', label: '更新策略（ap-update-policy）' },
]

function statusPill(status: PublishRecord['status'] | undefined): ReactNode {
  if (status === 'success') return <span className="pill pill-ok">成功</span>
  if (status === 'failed') return <span className="pill pill-err">失败</span>
  if (status === 'skipped') return <span className="pill pill-warn">仅本地</span>
  return '—'
}

// 发布状态：每个 Worker 最近一次发布记录，30 秒自动刷新。
export function PublishStatusView() {
  const load = useLoad(() => api.publishStatus(), [])

  useEffect(() => {
    const timer = window.setInterval(load.reload, 30_000)
    return () => window.clearInterval(timer)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  const history = load.data?.publish_history ?? {}

  return (
    <Card
      title="静态文件发布状态"
      extra={
        <button type="button" onClick={load.reload} disabled={load.loading}>
          刷新
        </button>
      }
    >
      <p className="muted">发布凭据只保存在服务器端；此处展示每个 Worker 最近一次发布结果（每 30 秒自动刷新）。</p>
      {load.error && <p className="form-err">{load.error}</p>}
      <div className="grid">
        {WORKERS.map((w) => {
          const rec = history[w.key]
          return (
            <section className="card" key={w.key}>
              <header className="card-head">
                <h2>{w.label}</h2>
                {rec ? statusPill(rec.status) : <span className="muted">从未发布</span>}
              </header>
              {rec ? (
                <table className="detail">
                  <tbody>
                    <tr>
                      <th>内容摘要</th>
                      <td>
                        <code>{rec.content_hash || '—'}</code>
                      </td>
                    </tr>
                    <tr>
                      <th>时间</th>
                      <td>{formatTime(rec.created_at)}</td>
                    </tr>
                    {rec.error && (
                      <tr>
                        <th>错误</th>
                        <td className="pre">{rec.error}</td>
                      </tr>
                    )}
                  </tbody>
                </table>
              ) : (
                !load.loading && (
                  <p className="muted">还没有发布记录；在公告或更新策略页执行一次发布后这里会出现状态。</p>
                )
              )}
            </section>
          )
        })}
      </div>
    </Card>
  )
}
