import { FormEvent, useState } from 'react'
import { useTranslation } from 'react-i18next'
import Modal from './Modal'

/** Destructive confirm: the person must type `phrase` exactly. Paste is off. */
export default function ConfirmTypeDialog({
  title,
  hint,
  phrase,
  confirmLabel,
  busy,
  error,
  onClose,
  onConfirm,
}: {
  title: string
  hint: string
  phrase: string
  confirmLabel: string
  busy?: boolean
  error?: string
  onClose: () => void
  onConfirm: () => void | Promise<void>
}) {
  const { t } = useTranslation()
  const [typed, setTyped] = useState('')
  const matches = typed === phrase

  const submit = async (event: FormEvent) => {
    event.preventDefault()
    if (!matches || busy) return
    await onConfirm()
  }

  const blockPaste = (event: { preventDefault: () => void }) => {
    event.preventDefault()
  }

  return (
    <Modal title={title} onClose={onClose}>
      <form onSubmit={submit} className="flex flex-col gap-4">
        <p className="text-sm text-fg-muted">{hint}</p>
        <p className="font-mono text-sm text-fg">{phrase}</p>
        <div>
          <label htmlFor="confirm-type-phrase" className="field-label">
            {t('code.deleteTypeLabel')}
          </label>
          <input
            id="confirm-type-phrase"
            className="field-input mt-2 font-mono"
            value={typed}
            onChange={(e) => setTyped(e.target.value)}
            onPaste={blockPaste}
            onDrop={blockPaste}
            autoComplete="off"
            autoCorrect="off"
            autoCapitalize="off"
            spellCheck={false}
            required
          />
        </div>
        {error ? <p className="text-sm text-fail-fg">{error}</p> : null}
        <div className="flex justify-end gap-2 border-t border-line-1 pt-4">
          <button type="button" className="btn-secondary" onClick={onClose} disabled={busy}>
            {t('common.cancel')}
          </button>
          <button type="submit" className="btn-danger" disabled={!matches || busy}>
            {busy ? t('common.loading') : confirmLabel}
          </button>
        </div>
      </form>
    </Modal>
  )
}
