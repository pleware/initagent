import { useCallback, useEffect, useRef, useState, type FormEvent } from 'react'
import { useTranslation } from 'react-i18next'
import { api, ApiError, hfRepoFiles, localizeError, searchHf } from '../api'
import { usePoll } from '../hooks'
import DataTable from '../components/DataTable'
import Modal from '../components/Modal'
import { SimpleSelect } from '@ia/web/components/SimpleSelect'
import { PURPOSES, modelLabel } from '../models'
import type { HfRepoFile, HfSearchResult, Model, ModelAssignment, Purpose } from '../types'

// The platform operator's model layer surface: the registry of pinned
// models and the factory assignments each purpose resolves to. The hub owns
// the rules — an empty digest comes back as a 400 on assign, a referenced
// pin as a 409 on delete — this screen submits and shows what the hub
// answered, the same posture as SkillsPage. Per-box overrides live on
// BoxesPage; `Staff.avatarModel3d` (the avatar GLB) is a different thing and stays
// untouched.
export default function ModelsPage() {
  const { t } = useTranslation()
  const [models, setModels] = useState<Model[] | null>(null)
  const [assignments, setAssignments] = useState<ModelAssignment[] | null>(null)
  const [catalog, setCatalog] = useState<Model[] | null>(null)
  const [error, setError] = useState('')
  const [editor, setEditor] = useState<Model | 'new' | null>(null)
  const [busy, setBusy] = useState<Purpose | null>(null)
  // The Hugging Face browser: query + debounced search state, the picked
  // repo whose quants are open, and the pre-fill the next "add" hands to the
  // create form.
  const [hfQuery, setHfQuery] = useState('')
  const [hfResults, setHfResults] = useState<HfSearchResult[] | null>(null)
  const [hfBusy, setHfBusy] = useState(false)
  const [hfError, setHfError] = useState('')
  const [quantsFor, setQuantsFor] = useState<HfSearchResult | null>(null)
  const [quantFiles, setQuantFiles] = useState<HfRepoFile[] | null>(null)
  const [quantError, setQuantError] = useState('')
  const [prefill, setPrefill] = useState<Partial<Model> | null>(null)
  const hfSeq = useRef(0)

  const load = useCallback(async () => {
    try {
      const [adminModels, assignmentRows, publicModels] = await Promise.all([
        api.get<Model[]>('/api/admin/models'),
        api.get<ModelAssignment[]>('/api/admin/models/assignments'),
        api.get<Model[]>('/api/models'),
      ])
      setModels(adminModels)
      setAssignments(assignmentRows)
      setCatalog(publicModels)
      setError('')
    } catch (err) {
      setError(err instanceof Error ? err.message : t('models.loadFailed'))
    }
  }, [t])

  usePoll(load, 30_000)

  // Debounced HF search: only the last query in a typing burst fires, and a
  // stale in-flight answer (hfSeq) never overwrites a newer one.
  useEffect(() => {
    const q = hfQuery.trim()
    const seq = ++hfSeq.current
    if (q === '') {
      setHfResults(null)
      setHfBusy(false)
      setHfError('')
      return
    }
    setHfBusy(true)
    const timer = window.setTimeout(async () => {
      try {
        const results = await searchHf(q)
        if (hfSeq.current !== seq) return
        setHfResults(results)
        setHfError('')
      } catch (err) {
        if (hfSeq.current !== seq) return
        setHfError(localizeError(err, t))
      } finally {
        if (hfSeq.current === seq) setHfBusy(false)
      }
    }, 400)
    return () => window.clearTimeout(timer)
  }, [hfQuery, t])

  const openQuants = async (item: HfSearchResult) => {
    setQuantsFor(item)
    setQuantFiles(null)
    setQuantError('')
    try {
      setQuantFiles(await hfRepoFiles(item.org, item.name))
    } catch (err) {
      setQuantError(localizeError(err, t))
    }
  }

  const addFromHf = (item: HfSearchResult, file: HfRepoFile) => {
    setPrefill({
      org: item.org,
      source: item.id,
      quant: file.quant,
      licence: item.licence,
      purpose: item.suggestedPurpose || 'persona',
    })
    setQuantsFor(null)
    setEditor('new')
  }

  const remove = async (model: Model) => {
    if (!window.confirm(t('models.confirmDelete', { id: model.id }))) return
    setError('')
    try {
      await api.del(`/api/admin/models/${model.id}`)
      await load()
    } catch (err) {
      // A 404 means the pin is already gone — refetch and stay quiet; a 409
      // (the pin is referenced) and everything else land in the banner.
      if (err instanceof ApiError && err.status === 404) {
        void load()
      } else {
        setError(err instanceof Error ? err.message : t('models.deleteFailed'))
      }
    }
  }

  const changeAssignment = async (purpose: Purpose, modelId: string) => {
    setBusy(purpose)
    setError('')
    try {
      if (modelId === '') {
        await api.del(`/api/admin/models/assignments/${purpose}`)
      } else {
        await api.put(`/api/admin/models/assignments`, { purpose, modelId })
      }
      await load()
    } catch (err) {
      setError(err instanceof Error ? err.message : t('models.assignFailed'))
    } finally {
      setBusy(null)
    }
  }

  const hfGroups = hfResults === null ? [] : groupHf(hfResults)

  return (
    <div className="page-shell">
      <div className="mb-6 flex items-end justify-between">
        <div>
          <p className="eyebrow mb-3">{t('models.eyebrow')}</p>
          <h1 className="text-3xl font-semibold tracking-[-0.04em] text-fg-strong">
            {t('models.title')}
          </h1>
          <p className="mt-1 text-sm text-fg-muted">{t('models.subtitle')}</p>
        </div>
        <button
          onClick={() => {
            setPrefill(null)
            setEditor('new')
          }}
          className="btn-primary"
        >
          {t('models.newModel')}
        </button>
      </div>

      {error && (
        <p className="mb-4 rounded-lg border border-fail/20 px-3 py-2 text-sm text-fail-fg">
          {error}
        </p>
      )}

      <section>
        <h2 className="text-lg font-semibold tracking-[-0.02em] text-fg-strong">
          {t('models.registry')}
        </h2>
        <p className="mt-1 text-sm text-fg-muted">{t('models.registryHint')}</p>
        <div className="mt-4">
          <DataTable
            rows={models}
            rowKey={(m) => m.id}
            empty={<p className="text-sm text-fg-subtle">{t('models.noModels')}</p>}
            columns={[
              {
                header: t('models.id'),
                cell: (m) => <span className="font-mono text-[12px] text-fg">{m.id}</span>,
              },
              {
                header: t('models.org'),
                cell: (m) => <span className="text-fg-subtle">{m.org || '—'}</span>,
              },
              {
                header: t('models.purpose'),
                cell: (m) => (
                  <span className="rounded-full border border-line-2 px-2 py-0.5 text-xs text-fg-soft">
                    {t('purpose.' + m.purpose)}
                  </span>
                ),
              },
              {
                header: t('models.quant'),
                cell: (m) => (
                  <span className="font-mono text-[12px] text-fg-subtle">{m.quant || '—'}</span>
                ),
              },
              {
                header: t('models.licence'),
                cell: (m) => <span className="text-fg-subtle">{m.licence || '—'}</span>,
              },
              {
                header: t('models.digest'),
                cell: (m) => <DigestBadge model={m} />,
              },
              {
                header: '',
                srHeader: t('models.actions'),
                width: 'w-28',
                cell: (m) => (
                  <div className="flex items-center gap-3">
                    <button
                      onClick={() => setEditor(m)}
                      className="text-xs text-fg-subtle hover:text-fg"
                    >
                      {t('common.edit')}
                    </button>
                    <button
                      onClick={() => void remove(m)}
                      className="text-xs text-fg-subtle hover:text-fail-fg"
                    >
                      {t('common.delete')}
                    </button>
                  </div>
                ),
              },
            ]}
          />
        </div>
      </section>

      <section className="mt-8">
        <h2 className="text-lg font-semibold tracking-[-0.02em] text-fg-strong">
          {t('models.hfSearch')}
        </h2>
        <p className="mt-1 text-sm text-fg-muted">{t('models.hfSearchHint')}</p>
        <input
          type="search"
          value={hfQuery}
          onChange={(e) => setHfQuery(e.target.value)}
          placeholder={t('models.hfSearchPlaceholder')}
          aria-label={t('models.hfSearch')}
          autoCapitalize="none"
          autoCorrect="off"
          spellCheck={false}
          className="field-input mt-4 w-full max-w-md"
        />
        {hfBusy && <p className="mt-2 text-sm text-fg-subtle">{t('models.hfSearching')}</p>}
        {hfError && (
          <p className="mt-2 rounded-lg border border-fail/20 px-3 py-2 text-sm text-fail-fg">
            {hfError}
          </p>
        )}
        {hfResults !== null && hfResults.length === 0 && !hfBusy && (
          <p className="mt-2 text-sm text-fg-subtle">{t('models.hfNoResults')}</p>
        )}
        {hfGroups.length > 0 && (
          <div className="mt-4 rounded-2xl border border-line-2/60">
            {hfGroups.map(([org, items]) => (
              <div key={org} className="border-b border-line-2/60 last:border-b-0">
                <div className="flex items-center gap-2 bg-sidebar px-4 py-2">
                  <span className="text-xs font-semibold uppercase tracking-wider text-fg-muted">
                    {t('models.org')}
                  </span>
                  <span className="font-mono text-sm text-fg">{org}</span>
                </div>
                <ul className="divide-y divide-line-2/60">
                  {items.map((item) => (
                    <li key={item.id} className="flex flex-wrap items-center gap-3 px-4 py-2.5">
                      <span className="min-w-0 flex-1 truncate font-mono text-[12px] text-fg">
                        {item.id}
                      </span>
                      {item.pipelineTag !== '' && (
                        <span className="rounded-full border border-line-2 px-2 py-0.5 text-xs text-fg-soft">
                          {item.pipelineTag}
                        </span>
                      )}
                      {item.suggestedPurpose !== '' && (
                        <span className="rounded-full border border-line-2 px-2 py-0.5 text-xs text-fg-soft">
                          {t('purpose.' + item.suggestedPurpose)}
                        </span>
                      )}
                      {item.licence !== '' && (
                        <span className="text-xs text-fg-subtle">{item.licence}</span>
                      )}
                      <span className="text-xs text-fg-subtle">
                        {t('models.hfDownloads', { count: item.downloads.toLocaleString() })}
                      </span>
                      <button
                        onClick={() => void openQuants(item)}
                        className="text-xs text-fg-subtle hover:text-fg"
                      >
                        {t('models.browseQuants')}
                      </button>
                    </li>
                  ))}
                </ul>
              </div>
            ))}
          </div>
        )}
      </section>

      <section className="mt-8">
        <h2 className="text-lg font-semibold tracking-[-0.02em] text-fg-strong">
          {t('models.assignments')}
        </h2>
        <p className="mt-1 text-sm text-fg-muted">{t('models.assignmentsHint')}</p>
        <div className="mt-4 divide-y divide-line-2/60 rounded-2xl border border-line-2/60">
          {PURPOSES.map((purpose) => {
            const assignment = assignments?.find((a) => a.purpose === purpose)
            const pickable = (catalog ?? []).filter((m) => m.purpose === purpose)
            const assigned = assignment
              ? (catalog ?? []).find((m) => m.id === assignment.modelId)
              : undefined
            return (
              <div key={purpose} className="flex flex-wrap items-center gap-3 px-4 py-3">
                <span className="w-28 shrink-0 text-sm font-medium text-fg">
                  {t('purpose.' + purpose)}
                </span>
                <span className="min-w-0 flex-1 truncate text-sm text-fg-muted">
                  {assigned ? (
                    <span className="font-mono text-[12px]">{modelLabel(assigned)}</span>
                  ) : (
                    t('models.unassigned')
                  )}
                </span>
                {catalog !== null && pickable.length === 0 ? (
                  <span className="text-xs text-fg-subtle">{t('models.noModelsForPurpose')}</span>
                ) : (
                  <SimpleSelect
                    className="w-64"
                    value={assignment?.modelId ?? ''}
                    disabled={busy === purpose}
                    onValueChange={(value) => void changeAssignment(purpose, value)}
                    aria-label={t('models.assignments')}
                    items={[
                      { value: '', label: t('models.unassigned') },
                      ...pickable.map((m) => ({ value: m.id, label: modelLabel(m) })),
                    ]}
                  />
                )}
              </div>
            )
          })}
        </div>
      </section>

      {quantsFor !== null && (
        <Modal
          wide
          title={t('models.quantsTitle', { model: quantsFor.id })}
          onClose={() => setQuantsFor(null)}
        >
          <div className="flex flex-col gap-3">
            <p className="text-sm text-fg-muted">
              {quantsFor.licence !== ''
                ? `${t('models.licence')}: ${quantsFor.licence}`
                : t('models.licence') + ': —'}
            </p>
            {quantsFor.suggestedPurpose !== '' && (
              <p className="text-sm text-fg-muted">
                {t('models.suggestedPurpose')}: {t('purpose.' + quantsFor.suggestedPurpose)}
              </p>
            )}
            <p className="text-xs text-fg-subtle">{t('models.quantsHint')}</p>
            {quantError && (
              <p className="rounded-lg border border-fail/20 px-3 py-2 text-sm text-fail-fg">
                {quantError}
              </p>
            )}
            {quantFiles === null ? (
              <p className="text-sm text-fg-subtle">{t('common.loading')}</p>
            ) : quantFiles.length === 0 ? (
              <p className="text-sm text-fg-subtle">{t('models.noQuants')}</p>
            ) : (
              <ul className="divide-y divide-line-2/60 rounded-2xl border border-line-2/60">
                {quantFiles.map((file) => (
                  <li key={file.filename} className="flex items-center gap-3 px-4 py-2.5">
                    <span className="min-w-0 flex-1 truncate font-mono text-[12px] text-fg">
                      {file.filename}
                    </span>
                    <span className="rounded-full border border-line-2 px-2 py-0.5 font-mono text-[10px] text-fg-soft">
                      {file.quant}
                    </span>
                    <button
                      onClick={() => addFromHf(quantsFor, file)}
                      className="text-xs text-fg-subtle hover:text-fg"
                    >
                      {t('models.addModel')}
                    </button>
                  </li>
                ))}
              </ul>
            )}
          </div>
        </Modal>
      )}

      {editor !== null && (
        <Modal
          title={
            editor === 'new'
              ? prefill
                ? t('models.addModelTitle', { name: prefill.source ?? '' })
                : t('models.newTitle')
              : t('models.editTitle', { id: editor.id })
          }
          onClose={() => {
            setEditor(null)
            setPrefill(null)
          }}
        >
          <ModelForm
            model={editor === 'new' ? null : editor}
            prefill={editor === 'new' ? prefill : null}
            onClose={() => {
              setEditor(null)
              setPrefill(null)
            }}
            onSaved={() => {
              setEditor(null)
              setPrefill(null)
              void load()
            }}
          />
        </Modal>
      )}
    </div>
  )
}

