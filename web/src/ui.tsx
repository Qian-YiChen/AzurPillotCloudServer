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

// Cloudflare 风格的橙色云朵标识。
export function Logo({ size = 22 }: { size?: number }) {
  return (
    <svg width={size} height={Math.round(size * 0.62)} viewBox="0 0 64 40" fill="none" aria-hidden="true">
      <path
        d="M44.6 33.5H16.9c-4.6 0-8.4-3.8-8.4-8.4 0-4.2 3.1-7.7 7.2-8.3.6-5.4 5.2-9.6 10.7-9.6 3.4 0 6.5 1.6 8.5 4.1 1.5-1 3.3-1.5 5.2-1.5 5 0 9.1 3.9 9.5 8.8 3.9.3 7 3.6 7 7.6 0 4-3.3 7.3-7.3 7.3h-4.7z"
        fill="#F6821F"
      />
      <path
        d="M52.3 20.6c-.4 0-.9 0-1.3.1-1.9-5.6-7.2-9.7-13.5-9.7-2.4 0-4.7.6-6.7 1.6 2.9.9 5.4 2.7 7.1 5.2 1.3-.7 2.8-1.1 4.4-1.1 4.5 0 8.3 3.2 9.2 7.5h.8c3 0 5.5 2.5 5.5 5.5 0 2.6-1.8 4.8-4.2 5.4h4.9c4 0 7.3-3.3 7.3-7.3 0-4-3.1-7.2-7-7.6-.5-4.9-4.6-8.8-9.5-8.8-1.9 0-3.7.5-5.2 1.5"
        fill="#FBAD41"
      />
    </svg>
  )
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
