import { KeyRound, Plus, RotateCw, Trash2 } from 'lucide-react';
import { useRouter } from 'next/router';
import { FormEvent, useEffect, useMemo, useState } from 'react';
import DataTable, { Columns, Strong } from '../components/DataTable';
import { FormActions, Modal, useConfirm } from '../components/Modal';
import { Badge, Button, Callout, Card, Empty, Field, FormError, Input, PageHeader } from '../components/ui';
import clsx from 'clsx';
import { api, describeError } from '../lib/api';
import { formatDate, timeAgo } from '../lib/format';
import { useSession } from '../lib/session';
import { useToast } from '../lib/toast';
import type { ApiKey } from '../lib/types';
import { useLoad } from '../lib/useLoad';

function KeyReveal({ plain, onClose }: { plain: string; onClose: () => void }) {
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
    <Modal title="Your new API key" onClose={onClose} footer={<Button variant="primary" onClick={onClose}>Done</Button>}>
      <Callout tone="warn">Copy this key now. For your security it won&apos;t be shown again.</Callout>
      <div className="flex gap-2">
        <Input readOnly value={plain} spellCheck={false} autoFocus onFocus={(e) => e.target.select()} className="font-mono text-[13px]" />
        <Button onClick={copy}>{copied ? 'Copied' : 'Copy'}</Button>
      </div>
      <p className="text-xs text-muted">Use it as <code className="rounded bg-surface-3 px-1 font-mono">Authorization: Bearer {plain.slice(0, 12)}…</code></p>
    </Modal>
  );
}

function ScopeOption({ checked, disabled, onChange, title, text }: { checked: boolean; disabled?: boolean; onChange: () => void; title: string; text: string }) {
  return (
    <label className={clsx(
      'flex cursor-pointer items-start gap-3 rounded-xl p-3.5 transition-colors',
      checked ? 'bg-accent-soft shadow-[inset_0_0_0_1px_var(--accent-line)]' : 'shadow-[inset_0_0_0_1px_var(--line-strong)] hover:bg-surface-2',
      disabled && 'pointer-events-none opacity-50',
    )}>
      <input type="radio" name="scope" className="mt-0.5 accent-accent" checked={checked} disabled={disabled} onChange={onChange} />
      <div><div className="text-sm font-medium text-ink">{title}</div><p className="mt-0.5 text-xs text-muted">{text}</p></div>
    </label>
  );
}

function KeyForm({ mode, keyRow, trial, onClose, onCreated }: { mode: 'create' | 'rotate'; keyRow?: ApiKey; trial: boolean; onClose: () => void; onCreated: (k: { key: string }) => void }) {
  const [name, setName] = useState('');
  const [scope, setScope] = useState('sending_access');
  const [password, setPassword] = useState('');
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  const submit = async (e: FormEvent) => {
    e.preventDefault();
    setBusy(true);
    setError('');
    try {
      const created = mode === 'rotate' && keyRow
        ? await api<{ key: string }>('POST', `/api/keys/${keyRow.id}/rotate`, { password })
        : await api<{ key: string }>('POST', '/api/keys', { name: name.trim(), scope, password });
      onCreated(created);
    } catch (err) {
      setError(describeError(err));
      setBusy(false);
    }
  };
  return (
    <Modal
      title={mode === 'rotate' ? `Rotate “${keyRow?.name}”` : 'Create API key'}
      description={mode === 'rotate' ? 'A new key is created with the same permission and the old key stops working immediately.' : undefined}
      onClose={onClose}
    >
      <form className="flex flex-col gap-4" onSubmit={submit}>
        {mode === 'create' ? (
          <>
            <Field label="Name"><Input required autoFocus maxLength={100} placeholder="forke-web production" value={name} onChange={(e) => setName(e.target.value)} /></Field>
            <div className="flex flex-col gap-2">
              <span className="text-[13px] font-medium text-ink-2">Permission</span>
              <ScopeOption checked={scope === 'sending_access'} onChange={() => setScope('sending_access')} title="Sending access" text="Send transactional email, import contacts, create and send broadcasts, read reports." />
              <ScopeOption checked={scope === 'full_access'} disabled={trial} onChange={() => setScope('full_access')} title="Full access" text={trial ? 'Available on paid plans.' : 'Everything the API offers, including domains, webhooks and suppressions.'} />
            </div>
          </>
        ) : null}
        <Field label="Confirm your password"><Input type="password" required autoComplete="current-password" autoFocus={mode === 'rotate'} value={password} onChange={(e) => setPassword(e.target.value)} /></Field>
        <FormError>{error}</FormError>
        <FormActions>
          <Button onClick={onClose}>Cancel</Button>
          <Button variant="primary" type="submit" loading={busy}>{mode === 'rotate' ? 'Rotate key' : 'Create key'}</Button>
        </FormActions>
      </form>
    </Modal>
  );
}

