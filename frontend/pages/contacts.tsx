import { RowSelectionState } from '@tanstack/react-table';
import { Tag as TagIcon, Trash2, Upload, X } from 'lucide-react';
import { useRouter } from 'next/router';
import { FormEvent, useEffect, useMemo, useState } from 'react';
import DataTable, { Columns, Strong } from '../components/DataTable';
import ImportForm from '../components/ImportForm';
import { Modal, useConfirm } from '../components/Modal';
import { Badge, Button, Card, Chip, Input, PageHeader, SearchInput, Select, Toolbar } from '../components/ui';
import { api, describeError } from '../lib/api';
import { formatDate, fullName, number, plural } from '../lib/format';
import { useToast } from '../lib/toast';
import type { Contact, Tag } from '../lib/types';
import { useLoad } from '../lib/useLoad';

const pageSize = 25;
const statuses: [string, string][] = [['', 'All statuses'], ['active', 'Active'], ['unsubscribed', 'Unsubscribed'], ['bounced', 'Bounced'], ['complained', 'Complained']];

export default function Contacts() {
  const router = useRouter();
  const toast = useToast();
  const confirm = useConfirm();
  const [importing, setImporting] = useState(false);
  const [searchInput, setSearchInput] = useState('');
  const [filters, setFilters] = useState({ search: '', status: '', tag: '' });
  const [page, setPage] = useState(0);
  const [selection, setSelection] = useState<RowSelectionState>({});
  const [bulkTag, setBulkTag] = useState('');

  useEffect(() => {
    if (router.query.import) {
      setImporting(true);
      router.replace('/contacts/', undefined, { shallow: true });
    }
  }, [router.query.import]);

  useEffect(() => {
    const t = setTimeout(() => { setFilters((f) => ({ ...f, search: searchInput.trim() })); setPage(0); }, 250);
    return () => clearTimeout(t);
  }, [searchInput]);

  const [tagsState, reloadTags] = useLoad(() => api<{ tags: Tag[] }>('GET', '/api/tags'), []);
  const [state, reload] = useLoad(() => {
    const params = new URLSearchParams({ limit: String(pageSize), offset: String(page * pageSize) });
    if (filters.search) params.set('search', filters.search);
    if (filters.status) params.set('status', filters.status);
    if (filters.tag) params.set('tag_id', filters.tag);
    return api<{ contacts: Contact[]; total: number }>('GET', `/api/contacts?${params}`);
  }, [filters, page]);

  const setFilter = (key: 'status' | 'tag') => (e: React.ChangeEvent<HTMLSelectElement>) => { setFilters((f) => ({ ...f, [key]: e.target.value })); setPage(0); };
  const selectedIds = Object.keys(selection);

  const remove = async (contact: Contact) => {
    if (!(await confirm('Delete contact', `Delete ${contact.email}? This cannot be undone.`, 'Delete', true))) return;
    try {
      await api('DELETE', `/api/contacts/${contact.id}`);
      setSelection((s) => { const next = { ...s }; delete next[contact.id]; return next; });
      toast('Contact deleted.', 'success');
      reload();
    } catch (err) {
      toast(describeError(err), 'error');
    }
  };

  const applyTag = async (e: FormEvent) => {
    e.preventDefault();
    const tags = bulkTag.split(',').map((t) => t.trim()).filter(Boolean);
    if (!tags.length) return;
    try {
      await api('POST', '/api/contacts/tags', { contact_ids: selectedIds, tags });
      toast(`Tagged ${plural(selectedIds.length, 'contact')}.`, 'success');
      setBulkTag('');
      setSelection({});
      reload();
      reloadTags();
    } catch (err) {
      toast(describeError(err), 'error');
    }
  };

  const columns = useMemo<Columns<Contact>>(() => [
    {
      accessorKey: 'email', header: 'Contact',
      cell: ({ row }) => (
        <div className="flex items-center gap-3">
          <span className="grid size-8 shrink-0 place-items-center rounded-full bg-surface-3 text-[11px] font-semibold text-ink-2 uppercase">{row.original.email.slice(0, 2)}</span>
          <Strong sub={fullName(row.original) === '—' ? undefined : fullName(row.original)}>{row.original.email}</Strong>
        </div>
      ),
    },
    { accessorKey: 'tags', header: 'Tags', cell: ({ getValue }) => <div className="flex flex-wrap gap-1">{(getValue() as string[]).length ? (getValue() as string[]).map((t) => <Chip key={t}>{t}</Chip>) : <span className="text-faint">—</span>}</div> },
    { accessorKey: 'status', header: 'Status', cell: ({ getValue }) => <Badge value={getValue()} /> },
    { accessorKey: 'created_at', header: 'Added', cell: ({ getValue }) => <span className="text-muted">{formatDate(getValue())}</span> },
    {
      id: 'actions', header: '', meta: { className: 'w-12 text-right' },
      cell: ({ row }) => <Button size="xs" variant="ghost" aria-label={`Delete ${row.original.email}`} className="text-muted hover:text-danger" icon={Trash2} onClick={(e) => { e.stopPropagation(); remove(row.original); }} />,
    },
  ], []);

  const tags = tagsState.data ? tagsState.data.tags : [];
  const filtered = filters.search || filters.status || filters.tag;
  return (
    <>
      <PageHeader
        title="Contacts"
        description={state.data ? `${plural(state.data.total, 'contact')}${filtered ? ' match these filters' : ' in this workspace'}.` : 'Everyone you can email from this workspace.'}
        actions={<Button variant="primary" icon={Upload} onClick={() => setImporting(true)}>Import contacts</Button>}
      />
      <Card flush>
        {selectedIds.length ? (
          <form onSubmit={applyTag} className="flex flex-col gap-2 border-b border-accent-line bg-accent-soft px-4 py-3 sm:flex-row sm:items-center">
            <span className="text-sm font-medium text-ink">{number(selectedIds.length)} selected</span>
            <Input className="sm:ml-2 sm:max-w-xs" placeholder="Add tags, comma separated" value={bulkTag} onChange={(e) => setBulkTag(e.target.value)} />
            <div className="flex gap-2">
              <Button size="sm" variant="primary" type="submit" icon={TagIcon}>Add tag</Button>
              <Button size="sm" variant="ghost" icon={X} onClick={() => setSelection({})}>Clear</Button>
            </div>
          </form>
        ) : (
          <Toolbar>
            <SearchInput className="flex-1" placeholder="Search by email or name" value={searchInput} onChange={setSearchInput} />
            <Select value={filters.status} onChange={setFilter('status')} aria-label="Status" className="sm:w-44">
              {statuses.map(([v, l]) => <option key={v} value={v}>{l}</option>)}
            </Select>
            <Select value={filters.tag} onChange={setFilter('tag')} aria-label="Tag" className="sm:w-48">
              <option value="">All tags</option>
              {tags.map((t) => <option key={t.id} value={t.id}>{t.name} ({number(t.contacts)})</option>)}
            </Select>
          </Toolbar>
        )}
        <DataTable
          columns={columns}
          data={state.data ? state.data.contacts : null}
          error={state.error}
          loading={state.loading}
          noun="contacts"
          selectable
          rowSelection={selection}
          onRowSelectionChange={setSelection}
          server={{ pageIndex: page, pageSize, total: state.data ? state.data.total : 0, onPageChange: setPage }}
          empty={{
            title: filtered ? 'No matching contacts' : 'No contacts yet',
            hint: 'Import a CSV or paste a list. Add tags so you can target them in emails.',
            action: filtered ? undefined : <Button variant="primary" icon={Upload} onClick={() => setImporting(true)}>Import contacts</Button>,
          }}
        />
      </Card>
      {importing ? (
        <Modal title="Import contacts" description="Duplicates are merged. Unsubscribed and bounced contacts are never re-activated." onClose={() => setImporting(false)}>
          <ImportForm onDone={() => { reload(); reloadTags(); }} onCancel={() => setImporting(false)} />
        </Modal>
      ) : null}
    </>
  );
}
