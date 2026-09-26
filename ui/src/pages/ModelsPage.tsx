import {
  useCallback,
  useEffect,
  useRef,
  useState,
  type FormEvent,
  type ReactNode,
} from 'react'
import { useTranslation } from 'react-i18next'
import { api, ApiError, hfRepoFiles, inspectModel, localizeError, searchHf } from '../api'
import { usePoll } from '../hooks'
import DataTable from '../components/DataTable'
import Modal from '../components/Modal'
import LimitsEditor from '../components/LimitsEditor'
import { SimpleSelect } from '@ia/web/components/SimpleSelect'
import { GENERATIVE_PURPOSES, PURPOSES, modelLabel, modelPurposes, modelServes } from '../models'
import type { HfRepoFile, HfSearchResult, Model, ModelAssignment, ModelLimits, Purpose } from '../types'

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
  const [limits, setLimits] = useState<ModelLimits[] | null>(null)
  const [limitBusy, setLimitBusy] = useState<Purpose | null>(null)
  // The pre-fill the Hugging Face browser hands to the create form; nothing
  // saves until the admin confirms.
  const [prefill, setPrefill] = useState<Partial<Model> | null>(null)

  const load = useCallback(async () => {
    try {
      const [adminModels, assignmentRows, publicModels, limitRows] = await Promise.all([
        api.get<Model[]>('/api/admin/models'),
        api.get<ModelAssignment[]>('/api/admin/models/assignments'),
        api.get<Model[]>('/api/models'),
        api.get<ModelLimits[]>('/api/admin/models/limits'),
      ])
      setModels(adminModels)
      setAssignments(assignmentRows)
      setCatalog(publicModels)
      setLimits(limitRows)
      setError('')
    } catch (err) {
      setError(err instanceof Error ? err.message : t('models.loadFailed'))
    }
  }, [t])

  usePoll(load, 30_000)

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

  const setFactoryLimit = async (purpose: Purpose, maxTokens: number, timeoutSeconds: number) => {
    setLimitBusy(purpose)
    setError('')
    try {
      await api.put('/api/admin/models/limits', { purpose, maxTokens, timeoutSeconds })
      await load()
    } catch (err) {
      setError(err instanceof Error ? err.message : t('models.limitSetFailed'))
      throw err
    } finally {
      setLimitBusy(null)
    }
  }

  const inspect = async (model: Model) => {
    setError('')
    try {
      await inspectModel(model.id)
      await load()
    } catch (err) {
      setError(err instanceof Error ? err.message : t('models.inspectFailed'))
    }
  }

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
                  <span className="flex flex-wrap gap-1">
                    {modelPurposes(m).map((p) => (
                      <span
                        key={p}
                        className="rounded-full border border-line-2 px-2 py-0.5 text-xs text-fg-soft"
                      >
                        {t('purpose.' + p)}
                      </span>
                    ))}
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
                header: t('models.metadata'),
                cell: (m) => <ModelMetaCell model={m} />,
              },
              {
                header: t('models.digest'),
                cell: (m) => <DigestBadge model={m} />,
              },
              {
                header: '',
                srHeader: t('models.actions'),
                width: 'w-40',
                cell: (m) => (
                  <div className="flex items-center gap-3">
                    <button
                      onClick={() => void inspect(m)}
                      className="text-xs text-fg-subtle hover:text-fg"
                    >
                      {t('models.inspect')}
                    </button>
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
          {t('models.assignments')}
        </h2>
        <p className="mt-1 text-sm text-fg-muted">{t('models.assignmentsHint')}</p>
        <div className="mt-4 divide-y divide-line-2/60 rounded-2xl border border-line-2/60">
          {PURPOSES.map((purpose) => {
            const assignment = assignments?.find((a) => a.purpose === purpose)
            const pickable = (catalog ?? []).filter((m) => modelServes(m, purpose))
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

      <section className="mt-8">
        <h2 className="text-lg font-semibold tracking-[-0.02em] text-fg-strong">
          {t('models.limits')}
        </h2>
        <p className="mt-1 text-sm text-fg-muted">{t('models.limitsHint')}</p>
        <div className="mt-4 divide-y divide-line-2/60 rounded-2xl border border-line-2/60">
          {GENERATIVE_PURPOSES.map((purpose) => {
            const limit = limits?.find((l) => l.purpose === purpose)
            return (
              <div key={purpose} className="flex flex-wrap items-center gap-3 px-4 py-3">
                <span className="w-28 shrink-0 text-sm font-medium text-fg">
                  {t('purpose.' + purpose)}
                </span>
                <LimitsEditor
                  current={limit}
                  disabled={limitBusy === purpose}
                  onCommit={(maxTokens, timeoutSeconds) =>
                    setFactoryLimit(purpose, maxTokens, timeoutSeconds)
                  }
                />
              </div>
            )
          })}
        </div>
      </section>

      {editor !== null && (
        <Modal
          className={editor === 'new' ? 'sm:max-w-4xl' : undefined}
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
            browser={editor === 'new' ? <HfBrowser onAdd={setPrefill} /> : undefined}
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

// slugify turns a name into a pin-id token: lowercase, every run of
// non-[a-z0-9_] collapses to one dash, edges trimmed. Underscores survive
// so canonical GGUF quants stay readable in the slug (Q4_K_M → q4_k_m).
function slugify(value: string): string {
  return value
    .toLowerCase()
    .replace(/[^a-z0-9_]+/g, '-')
    .replace(/^-+|-+$/g, '')
}

// HF_TAGS is the curated pipeline-tag filter of the browser. `tag` is the
// HF pipeline_tag the search endpoint filters on ('' = no filter); `key` is
// the i18n label under models.*.
const HF_TAGS = [
  { tag: '', key: 'hfTagAll' },
  { tag: 'text-generation', key: 'hfTagTextGeneration' },
  { tag: 'image-text-to-text', key: 'hfTagImageTextToText' },
  { tag: 'sentence-similarity', key: 'hfTagSentenceSimilarity' },
  { tag: 'feature-extraction', key: 'hfTagFeatureExtraction' },
  { tag: 'automatic-speech-recognition', key: 'hfTagSpeechRecognition' },
] as const

// HfBrowser is the right column of the create-model modal: a debounced
// Hugging Face search with tag filters, results grouped by org, and an
// inline quants panel per result. It never saves anything — "add" hands the
// suggestion to the form through onAdd.
function HfBrowser({ onAdd }: { onAdd: (prefill: Partial<Model>) => void }) {
  const { t, i18n } = useTranslation()
  const [query, setQuery] = useState('')
  const [tag, setTag] = useState('')
  const [results, setResults] = useState<HfSearchResult[] | null>(null)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [quantsOf, setQuantsOf] = useState<string | null>(null)
  const [quantFiles, setQuantFiles] = useState<HfRepoFile[] | null>(null)
  const [quantBusy, setQuantBusy] = useState(false)
  const [quantError, setQuantError] = useState('')
  const seq = useRef(0)
  const quantSeq = useRef(0)

  // Debounced search: only the last query/tag in a burst fires, and a stale
  // in-flight answer (seq) never overwrites a newer one.
  useEffect(() => {
    const q = query.trim()
    const s = ++seq.current
    if (q === '') {
      setResults(null)
      setBusy(false)
      setError('')
      return
    }
    setBusy(true)
    const timer = window.setTimeout(async () => {
      try {
        const hits = await searchHf(q, undefined, tag === '' ? undefined : tag)
        if (seq.current !== s) return
        setResults(hits)
        setError('')
      } catch (err) {
        if (seq.current !== s) return
        setError(localizeError(err, t))
      } finally {
        if (seq.current === s) setBusy(false)
      }
    }, 400)
    return () => window.clearTimeout(timer)
  }, [query, tag, t])

  // toggleQuants expands or collapses one result's quants panel. quantSeq
  // invalidates an in-flight fetch when the admin collapses or switches rows.
  const toggleQuants = async (item: HfSearchResult) => {
    if (quantsOf === item.id) {
      quantSeq.current++
      setQuantsOf(null)
      return
    }
    const s = ++quantSeq.current
    setQuantsOf(item.id)
    setQuantFiles(null)
    setQuantError('')
    setQuantBusy(true)
    try {
      const files = await hfRepoFiles(item.org, item.name)
      if (quantSeq.current !== s) return
      setQuantFiles(files)
    } catch (err) {
      if (quantSeq.current !== s) return
      setQuantError(localizeError(err, t))
    } finally {
      if (quantSeq.current === s) setQuantBusy(false)
    }
  }

  const add = (item: HfSearchResult, quant: string) => {
    const slug = slugify(item.name) + (quant ? '-' + slugify(quant) : '')
    onAdd({
      id: slug,
      org: item.org,
      source: item.id,
      quant,
      licence: item.licence,
      purpose: item.suggestedPurpose || 'persona',
    })
  }

  const groups = results === null ? [] : groupHf(results)

  return (
    <div className="flex min-w-0 flex-col gap-3">
      <h3 className="text-sm font-semibold text-fg-strong">{t('models.hfSearch')}</h3>
      <p className="text-xs text-fg-subtle">{t('models.hfSearchHint')}</p>
      <input
        type="search"
        value={query}
        onChange={(e) => setQuery(e.target.value)}
        placeholder={t('models.hfSearchPlaceholder')}
        aria-label={t('models.hfSearch')}
        autoCapitalize="none"
        autoCorrect="off"
        spellCheck={false}
        className="field-input w-full"
      />
      <div className="flex flex-wrap gap-2">
        {HF_TAGS.map(({ tag: value, key }) => (
          <button
            key={key}
            type="button"
            onClick={() => setTag(value)}
            aria-pressed={tag === value}
            className={
              tag === value
                ? 'rounded-full border border-accent/40 bg-accent/10 px-3 py-1 text-xs text-accent'
                : 'rounded-full border border-line-2 px-3 py-1 text-xs text-fg-soft hover:text-fg'
            }
          >
            {t('models.' + key)}
          </button>
        ))}
      </div>
      {busy && <p className="text-sm text-fg-subtle">{t('models.hfSearching')}</p>}
      {error && (
        <p className="rounded-lg border border-fail/20 px-3 py-2 text-sm text-fail-fg">{error}</p>
      )}
      {results !== null && results.length === 0 && !busy && (
        <p className="text-sm text-fg-subtle">{t('models.hfNoResults')}</p>
      )}
      {groups.length > 0 && (
        <div className="max-h-[40vh] overflow-y-auto rounded-2xl border border-line-2/60">
          {groups.map(([org, items]) => (
            <div key={org} className="border-b border-line-2/60 last:border-b-0">
              <div className="flex items-center gap-2 bg-sidebar px-3 py-1.5">
                <span className="text-xs font-semibold uppercase tracking-wider text-fg-muted">
                  {t('models.org')}
                </span>
                <span className="truncate font-mono text-sm text-fg">{org}</span>
              </div>
              <ul className="divide-y divide-line-2/60">
                {items.map((item) => (
                  <li key={item.id} className="px-3 py-2">
                    <div className="flex flex-wrap items-center justify-between gap-2">
                      <span className="min-w-0 truncate text-sm font-medium text-fg">
                        {item.name}
                      </span>
                      <span className="flex flex-wrap items-center gap-2">
                        {item.suggestedPurpose !== '' && (
                          <span className="rounded-full border border-line-2 px-2 py-0.5 text-xs text-fg-soft">
                            {t('purpose.' + item.suggestedPurpose)}
                          </span>
                        )}
                        {item.licence !== '' && (
                          <span className="rounded-full border border-line-2 px-2 py-0.5 text-xs text-fg-soft">
                            {item.licence}
                          </span>
                        )}
                        <span className="text-xs text-fg-subtle">
                          {t('models.hfDownloads', {
                            count: item.downloads.toLocaleString(
                              i18n.resolvedLanguage === 'pl' ? 'pl-PL' : 'en-US',
                            ),
                          })}
                        </span>
                      </span>
                    </div>
                    <div className="mt-1 flex flex-wrap items-center justify-between gap-2">
                      <span className="min-w-0 truncate font-mono text-[11px] text-fg-subtle">
                        {item.pipelineTag || '—'}
                      </span>
                      <button
                        type="button"
                        onClick={() => void toggleQuants(item)}
                        className="text-xs text-fg-subtle hover:text-fg"
                      >
                        {quantsOf === item.id
                          ? t('models.hideDetails')
                          : t('models.moreDetails')}
                      </button>
                    </div>
                    {quantsOf === item.id && (
                      <div className="mt-2 flex flex-col gap-2">
                        {quantBusy && (
                          <p className="text-xs text-fg-subtle">{t('common.loading')}</p>
                        )}
                        {quantError && (
                          <p className="rounded-lg border border-fail/20 px-3 py-2 text-xs text-fail-fg">
                            {quantError}
                          </p>
                        )}
                        {quantFiles !== null && quantFiles.length === 0 && !quantBusy && (
                          <div className="flex flex-col gap-1">
                            <p className="text-xs text-fg-subtle">{t('models.noQuants')}</p>
                            <p className="text-xs text-fg-subtle">{t('models.noQuantsHint')}</p>
                            <button
                              type="button"
                              onClick={() => add(item, '')}
                              className="self-start text-xs text-fg-subtle hover:text-fg"
                            >
                              {t('models.addModel')}
                            </button>
                          </div>
                        )}
                        {quantFiles !== null && quantFiles.length > 0 && (
                          <ul className="divide-y divide-line-2/60 rounded-xl border border-line-2/60">
                            {quantFiles.map((file) => (
                              <li key={file.filename} className="flex items-center gap-2 px-3 py-1.5">
                                <span className="min-w-0 flex-1 truncate font-mono text-[11px] text-fg">
                                  {file.filename}
                                </span>
                                <span className="rounded-full border border-line-2 px-2 py-0.5 font-mono text-[10px] text-fg-soft">
                                  {file.quant}
                                </span>
                                <button
                                  type="button"
                                  onClick={() => add(item, file.quant)}
                                  className="text-xs text-fg-subtle hover:text-fg"
                                >
                                  {t('models.addModel')}
                                </button>
                              </li>
                            ))}
                          </ul>
                        )}
                      </div>
                    )}
                  </li>
                ))}
              </ul>
            </div>
          ))}
        </div>
      )}
    </div>
  )
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

// ModelMetaCell shows a pin's derived metadata in one compact line: the HF
// pipeline tag (the modality signal), the GGUF architecture and the context
// length. All empty — a pin never inspected — renders an em dash.
function ModelMetaCell({ model }: { model: Model }) {
  const parts: string[] = []
  if (model.pipelineTag) parts.push(model.pipelineTag)
  if (model.architecture) parts.push(model.architecture)
  if (model.contextLength > 0) parts.push(`${model.contextLength.toLocaleString()} ctx`)
  if (parts.length === 0) return <span className="text-fg-subtle">—</span>
  return <span className="font-mono text-[11px] text-fg-subtle">{parts.join(' · ')}</span>
}

// ModelForm creates or edits one pin. The id is the pin's key and immutable
// on edit; the hub takes it from the path there, so the body id is ignored.
// `prefill` carries the values the Hugging Face browser suggests for a new
// pin — the admin still fills id and digest before saving. `browser` is the
// optional right column (create mode only): the form lays out in two columns
// when it is present, and the cancel/save footer stays full-width below.
function ModelForm({
  model,
  prefill,
  browser,
  onClose,
  onSaved,
}: {
  model: Model | null
  prefill?: Partial<Model> | null
  browser?: ReactNode
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

  // A later "add" click arrives as a new prefill while the form is already
  // mounted, so sync the suggested fields here. id is the slug the browser
  // derived — still editable; only digest stays untouched.
  useEffect(() => {
    if (!prefill) return
    setId(prefill.id ?? '')
    setOrg(prefill.org ?? '')
    setSource(prefill.source ?? '')
    setQuant(prefill.quant ?? '')
    setLicence(prefill.licence ?? '')
    setPurpose(prefill.purpose ?? 'persona')
  }, [prefill])

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

      <div className={browser ? 'grid items-start gap-6 sm:grid-cols-2' : undefined}>
        <div className="flex flex-col gap-4">
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
            {purpose === 'persona' && (
              <span className="mt-1 block text-xs text-fg-subtle">
                {t('models.personaServesNarrator')}
              </span>
            )}
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
        </div>
        {browser}
      </div>

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
