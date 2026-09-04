import { useCallback, useEffect, useState } from 'react'
import { Navigate, Route, Routes, useNavigate } from 'react-router-dom'
import { api, setUnauthorizedHandler } from './api'
import Layout from './components/Layout'
import Login from './pages/Login'
import Dashboard from './pages/Dashboard'
import DevicePage from './pages/DevicePage'
import AgentsPage from './pages/AgentsPage'
import SettingsPage from './pages/SettingsPage'
import SetupPage from './pages/SetupPage'
import CodingPage from './pages/CodingPage'
import TasksPage from './pages/TasksPage'

interface Me {
  claimed: boolean
  offering: string
  passwordMinLength: number
  authenticated: boolean
  version: string
}

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
        authenticated: false,
        version: '',
      })
    }
  }, [])

  useEffect(() => {
    refresh()
  }, [refresh])

  useEffect(() => {
    setUnauthorizedHandler(() => {
      setMe((m) => (m ? { ...m, authenticated: false } : m))
      navigate('/login')
    })
  }, [navigate])

  if (me === null) {
    return (
      <div className="flex h-full items-center justify-center text-zinc-500">
        Loading…
      </div>
    )
  }

  if (!me.authenticated) {
    return (
      <Routes>
        <Route
          path="*"
          element={
            <Login
              claimed={me.claimed}
              offering={me.offering}
              passwordMinLength={me.passwordMinLength}
              onSuccess={refresh}
            />
          }
        />
      </Routes>
    )
  }

  return (
    <Routes>
      <Route element={<Layout version={me.version} />}>
        <Route path="/" element={<Navigate to="/code" replace />} />
        <Route path="/code" element={<CodingPage />} />
        <Route path="/code/:projectId" element={<CodingPage />} />
        <Route path="/tasks" element={<TasksPage />} />
        <Route path="/fleet" element={<Dashboard />} />
        <Route path="/devices/:id" element={<DevicePage />} />
        <Route path="/agents" element={<AgentsPage />} />
        <Route path="/setup" element={<SetupPage />} />
        <Route path="/settings" element={<SettingsPage />} />
        <Route path="*" element={<Navigate to="/" replace />} />
      </Route>
    </Routes>
  )
}
