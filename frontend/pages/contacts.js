import { useEffect, useMemo, useState } from 'react';
import DataTable from '../components/DataTable';
import Icon from '../components/Icon';
import ImportForm from '../components/ImportForm';
import { Modal, useConfirm } from '../components/Modal';
import { Badge, Card, PageHead } from '../components/ui';
import { api, describeError } from '../lib/api';
import { formatDate, fullName, number, plural } from '../lib/format';
import { useLoad } from '../lib/useLoad';
import { useToast } from '../lib/toast';

const pageSize = 25;
const statuses = [['', 'All statuses'], ['active', 'Active'], ['unsubscribed', 'Unsubscribed'], ['bounced', 'Bounced'], ['complained', 'Complained']];

export default function Contacts() {
  const toast = useToast();
  const confirm = useConfirm();
  const [importing, setImporting] = useState(false);
  const [searchInput, setSearchInput] = useState('');
  const [filters, setFilters] = useState({ search: '', status: '', tag: '' });
  const [page, setPage] = useState(0);
  const [selection, setSelection] = useState({});
  const [bulkTag, setBulkTag] = useState('');

  useEffect(() => {
    const t = setTimeout(() => { setFilters((f) => ({ ...f, search: searchInput.trim() })); setPage(0); }, 250);
    return () => clearTimeout(t);
  }, [searchInput]);

  const [tagsState, reloadTags] = useLoad(() => api('GET', '/api/tags'), []);
  const [state, reload] = useLoad(() => {
    const params = new URLSearchParams({ limit: pageSize, offset: page * pageSize });
    if (filters.search) params.set('search', filters.search);
    if (filters.status) params.set('status', filters.status);
    if (filters.tag) params.set('tag_id', filters.tag);
    return api('GET', `/api/contacts?${params}`);
  }, [filters, page]);

  const setFilter = (key) => (e) => { setFilters((f) => ({ ...f, [key]: e.target.value })); setPage(0); };
  const selectedIds = Object.keys(selection);

  const remove = async (contact) => {
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

  const applyTag = async (e) => {
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

  const columns = useMemo(() => [
    { accessorKey: 'email', header: 'Email', cell: ({ getValue }) => <span className="cell-strong">{getValue()}</span> },
    { id: 'name', header: 'Name', cell: ({ row }) => fullName(row.original) },
    { accessorKey: 'tags', header: 'Tags', cell: ({ getValue }) => <div className="chips">{getValue().map((t) => <span key={t} className="chip">{t}</span>)}</div> },
    { accessorKey: 'status', header: 'Status', cell: ({ getValue }) => <Badge value={getValue()} /> },
    { accessorKey: 'created_at', header: 'Added', cell: ({ getValue }) => formatDate(getValue()) },
    {
      id: 'actions', header: '', meta: { className: 'col-actions' },
      cell: ({ row }) => <button className="btn btn-ghost btn-sm btn-danger" type="button" onClick={(e) => { e.stopPropagation(); remove(row.original); }}>Delete</button>,
    },
  ], []);

  const tags = tagsState.data ? tagsState.data.tags : [];
  return (
    <>
      <PageHead title="Contacts" subtitle={state.data ? plural(state.data.total, 'contact') : 'Everyone you can email from this workspace.'}>
        <button className="btn btn-primary" type="button" onClick={() => setImporting(true)}><Icon name="upload" />Import contacts</button>
      </PageHead>
      <div className="toolbar">
        <input type="search" className="toolbar-search" placeholder="Search by email or name" value={searchInput} onChange={(e) => setSearchInput(e.target.value)} />
        <select value={filters.status} onChange={setFilter('status')} aria-label="Status">
          {statuses.map(([v, l]) => <option key={v} value={v}>{l}</option>)}
        </select>
        <select value={filters.tag} onChange={setFilter('tag')} aria-label="Tag">
          <option value="">All tags</option>
          {tags.map((t) => <option key={t.id} value={t.id}>{t.name} ({number(t.contacts)})</option>)}
        </select>
      </div>
      {selectedIds.length ? (
        <form className="bulk-bar" onSubmit={applyTag}>
          <span className="cell-strong">{number(selectedIds.length)} selected</span>
          <input type="text" placeholder="Add tags, comma separated" value={bulkTag} onChange={(e) => setBulkTag(e.target.value)} />
          <button className="btn btn-sm btn-primary" type="submit">Add tag</button>
          <button className="btn btn-sm btn-ghost" type="button" onClick={() => setSelection({})}>Clear</button>
        </form>
      ) : null}
      <Card flush>
        <DataTable
          columns={columns}
          data={state.data ? state.data.contacts : null}
          error={state.error}
          loading={state.loading}
          selectable
          rowSelection={selection}
          onRowSelectionChange={setSelection}
          server={{ pageIndex: page, pageSize, total: state.data ? state.data.total : 0, onPageChange: setPage }}
          empty={{
            title: filters.search || filters.status || filters.tag ? 'No matching contacts' : 'No contacts yet',
            hint: 'Import a CSV or paste a list. Add tags so you can target them in emails.',
            action: <button className="btn btn-primary" type="button" onClick={() => setImporting(true)}>Import contacts</button>,
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
