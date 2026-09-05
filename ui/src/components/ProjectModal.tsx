import { FormEvent, useMemo, useState } from 'react'
import { api } from '../api'
import type { Device, Project } from '../types'
import Modal from './Modal'
import { HubError } from './PlanWall'

function enrolledIds(project?: Project): string[] {
  if (project?.deviceIds?.length) return project.deviceIds
  if (project?.deviceId) return [project.deviceId]
  return []
}

export default function ProjectModal({
  devices,
  project,
  onClose,
  onSaved,
  onUpdated,
}: {
  devices: Device[]
  project?: Project
  onClose: () => void
  onSaved: (project: Project) => void
  onUpdated?: (project: Project) => void
}) {
  const online = useMemo(() => devices.filter((device) => device.online), [devices])
  const firstDevice = online[0]?.id ?? devices[0]?.id ?? ''
  const [name, setName] = useState(project?.name ?? '')
  const [enrolled, setEnrolled] = useState<string[]>(() => enrolledIds(project))
  const [deviceId, setDeviceId] = useState(project?.deviceId ?? firstDevice)
  const [addId, setAddId] = useState('')
  const [path, setPath] = useState(project?.path ?? '')
  const [saving, setSaving] = useState(false)
  const [adding, setAdding] = useState(false)
  const [error, setError] = useState<unknown>(null)

  const editing = Boolean(project)
  const deviceById = useMemo(() => new Map(devices.map((device) => [device.id, device])), [devices])
  const available = devices.filter((device) => !enrolled.includes(device.id))
  const runOnIds = enrolled.length > 0 ? enrolled : devices.map((device) => device.id)

  const apply = (saved: Project) => {
    setEnrolled(enrolledIds(saved))
    setDeviceId(saved.deviceId || saved.deviceIds?.[0] || '')
    onUpdated?.(saved)
  }

  const addMachine = async () => {
    if (!project || !addId) return
    setAdding(true)
    setError(null)
    try {
      const saved = await api.post<Project>(`/api/projects/${project.id}/devices`, { deviceId: addId })
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
      const saved = await api.del<Project>(`/api/projects/${project.id}/devices/${id}`)
      apply(saved)
    } catch (cause) {
      setError(cause)
    } finally {
      setAdding(false)
    }
  }

  const save = async (event: FormEvent) => {
    event.preventDefault()
    if (!name.trim() || !deviceId || !path.trim()) return
    setSaving(true)
    setError(null)
    try {
      const body = { name: name.trim(), deviceId, path: path.trim() }
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
                <select
                  value={addId}
                  onChange={(event) => setAddId(event.target.value)}
                  className="field-input min-w-0 flex-1"
                >
                  <option value="" className="bg-zinc-900">Add a machine…</option>
                  {available.map((device) => (
                    <option key={device.id} value={device.id} className="bg-zinc-900">
                      {device.name} {device.online ? '· online' : '· offline'}
                    </option>
                  ))}
                </select>
                <button type="button" onClick={addMachine} disabled={adding || !addId} className="btn-secondary shrink-0">
                  {adding ? 'Adding…' : 'Add'}
                </button>
              </div>
            ) : null}
          </div>
        ) : null}

        <label className="block">
          <span className="field-label">Run on</span>
          <select value={deviceId} onChange={(event) => setDeviceId(event.target.value)} className="field-input mt-2">
            {runOnIds.map((id) => {
              const device = deviceById.get(id)
              return (
                <option key={id} value={id} className="bg-zinc-900">
                  {device?.name ?? id} {device?.online ? '· online' : '· offline'}
                </option>
              )
            })}
          </select>
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
          <button type="submit" disabled={saving || !name.trim() || !deviceId || !path.trim()} className="btn-primary">
            {saving ? 'Saving…' : project ? 'Save changes' : 'Add project'}
          </button>
        </div>
      </form>
    </Modal>
  )
}
