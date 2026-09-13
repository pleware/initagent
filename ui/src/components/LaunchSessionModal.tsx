import { FormEvent, useEffect, useState } from 'react'
import { SimpleSelect } from '@ia/web/components/SimpleSelect'
import { api } from '../api'
import type { Connector, Preset } from '../types'
import Modal from './Modal'

// LaunchSessionModal starts a named session (usually a coding agent) on a
// device. Used from both the device page (single device) and the agents page
// (device picker).
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
    'w-full rounded-lg border border-zinc-700 bg-zinc-950 px-3 py-2 text-sm text-zinc-100 outline-none focus:border-lime-500'

  return (
    <Modal title="Launch session" onClose={onClose}>
      <form onSubmit={submit} className="flex flex-col gap-4">
        {connectors.length > 1 && (
          <div>
            <label className="mb-1 block text-sm font-medium text-zinc-300">
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
          <label className="mb-1 block text-sm font-medium text-zinc-300">
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
                    ? 'border-lime-500 bg-lime-500/10 text-lime-300'
                    : 'border-zinc-700 text-zinc-300 hover:border-zinc-600'
                }`}
              >
                {p.name}
              </button>
            ))}
          </div>
        </div>

        <div>
          <label className="mb-1 block text-sm font-medium text-zinc-300">
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
            <label className="mb-1 block text-sm font-medium text-zinc-300">
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
            <label className="mb-1 block text-sm font-medium text-zinc-300">
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

        {error && <p className="text-sm text-rose-400">{error}</p>}
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
