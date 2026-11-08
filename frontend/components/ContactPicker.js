import { useEffect, useMemo, useState } from 'react';
import { api } from '../lib/api';
import { fullName } from '../lib/format';
import { useLoad } from '../lib/useLoad';
import DataTable from './DataTable';
import { Modal } from './Modal';
import { Badge } from './ui';

const pageSize = 25;

export default function ContactPicker({ selected, onClose, onSave }) {
  const [selection, setSelection] = useState(() => Object.fromEntries(selected.map((id) => [id, true])));
  const [searchInput, setSearchInput] = useState('');
  const [search, setSearch] = useState('');
  const [page, setPage] = useState(0);

  useEffect(() => {
    const t = setTimeout(() => { setSearch(searchInput.trim()); setPage(0); }, 250);
    return () => clearTimeout(t);
  }, [searchInput]);

  const [state] = useLoad(() => {
    const params = new URLSearchParams({ limit: pageSize, offset: page * pageSize, status: 'active' });
    if (search) params.set('search', search);
    return api('GET', `/api/contacts?${params}`);
  }, [search, page]);

  const columns = useMemo(() => [
    { accessorKey: 'email', header: 'Email', cell: ({ getValue }) => <span className="cell-strong">{getValue()}</span> },
    { id: 'name', header: 'Name', cell: ({ row }) => fullName(row.original) },
    { accessorKey: 'tags', header: 'Tags', cell: ({ getValue }) => <div className="chips">{getValue().map((t) => <span key={t} className="chip">{t}</span>)}</div> },
    { accessorKey: 'status', header: 'Status', cell: ({ getValue }) => <Badge value={getValue()} /> },
  ], []);

  const count = Object.keys(selection).length;
  return (
    <Modal
      wide
      title="Choose contacts"
      description="Only active contacts are shown. Selections are kept as you search and page."
      onClose={onClose}
      footer={(
        <>
          {count ? <button className="btn btn-ghost" type="button" onClick={() => setSelection({})}>Clear selection</button> : null}
          <span className="spacer" />
          <button className="btn" type="button" onClick={onClose}>Cancel</button>
          <button className="btn btn-primary" type="button" onClick={() => onSave(Object.keys(selection))}>Use {count} contact{count === 1 ? '' : 's'}</button>
        </>
      )}
    >
      <input type="search" placeholder="Search by email or name" value={searchInput} onChange={(e) => setSearchInput(e.target.value)} autoFocus />
      <div className="card card-flush">
        <DataTable
          columns={columns}
          data={state.data ? state.data.contacts : null}
          error={state.error}
          loading={state.loading}
          empty={{ title: search ? 'No matching contacts' : 'No active contacts', hint: 'Import contacts to add them here.' }}
          selectable
          rowSelection={selection}
          onRowSelectionChange={setSelection}
          onRowClick={(c) => setSelection((s) => {
            const next = { ...s };
            if (next[c.id]) delete next[c.id]; else next[c.id] = true;
            return next;
          })}
          server={{ pageIndex: page, pageSize, total: state.data ? state.data.total : 0, onPageChange: setPage }}
        />
      </div>
    </Modal>
  );
}
