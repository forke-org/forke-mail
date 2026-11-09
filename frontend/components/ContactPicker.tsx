import { RowSelectionState } from '@tanstack/react-table';
import { Search } from 'lucide-react';
import { useEffect, useMemo, useState } from 'react';
import { api } from '../lib/api';
import { fullName } from '../lib/format';
import type { Contact } from '../lib/types';
import { useLoad } from '../lib/useLoad';
import DataTable, { Columns, Strong } from './DataTable';
import { Modal } from './Modal';
import { Badge, Button, Chip, Input } from './ui';

const pageSize = 25;

export default function ContactPicker({ selected, onClose, onSave }: { selected: string[]; onClose: () => void; onSave: (ids: string[]) => void }) {
  const [selection, setSelection] = useState<RowSelectionState>(() => Object.fromEntries(selected.map((id) => [id, true])));
  const [searchInput, setSearchInput] = useState('');
  const [search, setSearch] = useState('');
  const [page, setPage] = useState(0);

  useEffect(() => {
    const t = setTimeout(() => { setSearch(searchInput.trim()); setPage(0); }, 250);
    return () => clearTimeout(t);
  }, [searchInput]);

  const [state] = useLoad(() => {
    const params = new URLSearchParams({ limit: String(pageSize), offset: String(page * pageSize), status: 'active' });
    if (search) params.set('search', search);
    return api<{ contacts: Contact[]; total: number }>('GET', `/api/contacts?${params}`);
  }, [search, page]);

  const columns = useMemo<Columns<Contact>>(() => [
    { accessorKey: 'email', header: 'Email', cell: ({ row }) => <Strong sub={fullName(row.original)}>{row.original.email}</Strong> },
    { accessorKey: 'tags', header: 'Tags', cell: ({ getValue }) => <div className="flex flex-wrap gap-1">{(getValue() as string[]).map((t) => <Chip key={t}>{t}</Chip>)}</div> },
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
          {count ? <Button variant="ghost" className="mr-auto" onClick={() => setSelection({})}>Clear selection</Button> : null}
          <Button onClick={onClose}>Cancel</Button>
          <Button variant="primary" onClick={() => onSave(Object.keys(selection))}>Use {count} contact{count === 1 ? '' : 's'}</Button>
        </>
      )}
    >
      <div className="relative">
        <Search className="pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2 text-muted" />
        <Input type="search" className="pl-9" placeholder="Search by email or name" value={searchInput} onChange={(e) => setSearchInput(e.target.value)} autoFocus />
      </div>
      <div className="overflow-hidden rounded-xl shadow-card">
        <DataTable
          columns={columns}
          data={state.data ? state.data.contacts : null}
          error={state.error}
          loading={state.loading}
          noun="contacts"
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
