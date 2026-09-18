import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { api, forProject } from '../api'
import { useHubEvents } from '../hooks'
import Modal from './Modal'

// projectId names which project the new connector joins. Omitting it is correct
// on a hub with one project, which the hub resolves for us.
export default function AddConnectorModal({ onClose, projectId }: { onClose: () => void; projectId?: string }) {
  const { t } = useTranslation()
  const [command, setCommand] = useState('')
  const [windowsCommand, setWindowsCommand] = useState('')
  const [platform, setPlatform] = useState<'unix' | 'windows'>('unix')
  const [error, setError] = useState('')
  const [copied, setCopied] = useState(false)
  const [joined, setJoined] = useState(false)

  useEffect(() => {
    api
      .post<{ command: string; windowsCommand: string }>(forProject('/api/enroll-tokens', projectId))
      .then((r) => {
        setCommand(r.command)
        setWindowsCommand(r.windowsCommand)
      })
      .catch((e) => setError(e.message))
  }, [projectId])

  // The modal celebrates live when the new connector connects.
  useHubEvents((e) => {
    if (e.type === 'connector.online') setJoined(true)
  })

  const copy = async () => {
    await navigator.clipboard.writeText(activeCommand)
    setCopied(true)
    setTimeout(() => setCopied(false), 1500)
  }

  const activeCommand = platform === 'windows' ? windowsCommand : command

  return (
    <Modal title={t('addConnector.title')} onClose={onClose}>
      <p className="mb-4 text-sm text-fg-muted">
        {t('addConnector.hint')}
      </p>
      {error ? (
        <p className="text-sm text-fail-fg">{error}</p>
      ) : (
        <>
          <div className="mb-3 inline-flex rounded-lg border border-line-3 bg-canvas-sunken p-1">
            <button
              type="button"
              onClick={() => setPlatform('unix')}
              className={`rounded-md px-3 py-1.5 text-sm transition ${
                platform === 'unix'
                  ? 'bg-sidebar text-fg-strong'
                  : 'text-fg-muted hover:text-fg'
              }`}
            >
              {t('boarding.unix')}
            </button>
            <button
              type="button"
              onClick={() => setPlatform('windows')}
              className={`rounded-md px-3 py-1.5 text-sm transition ${
                platform === 'windows'
                  ? 'bg-sidebar text-fg-strong'
                  : 'text-fg-muted hover:text-fg'
              }`}
            >
              {t('boarding.windows')}
            </button>
          </div>
          <div className="mb-4 flex items-stretch gap-2">
            <code className="flex-1 overflow-x-auto whitespace-nowrap rounded-lg border border-line-3 bg-canvas-sunken p-3 font-mono text-[13px] text-ok">
              {activeCommand || t('addConnector.generating')}
            </code>
            <button
              onClick={copy}
              disabled={!activeCommand}
              className="shrink-0 rounded-lg border border-line-3 px-3 text-sm text-fg-soft transition hover:bg-sidebar disabled:opacity-50"
            >
              {copied ? t('boarding.copied') : t('boarding.copy')}
            </button>
          </div>
        </>
      )}
      <p className="mb-4 text-xs text-fg-subtle">
        {t('addConnector.expire')}
      </p>
      {joined ? (
        <div className="flex items-center justify-between rounded-lg border border-ok/30 bg-ok/10 p-3">
          <span className="text-sm font-medium text-ok">
            {t('addConnector.joined')}
          </span>
          <button
            onClick={onClose}
            className="rounded-lg bg-ok px-3 py-1.5 text-sm font-medium text-ok-on hover:brightness-110"
          >
            {t('addConnector.seeIt')}
          </button>
        </div>
      ) : (
        <div className="flex items-center gap-2 text-sm text-fg-subtle">
          <span className="h-2 w-2 animate-pulse rounded-full bg-accent" />
          {t('addConnector.waiting')}
        </div>
      )}
    </Modal>
  )
}
