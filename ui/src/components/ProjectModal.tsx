import { FormEvent, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
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
  const { t } = useTranslation()
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
  const connectorById = useMemo(() => new Map(connectors.map((connector) => [connector.id, connector])), [connectors])
  const available = connectors.filter((connector) => !enrolled.includes(connector.id))
  const runOnIds = enrolled.length > 0 ? enrolled : connectors.map((connector) => connector.id)

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
    <Modal title={project ? t('project.editTitle') : t('project.addTitle')} onClose={onClose}>
      <form onSubmit={save} className="space-y-5">
        <label className="block">
          <span className="field-label">{t('project.name')}</span>
          <input
            value={name}
            onChange={(event) => setName(event.target.value)}
            autoFocus
            maxLength={80}
            placeholder={t('project.namePlaceholder')}
            className="field-input mt-2"
          />
        </label>

        {editing ? (
          <div>
            <span className="field-label">{t('project.machines')}</span>
            <ul className="mt-2 space-y-2">
              {enrolled.length === 0 ? (
                <li className="text-xs text-fg-faint">{t('project.noMachines')}</li>
              ) : (
                enrolled.map((id) => {
                  const connector = connectorById.get(id)
                  return (
                    <li key={id} className="flex items-center gap-2 rounded-lg border border-line-2 px-3 py-2">
                      <span className="min-w-0 flex-1 truncate text-sm text-fg">
                        {connector?.name ?? id}
                        <span className="ml-2 text-xs text-fg-faint">
                          {connector?.online ? t('project.online') : t('project.offline')}
                        </span>
                      </span>
                      <button
                        type="button"
                        onClick={() => removeMachine(id)}
                        disabled={adding}
                        className="text-xs text-fg-subtle hover:text-fail-fg"
                      >
                        {t('team.remove')}
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
                    { value: '', label: t('project.addMachine') },
                    ...available.map((connector) => ({
                      value: connector.id,
                      label: `${connector.name} · ${t(connector.online ? 'project.online' : 'project.offline')}`,
                    })),
                  ]}
                />
                <button type="button" onClick={addMachine} disabled={adding || !addId} className="btn-secondary shrink-0">
                  {adding ? t('project.adding') : t('common.add')}
                </button>
              </div>
            ) : null}
          </div>
        ) : null}

        <label className="block">
          <span className="field-label">{t('project.runOn')}</span>
          <div className="mt-2">
            <SimpleSelect
              size="default"
              value={connectorId}
              onValueChange={setConnectorId}
              className="w-full"
              items={runOnIds.map((id) => {
                const connector = connectorById.get(id)
                return {
                  value: id,
                  label: `${connector?.name ?? id} · ${t(connector?.online ? 'project.online' : 'project.offline')}`,
                }
              })}
            />
          </div>
          <span className="mt-2 block text-xs text-fg-faint">
            {editing ? t('project.fxRunOnEdit') : t('project.fxRunOnNew')}
          </span>
        </label>

        <label className="block">
          <span className="field-label">{t('common.workingDirectory')}</span>
          <input
            value={path}
            onChange={(event) => setPath(event.target.value)}
            placeholder={t('project.pathPlaceholder')}
            className="field-input mt-2 font-mono text-xs"
          />
        </label>

        {error ? <HubError error={error} fallback={t('project.saveFailed')} className="rounded-lg border border-fail/20 bg-fail/10 px-3 py-2 text-sm text-fail-fg" /> : null}

        <div className="flex justify-end gap-2 border-t border-line-2 pt-4">
          <button type="button" onClick={onClose} className="btn-secondary">{t('common.cancel')}</button>
          <button type="submit" disabled={saving || !name.trim() || !connectorId || !path.trim()} className="btn-primary">
            {saving ? t('project.saving') : project ? t('project.saveChanges') : t('project.addProject')}
          </button>
        </div>
      </form>
    </Modal>
  )
}
