import { StrictMode } from "react";
import { createRoot } from "react-dom/client";

import "@fontsource-variable/geist";
import "@fontsource-variable/geist-mono";
import "./index.css";
import { initTheme } from "../../web/theme/index.ts";
import { initTelemetry, TelemetryBoundary } from "../../web/telemetry.tsx";

import App from "./App";
import "./i18n/config";

initTheme();
initTelemetry(import.meta.env.VITE_SENTRY_DSN as string | undefined);

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <TelemetryBoundary>
      <App />
    </TelemetryBoundary>
  </StrictMode>,
);
