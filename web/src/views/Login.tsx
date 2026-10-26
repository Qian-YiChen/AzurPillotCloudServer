import { useState } from 'react'
import { api, ApiError, getToken, setToken } from '../api'
import { Logo } from '../ui'

// Token 登录页：凭据只保存在浏览器 localStorage。
export function Login({ onLogin }: { onLogin: () => void }) {
  const [token, setLocalToken] = useState(getToken())
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)

  const connect = async () => {
    setBusy(true)
    setError(null)
    setToken(token.trim())
    try {
      await api.probe()
      onLogin()
    } catch (e) {
      const err = e as ApiError
      if (err.status === 403) {
        setError('服务端未启用管理接口（未配置 ADMIN_TOKEN）')
      } else if (err.status === 401) {
        setError('管理凭据无效，请核对 ADMIN_TOKEN')
      } else {
        setError(err.message)
      }
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="login-wrap">
      <form
        className="card login-card"
        onSubmit={(e) => {
          e.preventDefault()
          void connect()
        }}
      >
        <div className="brand login-brand">
          <Logo size={30} />
          <h1>AzurPilot 管理后台</h1>
        </div>
        <p className="muted">使用服务端配置的 ADMIN_TOKEN 登录，凭据仅保存在本浏览器。</p>
        <input
          type="password"
          placeholder="ADMIN_TOKEN"
          value={token}
          onChange={(e) => setLocalToken(e.target.value)}
          autoFocus
        />
        {error && <p className="form-err">{error}</p>}
        <button type="submit" disabled={busy || !token.trim()}>
          {busy ? '连接中…' : '登录'}
        </button>
      </form>
    </div>
  )
}
