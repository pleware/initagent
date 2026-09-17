import { FormEvent, useEffect, useState } from 'react'
import { SimpleSelect } from '@ia/web/components/SimpleSelect'
import { api } from '../api'
import type { Connector, Preset } from '../types'
import Modal from './Modal'

// LaunchSessionModal starts a named session (usually a coding agent) on a
// connector. Used from both the connector page (single connector) and the agents page
// (connector picker).
export default function LaunchSessionModal({
  connectors,
  onLaunched,
  onClose,
}: {
  connectors: Connector[]
  onLaunched: (connectorId: string, session: string) => void
  onClose: () => void
}) {
  const online = connectors.filter((d) => d.online)
  const [connectorId, setConnectorId] = useState(online[0]?.id ?? '')
  const [presets, setPresets] = useState<Preset[]>([])
  const [presetId, setPresetId] = useState<number | null>(null)
  const [name, setName] = useState('')
  const [cwd, setCwd] = useState('')
  const [command, setCommand] = useState('')
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)

  useEffect(() => {
    api.get<Preset[]>('/api/presets').then((ps) => {
      setPresets(ps)
      const claude = ps.find((p) => p.kind === 'claude')
      if (claude) selectPreset(claude, ps)
    })
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  const selectPreset = (p: Preset, _all?: Preset[]) => {
    setPresetId(p.id)
    setCommand(p.command)
    if (!name || presets.some((x) => name === defaultName(x))) {
      setName(defaultName(p))
    }
  }

  const defaultName = (p: Preset) =>
    p.kind === 'shell' ? 'term-1' : `${p.kind}-${new Date().getMinutes()}${new Date().getSeconds()}`

  const submit = async (e: FormEvent) => {
    e.preventDefault()
    setError('')
    if (!connectorId || !name) return
    setBusy(true)
    const preset = presets.find((p) => p.id === presetId)
    try {
      await api.post(`/api/connectors/${connectorId}/sessions`, {
        name,
        cwd,
        command,
        kind: preset?.kind ?? (command ? command.split(/\s+/)[0] : 'shell'),
      })
      onLaunched(connectorId, name)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'failed to launch')
      setBusy(false)
    }
  }

  const inputClass =
    'w-full rounded-lg border border-line-3 bg-canvas-sunken px-3 py-2 text-sm text-fg-strong outline-none focus:border-accent'

  return (
    <Modal title="Launch session" onClose={onClose}>
      <form onSubmit={submit} className="flex flex-col gap-4">
        {connectors.length > 1 && (
          <div>
            <label className="mb-1 block text-sm font-medium text-fg-soft">
              Connector
            </label>
            <SimpleSelect
              size="default"
              value={connectorId}
              onValueChange={setConnectorId}
              className="w-full"
              items={online.map((d) => ({
                value: d.id,
                label: `${d.name} (${d.os}/${d.arch})`,
              }))}
            />
          </div>
        )}

        <div>
          <label className="mb-1 block text-sm font-medium text-fg-soft">
            What to run
          </label>
          <div className="flex flex-wrap gap-2">
            {presets.map((p) => (
              <button
                key={p.id}
                type="button"
                onClick={() => selectPreset(p)}
                className={`rounded-lg border px-3 py-1.5 text-sm transition ${
                  presetId === p.id
                    ? 'border-accent bg-accent/10 text-accent'
                    : 'border-line-3 text-fg-soft hover:border-line-4'
                }`}
              >
                {p.name}
              </button>
            ))}
          </div>
        </div>

        <div>
          <label className="mb-1 block text-sm font-medium text-fg-soft">
            Command
          </label>
          <input
            value={command}
            onChange={(e) => {
              setCommand(e.target.value)
              setPresetId(null)
            }}
            placeholder="empty = plain shell"
            className={`${inputClass} font-mono`}
          />
        </div>

        <div className="grid grid-cols-2 gap-3">
          <div>
            <label className="mb-1 block text-sm font-medium text-fg-soft">
              Session name
            </label>
            <input
              value={name}
              onChange={(e) => setName(e.target.value)}
              required
              pattern="[a-zA-Z0-9._\-]+"
              title="Letters, digits, dots, dashes, underscores"
              className={`${inputClass} font-mono`}
            />
          </div>
          <div>
            <label className="mb-1 block text-sm font-medium text-fg-soft">
              Working directory
            </label>
            <input
              value={cwd}
              onChange={(e) => setCwd(e.target.value)}
              placeholder="~ (home)"
              className={`${inputClass} font-mono`}
            />
          </div>
        </div>

        {error && <p className="text-sm text-fail-fg">{error}</p>}
        <button
          type="submit"
          disabled={busy || !connectorId}
          className="btn-primary"
        >
          {busy ? 'Launching…' : 'Launch'}
        </button>
      </form>
    </Modal>
  )
}
