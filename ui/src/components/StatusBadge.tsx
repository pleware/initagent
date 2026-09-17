const kindIcons: Record<string, string> = {
  claude: '✳',
  codex: '◆',
  shell: '❯',
}

export default function StatusBadge({
  status,
  kind,
}: {
  status: 'working' | 'idle' | 'exited'
  kind: string
}) {
  const color =
    status === 'working'
      ? 'bg-ok animate-pulse'
      : status === 'idle'
        ? 'bg-fg-subtle'
        : 'bg-fail'
  return (
    <span className="flex items-center gap-1.5" title={`${kind || 'terminal'} — ${status}`}>
      <span className={`h-1.5 w-1.5 rounded-full ${color}`} />
      {kind && kind !== 'shell' && (
        <span className="text-[11px] text-accent">{kindIcons[kind] ?? '●'}</span>
      )}
    </span>
  )
}
