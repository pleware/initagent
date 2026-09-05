import type { ReactNode } from 'react'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from '@ia/web/ui/dialog'

export default function Modal({
  title,
  onClose,
  children,
  wide,
}: {
  title: string
  onClose: () => void
  children: ReactNode
  wide?: boolean
}) {
  return (
    <Dialog
      open
      onOpenChange={(next: boolean) => {
        if (!next) onClose()
      }}
    >
      <DialogContent
        className={wide ? 'sm:max-w-2xl' : 'sm:max-w-lg'}
        aria-describedby={undefined}
      >
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
        </DialogHeader>
        {children}
      </DialogContent>
    </Dialog>
  )
}
