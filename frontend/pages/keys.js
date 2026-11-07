import { useMemo, useState } from 'react';
import DataTable from '../components/DataTable';
import Icon from '../components/Icon';
import { Modal, useConfirm } from '../components/Modal';
import { Badge, Card, Field, PageHead } from '../components/ui';
import { api, describeError } from '../lib/api';
import { formatDate, timeAgo } from '../lib/format';
import { useLoad } from '../lib/useLoad';
import { useSession } from '../lib/session';
import { useToast } from '../lib/toast';

function KeyReveal({ plain, onClose }) {
  const toast = useToast();
  const [copied, setCopied] = useState(false);
  const copy = async () => {
    try {
      await navigator.clipboard.writeText(plain);
      setCopied(true);
    } catch {
      toast('Select the key and copy it manually.', 'warn');
    }
  };
  return (
    <Modal title="Your new API key" onClose={onClose} footer={<button className="btn btn-primary" type="button" onClick={onClose}>Done</button>}>
      <div className="callout">Copy this key now. For your security it won&apos;t be shown again.</div>
      <div className="key-reveal">
        <input type="text" value={plain} readOnly spellCheck="false" autoFocus onFocus={(e) => e.target.select()} />
        <button className="btn" type="button" onClick={copy}>{copied ? 'Copied' : 'Copy'}</button>
      </div>
      <p className="small muted">Use it as <code>Authorization: Bearer {plain.slice(0, 12)}…</code></p>
    </Modal>
  );
}

function KeyForm({ mode, keyRow, trial, onClose, onCreated }) {
  const [name, setName] = useState('');
  const [scope, setScope] = useState('sending_access');
  const [password, setPassword] = useState('');
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  const submit = async (e) => {
    e.preventDefault();
    setBusy(true);
    setError('');
    try {
      const created = mode === 'rotate'
        ? await api('POST', `/api/keys/${keyRow.id}/rotate`, { password })
        : await api('POST', '/api/keys', { name: name.trim(), scope, password });
      onCreated(created);
    } catch (err) {
      setError(describeError(err));
      setBusy(false);
    }
  };
  return (
    <Modal
      title={mode === 'rotate' ? `Rotate "${keyRow.name}"` : 'Create API key'}
      description={mode === 'rotate' ? 'A new key is created with the same permission and the old key stops working immediately.' : null}
      onClose={onClose}
    >
      <form className="stack-sm" onSubmit={submit}>
        {mode === 'create' ? (
          <>
            <Field label="Name"><input type="text" required autoFocus maxLength={100} placeholder="Production server" value={name} onChange={(e) => setName(e.target.value)} /></Field>
            <div className="field">
              <span>Permission</span>
              <label className="radio-card">
                <input type="radio" name="scope" checked={scope === 'sending_access'} onChange={() => setScope('sending_access')} />
                <div><strong>Sending access</strong><p className="small muted">Import contacts, create and send emails, read reports.</p></div>
              </label>
              <label className={`radio-card${trial ? ' disabled' : ''}`}>
                <input type="radio" name="scope" disabled={trial} checked={scope === 'full_access'} onChange={() => setScope('full_access')} />
                <div><strong>Full access</strong><p className="small muted">{trial ? 'Available on paid plans.' : 'Everything the API offers, including contacts and campaigns.'}</p></div>
              </label>
            </div>
          </>
        ) : null}
        <Field label="Confirm your password"><input type="password" required autoComplete="current-password" autoFocus={mode === 'rotate'} value={password} onChange={(e) => setPassword(e.target.value)} /></Field>
        {error ? <p className="form-error">{error}</p> : null}
        <div className="actions actions-end">
          <button className="btn" type="button" onClick={onClose}>Cancel</button>
          <button className="btn btn-primary" type="submit" disabled={busy}>{mode === 'rotate' ? 'Rotate key' : 'Create key'}</button>
        </div>
      </form>
    </Modal>
  );
}

export default function Keys() {
  const { trial } = useSession();
  const toast = useToast();
  const confirm = useConfirm();
  const [form, setForm] = useState(null);
  const [revealed, setRevealed] = useState(null);
  const [state, reload] = useLoad(() => api('GET', '/api/keys'), []);

  const revoke = async (k) => {
    if (!(await confirm('Revoke API key', `Revoke "${k.name}" (${k.prefix}…)? Requests using it will fail immediately.`, 'Revoke', true))) return;
    try {
      await api('DELETE', `/api/keys/${k.id}`);
      toast('Key revoked.', 'success');
      reload();
    } catch (err) {
      toast(describeError(err), 'error');
    }
  };

  const columns = useMemo(() => [
    { accessorKey: 'name', header: 'Name', cell: ({ getValue }) => <span className="cell-strong">{getValue()}</span> },
    { accessorKey: 'prefix', header: 'Key', cell: ({ getValue }) => <code>{getValue()}…</code> },
    { accessorKey: 'scope', header: 'Permission', cell: ({ getValue }) => <Badge value={getValue()} /> },
    { accessorKey: 'last_used_at', header: 'Last used', cell: ({ getValue }) => timeAgo(getValue()) },
    { accessorKey: 'created_at', header: 'Created', cell: ({ getValue }) => formatDate(getValue()) },
    { id: 'status', header: 'Status', accessorFn: (k) => (k.revoked_at ? 'revoked' : 'active'), cell: ({ getValue }) => <Badge value={getValue()} /> },
    {
      id: 'actions', header: '', meta: { className: 'col-actions' },
      cell: ({ row }) => (row.original.revoked_at ? null : (
        <div className="actions actions-end">
          <button className="btn btn-ghost btn-sm" type="button" onClick={() => setForm({ mode: 'rotate', keyRow: row.original })}>Rotate</button>
          <button className="btn btn-ghost btn-sm btn-danger" type="button" onClick={() => revoke(row.original)}>Revoke</button>
        </div>
      )),
    },
  ], []);

  const createButton = <button className="btn btn-primary" type="button" onClick={() => setForm({ mode: 'create' })}><Icon name="plus" />Create API key</button>;
  return (
    <>
      <PageHead title="API Keys" subtitle="Use API keys to import contacts and send emails from your own code.">{createButton}</PageHead>
      <Card flush>
        <DataTable columns={columns} data={state.data ? state.data.keys : null} error={state.error} empty={{ title: 'No API keys yet', hint: 'Create a key to start using the API.', action: createButton }} />
      </Card>
      {form ? (
        <KeyForm
          mode={form.mode}
          keyRow={form.keyRow}
          trial={trial}
          onClose={() => setForm(null)}
          onCreated={(created) => { setForm(null); setRevealed(created.key); reload(); }}
        />
      ) : null}
      {revealed ? <KeyReveal plain={revealed} onClose={() => setRevealed(null)} /> : null}
    </>
  );
}
