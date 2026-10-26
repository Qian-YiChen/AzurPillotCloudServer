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

// 双色调云形标识：后层浅橙云 + 前层橙云，几何图形拼合保证小尺寸清晰。
export function Logo({ size = 24 }: { size?: number }) {
  const h = Math.round(size * 0.66)
  return (
    <svg width={size} height={h} viewBox="0 0 48 32" fill="none" aria-hidden="true">
      {/* 后层云（浅橙） */}
      <g fill="#FBAD41">
        <circle cx="21" cy="10" r="6.5" />
        <rect x="12" y="8" width="20" height="9" rx="4.5" />
      </g>
      {/* 前层云（橙），底部带一级台阶 */}
      <g fill="#F6821F">
        <circle cx="13" cy="20" r="7" />
        <circle cx="25" cy="16" r="9.5" />
        <rect x="7" y="19" width="30" height="8" rx="4" />
        <rect x="7" y="24" width="37" height="4" rx="2" />
      </g>
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
