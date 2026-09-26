import { useEffect, useState, type KeyboardEvent } from 'react'
import { useTranslation } from 'react-i18next'
import type { ModelLimits } from '../types'

// LimitsEditor edits one generative slot's generation ceilings. The two
// fields commit together on blur or Enter (Escape reverts to the committed
// value); an empty field is the "no limit" 0. onCommit resolves on a saved
// value and rejects when the hub refused it — the parent re-fetches on
// success, and this component reverts the fields on failure.
export default function LimitsEditor({
  current,
  onCommit,
  disabled,
}: {
  current?: ModelLimits
  onCommit: (maxTokens: number, timeoutSeconds: number) => Promise<void>
  disabled?: boolean
}) {
  const { t } = useTranslation()
  const [maxTokens, setMaxTokens] = useState(display(current?.maxTokens))
  const [timeoutSeconds, setTimeoutSeconds] = useState(display(current?.timeoutSeconds))

  useEffect(() => {
    setMaxTokens(display(current?.maxTokens))
    setTimeoutSeconds(display(current?.timeoutSeconds))
  }, [current?.maxTokens, current?.timeoutSeconds])

  const commit = async () => {
    const mt = parse(maxTokens)
    const ts = parse(timeoutSeconds)
    if (mt === (current?.maxTokens ?? 0) && ts === (current?.timeoutSeconds ?? 0)) return
    try {
      await onCommit(mt, ts)
    } catch {
      setMaxTokens(display(current?.maxTokens))
      setTimeoutSeconds(display(current?.timeoutSeconds))
    }
  }

  const revert = () => {
    setMaxTokens(display(current?.maxTokens))
    setTimeoutSeconds(display(current?.timeoutSeconds))
  }

  const onKeyDown = (e: KeyboardEvent<HTMLInputElement>) => {
    if (e.key === 'Enter') void commit()
    else if (e.key === 'Escape') revert()
  }

  const inputClass =
    'rounded-lg border border-line-2 bg-card px-2 py-1 font-mono text-[12px] text-fg placeholder:text-fg-subtle focus:border-accent focus:outline-none disabled:opacity-50'

  return (
    <div className="flex flex-wrap items-center gap-2">
      <label className="flex items-center gap-1.5">
        <span className="text-xs text-fg-subtle">{t('models.maxTokens')}</span>
        <input
          type="number"
          min={0}
          max={32768}
          value={maxTokens}
          disabled={disabled}
          onChange={(e) => setMaxTokens(e.target.value)}
          onBlur={() => void commit()}
          onKeyDown={onKeyDown}
          placeholder="∞"
          aria-label={t('models.maxTokens')}
          className={`w-24 ${inputClass}`}
        />
      </label>
      <label className="flex items-center gap-1.5">
        <span className="text-xs text-fg-subtle">{t('models.timeoutSeconds')}</span>
        <input
          type="number"
          min={0}
          value={timeoutSeconds}
          disabled={disabled}
          onChange={(e) => setTimeoutSeconds(e.target.value)}
          onBlur={() => void commit()}
          onKeyDown={onKeyDown}
          placeholder="∞"
          aria-label={t('models.timeoutSeconds')}
          className={`w-20 ${inputClass}`}
        />
      </label>
    </div>
  )
}

// display renders 0 (no limit) as an empty input so the ∞ placeholder shows.
function display(v: number | undefined): string {
  return v ? String(v) : ''
}

// parse reads a non-negative integer, treating an empty or invalid field as
// the "no limit" 0.
function parse(s: string): number {
  const n = parseInt(s, 10)
  if (Number.isNaN(n) || n < 0) return 0
  return n
}
