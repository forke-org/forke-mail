import clsx from 'clsx';
import { Plus, RotateCw, Trash2, Webhook as WebhookIcon } from 'lucide-react';
import { FormEvent, useMemo, useState } from 'react';
import DataTable, { Columns, Strong } from '../components/DataTable';
import { FormActions, Modal, useConfirm } from '../components/Modal';
import { Badge, Button, Callout, Card, Chip, Field, FormError, Input, PageHeader } from '../components/ui';
import { api, describeError } from '../lib/api';
import { formatDate, timeAgo } from '../lib/format';
import { useSession } from '../lib/session';
import { useToast } from '../lib/toast';
import type { Webhook, WebhookDelivery } from '../lib/types';
import { never, useLoad } from '../lib/useLoad';

const eventGroups: [string, string[]][] = [
  ['Contacts', ['contact.updated', 'contact.deleted']],
  ['Delivery', ['email.sent', 'email.delivered', 'email.delayed', 'email.bounced', 'email.soft_bounced', 'email.complained', 'email.rejected', 'email.failed']],
  ['Engagement', ['email.opened', 'email.clicked', 'email.unsubscribed']],
  ['Other', ['email.queued', 'workspace.sending_paused']],
];

function SecretReveal({ secret, onClose }: { secret: string; onClose: () => void }) {
  const toast = useToast();
  const [copied, setCopied] = useState(false);
  const copy = async () => {
    try {
      await navigator.clipboard.writeText(secret);
      setCopied(true);
    } catch {
      toast('Select the secret and copy it manually.', 'warn');
    }
  };
  return (
    <Modal title="Signing secret" onClose={onClose} footer={<Button variant="primary" onClick={onClose}>Done</Button>}>
      <Callout tone="warn">Copy this secret now. It won&apos;t be shown again.</Callout>
      <div className="flex gap-2">
        <Input readOnly value={secret} spellCheck={false} autoFocus onFocus={(e) => e.target.select()} className="font-mono text-[13px]" />
        <Button onClick={copy}>{copied ? 'Copied' : 'Copy'}</Button>
      </div>
      <p className="text-xs leading-relaxed text-muted">
        Store it as a secret in the receiving app and use it to verify every request. Requests are signed as <code className="rounded bg-surface-3 px-1 font-mono">v1=HMAC-SHA256(id.timestamp.body)</code> in the <code className="rounded bg-surface-3 px-1 font-mono">Webhook-Signature</code> header.
      </p>
    </Modal>
  );
}

function CreateWebhook({ onClose, onCreated }: { onClose: () => void; onCreated: (w: Webhook & { secret: string }) => void }) {
  const [url, setUrl] = useState('');
  const [events, setEvents] = useState<string[]>([]);
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  const toggle = (e: string) => setEvents((list) => (list.includes(e) ? list.filter((x) => x !== e) : [...list, e]));
  const submit = async (e: FormEvent) => {
    e.preventDefault();
    setBusy(true);
    setError('');
    try {
      onCreated(await api<Webhook & { secret: string }>('POST', '/api/webhooks', { url: url.trim(), events }));
    } catch (err) {
      setError(describeError(err));
      setBusy(false);
    }
  };
  return (
    <Modal wide title="Add webhook" description="We POST signed JSON to this URL whenever one of the chosen events happens." onClose={onClose}>
      <form className="flex flex-col gap-5" onSubmit={submit}>
        <Field label="Endpoint URL" hint="Public https only.">
          <Input type="url" required autoFocus placeholder="https://example.com/webhooks/email" value={url} onChange={(e) => setUrl(e.target.value)} />
        </Field>
        <div className="grid gap-4 sm:grid-cols-2">
          {eventGroups.map(([group, list]) => (
            <div key={group} className="flex flex-col gap-2">
              <span className="text-[13px] font-medium text-ink-2">{group}</span>
              {list.map((ev) => (
                <label key={ev} className="flex cursor-pointer items-center gap-2 font-mono text-[12.5px] text-ink-2">
                  <input type="checkbox" className="size-4 accent-accent" checked={events.includes(ev)} onChange={() => toggle(ev)} />
                  {ev}
                </label>
              ))}
            </div>
          ))}
        </div>
        <FormError>{error}</FormError>
        <FormActions>
          <Button onClick={onClose}>Cancel</Button>
          <Button variant="primary" type="submit" loading={busy} disabled={!events.length}>Add webhook</Button>
        </FormActions>
      </form>
    </Modal>
  );
}

function Deliveries({ webhook }: { webhook: Webhook }) {
  const [state, reload] = useLoad(() => (webhook ? api<{ deliveries: WebhookDelivery[] }>('GET', `/api/webhooks/${webhook.id}/deliveries?limit=50`) : never<{ deliveries: WebhookDelivery[] }>()), [webhook.id]);
  const columns = useMemo<Columns<WebhookDelivery>>(() => [
    { accessorKey: 'status', header: 'Status', cell: ({ getValue }) => <Badge value={getValue()} tone={getValue() === 'delivered' ? 'green' : getValue() === 'failed' ? 'red' : 'amber'} /> },
    { accessorKey: 'response_code', header: 'Response', cell: ({ getValue }) => <span className="font-mono text-xs">{getValue() ?? '—'}</span> },
    { accessorKey: 'attempts', header: 'Attempts', meta: { className: 'text-right' }, cell: ({ getValue }) => <span className="tabular-nums">{getValue()}</span> },
    { accessorKey: 'last_error', header: 'Error', cell: ({ getValue }) => (getValue() ? <span className="block max-w-xs truncate text-xs text-muted">{getValue()}</span> : <span className="text-faint">—</span>) },
    { accessorKey: 'created_at', header: 'Created', cell: ({ getValue }) => <span className="text-muted" title={formatDate(getValue())}>{timeAgo(getValue())}</span> },
  ], []);
  return (
    <Card title="Recent deliveries" description={webhook.url} flush actions={<Button size="sm" icon={RotateCw} loading={state.loading} onClick={reload}>Refresh</Button>}>
      <DataTable columns={columns} data={state.data ? state.data.deliveries : null} error={state.error} noun="deliveries" empty={{ title: 'No deliveries yet', hint: 'Deliveries appear here when a subscribed event happens.' }} />
    </Card>
  );
}

