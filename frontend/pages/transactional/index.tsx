import { MousePointerClick, RefreshCw, Send, Timer, MailCheck, Zap, Eye } from 'lucide-react';
import { useRouter } from 'next/router';
import { FormEvent, useEffect, useMemo, useState } from 'react';
import DataTable, { Columns, Strong } from '../../components/DataTable';
import { FormActions, Modal } from '../../components/Modal';
import { Badge, Button, Card, Checkbox, Field, FormError, Input, PageHeader, Select, Stat, Textarea, Toolbar } from '../../components/ui';
import { api, describeError } from '../../lib/api';
import { formatDate, number, percent } from '../../lib/format';
import { useMe } from '../../lib/session';
import { useToast } from '../../lib/toast';
import { ms, outcome } from '../../lib/transactional';
import type { TransactionalMessage, TransactionalStats } from '../../lib/types';
import { useLoad } from '../../lib/useLoad';

const statuses = ['queued', 'sending', 'sent', 'failed'];

const resetTemplate = '<p>Hi,</p>\n<p>Someone asked to reset the password for your account. Use this code within 15 minutes:</p>\n<p style="font-size:24px;font-weight:600;letter-spacing:4px">482913</p>\n<p>If this wasn\'t you, ignore this email.</p>';

function SendTest({ onClose, onSent }: { onClose: () => void; onSent: (m: TransactionalMessage) => void }) {
  const me = useMe();
  const domain = me.domains[0] || '';
  const [form, setForm] = useState({
    to: me.user?.email || '',
    from_name: me.workspace.name,
    from_address: domain.includes('@') ? domain : domain ? `no-reply@${domain}` : '',
    subject: 'Your password reset code',
    html: resetTemplate,
    track_opens: false,
    track_clicks: false,
  });
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  const set = (key: keyof typeof form) => (e: React.ChangeEvent<HTMLInputElement | HTMLTextAreaElement>) => {
    const target = e.target as HTMLInputElement;
    setForm((f) => ({ ...f, [key]: target.type === 'checkbox' ? target.checked : target.value }));
  };
  const submit = async (e: FormEvent) => {
    e.preventDefault();
    setBusy(true);
    setError('');
    try {
      onSent(await api<TransactionalMessage>('POST', '/api/transactional', form));
    } catch (err) {
      setError(describeError(err));
      setBusy(false);
    }
  };
  return (
    <Modal title="Send a test message" description="Goes through the same priority path as API sends." onClose={onClose} wide>
      <form className="flex flex-col gap-4" onSubmit={submit}>
        <div className="grid gap-4 sm:grid-cols-2">
          <Field label="To"><Input type="email" required value={form.to} onChange={set('to')} /></Field>
          <Field label="From address" hint="Must use a verified domain or address."><Input type="email" required value={form.from_address} onChange={set('from_address')} /></Field>
          <Field label="From name"><Input value={form.from_name} onChange={set('from_name')} /></Field>
          <Field label="Subject"><Input required value={form.subject} onChange={set('subject')} /></Field>
        </div>
        <Field label="HTML"><Textarea code rows={7} value={form.html} onChange={set('html')} /></Field>
        <div className="flex flex-wrap gap-5">
          <Checkbox label="Track opens" checked={form.track_opens} onChange={set('track_opens')} />
          <Checkbox label="Track clicks" checked={form.track_clicks} onChange={set('track_clicks')} />
        </div>
        <p className="text-xs leading-relaxed text-muted">Leave tracking off for one-time codes and security alerts. Opens are estimates: some mail apps load images automatically or block them.</p>
        <FormError>{error}</FormError>
        <FormActions>
          <Button onClick={onClose}>Cancel</Button>
          <Button variant="primary" type="submit" icon={Send} loading={busy}>Send</Button>
        </FormActions>
      </form>
    </Modal>
  );
}

