import { useCallback, useState, type FormEvent } from 'react'
import { useTranslation } from 'react-i18next'
import { api, ApiError } from '../api'
import { usePoll } from '../hooks'
import DataTable from '../components/DataTable'
import Modal from '../components/Modal'
import type { MCPConfig, Skill } from '../types'

// The platform operator's curation surface for the skill store (draft 57):
// one list, one form for create and edit. The hub owns the rules — duplicate
// names come back as a 409 — this screen only submits and shows what the hub
// answered. Same posture as TeamPage: the route and the endpoints behind it
// are what gate access, not a button here.
export default function SkillsPage() {
  const { t } = useTranslation()
  const [skills, setSkills] = useState<Skill[] | null>(null)
  const [error, setError] = useState('')
  const [editor, setEditor] = useState<Skill | 'new' | null>(null)

  const load = useCallback(async () => {
    try {
      setSkills(await api.get<Skill[]>('/api/admin/skills'))
      setError('')
    } catch (err) {
      setError(err instanceof Error ? err.message : t('skills.loadFailed'))
    }
  }, [t])

  usePoll(load, 30_000)

  const closeEditor = () => setEditor(null)

  const remove = async (skill: Skill) => {
    if (!window.confirm(t('skills.confirmDelete', { name: skill.name }))) return
    setError('')
    try {
      await api.del(`/api/admin/skills/${skill.id}`)
      await load()
    } catch (err) {
      setError(err instanceof Error ? err.message : t('skills.deleteFailed'))
    }
  }

  return (
    <div className="page-shell">
      <div className="mb-6 flex items-end justify-between">
        <div>
          <p className="eyebrow mb-3">{t('skills.eyebrow')}</p>
          <h1 className="text-3xl font-semibold tracking-[-0.04em] text-fg-strong">
            {t('skills.title')}
          </h1>
          <p className="mt-1 text-sm text-fg-muted">{t('skills.subtitle')}</p>
        </div>
        <button onClick={() => setEditor('new')} className="btn-primary">
          {t('skills.newSkill')}
        </button>
      </div>

      {error && (
        <p className="mb-4 rounded-lg border border-fail/20 px-3 py-2 text-sm text-fail-fg">
          {error}
        </p>
      )}

      <DataTable
        rows={skills}
        rowKey={(s) => s.id}
        empty={<p className="text-sm text-fg-subtle">{t('skills.noSkills')}</p>}
        columns={[
          {
            header: t('skills.name'),
            cell: (s) => <span className="text-fg">{s.name}</span>,
          },
          {
            header: t('skills.description'),
            cell: (s) => (
              <span className="text-fg-muted">{s.description || '—'}</span>
            ),
          },
          {
            header: t('skills.mcp'),
            cell: (s) =>
              s.mcp ? (
                <span className="rounded-full border border-accent/30 px-2 py-0.5 text-xs text-accent">
                  {t('skills.mcpBadge')}
                </span>
              ) : (
                <span className="text-fg-faint">—</span>
              ),
          },
          {
            header: t('skills.enabled'),
            cell: (s) =>
              s.enabled ? (
                <span className="rounded-full border border-accent/30 px-2 py-0.5 text-xs text-accent">
                  {t('skills.enabled')}
                </span>
              ) : (
                <span className="rounded-full border border-line-2 px-2 py-0.5 text-xs text-fg-subtle">
                  {t('skills.disabled')}
                </span>
              ),
          },
          {
            header: '',
            srHeader: t('skills.actions'),
            width: 'w-28',
            cell: (s) => (
              <div className="flex items-center gap-3">
                <button
                  onClick={() => setEditor(s)}
                  className="text-xs text-fg-subtle hover:text-fg"
                >
                  {t('common.edit')}
                </button>
                <button
                  onClick={() => void remove(s)}
                  className="text-xs text-fg-subtle hover:text-fail-fg"
                >
                  {t('common.delete')}
                </button>
              </div>
            ),
          },
        ]}
      />

      {editor !== null && (
        <Modal
          title={editor === 'new' ? t('skills.newTitle') : t('skills.editTitle')}
          onClose={closeEditor}
          wide
        >
          <SkillForm
            skill={editor === 'new' ? null : editor}
            onClose={closeEditor}
            onSaved={() => {
              closeEditor()
              void load()
            }}
          />
        </Modal>
      )}
    </div>
  )
}