export default function Webhooks() {
  const toast = useToast();
  const confirm = useConfirm();
  const { canManage } = useSession();
  const [creating, setCreating] = useState(false);
  const [secret, setSecret] = useState<string | null>(null);
  const [openId, setOpenId] = useState<string | null>(null);
  const [state, reload] = useLoad(() => api<{ webhooks: Webhook[] }>('GET', '/api/webhooks'), []);
  const hooks = state.data ? state.data.webhooks : null;
  const open = hooks?.find((h) => h.id === openId);

  const toggle = async (h: Webhook) => {
    try {
      await api('PATCH', `/api/webhooks/${h.id}`, { enabled: !h.enabled });
      toast(h.enabled ? 'Webhook disabled.' : 'Webhook enabled.', 'success');
      reload();
    } catch (err) {
      toast(describeError(err), 'error');
    }
  };

  const rotate = async (h: Webhook) => {
    if (!(await confirm('Rotate signing secret', `Create a new secret for ${h.url}? The old one stops working immediately, so update the receiver right away.`, 'Rotate'))) return;
    try {
      const res = await api<{ secret: string }>('POST', `/api/webhooks/${h.id}/rotate`);
      setSecret(res.secret);
      reload();
    } catch (err) {
      toast(describeError(err), 'error');
    }
  };

  const remove = async (h: Webhook) => {
    if (!(await confirm('Delete webhook', `Stop sending events to ${h.url}?`, 'Delete', true))) return;
    try {
      await api('DELETE', `/api/webhooks/${h.id}`);
      toast('Webhook deleted.', 'success');
      if (openId === h.id) setOpenId(null);
      reload();
    } catch (err) {
      toast(describeError(err), 'error');
    }
  };

  const columns = useMemo<Columns<Webhook>>(() => [
    {
      accessorKey: 'url', header: 'Endpoint',
      cell: ({ row }) => (
        <div className="flex items-center gap-3">
          <span className={clsx('grid size-8 shrink-0 place-items-center rounded-lg', row.original.enabled ? 'bg-accent-soft text-accent' : 'bg-surface-3 text-muted')}><WebhookIcon className="size-4" /></span>
          <Strong sub={`Created ${formatDate(row.original.created_at)}`}><span className="font-mono text-[13px]">{row.original.url}</span></Strong>
        </div>
      ),
    },
    { accessorKey: 'events', header: 'Events', enableSorting: false, cell: ({ getValue }) => <div className="flex max-w-sm flex-wrap gap-1">{(getValue() as string[]).map((e) => <Chip key={e}>{e}</Chip>)}</div> },
    { id: 'state', header: 'Status', accessorFn: (h) => (h.enabled ? 'active' : 'paused'), cell: ({ getValue }) => <Badge value={getValue()} /> },
    ...(canManage ? [{
      id: 'actions', header: '', meta: { className: 'text-right' },
      cell: ({ row }: { row: { original: Webhook } }) => (
        <div className="flex justify-end gap-1" onClick={(e) => e.stopPropagation()}>
          <Button size="xs" variant="ghost" onClick={() => toggle(row.original)}>{row.original.enabled ? 'Disable' : 'Enable'}</Button>
          <Button size="xs" variant="ghost" icon={RotateCw} onClick={() => rotate(row.original)}>Rotate</Button>
          <Button size="xs" variant="ghost" aria-label="Delete webhook" className="text-muted hover:text-danger" icon={Trash2} onClick={() => remove(row.original)} />
        </div>
      ),
    }] : []),
  ], [canManage]);

  const addButton = canManage ? <Button variant="primary" icon={Plus} onClick={() => setCreating(true)}>Add webhook</Button> : null;
  return (
    <>
      <PageHeader title="Webhooks" description="Signed HTTPS callbacks for delivery events and contact changes, retried with backoff until they succeed." actions={addButton} />
      <div className="flex flex-col gap-5">
        <Card flush>
          <DataTable
            columns={columns}
            data={hooks}
            error={state.error}
            noun="webhooks"
            onRowClick={(h) => setOpenId(h.id === openId ? null : h.id)}
            empty={{ title: 'No webhooks yet', hint: 'Add an endpoint to keep your own database in step with unsubscribes, bounces and contact changes.', action: addButton }}
          />
        </Card>
        {open ? <Deliveries key={open.id} webhook={open} /> : null}
      </div>
      {creating ? (
        <CreateWebhook
          onClose={() => setCreating(false)}
          onCreated={(w) => { setCreating(false); setSecret(w.secret); setOpenId(w.id); reload(); }}
        />
      ) : null}
      {secret ? <SecretReveal secret={secret} onClose={() => setSecret(null)} /> : null}
    </>
  );
}
