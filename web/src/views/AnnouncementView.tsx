import { useEffect, useState } from 'react'
import { api } from '../api'
import type { Announcement, PublishResult } from '../types'
import { Card, PublishBadge, useAction, useLoad } from '../ui'

// 公告管理：查看当前公告、发布新公告、撤回（发布 announcement: null）。
export function AnnouncementView() {
  const load = useLoad(() => api.getAnnouncement(), [])
  const [title, setTitle] = useState('')
  const [content, setContent] = useState('')
  const [url, setUrl] = useState('')
  const action = useAction()
  const [lastPublish, setLastPublish] = useState<PublishResult | null>(null)

  const current: Announcement | null = load.data?.announcement ?? null

  useEffect(() => {
    if (current) {
      setTitle(current.title)
      setContent(current.content ?? '')
      setUrl(current.url ?? '')
    }
    // 仅在公告 ID 变化时回填表单。
  }, [current?.id]) // eslint-disable-line react-hooks/exhaustive-deps

  const publish = () =>
    action.run(async () => {
      const payload: { title: string; content?: string; url?: string } = { title: title.trim() }
      if (content.trim()) payload.content = content.trim()
      if (url.trim()) payload.url = url.trim()
      const res = await api.publishAnnouncement(payload)
      setLastPublish(res.publish)
      load.reload()
      return `公告 ${res.announcement.id} 已保存`
    })

  const withdraw = () =>
    action.run(async () => {
      if (!window.confirm('确认撤回当前公告？将发布 announcement: null。')) return ''
      const res = await api.withdrawAnnouncement()
      setLastPublish(res.publish)
      load.reload()
      setTitle('')
      setContent('')
      setUrl('')
      return '公告已撤回'
    })

  return (
    <>
      <Card title="当前公告">
        {load.loading && <p className="muted">加载中…</p>}
        {load.error && <p className="form-err">{load.error}</p>}
        {current ? (
          <table className="detail">
            <tbody>
              <tr>
                <th>公告 ID</th>
                <td>
                  <code>{current.id}</code>
                </td>
              </tr>
              <tr>
                <th>标题</th>
                <td>{current.title}</td>
              </tr>
              <tr>
                <th>内容</th>
                <td className="pre">{current.content ?? '—'}</td>
              </tr>
              <tr>
                <th>链接</th>
                <td>
                  {current.url ? (
                    <a href={current.url} target="_blank" rel="noreferrer">
                      {current.url}
                    </a>
                  ) : (
                    '—'
                  )}
                </td>
              </tr>
              <tr>
                <th>更新时间</th>
                <td>{formatTime(current.updated_at)}</td>
              </tr>
            </tbody>
          </table>
        ) : (
          !load.loading && !load.error && <p className="muted">当前没有有效公告（客户端显示 announcement: null）。</p>
        )}
        <div className="pills">
          <PublishBadge publish={lastPublish} />
        </div>
      </Card>

      <Card title="发布新公告">
        <form
          className="stack"
          onSubmit={(e) => {
            e.preventDefault()
            void publish()
          }}
        >
          <label>
            标题（必填）
            <input value={title} onChange={(e) => setTitle(e.target.value)} placeholder="更新公告" />
          </label>
          <label>
            内容
            <textarea
              rows={4}
              value={content}
              onChange={(e) => setContent(e.target.value)}
              placeholder="本次更新内容。"
            />
          </label>
          <label>
            链接（HTTPS）
            <input
              type="url"
              value={url}
              onChange={(e) => setUrl(e.target.value)}
              placeholder="https://example.com/releases"
            />
          </label>
          <p className="muted">内容与链接至少填一项；发布后自动生成静态文件并推送到公告 Worker。</p>
          {action.message && <p className={action.message.kind === 'ok' ? 'form-ok' : 'form-err'}>{action.message.text}</p>}
          <div className="row">
            <button type="submit" disabled={action.running || !title.trim() || (!content.trim() && !url.trim())}>
              {action.running ? '发布中…' : '发布'}
            </button>
            {current && (
              <button type="button" className="danger" disabled={action.running} onClick={() => void withdraw()}>
                撤回当前公告
              </button>
            )}
          </div>
        </form>
      </Card>
    </>
  )
}

export function formatTime(iso: string): string {
  if (!iso || iso.startsWith('0001-')) return '—'
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return iso
  return d.toLocaleString('zh-CN', { hour12: false })
}
