import { useRouter } from 'next/router';
import { useMemo, useState } from 'react';
import DataTable from '../../components/DataTable';
import { Modal } from '../../components/Modal';
import { Badge, Card, Field, PageHead, Stat } from '../../components/ui';
import { api, describeError } from '../../lib/api';
import { formatDate, number } from '../../lib/format';
import { useLoad } from '../../lib/useLoad';
import { useSession } from '../../lib/session';
import { useToast } from '../../lib/toast';
import { ms, outcome } from '../../lib/transactional';

const statuses = ['queued', 'sending', 'sent', 'failed'];

const resetTemplate = '<p>Hi,</p>\n<p>Someone asked to reset the password for your account. Use this code within 15 minutes:</p>\n<p style="font-size:24px;font-weight:600;letter-spacing:4px">482913</p>\n<p>If this wasn\'t you, ignore this email.</p>';

function percent(part, whole) {
  if (!whole) return '—';
  return `${Math.round((part / whole) * 100)}%`;
}

function SendTest({ onClose, onSent }) {
  const { me } = useSession();
  const domain = me.domains[0] || '';
  const [form, setForm] = useState({
    to: me.user.email,
    from_name: me.workspace.name,
    from_address: domain.includes('@') ? domain : domain ? `no-reply@${domain}` : '',
    subject: 'Your password reset code',
    html: resetTemplate,
    track_opens: false,
    track_clicks: false,
  });
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  const set = (key) => (e) => setForm((f) => ({ ...f, [key]: e.target.type === 'checkbox' ? e.target.checked : e.target.value }));
  const submit = async (e) => {
    e.preventDefault();
    setBusy(true);
    setError('');
    try {
      onSent(await api('POST', '/api/transactional', form));
    } catch (err) {
      setError(describeError(err));
      setBusy(false);
    }
  };
  return (
    <Modal title="Send a test message" description="Goes through the same priority path as API sends." onClose={onClose} wide>
      <form className="stack-sm" onSubmit={submit}>
        <div className="grid grid-2">
          <Field label="To"><input type="email" required value={form.to} onChange={set('to')} /></Field>
          <Field label="From address" hint="Must use a verified domain or address."><input type="email" required value={form.from_address} onChange={set('from_address')} /></Field>
        </div>
        <div className="grid grid-2">
          <Field label="From name"><input type="text" value={form.from_name} onChange={set('from_name')} /></Field>
          <Field label="Subject"><input type="text" required value={form.subject} onChange={set('subject')} /></Field>
        </div>
        <Field label="HTML"><textarea className="code" rows={7} value={form.html} onChange={set('html')} /></Field>
        <div className="check-row">
          <label><input type="checkbox" className="checkbox" checked={form.track_opens} onChange={set('track_opens')} /> Track opens</label>
          <label><input type="checkbox" className="checkbox" checked={form.track_clicks} onChange={set('track_clicks')} /> Track clicks</label>
        </div>
        <p className="small muted">Leave tracking off for one-time codes and security alerts. Opens are estimates: some mail apps load images automatically or block them.</p>
        {error ? <p className="form-error">{error}</p> : null}
        <div className="actions actions-end">
          <button className="btn" type="button" onClick={onClose}>Cancel</button>
          <button className="btn btn-primary" type="submit" disabled={busy}>{busy ? 'Sending…' : 'Send'}</button>
        </div>
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
    const params = new URLSearchParams({ limit: 200 });
    if (status) params.set('status', status);
    return Promise.all([api('GET', `/api/transactional?${params}`), api('GET', '/api/transactional/stats')]);
  }, [status]);
  const [list, s] = state.data || [null, null];

  const columns = useMemo(() => [
    { accessorKey: 'to', header: 'To', cell: ({ row }) => <div><div className="cell-strong">{row.original.to}</div><div className="small muted truncate">{row.original.subject}</div></div> },
    { accessorKey: 'status', header: 'Status', cell: ({ getValue }) => <Badge value={getValue()} /> },
    { id: 'outcome', header: 'Outcome', accessorFn: outcome, cell: ({ getValue }) => (getValue() ? <Badge value={getValue()} /> : '—') },
    { accessorKey: 'dispatch_ms', header: 'Dispatch', meta: { className: 'num' }, cell: ({ getValue }) => ms(getValue()) },
    { accessorKey: 'created_at', header: 'Created', cell: ({ getValue }) => formatDate(getValue()) },
  ], []);

  return (
    <>
      <PageHead title="Transactional" subtitle="One-to-one messages like password resets, sign-in codes and receipts. They skip ahead of broadcasts.">
        <select value={status} onChange={(e) => setStatus(e.target.value)} aria-label="Status">
          <option value="">All statuses</option>
          {statuses.map((v) => <option key={v} value={v}>{v[0].toUpperCase() + v.slice(1)}</option>)}
        </select>
        <button className="btn" type="button" onClick={reload}>Refresh</button>
        <button className="btn btn-primary" type="button" onClick={() => setSending(true)}>Send test</button>
      </PageHead>
      <div className="grid grid-4">
        <Stat label="Dispatch p50" value={s ? ms(s.dispatch.p50_ms) : '—'} hint="Accepted to handed to SES, last 24 h" />
        <Stat label="Dispatch p95" value={s ? ms(s.dispatch.p95_ms) : '—'} hint={s ? `p99 ${ms(s.dispatch.p99_ms)}` : null} />
        <Stat label="Sent, last 24 h" value={s ? number(s.sent) : '—'} hint={s ? `${number(s.failed)} failed · ${number(s.queued + s.sending)} in flight` : null} />
        <Stat label="Delivered" value={s ? percent(s.delivered, s.sent) : '—'} hint={s ? `${number(s.bounced)} bounced · ${number(s.complained)} complaints` : null} />
      </div>
      {s && (s.tracked_opens || s.tracked_clicks) ? (
        <div className="grid grid-2">
          <Stat label="Opened (estimate)" value={percent(s.opened, s.tracked_opens)} hint={`of ${number(s.tracked_opens)} tracked messages. Privacy features hide some opens and fake others.`} />
          <Stat label="Clicked" value={percent(s.clicked, s.tracked_clicks)} hint={`of ${number(s.tracked_clicks)} tracked messages. Known link scanners are ignored.`} />
        </div>
      ) : null}
      <Card flush>
        <DataTable
          columns={columns}
          data={list ? list.messages : null}
          error={state.error}
          pageSize={25}
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
