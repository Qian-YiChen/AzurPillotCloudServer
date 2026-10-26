import { useEffect, useState } from 'react'
import { api } from '../api'
import { Card, PublishBadge, useAction, useLoad } from '../ui'

// 强制更新开关：保存后自动发布更新策略 Worker。
export function UpdatePolicyView() {
  const load = useLoad(() => api.getUpdatePolicy(), [])
  const [force, setForce] = useState(false)
  const action = useAction()
  const [lastPublish, setLastPublish] = useState<Parameters<typeof PublishBadge>[0]['publish']>(null)

  useEffect(() => {
    if (load.data) setForce(load.data.force_update)
  }, [load.data])

  const save = () =>
    action.run(async () => {
      const res = await api.setUpdatePolicy(force)
      setLastPublish(res.publish)
      load.reload()
      return `已保存并发布：force_update = ${res.force_update}`
    })

  return (
    <Card title="更新策略">
      {load.loading && <p className="muted">加载中…</p>}
      {load.error && <p className="form-err">{load.error}</p>}
      {load.data && (
        <form
          className="stack"
          onSubmit={(e) => {
            e.preventDefault()
            void save()
          }}
        >
          <label className="check">
            <input type="checkbox" checked={force} onChange={(e) => setForce(e.target.checked)} />
            强制更新（force_update）——开启后客户端更新策略文档将要求强制更新
          </label>
          <div className="pills">
            <PublishBadge publish={lastPublish} />
          </div>
          {action.message && <p className={action.message.kind === 'ok' ? 'form-ok' : 'form-err'}>{action.message.text}</p>}
          <div className="row">
            <button type="submit" disabled={action.running}>
              {action.running ? '发布中…' : '保存并发布'}
            </button>
          </div>
        </form>
      )}
    </Card>
  )
}
