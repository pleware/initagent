import { useCallback, useEffect, useState, type FormEvent } from 'react'
import { useTranslation } from 'react-i18next'
import {
  api,
  ApiError,
  timeAgo,
  deleteBox,
  listBoxTokens,
  mintBoxToken,
  revokeBoxToken,
} from '../api'
import { usePoll } from '../hooks'
import DataTable from '../components/DataTable'
import Modal from '../components/Modal'
import { SimpleSelect } from '@ia/web/components/SimpleSelect'
import type { Box, BoxEdition, BoxToken, Org, Staff } from '../types'

// The editions the hub knows, weakest-named first, in select order. The
// labels live in i18n under boxes.edition*.
const EDITIONS: { value: BoxEdition; labelKey: string }[] = [
  { value: 'company', labelKey: 'boxes.editionCompany' },
  { value: 'home', labelKey: 'boxes.editionHome' },
  { value: 'assist', labelKey: 'boxes.editionAssist' },
  { value: 'care', labelKey: 'boxes.editionCare' },
  { value: 'lite', labelKey: 'boxes.editionLite' },
]

// The platform operator's boxes (58): the PWare OS appliances this
// installation configures. One list, a create form, an organization binding
// editor, a narrator preview, an edit form, a delete, and per-box tokens.
// The hub owns the rules — a duplicate slug comes back as a 409 — this
// screen only submits and shows what the hub answered, the same posture as
// SkillsPage.
export default function BoxesPage() {
  const { t } = useTranslation()
  const [boxes, setBoxes] = useState<Box[] | null>(null)
  // Each box's bound org set, keyed by box id. Loaded from the hub on every
  // refresh so the org count and the org editor's checkboxes reflect the
  // database, not just this session's edits.
  const [boundOrgs, setBoundOrgs] = useState<Record<string, string[]>>({})
  const [error, setError] = useState('')
  const [createOpen, setCreateOpen] = useState(false)
  const [orgEditor, setOrgEditor] = useState<Box | null>(null)
  const [narratorBox, setNarratorBox] = useState<Box | null>(null)
  const [editBox, setEditBox] = useState<Box | null>(null)
  const [tokensBox, setTokensBox] = useState<Box | null>(null)
  const [deleteTarget, setDeleteTarget] = useState<Box | null>(null)
  const [busyDelete, setBusyDelete] = useState(false)

  const load = useCallback(async () => {
    try {
      const boxes = await api.get<Box[]>('/api/boxes')
      setBoxes(boxes)
      // Fetch each box's bound org set so the count column and the org
      // editor's checkboxes reflect what is actually bound in the database.
      const sets = await Promise.all(
        boxes.map(async (b) => {
          const { orgIds } = await api.get<{ orgIds: string[] }>(
            `/api/boxes/${b.id}/orgs`,
          )
          return [b.id, orgIds] as const
        }),
      )
      setBoundOrgs(Object.fromEntries(sets))
      setError('')
    } catch (err) {
      setError(err instanceof Error ? err.message : t('boxes.loadFailed'))
    }
  }, [t])

  usePoll(load, 30_000)

  const confirmDelete = async () => {
    if (!deleteTarget) return
    setBusyDelete(true)
    try {
      await deleteBox(deleteTarget.id)
      setDeleteTarget(null)
      void load()
    } catch (err) {
      // A 404 means the box is already gone — refetch and close, no banner.
      if (err instanceof ApiError && err.status === 404) {
        setDeleteTarget(null)
        void load()
      } else {
        setError(err instanceof Error ? err.message : t('boxes.deleteFailed'))
        setDeleteTarget(null)
      }
    } finally {
      setBusyDelete(false)
    }
  }

  return (
    <div className="page-shell">
      <div className="mb-6 flex items-end justify-between">
        <div>
          <p className="eyebrow mb-3">{t('boxes.eyebrow')}</p>
          <h1 className="text-3xl font-semibold tracking-[-0.04em] text-fg-strong">
            {t('boxes.title')}
          </h1>
          <p className="mt-1 text-sm text-fg-muted">{t('boxes.subtitle')}</p>
        </div>
        <button onClick={() => setCreateOpen(true)} className="btn-primary">
          {t('boxes.newBox')}
        </button>
      </div>

      {error && (
        <p className="mb-4 rounded-lg border border-fail/20 px-3 py-2 text-sm text-fail-fg">
          {error}
        </p>
      )}

      <DataTable
        rows={boxes}
        rowKey={(b) => b.id}
        empty={<p className="text-sm text-fg-subtle">{t('boxes.noBoxes')}</p>}
        columns={[
          {
            header: t('boxes.name'),
            cell: (b) => <span className="text-fg">{b.name}</span>,
          },
          {
            header: t('boxes.slug'),
            cell: (b) => (
              <span className="font-mono text-[12px] text-fg-subtle">{b.slug}</span>
            ),
          },
          {
            header: t('boxes.host'),
            cell: (b) => (
              <span className="font-mono text-[12px] text-fg-subtle">{b.hostId || '—'}</span>
            ),
          },
          {
            header: t('boxes.orgs'),
            cell: (b) => (
              <span className="text-fg-soft tabular-nums">
                {boundOrgs[b.id] ? boundOrgs[b.id].length : '—'}
              </span>
            ),
          },
          {
            header: t('boxes.box'),
            cell: (b) => (
              <span className="font-mono text-[12px] text-fg-subtle">{b.id}</span>
            ),
          },
          {
            header: '',
            srHeader: t('boxes.actions'),
            width: 'w-72',
            cell: (b) => (
              <div className="flex items-center gap-3">
                <button
                  onClick={() => setOrgEditor(b)}
                  className="text-xs text-fg-subtle hover:text-fg"
                >
                  {t('boxes.orgs')}
                </button>
                <button
                  onClick={() => setNarratorBox(b)}
                  className="text-xs text-fg-subtle hover:text-fg"
                >
                  {t('boxes.narrator')}
                </button>
                <button
                  onClick={() => setEditBox(b)}
                  className="text-xs text-fg-subtle hover:text-fg"
                >
                  {t('boxes.edit')}
                </button>
                <button
                  onClick={() => setTokensBox(b)}
                  className="text-xs text-fg-subtle hover:text-fg"
                >
                  {t('boxes.tokens')}
                </button>
                <button
                  onClick={() => setDeleteTarget(b)}
                  className="text-xs text-fg-subtle hover:text-fail-fg"
                >
                  {t('boxes.delete')}
                </button>
              </div>
            ),
          },
        ]}
      />

      {createOpen && (
        <Modal title={t('boxes.newTitle')} onClose={() => setCreateOpen(false)}>
          <CreateBoxForm
            onClose={() => setCreateOpen(false)}
            onSaved={(created) => {
              setCreateOpen(false)
              setBoundOrgs((prev) => ({ ...prev, [created.id]: [] }))
              void load()
            }}
          />
        </Modal>
      )}

      {orgEditor !== null && (
        <Modal
          title={t('boxes.orgsTitle', { name: orgEditor.name })}
          onClose={() => setOrgEditor(null)}
        >
          <OrgEditor
            box={orgEditor}
            bound={boundOrgs[orgEditor.id] ?? []}
            onClose={() => setOrgEditor(null)}
            onSaved={(orgIds) => {
              setBoundOrgs((prev) => ({ ...prev, [orgEditor.id]: orgIds }))
              setOrgEditor(null)
            }}
          />
        </Modal>
      )}

      {narratorBox !== null && (
        <Modal
          title={t('boxes.narratorTitle', { name: narratorBox.name })}
          onClose={() => setNarratorBox(null)}
        >
          <NarratorPreview box={narratorBox} />
        </Modal>
      )}

      {editBox !== null && (
        <Modal
          title={t('boxes.editTitle', { name: editBox.name })}
          onClose={() => setEditBox(null)}
        >
          <EditBoxForm
            box={editBox}
            onClose={() => setEditBox(null)}
            onSaved={() => {
              setEditBox(null)
              void load()
            }}
          />
        </Modal>
      )}

      {tokensBox !== null && (
        <Modal
          title={t('boxes.tokensTitle', { name: tokensBox.name })}
          onClose={() => setTokensBox(null)}
        >
          <TokensPanel box={tokensBox} />
        </Modal>
      )}

      {deleteTarget !== null && (
        <ConfirmDelete
          box={deleteTarget}
          busy={busyDelete}
          onClose={() => setDeleteTarget(null)}
          onConfirm={() => void confirmDelete()}
        />
      )}
    </div>
  )
}

