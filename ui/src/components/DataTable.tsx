import type { ReactNode } from 'react'

// One table for the hub's own surfaces.
//
// Accounts, organizations and members are three lists that differ only in
// their columns, and writing the same header markup, row borders and empty
// state three times is how they drift apart. The inherited screens
// (agent runs, the file browser) keep their own markup on purpose: rewriting
// them here would be a diff with no behaviour in it, and every unrelated
// change to an upstream file costs us at the next merge.

export interface Column<T> {
  header: string
  cell: (row: T) => ReactNode
  // Tailwind width class for a column that should not stretch, e.g. actions.
  width?: string
  // Column that carries no header text still needs an accessible name.
  srHeader?: string
}

export default function DataTable<T>({
  rows,
  columns,
  rowKey,
  empty,
}: {
  // null means "not loaded yet", which is a different screen from an empty
  // list: one says wait, the other says there is nothing here.
  rows: T[] | null
  columns: Column<T>[]
  rowKey: (row: T) => string
  empty: ReactNode
}) {
  if (rows === null) {
    return <p className="text-zinc-500">Loading…</p>
  }
  if (rows.length === 0) {
    return <div className="surface rounded-2xl p-12 text-center">{empty}</div>
  }
  return (
    <div className="surface overflow-hidden rounded-2xl">
      <table className="w-full text-sm">
        <thead className="bg-zinc-900 text-left text-xs text-zinc-500">
          <tr>
            {columns.map((column, i) => (
              <th
                key={column.header || `col-${i}`}
                className={`px-4 py-2.5 font-medium ${column.width ?? ''}`}
              >
                {column.header || (
                  <span className="sr-only">{column.srHeader ?? 'Actions'}</span>
                )}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {rows.map((row) => (
            <tr key={rowKey(row)} className="border-t border-zinc-800/60">
              {columns.map((column, i) => (
                <td key={column.header || `col-${i}`} className="px-4 py-3">
                  {column.cell(row)}
                </td>
              ))}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}
