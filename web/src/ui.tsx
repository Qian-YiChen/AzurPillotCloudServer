import { useEffect, useState } from 'react'
import type { ReactNode } from 'react'

// 带加载与错误状态的通用数据拉取钩子。
export function useLoad<T>(loader: () => Promise<T>, deps: unknown[]): {
  data: T | null
  error: string | null
  loading: boolean
  reload: () => void
} {
  const [data, setData] = useState<T | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [loading, setLoading] = useState(true)
  const [tick, setTick] = useState(0)

  useEffect(() => {
    let alive = true
    setLoading(true)
    loader()
      .then((d) => {
        if (alive) {
          setData(d)
          setError(null)
        }
      })
      .catch((e: Error) => {
        if (alive) setError(e.message)
      })
      .finally(() => {
        if (alive) setLoading(false)
      })
    return () => {
      alive = false
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [...deps, tick])

  return { data, error, loading, reload: () => setTick((t) => t + 1) }
}

// 一次性的操作状态提示。
export function useAction(): {
  running: boolean
  message: { kind: 'ok' | 'err'; text: string } | null
  run: (fn: () => Promise<string>) => Promise<void>
} {
  const [running, setRunning] = useState(false)
  const [message, setMessage] = useState<{ kind: 'ok' | 'err'; text: string } | null>(null)

  const run = async (fn: () => Promise<string>) => {
    setRunning(true)
    setMessage(null)
    try {
      const text = await fn()
      setMessage({ kind: 'ok', text })
    } catch (e) {
      setMessage({ kind: 'err', text: (e as Error).message })
    } finally {
      setRunning(false)
    }
  }
  return { running, message, run }
}

// 发布结果渲染：区分「已发布并确认 / 仅本地生成 / 失败」。
export function PublishBadge({ publish }: { publish: { success: boolean; skipped: boolean; error: string } | null | undefined }) {
  if (!publish) return null
  if (publish.error) {
    return <span className="pill pill-err">发布失败：{publish.error}</span>
  }
  if (publish.skipped) {
    return <span className="pill pill-warn">已生成本地文件（未配置 Cloudflare，未上传）</span>
  }
  return <span className="pill pill-ok">已发布并通过读取确认</span>
}

export function Card({ title, children, extra }: { title: string; children: ReactNode; extra?: ReactNode }) {
  return (
    <section className="card">
      <header className="card-head">
        <h2>{title}</h2>
        {extra}
      </header>
      {children}
    </section>
  )
}