// CreateBoxForm mints a new box. The slug is the box's human key and must be
// unique; the id is minted on the hub. hostId is optional — an empty box
// stays unbound to a machine until a later PATCH. edition defaults to lite,
// the hub's fallback for an absent value.
function CreateBoxForm({
  onClose,
  onSaved,
}: {
  onClose: () => void
  onSaved: (created: Box) => void
}) {
  const { t } = useTranslation()
  const [slug, setSlug] = useState('')
  const [name, setName] = useState('')
  const [hostId, setHostId] = useState('')
  const [edition, setEdition] = useState<BoxEdition>('lite')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')

  const submit = async (e: FormEvent) => {
    e.preventDefault()
    setBusy(true)
    setError('')
    try {
      const created = await api.post<Box>('/api/boxes', {
        slug: slug.trim(),
        name: name.trim(),
        hostId: hostId.trim(),
        edition,
      })
      onSaved(created)
    } catch (err) {
      if (err instanceof ApiError && err.status === 409) {
        setError(t('boxes.slugTaken'))
      } else if (err instanceof ApiError && err.status === 400) {
        setError(t('boxes.slugInvalid'))
      } else {
        setError(err instanceof Error ? err.message : t('boxes.createFailed'))
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

      <label className="block">
        <span className="field-label">{t('boxes.slug')}</span>
        <input
          type="text"
          required
          value={slug}
          onChange={(e) => setSlug(e.target.value.toLowerCase().replace(/[^a-z0-9-]/g, ''))}
          placeholder={t('boxes.slugPlaceholder')}
          autoCapitalize="none"
          autoCorrect="off"
          spellCheck={false}
          className="field-input mt-2 font-mono text-[12px]"
        />
        <span className="mt-1 block text-xs text-fg-subtle">{t('boxes.slugHint')}</span>
      </label>

      <label className="block">
        <span className="field-label">{t('boxes.name')}</span>
        <input
          type="text"
          required
          value={name}
          onChange={(e) => setName(e.target.value)}
          className="field-input mt-2"
        />
      </label>

      <label className="block">
        <span className="field-label">{t('boxes.host')}</span>
        <input
          type="text"
          value={hostId}
          onChange={(e) => setHostId(e.target.value)}
          placeholder={t('boxes.hostPlaceholder')}
          className="field-input mt-2 font-mono text-[12px]"
        />
        <span className="mt-1 block text-xs text-fg-subtle">{t('boxes.hostHint')}</span>
      </label>

      <label className="block">
        <span className="field-label">{t('boxes.edition')}</span>
        <SimpleSelect
          className="mt-2 w-full"
          value={edition}
          onValueChange={(value) => setEdition(value as BoxEdition)}
          aria-label={t('boxes.edition')}
          items={EDITIONS.map(({ value, labelKey }) => ({
            value,
            label: t(labelKey),
          }))}
        />
        <span className="mt-1 block text-xs text-fg-subtle">{t('boxes.editionHint')}</span>
      </label>

      <div className="mt-2 flex items-center justify-end gap-3">
        <button type="button" onClick={onClose} className="btn-secondary">
          {t('common.cancel')}
        </button>
        <button type="submit" disabled={busy} className="btn-primary">
          {busy ? t('common.loading') : t('boxes.create')}
        </button>
      </div>
    </form>
  )
}

// OrgEditor replaces a box's organization set. The hub's PUT takes the whole
// new set, so one save is the attach and the detach; the checkboxes are
// seeded from what this session has bound — the box endpoints have no read
// for the current set yet.
function OrgEditor({
  box,
  bound,
  onClose,
  onSaved,
}: {
  box: Box
  bound: string[]
  onClose: () => void
  onSaved: (orgIds: string[]) => void
}) {
  const { t } = useTranslation()
  const [orgs, setOrgs] = useState<Org[] | null>(null)
  const [selected, setSelected] = useState<string[]>(bound)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')

  useEffect(() => {
    let cancelled = false
    const fetchOrgs = async () => {
      try {
        const list = await api.get<Org[]>('/api/admin/orgs')
        if (!cancelled) setOrgs(list)
      } catch (err) {
        if (!cancelled) {
          setError(err instanceof Error ? err.message : t('boxes.orgsLoadFailed'))
        }
      }
    }
    void fetchOrgs()
    return () => {
      cancelled = true
    }
  }, [t])

  const toggle = (orgId: string) => {
    setSelected((prev) =>
      prev.includes(orgId) ? prev.filter((id) => id !== orgId) : [...prev, orgId],
    )
  }

  const submit = async (e: FormEvent) => {
    e.preventDefault()
    setBusy(true)
    setError('')
    try {
      await api.put(`/api/boxes/${box.id}/orgs`, { orgIds: selected })
      onSaved(selected)
    } catch (err) {
      setError(err instanceof Error ? err.message : t('boxes.orgsSaveFailed'))
    } finally {
      setBusy(false)
    }
  }

  if (orgs === null && !error) {
    return <p className="py-6 text-sm text-fg-subtle">{t('common.loading')}</p>
  }

  return (
    <form onSubmit={(e) => void submit(e)} className="flex flex-col gap-4">
      {error && (
        <p className="rounded-lg border border-fail/20 px-3 py-2 text-sm text-fail-fg">
          {error}
        </p>
      )}

      {orgs && orgs.length === 0 && (
        <p className="text-sm text-fg-subtle">{t('boxes.noOrgs')}</p>
      )}

      {orgs && orgs.length > 0 && (
        <div className="max-h-72 overflow-y-auto rounded-lg border border-line-2 p-3">
          {orgs.map((org) => (
            <label
              key={org.id}
              className="flex items-center gap-2 py-1 text-sm text-fg-soft"
            >
              <input
                type="checkbox"
                checked={selected.includes(org.id)}
                onChange={() => toggle(org.id)}
                className="h-4 w-4 accent-accent"
              />
              <span className="min-w-0 flex-1 truncate">{org.name}</span>
              <span className="font-mono text-[11px] text-fg-faint">{org.id}</span>
            </label>
          ))}
        </div>
      )}

      <div className="mt-2 flex items-center justify-between gap-3">
        <span className="text-xs text-fg-subtle">
          {t('boxes.selected', { count: selected.length })}
        </span>
        <div className="flex items-center gap-3">
          <button type="button" onClick={onClose} className="btn-secondary">
            {t('common.cancel')}
          </button>
          <button type="submit" disabled={busy || orgs === null} className="btn-primary">
            {busy ? t('common.loading') : t('common.save')}
          </button>
        </div>
      </div>
    </form>
  )
}

// NarratorPreview reads the box's narrator — its box-scoped staff member —
// and shows the voice and the soul. A box without one answers 404, which is
// an empty state here, not an error.
function NarratorPreview({ box }: { box: Box }) {
  const { t } = useTranslation()
  const [narrator, setNarrator] = useState<Staff | null>(null)
  const [notFound, setNotFound] = useState(false)
  const [error, setError] = useState('')

  useEffect(() => {
    let cancelled = false
    const fetchNarrator = async () => {
      try {
        const staff = await api.get<Staff>(`/api/boxes/${box.id}/narrator`)
        if (!cancelled) setNarrator(staff)
      } catch (err) {
        if (cancelled) return
        if (err instanceof ApiError && err.status === 404) {
          setNotFound(true)
        } else {
          setError(err instanceof Error ? err.message : t('boxes.narratorLoadFailed'))
        }
      }
    }
    void fetchNarrator()
    return () => {
      cancelled = true
    }
  }, [box.id, t])

  if (notFound) {
    return (
      <div className="surface rounded-2xl p-8 text-center">
        <p className="mb-2 text-fg-soft">{t('boxes.narratorEmptyTitle')}</p>
        <p className="text-sm text-fg-subtle">{t('boxes.narratorEmptyHint')}</p>
      </div>
    )
  }

  if (error) {
    return (
      <p className="rounded-lg border border-fail/20 px-3 py-2 text-sm text-fail-fg">
        {error}
      </p>
    )
  }

  if (narrator === null) {
    return <p className="py-6 text-sm text-fg-subtle">{t('common.loading')}</p>
  }

  return (
    <div className="flex flex-col gap-4">
      <div className="flex items-center gap-2">
        <span className="text-fg">{narrator.name}</span>
        <span className="font-mono text-[12px] text-fg-subtle">{narrator.slug}</span>
      </div>
      <div className="rounded-lg border border-line-2 p-4">
        <p className="field-label">{t('staff.voice')}</p>
        <p className="mt-1 font-mono text-[12px] text-fg-soft">{narrator.voice || '—'}</p>
      </div>
      <div className="rounded-lg border border-line-2 p-4">
        <p className="field-label">{t('staff.soulCore')}</p>
        <p className="mt-1 whitespace-pre-wrap text-sm leading-5 text-fg-soft">
          {narrator.soulCore || '—'}
        </p>
      </div>
    </div>
  )
}

// EditBoxForm renames, re-hosts and re-editions a box. The slug and id are
// immutable once minted. A box that predates editions carries none on the
// wire; the select falls back to lite, the same default the hub applies.
function EditBoxForm({
  box,
  onClose,
  onSaved,
}: {
  box: Box
  onClose: () => void
  onSaved: () => void
}) {
  const { t } = useTranslation()
  const [name, setName] = useState(box.name)
  const [hostId, setHostId] = useState(box.hostId ?? '')
  const [edition, setEdition] = useState<BoxEdition>(
    EDITIONS.some(({ value }) => value === box.edition)
      ? (box.edition as BoxEdition)
      : 'lite',
  )
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')

  const submit = async (e: FormEvent) => {
    e.preventDefault()
    setBusy(true)
    setError('')
    try {
      await api.patch(`/api/boxes/${box.id}`, {
        name: name.trim(),
        hostId: hostId.trim(),
        edition,
      })
      onSaved()
    } catch (err) {
      setError(err instanceof Error ? err.message : t('boxes.editFailed'))
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

      <label className="block">
        <span className="field-label">{t('boxes.name')}</span>
        <input
          type="text"
          required
          value={name}
          onChange={(e) => setName(e.target.value)}
          className="field-input mt-2"
        />
      </label>

      <label className="block">
        <span className="field-label">{t('boxes.host')}</span>
        <input
          type="text"
          value={hostId}
          onChange={(e) => setHostId(e.target.value)}
          placeholder={t('boxes.hostPlaceholder')}
          className="field-input mt-2 font-mono text-[12px]"
        />
        <span className="mt-1 block text-xs text-fg-subtle">{t('boxes.hostHint')}</span>
      </label>

      <label className="block">
        <span className="field-label">{t('boxes.edition')}</span>
        <SimpleSelect
          className="mt-2 w-full"
          value={edition}
          onValueChange={(value) => setEdition(value as BoxEdition)}
          aria-label={t('boxes.edition')}
          items={EDITIONS.map(({ value, labelKey }) => ({
            value,
            label: t(labelKey),
          }))}
        />
        <span className="mt-1 block text-xs text-fg-subtle">{t('boxes.editionHint')}</span>
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

// TokensPanel mints, lists and revokes a box's credentials. The secret is
// shown exactly once, next to the mint that returned it — the list carries
// rows only, so a closed modal cannot leak one back.
function TokensPanel({ box }: { box: Box }) {
  const { t } = useTranslation()
  const [tokens, setTokens] = useState<BoxToken[] | null>(null)
  const [fresh, setFresh] = useState<{ id: string; token: string } | null>(null)
  const [busy, setBusy] = useState(false)
  const [copied, setCopied] = useState(false)
  const [error, setError] = useState('')

  const load = useCallback(async () => {
    try {
      setTokens(await listBoxTokens(box.id))
      setError('')
    } catch (err) {
      setError(err instanceof Error ? err.message : t('boxes.tokensLoadFailed'))
    }
  }, [box.id, t])

  useEffect(() => {
    void load()
  }, [load])

  const mint = async () => {
    setBusy(true)
    setError('')
    try {
      const res = await mintBoxToken(box.id)
      setFresh({ id: res.row.id, token: res.token })
      setCopied(false)
      void load()
    } catch (err) {
      setError(err instanceof Error ? err.message : t('boxes.mintFailed'))
    } finally {
      setBusy(false)
    }
  }

  const copy = async () => {
    if (!fresh) return
    try {
      await navigator.clipboard.writeText(fresh.token)
      setCopied(true)
      window.setTimeout(() => setCopied(false), 2000)
    } catch {
      /* clipboard unavailable — the secret stays readable below */
    }
  }

  const revoke = async (token: BoxToken) => {
    if (!window.confirm(t('boxes.revokeConfirm'))) return
    setError('')
    try {
      await revokeBoxToken(box.id, token.id)
      if (fresh?.id === token.id) setFresh(null)
      await load()
    } catch (err) {
      setError(err instanceof Error ? err.message : t('boxes.revokeFailed'))
    }
  }

  return (
    <div className="flex flex-col gap-4">
      {error && (
        <p className="rounded-lg border border-fail/20 px-3 py-2 text-sm text-fail-fg">
          {error}
        </p>
      )}

      <div className="flex items-center justify-between gap-3">
        <span className="text-xs text-fg-subtle">{t('boxes.mintHint')}</span>
        <button onClick={() => void mint()} disabled={busy} className="btn-primary">
          {busy ? t('common.loading') : t('boxes.mint')}
        </button>
      </div>

      {fresh && (
        <div className="rounded-lg border border-ok/30 bg-ok/10 p-3">
          <p className="mb-2 text-xs text-ok">{t('boxes.tokenShownOnce')}</p>
          <div className="flex items-center gap-2">
            <code className="min-w-0 flex-1 overflow-x-auto whitespace-nowrap font-mono text-[13px] text-ok">
              {fresh.token}
            </code>
            <button onClick={() => void copy()} className="btn-secondary">
              {copied ? t('boxes.copied') : t('boxes.copy')}
            </button>
          </div>
        </div>
      )}

      {tokens === null && !error && (
        <p className="py-6 text-sm text-fg-subtle">{t('common.loading')}</p>
      )}

      {tokens !== null && tokens.length === 0 && (
        <p className="text-sm text-fg-subtle">{t('boxes.noTokens')}</p>
      )}

      {tokens !== null && tokens.length > 0 && (
        <ul className="divide-y divide-line-2/60">
          {tokens.map((token) => (
            <li key={token.id} className="flex flex-wrap items-start justify-between gap-3 py-3">
              <div className="min-w-0">
                <p className="font-mono text-[12px] text-fg">{token.id}</p>
                <p className="mt-0.5 text-xs text-fg-subtle">
                  {t('boxes.tokenCreated', { ago: timeAgo(token.createdAt) })} ·{' '}
                  {token.lastUsedAt
                    ? t('boxes.tokenUsed', { ago: timeAgo(token.lastUsedAt) })
                    : t('boxes.tokenNeverUsed')}
                </p>
              </div>
              <button
                onClick={() => void revoke(token)}
                className="text-xs text-fg-subtle hover:text-fail-fg"
              >
                {t('boxes.revoke')}
              </button>
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}

// ConfirmDelete asks before a destructive delete. Two buttons, nothing to
// type — a box's blast radius is its own orgs and narrator, not the fleet.
function ConfirmDelete({
  box,
  busy,
  onClose,
  onConfirm,
}: {
  box: Box
  busy: boolean
  onClose: () => void
  onConfirm: () => void
}) {
  const { t } = useTranslation()
  return (
    <Modal title={t('boxes.confirmDelete', { name: box.name })} onClose={onClose}>
      <div className="flex flex-col gap-4">
        <p className="text-sm text-fg-muted">{t('boxes.confirmDeleteMessage')}</p>
        <div className="flex items-center justify-end gap-3">
          <button onClick={onClose} disabled={busy} className="btn-secondary">
            {t('common.cancel')}
          </button>
          <button onClick={onConfirm} disabled={busy} className="btn-danger">
            {busy ? t('common.loading') : t('boxes.delete')}
          </button>
        </div>
      </div>
    </Modal>
  )
}
