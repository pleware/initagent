import * as Sentry from "@sentry/react";
import type { ReactNode } from "react";

/**
 * Self-hosted GlitchTip (draft 55). The `initagent` project collects both the
 * marketing site and the cockpit. The DSN is public — safe to ship in client
 * code — and mirrors the `PROD_SITE` / `PROD_HUB` pattern in `origins.ts`:
 * a production default with a Vite env override.
 */
const DEFAULT_DSN = "https://0e7a8c799f0e4b318255e39ddff6d2b2@telemetry.pware.ai/1";

/**
 * Initialise the Sentry SDK for the browser. Call once, before the app
 * renders, so unhandled exceptions and promise rejections are captured from
 * the first frame. Override the DSN with `VITE_SENTRY_DSN`.
 */
export function initTelemetry(envDsn?: string): void {
  const dsn = (envDsn ?? "").trim() || DEFAULT_DSN;
  Sentry.init({
    dsn,
    environment: import.meta.env.PROD ? "production" : "development",
    // Error capture only; performance tracing is a separate decision (draft 55).
    tracesSampleRate: 0,
  });
}

/**
 * Wraps the app so a React render error is reported with its component stack
 * and shown a minimal fallback instead of a blank page.
 */
export function TelemetryBoundary({ children }: { children: ReactNode }) {
  return (
    <Sentry.ErrorBoundary
      fallback={
        <div
          style={{
            minHeight: "100vh",
            display: "flex",
            flexDirection: "column",
            alignItems: "center",
            justifyContent: "center",
            gap: "1rem",
            padding: "2rem",
            fontFamily: "system-ui, sans-serif",
            textAlign: "center",
          }}
        >
          <h1>Something went wrong</h1>
          <p>The error has been reported. Reload to continue.</p>
          <button type="button" onClick={() => window.location.reload()}>
            Reload
          </button>
        </div>
      }
    >
      {children}
    </Sentry.ErrorBoundary>
  );
}
