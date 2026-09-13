import React from 'react'
import ReactDOM from 'react-dom/client'
import { BrowserRouter } from 'react-router-dom'
import App from './App'
import './index.css'
import './i18n/config' // Initialize i18n
import { initTheme } from './theme'
import { initTelemetry, TelemetryBoundary } from '@ia/web/telemetry'

initTheme()
initTelemetry(import.meta.env.VITE_SENTRY_DSN as string | undefined)

ReactDOM.createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <TelemetryBoundary>
      <BrowserRouter>
        <App />
      </BrowserRouter>
    </TelemetryBoundary>
  </React.StrictMode>,
)
