import { useCallback, useEffect, useState } from 'react'
import { Navigate, Route, Routes, useNavigate } from 'react-router-dom'
import { api, setUnauthorizedHandler } from './api'
import Layout from './components/Layout'
import Login from './pages/Login'
import ForgotPassword from './pages/ForgotPassword'
import ResetPassword from './pages/ResetPassword'
import Dashboard from './pages/Dashboard'
import DevicePage from './pages/DevicePage'
import AgentsPage from './pages/AgentsPage'
import SettingsPage from './pages/SettingsPage'
import SetupPage from './pages/SetupPage'
import CodingPage from './pages/CodingPage'
import TasksPage from './pages/TasksPage'
import AdminPage from './pages/AdminPage'
import PeoplePage from './pages/PeoplePage'
import type { Me } from './types'
import i18n from './i18n/config'
import { resolveLocale } from '../../web/locale.ts'

export default function App() {
  const [me, setMe] = useState<Me | null>(null)
  const navigate = useNavigate()

  const refresh = useCallback(async () => {
    try {
      setMe(await api.get<Me>('/api/me'))
    } catch {
      // Hub unreachable; show the login screen, which will surface errors.
      // Claimed is the safe assumption: offering the first-run form to
      // someone we could not ask would invite a claim that cannot succeed.
      setMe({
        claimed: true,
        offering: '',
        passwordMinLength: 12,
        signup: false,
        defaultOrgName: 'default',
        authenticated: false,
        version: '',
      })
    }
  }, [])

  useEffect(() => {
    refresh()
  }, [refresh])

  useEffect(() => {
    if (!me?.authenticated || !me.locale) return
    const locale = resolveLocale(me.locale)
    document.documentElement.lang = locale
    if (resolveLocale(i18n.resolvedLanguage || i18n.language) !== locale) {
      void i18n.changeLanguage(locale)
    }
  }, [me])

  useEffect(() => {
    setUnauthorizedHandler(() => {
      setMe((m) => (m ? { ...m, authenticated: false } : m))
      navigate('/login')
    })
  }, [navigate])

  if (me === null) {
    return (
      <div className="flex h-full items-center justify-center text-fg-muted">
        Loading…
      </div>
    )
  }

  if (!me.authenticated) {
    return (
      <Routes>
        <Route path="/forgot" element={<ForgotPassword />} />
        <Route
          path="/reset"
          element={
            <ResetPassword passwordMinLength={me.passwordMinLength} onSuccess={refresh} />
          }
        />
        <Route
          path="*"
          element={
            <Login
              claimed={me.claimed}
              offering={me.offering}
              passwordMinLength={me.passwordMinLength}
              signup={me.signup === true}
              onSuccess={refresh}
            />
          }
        />
      </Routes>
    )
  }

  return (
    <Routes>
      <Route element={<Layout me={me} />}>
        <Route path="/" element={<Navigate to="/code" replace />} />
        <Route path="/code" element={<CodingPage me={me} onMeChanged={refresh} />} />
        <Route path="/code/:projectId" element={<CodingPage me={me} onMeChanged={refresh} />} />
        <Route path="/tasks" element={<TasksPage />} />
        <Route path="/fleet" element={<Dashboard />} />
        <Route path="/devices/:id" element={<DevicePage />} />
        <Route path="/agents" element={<AgentsPage />} />
        <Route path="/setup" element={<SetupPage />} />
        <Route path="/settings" element={<SettingsPage />} />
        <Route path="/people" element={<PeoplePage me={me} onChanged={refresh} />} />
        {/* The administration surface exists for the operator of this hub.
            The route is absent for everyone else rather than rendering a
            refusal, and the endpoints behind it check the same thing. */}
        {me.platformAdmin && <Route path="/admin" element={<AdminPage />} />}
        <Route path="*" element={<Navigate to="/" replace />} />
      </Route>
    </Routes>
  )
}
