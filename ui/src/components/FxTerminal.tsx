import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { FitAddon } from '@xterm/addon-fit'
import { Terminal } from '@xterm/xterm'
import { execProject } from '../api'
import { fxConfigStore, fxOAuthStore, fxPromptHistoryStore, fxSessionStore } from '../lib/fxStorage'
import type { Connector, ExecResult, Project } from '../types'

type FxRuntime = {
  interactive: Promise<void>
  exited: Promise<number>
  write(data: string): void
  resize(): void
  abort(): void
}

export default function FxTerminal({ project, connector }: { project: Project; connector?: Connector }) {
  const { t } = useTranslation()
  const hostRef = useRef<HTMLDivElement>(null)
  const runtimeRef = useRef<FxRuntime | null>(null)
  const [state, setState] = useState<'loading' | 'ready' | 'unsupported' | 'failed'>('loading')
  const [error, setError] = useState('')

  useEffect(() => {
    const host = hostRef.current
    if (!host || !connector?.online) return

    const wasmWithJspi = WebAssembly as typeof WebAssembly & {
      Suspending?: unknown
      promising?: unknown
    }
    if (typeof wasmWithJspi.Suspending !== 'function' || typeof wasmWithJspi.promising !== 'function') {
      setState('unsupported')
      return
    }

    let disposed = false
    setState('loading')
    setError('')

    const terminal = new Terminal({
      cursorBlink: true,
      cursorStyle: 'bar',
      fontSize: 13,
      lineHeight: 1.45,
      letterSpacing: 0.15,
      fontFamily: "'SF Mono', 'JetBrains Mono', ui-monospace, Menlo, monospace",
      scrollback: 20_000,
      allowProposedApi: true,
      theme: {
        background: '#151719',
        foreground: '#d7d9dc',
        cursor: '#ffb86a',
        cursorAccent: '#151719',
        selectionBackground: '#344050',
        black: '#202326',
        red: '#ff7c85',
        green: '#77d9ab',
        yellow: '#f2c879',
        blue: '#82aaff',
        magenta: '#c792ea',
        cyan: '#75d7e8',
        white: '#d7d9dc',
        brightBlack: '#686d73',
        brightWhite: '#ffffff',
      },
    })
    const fit = new FitAddon()
    terminal.loadAddon(fit)
    terminal.open(host)
    fit.fit()

    const resizeObserver = new ResizeObserver(() => {
      fit.fit()
      runtimeRef.current?.resize()
    })
    resizeObserver.observe(host)

    const virtualRoot = `/projects/${project.id}`
    const workspace = {
      info: {
        version: 1,
        root: virtualRoot,
        cwd: virtualRoot,
        home: '/home/liveagent',
        gitAvailable: false,
        ephemeral: true,
      },
      permission: 'prompt',
      async exec({
        command,
        signal,
        timeoutMs,
        outputLimitBytes,
      }: {
        command: string
        cwd: string
        signal?: AbortSignal
        timeoutMs: number
        outputLimitBytes: number
      }) {
        const result = await execProject(project.id, command, timeoutMs, signal) as ExecResult
        return {
          stdout: utf8Limit(result.stdout, outputLimitBytes),
          stderr: utf8Limit(result.stderr, outputLimitBytes),
          exitCode: result.exitCode,
        }
      },
    }

    void import('libfx/browser').then(({ createFxTerminal, xtermAdapter }) => createFxTerminal({
        terminal: xtermAdapter(terminal),
        workspace,
        oauthSessionStore: fxOAuthStore,
        sessionStore: fxSessionStore,
        promptHistoryStore: fxPromptHistoryStore,
        configStore: fxConfigStore,
        openUrl(url: string) {
          return window.open(url, '_blank', 'noopener,noreferrer') !== null
        },
        onEvent(event: { type?: string }) {
          if (event.type === 'runtime.ready' && !disposed) setState('ready')
        },
      })).then(async (runtime) => {
      if (disposed) {
        runtime.abort()
        return
      }
      runtimeRef.current = runtime
      await runtime.interactive
      if (!disposed) {
        setState('ready')
        terminal.focus()
      }
    }).catch((cause) => {
      if (disposed) return
      setState('failed')
      setError(cause instanceof Error ? cause.message : t('fx.couldNotStart'))
    })

    return () => {
      disposed = true
      resizeObserver.disconnect()
      runtimeRef.current?.abort()
      runtimeRef.current = null
      terminal.dispose()
    }
  }, [connector?.id, connector?.online, project.id, project.updatedAt])

  if (!connector?.online) {
    return (
      <TerminalNotice
        title={connector?.name ? t('fx.offlineNamed', { name: connector.name }) : t('fx.offlineGeneric')}
        body={t('fx.offlineBody')}
      />
    )
  }

  if (state === 'unsupported') {
    return (
      <TerminalNotice
        title={t('fx.unsupportedTitle')}
        body={t('fx.unsupportedBody')}
      />
    )
  }

  if (state === 'failed') {
    return <TerminalNotice title={t('fx.couldNotStart')} body={error} />
  }

  return (
    <div className="relative h-full min-h-0 overflow-hidden bg-canvas-sunken">
      <div ref={hostRef} className="fx-terminal-host h-full w-full px-3 py-4 sm:px-5" />
      {state === 'loading' && (
        <div className="absolute inset-0 grid place-items-center bg-canvas-sunken text-xs text-fg-subtle">
          <span className="flex items-center gap-2"><span className="spinner" />{t('fx.booting')}</span>
        </div>
      )}
    </div>
  )
}

function TerminalNotice({ title, body }: { title: string; body: string }) {
  return (
    <div className="grid h-full min-h-80 place-items-center bg-canvas-sunken p-8 text-center">
      <div className="max-w-md">
        <span className="mx-auto grid h-10 w-10 place-items-center rounded-xl border border-line-2 bg-fill-2 font-mono text-sm text-warn-fg">fx</span>
        <h2 className="mt-4 text-base font-semibold text-fg">{title}</h2>
        <p className="mt-2 text-sm leading-6 text-fg-subtle">{body}</p>
      </div>
    </div>
  )
}

function utf8Limit(value: string, maxBytes: number) {
  const bytes = new TextEncoder().encode(value)
  if (bytes.length <= maxBytes) return value
  return new TextDecoder().decode(bytes.slice(0, maxBytes))
}