export default function Transactional() {
  const router = useRouter();
  const toast = useToast();
  const [status, setStatus] = useState('');
  const [sending, setSending] = useState(false);
  const [state, reload] = useLoad(() => {
    const params = new URLSearchParams({ limit: '200' });
    if (status) params.set('status', status);
    return Promise.all([
      api<{ messages: TransactionalMessage[] }>('GET', `/api/transactional?${params}`),
      api<TransactionalStats>('GET', '/api/transactional/stats'),
    ]);
  }, [status]);
  const [list, s] = state.data || [null, null];

  useEffect(() => {
    if (router.query.test) {
      setSending(true);
      router.replace('/transactional/', undefined, { shallow: true });
    }
  }, [router.query.test]);

  const columns = useMemo<Columns<TransactionalMessage>>(() => [
    { accessorKey: 'to', header: 'Message', cell: ({ row }) => <Strong sub={row.original.subject}>{row.original.to}</Strong> },
    { accessorKey: 'status', header: 'Status', cell: ({ getValue }) => <Badge value={getValue()} /> },
    { id: 'outcome', header: 'Outcome', accessorFn: outcome, cell: ({ getValue }) => (getValue() ? <Badge value={getValue()} /> : <span className="text-faint">—</span>) },
    { accessorKey: 'dispatch_ms', header: 'Dispatch', meta: { className: 'text-right' }, cell: ({ getValue }) => <span className="font-mono text-xs tabular-nums">{ms(getValue())}</span> },
    { accessorKey: 'created_at', header: 'Created', cell: ({ getValue }) => <span className="text-muted">{formatDate(getValue())}</span> },
  ], []);

  return (
    <>
      <PageHeader
        title="Transactional"
        description="One-to-one messages like sign-in codes, password resets and receipts. They always skip ahead of broadcasts."
        actions={<Button variant="primary" icon={Zap} onClick={() => setSending(true)}>Send test</Button>}
      />
      <div className="mb-5 grid grid-cols-2 gap-3 lg:grid-cols-4">
        <Stat icon={Timer} tone="accent" label="Dispatch p50" value={s ? ms(s.dispatch.p50_ms) : '—'} hint="Accepted → handed to SES, 24 h" />
        <Stat icon={Timer} tone="blue" label="Dispatch p95" value={s ? ms(s.dispatch.p95_ms) : '—'} hint={s ? `p99 ${ms(s.dispatch.p99_ms)}` : ' '} />
        <Stat icon={Send} tone="neutral" label="Sent, last 24 h" value={s ? number(s.sent) : '—'} hint={s ? `${number(s.failed)} failed · ${number(s.queued + s.sending)} in flight` : ' '} />
        <Stat icon={MailCheck} tone="green" label="Delivered" value={s ? percent(s.delivered, s.sent) : '—'} hint={s ? `${number(s.bounced)} bounced · ${number(s.complained)} complaints` : ' '} />
      </div>
      {s && (s.tracked_opens || s.tracked_clicks) ? (
        <div className="mb-5 grid gap-3 sm:grid-cols-2">
          <Stat icon={Eye} tone="accent" label="Opened (estimate)" value={percent(s.opened, s.tracked_opens)} hint={`of ${number(s.tracked_opens)} tracked messages`} />
          <Stat icon={MousePointerClick} tone="accent" label="Clicked" value={percent(s.clicked, s.tracked_clicks)} hint={`of ${number(s.tracked_clicks)} tracked messages; link scanners ignored`} />
        </div>
      ) : null}
      <Card flush>
        <Toolbar>
          <Select value={status} onChange={(e) => setStatus(e.target.value)} aria-label="Status" className="sm:w-44">
            <option value="">All statuses</option>
            {statuses.map((v) => <option key={v} value={v}>{v[0].toUpperCase() + v.slice(1)}</option>)}
          </Select>
          <Button className="sm:ml-auto" icon={RefreshCw} loading={state.loading} onClick={reload}>Refresh</Button>
        </Toolbar>
        <DataTable
          columns={columns}
          data={list ? list.messages : null}
          error={state.error}
          pageSize={25}
          noun="messages"
          onRowClick={(m) => router.push(`/transactional/view/?id=${m.id}`)}
          empty={{ title: 'No transactional messages yet', hint: 'Send one with POST /api/transactional or the Send test button.' }}
        />
      </Card>
      {sending ? (
        <SendTest
          onClose={() => setSending(false)}
          onSent={(m) => {
            setSending(false);
            toast(m.replayed ? 'Duplicate request: showing the original message.' : 'Message queued.', 'success');
            router.push(`/transactional/view/?id=${m.id}`);
          }}
        />
      ) : null}
    </>
  );
}