function SkillForm({
  skill,
  onClose,
  onSaved,
}: {
  skill: Skill | null
  onClose: () => void
  onSaved: () => void
}) {
  const { t } = useTranslation()
  const [name, setName] = useState(skill?.name ?? '')
  const [description, setDescription] = useState(skill?.description ?? '')
  const [body, setBody] = useState(skill?.body ?? '')
  const [command, setCommand] = useState(skill?.mcp?.command ?? '')
  const [url, setUrl] = useState(skill?.mcp?.url ?? '')
  const [args, setArgs] = useState((skill?.mcp?.args ?? []).join(', '))
  const [env, setEnv] = useState(envToString(skill?.mcp?.env))
  const [enabled, setEnabled] = useState(skill?.enabled ?? true)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')

  const submit = async (e: FormEvent) => {
    e.preventDefault()
    setBusy(true)
    setError('')
    const payload = {
      name: name.trim(),
      description: description.trim(),
      body,
      mcp: buildMCP(command, url, args, env),
      enabled,
    }
    try {
      if (skill) {
        await api.patch<Skill>(`/api/admin/skills/${skill.id}`, payload)
      } else {
        await api.post<Skill>('/api/admin/skills', payload)
      }
      onSaved()
    } catch (err) {
      if (err instanceof ApiError && err.status === 409) {
        setError(t('skills.nameTaken'))
      } else {
        setError(err instanceof Error ? err.message : t('skills.saveFailed'))
      }
    } finally {
      setBusy(false)
    }
  }

  return (
    <form onSubmit={(e) => void submit(e)} className="flex flex-col gap-4">
      {error && (
        <p className="rounded-lg border border-fail/20 px-3 py-2 text-sm text-fail-fg">
          {error}
        </p>
      )}

      <label className="text-sm text-fg-soft">
        {t('skills.name')}
        <input
          type="text"
          required
          value={name}
          onChange={(e) => setName(e.target.value)}
          className="mt-1 w-full rounded-lg border border-line-2 bg-fill-2 px-3 py-2 text-fg-strong"
        />
      </label>

      <label className="text-sm text-fg-soft">
        {t('skills.description')}
        <input
          type="text"
          value={description}
          onChange={(e) => setDescription(e.target.value)}
          className="mt-1 w-full rounded-lg border border-line-2 bg-fill-2 px-3 py-2 text-fg-strong"
        />
      </label>

      <label className="text-sm text-fg-soft">
        {t('skills.body')}
        <textarea
          required
          rows={6}
          value={body}
          onChange={(e) => setBody(e.target.value)}
          className="mt-1 w-full rounded-lg border border-line-2 bg-fill-2 px-3 py-2 font-mono text-[12px] text-fg-strong"
        />
      </label>

      <section className="rounded-lg border border-line-2 p-4">
        <h3 className="text-sm font-medium text-fg">{t('skills.mcp')}</h3>
        <p className="mt-1 text-xs text-fg-subtle">{t('skills.mcpHint')}</p>
        <div className="mt-3 grid grid-cols-2 gap-3">
          <label className="text-sm text-fg-soft">
            {t('skills.mcpCommand')}
            <input
              type="text"
              value={command}
              onChange={(e) => setCommand(e.target.value)}
              className="mt-1 w-full rounded-lg border border-line-2 bg-fill-2 px-3 py-2 font-mono text-[12px] text-fg-strong"
            />
          </label>
          <label className="text-sm text-fg-soft">
            {t('skills.mcpUrl')}
            <input
              type="text"
              value={url}
              onChange={(e) => setUrl(e.target.value)}
              className="mt-1 w-full rounded-lg border border-line-2 bg-fill-2 px-3 py-2 font-mono text-[12px] text-fg-strong"
            />
          </label>
        </div>
        <label className="mt-3 block text-sm text-fg-soft">
          {t('skills.mcpArgs')}
          <input
            type="text"
            value={args}
            onChange={(e) => setArgs(e.target.value)}
            className="mt-1 w-full rounded-lg border border-line-2 bg-fill-2 px-3 py-2 font-mono text-[12px] text-fg-strong"
          />
          <span className="mt-1 block text-xs text-fg-subtle">{t('skills.mcpArgsHint')}</span>
        </label>
        <label className="mt-3 block text-sm text-fg-soft">
          {t('skills.mcpEnv')}
          <textarea
            rows={4}
            value={env}
            onChange={(e) => setEnv(e.target.value)}
            className="mt-1 w-full rounded-lg border border-line-2 bg-fill-2 px-3 py-2 font-mono text-[12px] text-fg-strong"
          />
          <span className="mt-1 block text-xs text-fg-subtle">{t('skills.mcpEnvHint')}</span>
        </label>
      </section>

      <label className="flex items-center gap-2 text-sm text-fg-soft">
        <input
          type="checkbox"
          checked={enabled}
          onChange={(e) => setEnabled(e.target.checked)}
          className="accent-accent"
        />
        {t('skills.enabled')}
      </label>

      <div className="mt-2 flex items-center justify-end gap-3">
        <button type="button" onClick={onClose} className="btn-secondary">
          {t('common.cancel')}
        </button>
        <button type="submit" disabled={busy} className="btn-primary">
          {busy ? t('common.loading') : t('common.save')}
        </button>
      </div>
    </form>
  )
}

// buildMCP assembles the MCP config from the form fields. An all-empty form
// means "no server", which travels as null rather than an empty object.
function buildMCP(command: string, url: string, args: string, env: string): MCPConfig | null {
  const argList = args.split(',').map((a) => a.trim()).filter(Boolean)
  const envMap = parseEnv(env)
  const cfg: MCPConfig = {}
  if (command.trim()) cfg.command = command.trim()
  if (url.trim()) cfg.url = url.trim()
  if (argList.length > 0) cfg.args = argList
  if (Object.keys(envMap).length > 0) cfg.env = envMap
  return Object.keys(cfg).length === 0 ? null : cfg
}

function parseEnv(env: string): Record<string, string> {
  const out: Record<string, string> = {}
  for (const line of env.split('\n')) {
    const trimmed = line.trim()
    if (!trimmed) continue
    const eq = trimmed.indexOf('=')
    if (eq <= 0) continue
    out[trimmed.slice(0, eq).trim()] = trimmed.slice(eq + 1).trim()
  }
  return out
}

function envToString(env: Record<string, string> | undefined): string {
  if (!env) return ''
  return Object.entries(env)
    .map(([key, value]) => `${key}=${value}`)
    .join('\n')
}
