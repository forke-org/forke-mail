import {
  ColumnDef, flexRender, getCoreRowModel, getPaginationRowModel, getSortedRowModel,
  OnChangeFn, PaginationState, RowSelectionState, SortingState, useReactTable,
} from '@tanstack/react-table';
import clsx from 'clsx';
import { ArrowDown, ArrowUp, ChevronLeft, ChevronRight, ChevronsUpDown } from 'lucide-react';
import { ReactNode, useMemo, useState } from 'react';
import { number } from '../lib/format';
import { Empty, Skeleton } from './ui';

declare module '@tanstack/react-table' {
  interface ColumnMeta<TData, TValue> {
    className?: string;
  }
}

export type Columns<T> = ColumnDef<T, any>[];

function selectColumn<T>(): ColumnDef<T, any> {
  return {
    id: 'select',
    enableSorting: false,
    meta: { className: 'w-10 !pr-0' },
    header: ({ table }) => (
      <input
        type="checkbox"
        className="size-4 cursor-pointer rounded accent-accent"
        aria-label="Select all on this page"
        checked={table.getIsAllPageRowsSelected()}
        ref={(el) => { if (el) el.indeterminate = table.getIsSomePageRowsSelected(); }}
        onChange={table.getToggleAllPageRowsSelectedHandler()}
      />
    ),
    cell: ({ row }) => (
      <input
        type="checkbox"
        className="size-4 cursor-pointer rounded accent-accent"
        aria-label="Select row"
        checked={row.getIsSelected()}
        onClick={(e) => e.stopPropagation()}
        onChange={row.getToggleSelectedHandler()}
      />
    ),
  };
}

interface Props<T> {
  columns: Columns<T>;
  data: T[] | null | undefined;
  loading?: boolean;
  error?: string;
  empty?: { title: ReactNode; hint?: ReactNode; action?: ReactNode };
  onRowClick?: (row: T) => void;
  getRowId?: (row: T) => string;
  selectable?: boolean;
  rowSelection?: RowSelectionState;
  onRowSelectionChange?: OnChangeFn<RowSelectionState>;
  server?: { pageIndex: number; pageSize: number; total: number; onPageChange: (page: number) => void };
  pageSize?: number;
  noun?: string;
}