export default function Keys() {
  const router = useRouter();
  const { trial, superAdmin } = useSession();
  const toast = useToast();
  const confirm = useConfirm();
  const [form, setForm] = useState<{ mode: 'create' | 'rotate'; keyRow?: ApiKey } | null>(null);
  const [revealed, setRevealed] = useState<string | null>(null);
  const [state, reload] = useLoad(() => (superAdmin ? api<{ keys: ApiKey[] }>('GET', '/api/keys') : Promise.resolve({ keys: [] })), [superAdmin]);

  useEffect(() => {
    if (router.query.new && superAdmin) {
      setForm({ mode: 'create' });
      router.replace('/keys/', undefined, { shallow: true });
    }
  }, [router.query.new, superAdmin]);

  const revoke = async (k: ApiKey) => {
    if (!(await confirm('Revoke API key', `Revoke "${k.name}" (${k.prefix}…)? Requests using it will fail immediately.`, 'Revoke', true))) return;
    try {
      await api('DELETE', `/api/keys/${k.id}`);
      toast('Key revoked.', 'success');
      reload();
    } catch (err) {
      toast(describeError(err), 'error');
    }
  };

  const columns = useMemo<Columns<ApiKey>>(() => [
    {
      accessorKey: 'name', header: 'Name',
      cell: ({ row }) => (
        <div className="flex items-center gap-3">
          <span className="grid size-8 place-items-center rounded-lg bg-surface-3 text-ink-2"><KeyRound className="size-4" /></span>
          <Strong sub={<code className="font-mono">{row.original.prefix}…</code>}>{row.original.name}</Strong>
        </div>
      ),
    },
    { accessorKey: 'scope', header: 'Permission', cell: ({ getValue }) => <Badge value={getValue()} /> },
    { accessorKey: 'last_used_at', header: 'Last used', cell: ({ getValue }) => <span className="text-muted">{timeAgo(getValue())}</span> },
    { accessorKey: 'created_at', header: 'Created', cell: ({ getValue }) => <span className="text-muted">{formatDate(getValue())}</span> },
    { id: 'status', header: 'Status', accessorFn: (k) => (k.revoked_at ? 'revoked' : 'active'), cell: ({ getValue }) => <Badge value={getValue()} /> },
    {
      id: 'actions', header: '', meta: { className: 'text-right' },
      cell: ({ row }) => (row.original.revoked_at ? null : (
        <div className="flex justify-end gap-1">
          <Button size="xs" variant="ghost" icon={RotateCw} onClick={() => setForm({ mode: 'rotate', keyRow: row.original })}>Rotate</Button>
          <Button size="xs" variant="danger" icon={Trash2} onClick={() => revoke(row.original)}>Revoke</Button>
        </div>
      )),
    },
  ], []);

  if (!superAdmin) return <Card><Empty icon={KeyRound} title="Super admins only" hint="Ask a Forke super admin to create or rotate API keys." /></Card>;

  const createButton = <Button variant="primary" icon={Plus} onClick={() => setForm({ mode: 'create' })}>Create API key</Button>;
  return (
    <>
      <PageHeader title="API keys" description="Send from Forke apps through the REST or Resend-compatible API. Keys are shown once, then only their prefix." actions={createButton} />
      <Card flush>
        <DataTable columns={columns} data={state.data ? state.data.keys : null} error={state.error} noun="keys" empty={{ title: 'No API keys yet', hint: 'Create a key to start using the API.', action: createButton }} />
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
