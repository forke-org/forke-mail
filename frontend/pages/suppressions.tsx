import { Plus, ShieldOff } from 'lucide-react';
import { FormEvent, useEffect, useMemo, useState } from 'react';
import DataTable, { Columns } from '../components/DataTable';
import FileDrop from '../components/FileDrop';
import { FormActions, Modal, useConfirm } from '../components/Modal';
import ResultGrid from '../components/ResultGrid';
import { Badge, Button, Card, Field, Input, PageHeader, SearchInput, Tabs, Textarea, Toolbar } from '../components/ui';
import { api, describeError } from '../lib/api';
import { formatDate, plural } from '../lib/format';
import { useSession } from '../lib/session';
import { useToast } from '../lib/toast';
import type { Suppression } from '../lib/types';
import { useLoad } from '../lib/useLoad';

const pageSize = 25;

const sources: Record<string, string> = {
  unsubscribe_link: 'Unsubscribe link',
  preference_center: 'Preference centre',
  dormancy: 'Inactive, no reply',
  ses_bounce: 'Hard bounce',
  ses_complaint: 'Spam complaint',
  manual: 'Added manually',
  import: 'Imported',
};

const riskyRemoval: Record<string, string> = {
  ses_bounce: 'This address hard bounced. Sending to it again will most likely bounce and hurts your bounce rate, which can pause sending for your whole workspace. Amazon SES may also still block it through its account-level suppression list.',
  ses_complaint: 'This recipient marked your email as spam. Emailing them again risks another complaint, which can pause sending for your whole workspace.',
};

type Mode = 'single' | 'text' | 'file';

function AddSuppressions({ onClose, onDone }: { onClose: () => void; onDone: (keepOpen?: boolean) => void }) {
  const toast = useToast();
  const [mode, setMode] = useState<Mode>('single');
  const [email, setEmail] = useState('');
  const [text, setText] = useState('');
  const [file, setFile] = useState<File | null>(null);
  const [busy, setBusy] = useState(false);
  const [result, setResult] = useState<{ added: number; skipped: number; invalid: number } | null>(null);

  const submit = async (e: FormEvent) => {
    e.preventDefault();
    setBusy(true);
    try {
      if (mode === 'single') {
        const res = await api<{ email: string; created: boolean }>('POST', '/api/suppressions', { email: email.trim() });
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
      const res = await api<{ added: number; skipped: number; invalid: number }>('POST', '/api/suppressions/import', data);
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
      <form className="flex flex-col gap-4" onSubmit={submit}>
        <Tabs value={mode} onChange={(m) => { setMode(m); setResult(null); }} options={[['single', 'Single address'], ['text', 'Paste list'], ['file', 'Upload CSV']]} />
        {mode === 'single' ? (
          <Field label="Email address"><Input type="email" required autoFocus value={email} onChange={(e) => setEmail(e.target.value)} placeholder="someone@example.com" /></Field>
        ) : null}
        {mode === 'text' ? (
          <Field label="Addresses" hint="One per line, or comma separated with the address first.">
            <Textarea code rows={7} required value={text} onChange={(e) => setText(e.target.value)} placeholder={'old@example.com\nnoreply@example.org'} />
          </Field>
        ) : null}
        {mode === 'file' ? <FileDrop file={file} onFile={setFile} hint='An "email" column, or addresses in the first column.' /> : null}
        {result ? <ResultGrid items={[['Added', result.added], ['Already suppressed', result.skipped], ['Invalid', result.invalid]]} /> : null}
        <FormActions>
          <Button onClick={onClose}>{result ? 'Done' : 'Cancel'}</Button>
          <Button variant="primary" type="submit" loading={busy}>Suppress</Button>
        </FormActions>
      </form>
    </Modal>
  );
}

export default function Suppressions() {
  const toast = useToast();
  const confirm = useConfirm();
  const { canManage } = useSession();
  const [adding, setAdding] = useState(false);
  const [searchInput, setSearchInput] = useState('');
  const [search, setSearch] = useState('');
  const [page, setPage] = useState(0);

  useEffect(() => {
    const t = setTimeout(() => { setSearch(searchInput.trim()); setPage(0); }, 250);
    return () => clearTimeout(t);
  }, [searchInput]);

  const [state, reload] = useLoad(() => {
    const params = new URLSearchParams({ limit: String(pageSize), offset: String(page * pageSize) });
    if (search) params.set('search', search);
    return api<{ suppressions: Suppression[]; total: number }>('GET', `/api/suppressions?${params}`);
  }, [search, page]);

  const remove = async (s: Suppression) => {
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

  const columns = useMemo<Columns<Suppression>>(() => [
    { accessorKey: 'email', header: 'Email', cell: ({ getValue }) => <span className="font-medium text-ink">{getValue()}</span> },
    { accessorKey: 'reason', header: 'Reason', cell: ({ getValue }) => <Badge value={getValue()} /> },
    { accessorKey: 'source', header: 'Source', cell: ({ getValue }) => <span className="text-ink-2">{sources[getValue()] || getValue()}</span> },
    { accessorKey: 'created_at', header: 'Date', cell: ({ getValue }) => <span className="text-muted">{formatDate(getValue())}</span> },
    ...(canManage ? [{
      id: 'actions', header: '', meta: { className: 'w-24 text-right' },
      cell: ({ row }: { row: { original: Suppression } }) => <Button size="xs" variant="danger" icon={ShieldOff} onClick={() => remove(row.original)}>Remove</Button>,
    }] : []),
  ], [canManage]);

  return (
    <>
      <PageHeader
        title="Suppressions"
        description="Addresses that are never emailed: unsubscribes, hard bounces, spam complaints and your own blocklist."
        actions={<Button variant="primary" icon={Plus} onClick={() => setAdding(true)}>Add suppressions</Button>}
      />
      <Card flush>
        <Toolbar><SearchInput className="flex-1" placeholder="Search by email" value={searchInput} onChange={setSearchInput} /></Toolbar>
        <DataTable
          columns={columns}
          data={state.data ? state.data.suppressions : null}
          error={state.error}
          loading={state.loading}
          noun="addresses"
          server={{ pageIndex: page, pageSize, total: state.data ? state.data.total : 0, onPageChange: setPage }}
          empty={{ title: search ? 'No matching addresses' : 'No suppressed addresses', hint: 'Unsubscribes, hard bounces and complaints appear here automatically.' }}
        />
      </Card>
      {adding ? <AddSuppressions onClose={() => setAdding(false)} onDone={(keepOpen) => { if (!keepOpen) setAdding(false); reload(); }} /> : null}
    </>
  );
}
