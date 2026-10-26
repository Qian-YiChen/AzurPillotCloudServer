import { useState } from 'react'
import { api, ApiError, getToken, setToken } from '../api'
import { Logo } from '../ui'

// 登录页：左右分屏（对照 Cloudflare 真实登录页）——
// 左半白底表单，右半品牌橙面板。
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
      <div className="login-pane">
        <div className="login-corner">
          <Logo size={28} />
        </div>
        <form
          className="login-card"
          onSubmit={(e) => {
            e.preventDefault()
            void connect()
          }}
        >
          <h1>AzurPilot 管理后台</h1>
          <p className="muted">使用服务端配置的 ADMIN_TOKEN 登录，凭据仅保存在本浏览器。</p>
          <label className="stack">
            管理凭据
            <input
              type="password"
              placeholder="ADMIN_TOKEN"
              value={token}
              onChange={(e) => setLocalToken(e.target.value)}
              autoFocus
            />
          </label>
          {error && <p className="form-err">{error}</p>}
          <button type="submit" disabled={busy || !token.trim()}>
            {busy ? '连接中…' : '登录'}
          </button>
        </form>
      </div>
      <div className="login-side">
        <div className="brand-tag">
          <Logo size={30} />
        </div>
        <p className="eyebrow">AZURPILOT CLOUD</p>
        <h2>一个控制台，管好全部云端状态。</h2>
        <p>公告、更新策略、区服维护与静态发布，都在这里完成。</p>
      </div>
    </div>
  )
}
