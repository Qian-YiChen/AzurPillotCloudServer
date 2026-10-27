import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { App } from './App'
import { PublicStatistics } from './views/PublicStatistics'
import './styles.css'

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    {window.location.pathname.startsWith('/admin') ? <App /> : <PublicStatistics />}
  </StrictMode>,
)
