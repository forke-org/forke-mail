import { flexRender, getCoreRowModel, getPaginationRowModel, getSortedRowModel, useReactTable } from '@tanstack/react-table';
import { useMemo, useState } from 'react';
import { number } from '../lib/format';
import { Empty, Loading } from './ui';

const selectColumn = {
  id: 'select',
  enableSorting: false,
  meta: { className: 'col-select' },
  header: ({ table }) => (
    <input
      type="checkbox"
      className="checkbox"
      aria-label="Select all on this page"
      checked={table.getIsAllPageRowsSelected()}
      ref={(el) => { if (el) el.indeterminate = table.getIsSomePageRowsSelected(); }}
      onChange={table.getToggleAllPageRowsSelectedHandler()}
    />
  ),
  cell: ({ row }) => (
    <input
      type="checkbox"
      className="checkbox"
      aria-label="Select row"
      checked={row.getIsSelected()}
      onClick={(e) => e.stopPropagation()}
      onChange={row.getToggleSelectedHandler()}
    />
  ),
};

function SortIcon({ direction }) {
  if (!direction) return <span className="sort-icon" aria-hidden="true">↕</span>;
  return <span className="sort-icon active" aria-hidden="true">{direction === 'asc' ? '↑' : '↓'}</span>;
}

export default function DataTable({
  columns, data, loading, error, empty, onRowClick, getRowId = (row) => row.id,
  selectable, rowSelection, onRowSelectionChange, server, pageSize = 20,
}) {
  const [sorting, setSorting] = useState([]);
  const [pagination, setPagination] = useState({ pageIndex: 0, pageSize });
  const allColumns = useMemo(() => (selectable ? [selectColumn, ...columns] : columns), [selectable, columns]);
  const serverPagination = server ? { pageIndex: server.pageIndex, pageSize: server.pageSize } : null;

  const table = useReactTable({
    data: data || [],
    columns: allColumns,
    getRowId,
    state: { sorting, pagination: serverPagination || pagination, rowSelection: rowSelection || {} },
    onSortingChange: setSorting,
    onPaginationChange: server
      ? (updater) => server.onPageChange((typeof updater === 'function' ? updater(serverPagination) : updater).pageIndex)
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
  if (!data) return <Loading />;
  if (!data.length) return <Empty title={empty?.title || 'Nothing here yet'} hint={empty?.hint} action={empty?.action} />;

  const { pageIndex, pageSize: size } = table.getState().pagination;
  const total = server ? server.total : table.getPrePaginationRowModel().rows.length;
  const first = pageIndex * size + 1;
  const last = Math.min(total, (pageIndex + 1) * size);

  return (
    <div className={`data-table${loading ? ' is-loading' : ''}`}>
      <div className="table-wrap">
        <table>
          <thead>
            {table.getHeaderGroups().map((group) => (
              <tr key={group.id}>
                {group.headers.map((header) => {
                  const sortable = header.column.getCanSort();
                  return (
                    <th key={header.id} className={header.column.columnDef.meta?.className}>
                      {header.isPlaceholder ? null : sortable ? (
                        <button type="button" className="th-sort" onClick={header.column.getToggleSortingHandler()}>
                          {flexRender(header.column.columnDef.header, header.getContext())}
                          <SortIcon direction={header.column.getIsSorted()} />
                        </button>
                      ) : flexRender(header.column.columnDef.header, header.getContext())}
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
                className={[onRowClick ? 'clickable' : '', row.getIsSelected() ? 'selected' : ''].join(' ').trim() || undefined}
                onClick={onRowClick ? () => onRowClick(row.original) : undefined}
              >
                {row.getVisibleCells().map((cell) => (
                  <td key={cell.id} className={cell.column.columnDef.meta?.className}>
                    {flexRender(cell.column.columnDef.cell, cell.getContext())}
                  </td>
                ))}
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      {total > size ? (
        <div className="pager">
          <span className="small muted">{number(first)}–{number(last)} of {number(total)}</span>
          <div className="actions">
            <button className="btn btn-sm" type="button" disabled={!table.getCanPreviousPage()} onClick={() => table.previousPage()}>Previous</button>
            <button className="btn btn-sm" type="button" disabled={!table.getCanNextPage()} onClick={() => table.nextPage()}>Next</button>
          </div>
        </div>
      ) : null}
    </div>
  );
}
