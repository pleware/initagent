import { FormEvent, useMemo, useState } from 'react'
import { SimpleSelect } from '@ia/web/components/SimpleSelect'
import { api } from '../api'
import type { Connector, Project } from '../types'
import Modal from './Modal'
import { HubError } from './PlanWall'

function enrolledIds(project?: Project): string[] {
  if (project?.connectorIds?.length) return project.connectorIds
  if (project?.connectorId) return [project.connectorId]
  return []
}

export default function ProjectModal({
  connectors,
  project,
  onClose,
  onSaved,
  onUpdated,
}: {
  connectors: Connector[]
  project?: Project
  onClose: () => void
  onSaved: (project: Project) => void
  onUpdated?: (project: Project) => void
}) {
  const online = useMemo(() => connectors.filter((item) => item.online), [connectors])
  const firstConnector = online[0]?.id ?? connectors[0]?.id ?? ''
  const [name, setName] = useState(project?.name ?? '')
  const [enrolled, setEnrolled] = useState<string[]>(() => enrolledIds(project))
  const [connectorId, setConnectorId] = useState(project?.connectorId ?? firstConnector)
  const [addId, setAddId] = useState('')
  const [path, setPath] = useState(project?.path ?? '')
  const [saving, setSaving] = useState(false)
  const [adding, setAdding] = useState(false)
  const [error, setError] = useState<unknown>(null)

  const editing = Boolean(project)
  const deviceById = useMemo(() => new Map(connectors.map((device) => [device.id, device])), [connectors])
  const available = connectors.filter((device) => !enrolled.includes(device.id))
  const runOnIds = enrolled.length > 0 ? enrolled : connectors.map((device) => device.id)

  const apply = (saved: Project) => {
    setEnrolled(enrolledIds(saved))
    setConnectorId(saved.connectorId || saved.connectorIds?.[0] || '')
    onUpdated?.(saved)
  }

  const addMachine = async () => {
    if (!project || !addId) return
    setAdding(true)
    setError(null)
    try {
      const saved = await api.post<Project>(`/api/projects/${project.id}/connectors`, { connectorId: addId })
      setAddId('')
      apply(saved)
    } catch (cause) {
      setError(cause)
    } finally {
      setAdding(false)
    }
  }

  const removeMachine = async (id: string) => {
    if (!project) return
    setAdding(true)
    setError(null)
    try {
      const saved = await api.del<Project>(`/api/projects/${project.id}/connectors/${id}`)
      apply(saved)
    } catch (cause) {
      setError(cause)
    } finally {
      setAdding(false)
    }
  }

  const save = async (event: FormEvent) => {
    event.preventDefault()
    if (!name.trim() || !connectorId || !path.trim()) return
    setSaving(true)
    setError(null)
    try {
      const body = { name: name.trim(), connectorId, path: path.trim() }
      const saved = project
        ? await api.patch<Project>(`/api/projects/${project.id}`, body)
        : await api.post<Project>('/api/projects', body)
      window.dispatchEvent(new Event('liveagent:projects-changed'))
      onSaved(saved)
    } catch (cause) {
      setError(cause)
    } finally {
      setSaving(false)
    }
  }

  return (
    <Modal title={project ? 'Edit project' : 'Add a project'} onClose={onClose}>
      <form onSubmit={save} className="space-y-5">
        <label className="block">
          <span className="field-label">Project name</span>
          <input
            value={name}
            onChange={(event) => setName(event.target.value)}
            autoFocus
            maxLength={80}
            placeholder="Storefront"
            className="field-input mt-2"
          />
        </label>

        {editing ? (
          <div>
            <span className="field-label">Your machines</span>
            <ul className="mt-2 space-y-2">
              {enrolled.length === 0 ? (
                <li className="text-xs text-zinc-600">No machine on this project yet.</li>
              ) : (
                enrolled.map((id) => {
                  const device = deviceById.get(id)
                  return (
                    <li key={id} className="flex items-center gap-2 rounded-lg border border-white/[0.07] px-3 py-2">
                      <span className="min-w-0 flex-1 truncate text-sm text-zinc-200">
                        {device?.name ?? id}
                        <span className="ml-2 text-xs text-zinc-600">{device?.online ? 'online' : 'offline'}</span>
                      </span>
                      <button
                        type="button"
                        onClick={() => removeMachine(id)}
                        disabled={adding}
                        className="text-xs text-zinc-500 hover:text-rose-200"
                      >
                        Remove
                      </button>
                    </li>
                  )
                })
              )}
            </ul>
            {available.length > 0 ? (
              <div className="mt-3 flex gap-2">
                <SimpleSelect
                  size="default"
                  value={addId}
                  onValueChange={setAddId}
                  className="min-w-0 flex-1"
                  items={[
                    { value: '', label: 'Add a machine…' },
                    ...available.map((device) => ({
                      value: device.id,
                      label: `${device.name} ${device.online ? '· online' : '· offline'}`,
                    })),
                  ]}
                />
                <button type="button" onClick={addMachine} disabled={adding || !addId} className="btn-secondary shrink-0">
                  {adding ? 'Adding…' : 'Add'}
                </button>
              </div>
            ) : null}
          </div>
        ) : null}

        <label className="block">
          <span className="field-label">Run on</span>
          <div className="mt-2">
            <SimpleSelect
              size="default"
              value={connectorId}
              onValueChange={setConnectorId}
              className="w-full"
              items={runOnIds.map((id) => {
                const device = deviceById.get(id)
                return {
                  value: id,
                  label: `${device?.name ?? id} ${device?.online ? '· online' : '· offline'}`,
                }
              })}
            />
          </div>
          <span className="mt-2 block text-xs text-zinc-600">
            {editing
              ? 'fx sends commands only to this machine. Other enrolled machines stay on the project.'
              : 'fx sends commands only to this machine for this project.'}
          </span>
        </label>

        <label className="block">
          <span className="field-label">Working directory</span>
          <input
            value={path}
            onChange={(event) => setPath(event.target.value)}
            placeholder="/Users/you/Projects/storefront"
            className="field-input mt-2 font-mono text-xs"
          />
        </label>

        {error ? <HubError error={error} fallback="Could not save project" className="rounded-lg border border-rose-400/20 bg-rose-400/[0.07] px-3 py-2 text-sm text-rose-200" /> : null}

        <div className="flex justify-end gap-2 border-t border-white/[0.07] pt-4">
          <button type="button" onClick={onClose} className="btn-secondary">Cancel</button>
          <button type="submit" disabled={saving || !name.trim() || !connectorId || !path.trim()} className="btn-primary">
            {saving ? 'Saving…' : project ? 'Save changes' : 'Add project'}
          </button>
        </div>
      </form>
    </Modal>
  )
}
