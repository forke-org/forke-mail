import { useEffect, useMemo, useState } from 'react';
import DataTable from '../components/DataTable';
import Icon from '../components/Icon';
import { Modal, useConfirm } from '../components/Modal';
import { Badge, Card, Field, PageHead, Tabs } from '../components/ui';
import { api, describeError } from '../lib/api';
import { formatDate, number, plural } from '../lib/format';
import { useLoad } from '../lib/useLoad';
import { useToast } from '../lib/toast';

const pageSize = 25;

const sources = {
  unsubscribe_link: 'Unsubscribe link',
  ses_bounce: 'Hard bounce',
  ses_complaint: 'Spam complaint',
  manual: 'Added manually',
  import: 'Imported',
};

const riskyRemoval = {
  ses_bounce: 'This address hard bounced. Sending to it again will most likely bounce and hurts your bounce rate, which can pause sending for your whole workspace. Amazon SES may also still block it through its account-level suppression list.',
  ses_complaint: 'This recipient marked your email as spam. Emailing them again risks another complaint, which can pause sending for your whole workspace.',
};

function AddSuppressions({ onClose, onDone }) {
  const toast = useToast();
  const [mode, setMode] = useState('single');
  const [email, setEmail] = useState('');
  const [text, setText] = useState('');
  const [file, setFile] = useState(null);
  const [busy, setBusy] = useState(false);
  const [result, setResult] = useState(null);

  const submit = async (e) => {
    e.preventDefault();
    setBusy(true);
    try {
      if (mode === 'single') {
        const res = await api('POST', '/api/suppressions', { email: email.trim() });
        toast(res.created ? `${res.email} suppressed.` : `${res.email} was already suppressed.`, 'success');
        onDone();
        return;
      }
      const data = new FormData();
      if (mode === 'file') {
        if (!file) { toast('Choose a CSV file first.', 'warn'); setBusy(false); return; }
        data.append('file', file);
      } else {
        data.append('text', text);
      }
      const res = await api('POST', '/api/suppressions/import', data);
      setResult(res);
      toast(`Suppressed ${plural(res.added, 'address', 'addresses')}.`, 'success');
      onDone(true);
    } catch (err) {
      toast(describeError(err), 'error');
    } finally {
      setBusy(false);
    }
  };

  return (
    <Modal title="Add suppressions" description="Suppressed addresses are never emailed and are skipped when importing contacts." onClose={onClose}>
      <form className="stack-sm" onSubmit={submit}>
        <Tabs value={mode} onChange={(m) => { setMode(m); setResult(null); }} options={[['single', 'Single address'], ['text', 'Paste list'], ['file', 'Upload CSV']]} />
        {mode === 'single' ? (
          <Field label="Email address"><input type="email" required autoFocus value={email} onChange={(e) => setEmail(e.target.value)} placeholder="someone@example.com" /></Field>
        ) : null}
        {mode === 'text' ? (
          <Field label="Addresses" hint="One per line, or comma separated with the address first.">
            <textarea className="code" rows={7} required value={text} onChange={(e) => setText(e.target.value)} placeholder={'old@example.com\nnoreply@example.org'} />
          </Field>
        ) : null}
        {mode === 'file' ? (
          <label className="dropzone">
            <input type="file" accept=".csv,text/csv,text/plain" onChange={(e) => setFile(e.target.files[0] || null)} />
            <strong>{file ? file.name : 'Choose a CSV file'}</strong>
            <span className="small muted">An &quot;email&quot; column, or addresses in the first column.</span>
          </label>
        ) : null}
        {result ? (
          <div className="result-grid">
            {[['Added', result.added], ['Already suppressed', result.skipped], ['Invalid', result.invalid]].map(([label, n]) => (
              <div key={label} className="result"><strong>{number(n)}</strong><span className="small muted">{label}</span></div>
            ))}
          </div>
        ) : null}
        <div className="actions actions-end">
          <button className="btn" type="button" onClick={onClose}>{result ? 'Done' : 'Cancel'}</button>
          <button className="btn btn-primary" type="submit" disabled={busy}>{busy ? 'Saving…' : 'Suppress'}</button>
        </div>
      </form>
    </Modal>
  );
}

export default function Suppressions() {
  const toast = useToast();
  const confirm = useConfirm();
  const [adding, setAdding] = useState(false);
  const [searchInput, setSearchInput] = useState('');
  const [search, setSearch] = useState('');
  const [page, setPage] = useState(0);

  useEffect(() => {
    const t = setTimeout(() => { setSearch(searchInput.trim()); setPage(0); }, 250);
    return () => clearTimeout(t);
  }, [searchInput]);

  const [state, reload] = useLoad(() => {
    const params = new URLSearchParams({ limit: pageSize, offset: page * pageSize });
    if (search) params.set('search', search);
    return api('GET', `/api/suppressions?${params}`);
  }, [search, page]);

  const remove = async (s) => {
    const warning = riskyRemoval[s.source];
    const message = `${warning ? `${warning} ` : ''}Remove ${s.email} from the suppression list so it can receive email again?`;
    if (!(await confirm(warning ? 'Remove a risky suppression?' : 'Remove suppression', message, 'Remove', Boolean(warning)))) return;
    try {
      await api('DELETE', `/api/suppressions/${s.id}`);
      toast(`${s.email} can receive email again.`, 'success');
      reload();
    } catch (err) {
      toast(describeError(err), 'error');
    }
  };

  const columns = useMemo(() => [
    { accessorKey: 'email', header: 'Email', cell: ({ getValue }) => <span className="cell-strong">{getValue()}</span> },
    { accessorKey: 'reason', header: 'Reason', cell: ({ getValue }) => <Badge value={getValue()} /> },
    { accessorKey: 'source', header: 'Source', cell: ({ getValue }) => sources[getValue()] || getValue() },
    { accessorKey: 'created_at', header: 'Date', cell: ({ getValue }) => formatDate(getValue()) },
    {
      id: 'actions', header: '', meta: { className: 'col-actions' },
      cell: ({ row }) => <button className="btn btn-ghost btn-sm btn-danger" type="button" onClick={() => remove(row.original)}>Remove</button>,
    },
  ], []);

  const addButton = <button className="btn btn-primary" type="button" onClick={() => setAdding(true)}><Icon name="plus" />Add suppressions</button>;
  return (
    <>
      <PageHead title="Suppressions" subtitle="Addresses that are never emailed: unsubscribes, hard bounces, spam complaints and your own blocklist.">{addButton}</PageHead>
      <div className="toolbar">
        <input type="search" className="toolbar-search" placeholder="Search by email" value={searchInput} onChange={(e) => setSearchInput(e.target.value)} />
      </div>
      <Card flush>
        <DataTable
          columns={columns}
          data={state.data ? state.data.suppressions : null}
          error={state.error}
          loading={state.loading}
          server={{ pageIndex: page, pageSize, total: state.data ? state.data.total : 0, onPageChange: setPage }}
          empty={{ title: search ? 'No matching addresses' : 'No suppressed addresses', hint: 'Unsubscribes, hard bounces and complaints appear here automatically.' }}
        />
      </Card>
      {adding ? <AddSuppressions onClose={() => setAdding(false)} onDone={(keepOpen) => { if (!keepOpen) setAdding(false); reload(); }} /> : null}
    </>
  );
}
