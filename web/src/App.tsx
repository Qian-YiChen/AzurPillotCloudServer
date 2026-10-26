import { useEffect, useState } from 'react'
import { clearToken, getToken, setAuthErrorHandler } from './api'
import { Login } from './views/Login'
import { AnnouncementView } from './views/AnnouncementView'
import { UpdatePolicyView } from './views/UpdatePolicyView'
import { MaintenanceView } from './views/MaintenanceView'
import { PublishStatusView } from './views/PublishStatusView'

type Section = 'announcement' | 'update-policy' | 'maintenance' | 'publish'

const SECTIONS: Array<{ key: Section; label: string; hint: string }> = [
  { key: 'announcement', label: '公告管理', hint: '发布与撤回公告' },
  { key: 'update-policy', label: '更新策略', hint: '强制更新开关' },
  { key: 'maintenance', label: '维护计划', hint: '区服维护安排' },
  { key: 'publish', label: '发布状态', hint: '静态文件发布结果' },
]

export function App() {
  const [authed, setAuthed] = useState(() => getToken() !== '')
  const [section, setSection] = useState<Section>('announcement')
  const [banner, setBanner] = useState<string | null>(null)

  // API 层收到 401 时回到登录页（凭据失效或被撤销）。
  useEffect(() => {
    setAuthErrorHandler(() => {
      setBanner('管理凭据已失效，请重新登录')
      setAuthed(false)
    })
  }, [])

  if (!authed) {
    return <Login onLogin={() => setAuthed(true)} />
  }

  const logout = () => {
    clearToken()
    setAuthed(false)
  }

  return (
    <div className="layout">
      <aside className="sidebar">
        <div className="brand">
          <h1>AzurPilot</h1>
          <span className="muted">云服务管理后台</span>
        </div>
        <nav>
          {SECTIONS.map((s) => (
            <button
              key={s.key}
              type="button"
              className={section === s.key ? 'nav-item active' : 'nav-item'}
              onClick={() => setSection(s.key)}
            >
              <span>{s.label}</span>
              <small>{s.hint}</small>
            </button>
          ))}
        </nav>
        <footer>
          <button type="button" className="ghost" onClick={logout}>
            退出登录
          </button>
        </footer>
      </aside>

      <main className="content">
        {banner && (
          <p className="banner">
            {banner}{' '}
            <button type="button" className="ghost" onClick={() => setBanner(null)}>
              关闭
            </button>
          </p>
        )}
        {section === 'announcement' && <AnnouncementView />}
        {section === 'update-policy' && <UpdatePolicyView />}
        {section === 'maintenance' && <MaintenanceView />}
        {section === 'publish' && <PublishStatusView />}
      </main>
    </div>
  )
}
