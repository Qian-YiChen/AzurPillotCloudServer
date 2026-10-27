import { useEffect, useState } from 'react'
import { api } from '../api'
import { MAINTENANCE_SERVERS, SERVER_LABELS } from '../types'
import type { Maintenance, MaintenanceServer } from '../types'
import { Card, useAction, useLoad } from '../ui'
import { formatTime } from './AnnouncementView'

// ISO 时间 -> datetime-local 输入值（本地时区）。
function toInput(iso: string | null): string {
  if (!iso) return ''
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return ''
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`
}

// datetime-local 输入值 -> 带时区的 RFC 3339（服务端要求带时区）。
function fromInput(value: string): string {
  return new Date(value).toISOString()
}

interface FormState {
  name: string
  startsAt: string
  endsAt: string
}

function emptyForm(): FormState {
  return { name: '', startsAt: '', endsAt: '' }
}

function ServerCard({
  server,
  plan,
  onChanged,
}: {
  server: MaintenanceServer
  plan: Maintenance | undefined
  onChanged: () => void
}) {
  const [form, setForm] = useState<FormState>(emptyForm)
  const [editing, setEditing] = useState(false)
  const action = useAction()

  const startEdit = () => {
    setForm({
      name: plan?.name ?? '',
      startsAt: toInput(plan?.starts_at ?? null),
      endsAt: toInput(plan?.ends_at ?? null),
    })
    setEditing(true)
  }

  const save = () =>
    action.run(async () => {
      const payload: { name: string; starts_at: string; ends_at?: string } = {
        name: form.name.trim(),
        starts_at: fromInput(form.startsAt),
      }
      if (form.endsAt) payload.ends_at = fromInput(form.endsAt)
      await api.saveMaintenance(server, payload)
      setEditing(false)
      onChanged()
      return `${SERVER_LABELS[server]} 维护计划已保存`
    })

  const remove = () =>
    action.run(async () => {
      if (!window.confirm(`确认删除 ${SERVER_LABELS[server]} 的维护计划？`)) return ''
      await api.deleteMaintenance(server)
      setEditing(false)
      onChanged()
      return `${SERVER_LABELS[server]} 维护计划已删除`
    })

  return (
    <section className="card">
      <header className="card-head">
        <h2>
          {SERVER_LABELS[server]} <code className="dim">{server}</code>
        </h2>
        {!editing && (
          <button type="button" onClick={startEdit}>
            {plan?.source === 'official' ? '人工覆盖' : plan ? '修改' : '设置'}
          </button>
        )}
      </header>

      {editing ? (
        <form
          className="stack"
          onSubmit={(e) => {
            e.preventDefault()
            void save()
          }}
        >
          <label>
            名称
            <input
              value={form.name}
              onChange={(e) => setForm({ ...form, name: e.target.value })}
              placeholder={SERVER_LABELS[server]}
            />
          </label>
          <label>
            开始时间（必填）
            <input
              type="datetime-local"
              value={form.startsAt}
              onChange={(e) => setForm({ ...form, startsAt: e.target.value })}
            />
          </label>
          <label>
            结束时间（可空，无法确定时留空）
            <input
              type="datetime-local"
              value={form.endsAt}
              onChange={(e) => setForm({ ...form, endsAt: e.target.value })}
            />
          </label>
          {action.message && <p className={action.message.kind === 'ok' ? 'form-ok' : 'form-err'}>{action.message.text}</p>}
          <div className="row">
            <button type="submit" disabled={action.running || !form.name.trim() || !form.startsAt}>
              {action.running ? '保存中…' : '保存'}
            </button>
            <button
              type="button"
              disabled={action.running}
              onClick={() => {
                setEditing(false)
              }}
            >
              取消
            </button>
            {plan && plan.source !== 'official' && (
              <button type="button" className="danger" disabled={action.running} onClick={() => void remove()}>
                {server === 'cn' ? '删除人工覆盖' : '删除计划'}
              </button>
            )}
          </div>
        </form>
      ) : (
        <table className="detail">
          <tbody>
            <tr><th>来源</th><td>{plan?.source === 'official' ? <a href={plan.source_url} target="_blank" rel="noreferrer">国服官方公告</a> : plan ? '人工录入' : '—'}</td></tr>
            <tr>
              <th>名称</th>
              <td>{plan?.name ?? '—'}</td>
            </tr>
            <tr>
              <th>开始</th>
              <td>{plan ? formatTime(plan.starts_at) : '—'}</td>
            </tr>
            <tr>
              <th>结束</th>
              <td>{plan?.ends_at ? formatTime(plan.ends_at) : plan ? '未确定' : '—'}</td>
            </tr>
          </tbody>
        </table>
      )}
    </section>
  )
}

// 维护计划管理：四个区服各一张卡片。
export function MaintenanceView() {
  const load = useLoad(() => api.listMaintenance(), [])
  const source = useLoad(() => api.maintenanceSource(), [])
  useEffect(() => {
    const timer = window.setInterval(source.reload, 30000)
    return () => window.clearInterval(timer)
    // reload only increments the hook's stable state setter.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])
  const action = useAction()
  const sync = () => action.run(async () => {
    try {
      await api.syncMaintenance()
      return '官方公告同步完成'
    } finally {
      source.reload()
      load.reload()
    }
  })

  const byServer = new Map<string, Maintenance>()
  for (const m of load.data?.maintenances ?? []) {
    byServer.set(m.server, m)
  }

  return (
    <>
      <Card title="国服官方维护公告" extra={
        <button type="button" disabled={action.running || source.loading || source.data?.running} onClick={() => void sync()}>
          {action.running ? '同步中…' : '立即同步'}
        </button>
      }>
        <p className="muted">国服默认每 10 分钟检查官方公告。人工覆盖优先生效，删除覆盖后恢复官方计划。其他区服暂由人工录入。</p>
        {source.data && <table className="detail"><tbody>
          <tr><th>自动同步</th><td>{source.data.enabled ? `已开启 · 每 ${source.data.interval_seconds / 60} 分钟` : '已关闭'}</td></tr>
          <tr><th>最新公告</th><td><a href={source.data.url} target="_blank" rel="noreferrer">{source.data.title || '查看官方公告页'}</a></td></tr>
          <tr><th>最近检查</th><td>{source.data.checked_at ? formatTime(source.data.checked_at) : '尚未检查'}</td></tr>
          <tr><th>最近成功</th><td>{source.data.synced_at ? formatTime(source.data.synced_at) : '尚未成功同步'}</td></tr>
          <tr><th>同步状态</th><td>{source.data.error ? <span className="form-err">{source.data.error}（保留上次结果）</span> : source.data.synced_at ? '正常' : '等待同步'}</td></tr>
        </tbody></table>}
        {source.error && <p className="form-err">{source.error}</p>}
        {action.message && <p className={action.message.kind === 'ok' ? 'form-ok' : 'form-err'}>{action.message.text}</p>}
      </Card>
      <Card
        title="区服维护计划"
        extra={
          <button type="button" onClick={load.reload} disabled={load.loading}>
            刷新
          </button>
        }
      >
        <p className="muted">
          已结束的安排仅供核对，不下发给客户端。未发布新的维护公告时，不推算下次维护时间。
        </p>
        {load.error && <p className="form-err">{load.error}</p>}
      </Card>
      <div className="grid">
        {MAINTENANCE_SERVERS.map((server) => (
          <ServerCard key={server} server={server} plan={byServer.get(server)} onChanged={load.reload} />
        ))}
      </div>
    </>
  )
}