export default function DataTable<T>({
  columns, data, loading, error, empty, onRowClick, getRowId = (row: any) => row.id,
  selectable, rowSelection, onRowSelectionChange, server, pageSize = 20, noun = 'rows',
}: Props<T>) {
  const [sorting, setSorting] = useState<SortingState>([]);
  const [pagination, setPagination] = useState<PaginationState>({ pageIndex: 0, pageSize });
  const allColumns = useMemo(() => (selectable ? [selectColumn<T>(), ...columns] : columns), [selectable, columns]);
  const serverPagination = server ? { pageIndex: server.pageIndex, pageSize: server.pageSize } : null;

  const table = useReactTable<T>({
    data: data || [],
    columns: allColumns,
    getRowId,
    state: { sorting, pagination: serverPagination || pagination, rowSelection: rowSelection || {} },
    onSortingChange: setSorting,
    onPaginationChange: server
      ? (updater) => server.onPageChange((typeof updater === 'function' ? updater(serverPagination as PaginationState) : updater).pageIndex)
      : setPagination,
    onRowSelectionChange,
    enableRowSelection: Boolean(selectable),
    manualPagination: Boolean(server),
    manualSorting: Boolean(server),
    enableSorting: !server,
    rowCount: server ? server.total : undefined,
    autoResetPageIndex: false,
    getCoreRowModel: getCoreRowModel(),
    getSortedRowModel: getSortedRowModel(),
    getPaginationRowModel: getPaginationRowModel(),
  });

  if (error) return <Empty title="Something went wrong" hint={error} />;
  if (!data) {
    return (
      <div className="divide-y divide-line">
        {Array.from({ length: 5 }, (_, i) => (
          <div key={i} className="flex items-center gap-4 px-5 py-3.5">
            <Skeleton className="h-4 w-1/3" />
            <Skeleton className="h-4 w-20" />
            <Skeleton className="ml-auto h-4 w-24" />
          </div>
        ))}
      </div>
    );
  }
  if (!data.length) return <Empty title={empty?.title || 'Nothing here yet'} hint={empty?.hint} action={empty?.action} />;

  const { pageIndex, pageSize: size } = table.getState().pagination;
  const total = server ? server.total : table.getPrePaginationRowModel().rows.length;
  const first = pageIndex * size + 1;
  const last = Math.min(total, (pageIndex + 1) * size);

  return (
    <div className={clsx('transition-opacity', loading && 'opacity-60')}>
      <div className="overflow-x-auto">
        <table className="w-full border-collapse text-sm">
          <thead>
            {table.getHeaderGroups().map((group) => (
              <tr key={group.id} className="border-b border-line bg-surface-2">
                {group.headers.map((header) => {
                  const sortable = header.column.getCanSort();
                  const sorted = header.column.getIsSorted();
                  const label = header.isPlaceholder ? null : flexRender(header.column.columnDef.header, header.getContext());
                  return (
                    <th key={header.id} className={clsx('h-10 px-4 text-left text-xs font-medium whitespace-nowrap text-muted first:pl-5 last:pr-5', header.column.columnDef.meta?.className)}>
                      {sortable && label ? (
                        <button type="button" className="group inline-flex cursor-pointer items-center gap-1 hover:text-ink" onClick={header.column.getToggleSortingHandler()}>
                          {label}
                          {sorted === 'asc' ? <ArrowUp className="size-3 text-accent" /> : sorted === 'desc' ? <ArrowDown className="size-3 text-accent" /> : <ChevronsUpDown className="size-3 opacity-0 transition-opacity group-hover:opacity-60" />}
                        </button>
                      ) : label}
                    </th>
                  );
                })}
              </tr>
            ))}
          </thead>
          <tbody>
            {table.getRowModel().rows.map((row) => (
              <tr
                key={row.id}
                className={clsx(
                  'border-b border-line transition-colors last:border-0',
                  onRowClick && 'cursor-pointer hover:bg-surface-2',
                  row.getIsSelected() && 'bg-accent-soft hover:bg-accent-soft',
                )}
                onClick={onRowClick ? () => onRowClick(row.original) : undefined}
              >
                {row.getVisibleCells().map((cell) => (
                  <td key={cell.id} className={clsx('h-12 px-4 align-middle text-ink-2 first:pl-5 last:pr-5', cell.column.columnDef.meta?.className)}>
                    {flexRender(cell.column.columnDef.cell, cell.getContext())}
                  </td>
                ))}
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      <div className="flex items-center justify-between gap-3 border-t border-line px-5 py-2.5">
        <span className="text-xs text-muted tabular-nums">
          {total > size ? `Showing ${number(first)}–${number(last)} of ${number(total)} ${noun}` : `${number(total)} ${noun}`}
        </span>
        {total > size ? (
          <div className="flex items-center gap-1">
            <button type="button" aria-label="Previous page" className="grid size-7 cursor-pointer place-items-center rounded-md text-muted hover:bg-surface-3 hover:text-ink disabled:pointer-events-none disabled:opacity-40" disabled={!table.getCanPreviousPage()} onClick={() => table.previousPage()}><ChevronLeft className="size-4" /></button>
            <span className="px-1 text-xs text-muted tabular-nums">{pageIndex + 1} / {Math.max(1, Math.ceil(total / size))}</span>
            <button type="button" aria-label="Next page" className="grid size-7 cursor-pointer place-items-center rounded-md text-muted hover:bg-surface-3 hover:text-ink disabled:pointer-events-none disabled:opacity-40" disabled={!table.getCanNextPage()} onClick={() => table.nextPage()}><ChevronRight className="size-4" /></button>
          </div>
        ) : null}
      </div>
    </div>
  );
}

export function Strong({ children, sub }: { children: ReactNode; sub?: ReactNode }) {
  return (
    <div className="min-w-0">
      <div className="truncate font-medium text-ink">{children}</div>
      {sub ? <div className="mt-0.5 max-w-md truncate text-xs text-muted">{sub}</div> : null}
    </div>
  );
}
