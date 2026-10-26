import { useState } from 'react'
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
            {plan ? '修改' : '设置'}
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
            {plan && (
              <button type="button" className="danger" disabled={action.running} onClick={() => void remove()}>
                删除计划
              </button>
            )}
          </div>
        </form>
      ) : (
        <table className="detail">
          <tbody>
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

  const byServer = new Map<string, Maintenance>()
  for (const m of load.data?.maintenances ?? []) {
    byServer.set(m.server, m)
  }

  return (
    <>
      <Card
        title="区服维护计划"
        extra={
          <button type="button" onClick={load.reload} disabled={load.loading}>
            刷新
          </button>
        }
      >
        <p className="muted">
          计划保存后立即对 GET /api/v2/get/maintenance 生效；已结束（ends_at 过去）的安排不会再下发给客户端。
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
