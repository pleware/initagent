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
  className,
}: {
  title: string
  onClose: () => void
  children: ReactNode
  wide?: boolean
  className?: string
}) {
  // An explicit className replaces the default width; `wide` is the shared
  // short form when a caller only needs the medium size.
  const width = className ?? (wide ? 'sm:max-w-2xl' : 'sm:max-w-lg')
  return (
    <Dialog
      open
      onOpenChange={(next: boolean) => {
        if (!next) onClose()
      }}
    >
      <DialogContent className={width} aria-describedby={undefined}>
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
        </DialogHeader>
        {children}
      </DialogContent>
    </Dialog>
  )
}