// groupHf buckets search hits by org, keeping the hub's download order
// within each bucket.
function groupHf(results: HfSearchResult[]): [string, HfSearchResult[]][] {
  const byOrg = new Map<string, HfSearchResult[]>()
  for (const r of results) {
    const list = byOrg.get(r.org)
    if (list) list.push(r)
    else byOrg.set(r.org, [r])
  }
  return [...byOrg.entries()]
}

// DigestBadge shows the pin's verification state: the truncated BLAKE3 when
// the admin filled it, the unverified marker when the pin still carries an
// empty digest.
function DigestBadge({ model }: { model: Model }) {
  const { t } = useTranslation()
  if (!model.digest) {
    return (
      <span className="rounded-full border border-fail/30 px-2 py-0.5 text-xs text-fail-fg">
        {t('models.unverified')}
      </span>
    )
  }
  return (
    <span
      title={model.digest}
      className="inline-block max-w-[18ch] truncate rounded-full border border-accent/30 px-2 py-0.5 font-mono text-[10px] text-accent"
    >
      {model.digest}
    </span>
  )
}

// ModelForm creates or edits one pin. The id is the pin's key and immutable
// on edit; the hub takes it from the path there, so the body id is ignored.
// `prefill` carries the values the Hugging Face browser suggests for a new
// pin — the admin still fills id and digest before saving.
function ModelForm({
  model,
  prefill,
  onClose,
  onSaved,
}: {
  model: Model | null
  prefill?: Partial<Model> | null
  onClose: () => void
  onSaved: () => void
}) {
  const { t } = useTranslation()
  const [org, setOrg] = useState(model?.org ?? prefill?.org ?? '')
  const [id, setId] = useState(model?.id ?? prefill?.id ?? '')
  const [source, setSource] = useState(model?.source ?? prefill?.source ?? '')
  const [quant, setQuant] = useState(model?.quant ?? prefill?.quant ?? '')
  const [digest, setDigest] = useState(model?.digest ?? prefill?.digest ?? '')
  const [licence, setLicence] = useState(model?.licence ?? prefill?.licence ?? '')
  const [purpose, setPurpose] = useState<Purpose>(
    model?.purpose ?? prefill?.purpose ?? 'persona',
  )
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')

  const submit = async (e: FormEvent) => {
    e.preventDefault()
    setBusy(true)
    setError('')
    const payload = {
      id: id.trim(),
      org: org.trim(),
      source: source.trim(),
      quant: quant.trim(),
      digest: digest.trim(),
      licence: licence.trim(),
      purpose,
    }
    try {
      if (model) {
        await api.patch<Model>(`/api/admin/models/${model.id}`, payload)
      } else {
        await api.post<Model>('/api/admin/models', payload)
      }
      onSaved()
    } catch (err) {
      if (err instanceof ApiError && err.status === 409) {
        setError(t('models.idTaken'))
      } else {
        setError(err instanceof Error ? err.message : t('models.saveFailed'))
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
        <span className="field-label">{t('models.org')}</span>
        <input
          type="text"
          value={org}
          onChange={(e) => setOrg(e.target.value)}
          className="field-input mt-2 font-mono text-[12px]"
          autoCapitalize="none"
          autoCorrect="off"
          spellCheck={false}
        />
        <span className="mt-1 block text-xs text-fg-subtle">{t('models.orgHint')}</span>
      </label>

      <label className="block">
        <span className="field-label">{t('models.id')}</span>
        <input
          type="text"
          required
          disabled={model !== null}
          value={id}
          onChange={(e) => setId(e.target.value)}
          placeholder={t('models.idHint')}
          autoCapitalize="none"
          autoCorrect="off"
          spellCheck={false}
          className="field-input mt-2 font-mono text-[12px] disabled:opacity-50"
        />
        <span className="mt-1 block text-xs text-fg-subtle">{t('models.idHint')}</span>
      </label>

      <label className="block">
        <span className="field-label">{t('models.purpose')}</span>
        <SimpleSelect
          className="mt-2 w-full"
          value={purpose}
          onValueChange={(value) => setPurpose(value as Purpose)}
          aria-label={t('models.purpose')}
          items={PURPOSES.map((p) => ({ value: p, label: t('purpose.' + p) }))}
        />
        {prefill?.purpose !== undefined && (
          <span className="mt-1 block text-xs text-fg-subtle">
            {t('models.suggestedPurposeHint')}
          </span>
        )}
      </label>

      <label className="block">
        <span className="field-label">{t('models.source')}</span>
        <input
          type="text"
          value={source}
          onChange={(e) => setSource(e.target.value)}
          className="field-input mt-2 font-mono text-[12px]"
        />
        <span className="mt-1 block text-xs text-fg-subtle">{t('models.sourceHint')}</span>
      </label>

      <div className="grid grid-cols-2 gap-3">
        <label className="block">
          <span className="field-label">{t('models.quant')}</span>
          <input
            type="text"
            value={quant}
            onChange={(e) => setQuant(e.target.value)}
            className="field-input mt-2 font-mono text-[12px]"
          />
          <span className="mt-1 block text-xs text-fg-subtle">{t('models.quantHint')}</span>
        </label>
        <label className="block">
          <span className="field-label">{t('models.licence')}</span>
          <input
            type="text"
            value={licence}
            onChange={(e) => setLicence(e.target.value)}
            className="field-input mt-2"
          />
        </label>
      </div>

      <label className="block">
        <span className="field-label">{t('models.digest')}</span>
        <input
          type="text"
          value={digest}
          onChange={(e) => setDigest(e.target.value)}
          className="field-input mt-2 font-mono text-[12px]"
        />
        <span className="mt-1 block text-xs text-fg-subtle">{t('models.digestHint')}</span>
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
