import clsx from 'clsx';
import { Globe, Mail, Plus, RefreshCw, Trash2 } from 'lucide-react';
import { useRouter } from 'next/router';
import { FormEvent, useEffect, useMemo, useState } from 'react';
import DataTable, { Columns, Strong } from '../components/DataTable';
import { FormActions, Modal, useConfirm } from '../components/Modal';
import { Badge, Button, Callout, Card, CopyButton, Field, FormError, Input, PageHeader, Tabs } from '../components/ui';
import { api, describeError } from '../lib/api';
import { formatDate, humanize, timeAgo } from '../lib/format';
import { useSession } from '../lib/session';
import { useToast } from '../lib/toast';
import type { DnsRecord, Domain } from '../lib/types';
import { useLoad } from '../lib/useLoad';

function AddIdentity({ onClose, onAdded }: { onClose: () => void; onAdded: (d: Domain) => void }) {
  const [kind, setKind] = useState<'domain' | 'email'>('domain');
  const [name, setName] = useState('');
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  const submit = async (e: FormEvent) => {
    e.preventDefault();
    setBusy(true);
    setError('');
    try {
      onAdded(await api<Domain>('POST', '/api/domains', { name: name.trim(), kind }));
    } catch (err) {
      setError(describeError(err));
      setBusy(false);
    }
  };
  return (
    <Modal title="Add sending identity" onClose={onClose}>
      <form className="flex flex-col gap-4" onSubmit={submit}>
        <Tabs value={kind} onChange={setKind} options={[['domain', 'Domain'], ['email', 'Single email address']]} />
        <Field
          label={kind === 'domain' ? 'Domain' : 'Email address'}
          hint={kind === 'domain'
            ? 'Send from any address on this domain once its DNS records are in place.'
            : 'For sandbox testing. Amazon SES emails this address a link that must be clicked to verify it.'}
        >
          <Input required autoFocus value={name} onChange={(e) => setName(e.target.value)} placeholder={kind === 'domain' ? 'mail.example.com' : 'you@example.com'} />
        </Field>
        <FormError>{error}</FormError>
        <FormActions>
          <Button onClick={onClose}>Cancel</Button>
          <Button variant="primary" type="submit" loading={busy}>Add</Button>
        </FormActions>
      </form>
    </Modal>
  );
}

function Value({ value }: { value: string }) {
  return (
    <span className="flex max-w-[420px] min-w-0 items-center gap-1">
      <code className="truncate font-mono text-xs text-ink">{value}</code>
      <CopyButton value={value} />
    </span>
  );
}

function Records({ domain }: { domain: Domain }) {
  const columns = useMemo<Columns<DnsRecord>>(() => [
    { accessorKey: 'type', header: 'Type', enableSorting: false, meta: { className: 'w-20' }, cell: ({ getValue }) => <span className="inline-flex h-6 items-center rounded-md bg-info-soft px-2 font-mono text-[11px] font-semibold text-info">{getValue()}</span> },
    { accessorKey: 'name', header: 'Name', enableSorting: false, cell: ({ getValue }) => <Value value={getValue()} /> },
    { accessorKey: 'value', header: 'Content', enableSorting: false, cell: ({ getValue }) => <Value value={getValue()} /> },
  ], []);
  if (domain.kind === 'email') {
    return <p className="px-5 py-4 text-[13px] text-muted">{domain.status === 'verified' ? 'This address is verified.' : `Open the verification email Amazon SES sent to ${domain.name} and click the link, then press Verify.`}</p>;
  }
  return (
    <>
      <div className="px-5 pt-4"><Callout tone="info">Add these records at your DNS provider. If you use Cloudflare, set the CNAME records to <strong>DNS only</strong> (grey cloud). The three CNAME records are required for DKIM; SPF and DMARC are recommended.</Callout></div>
      <div className="mt-4 border-t border-line"><DataTable columns={columns} data={domain.records} getRowId={(r) => `${r.type}-${r.name}`} noun="records" /></div>
    </>
  );
}

export default function Domains() {
  const router = useRouter();
  const toast = useToast();
  const confirm = useConfirm();
  const { refreshMe, canManage } = useSession();
  const [adding, setAdding] = useState(false);
  const [openId, setOpenId] = useState<string | null>(null);
  const [busyId, setBusyId] = useState<string | null>(null);
  const [state, reload] = useLoad(() => api<{ domains: Domain[] }>('GET', '/api/domains'), []);
  const domains = state.data ? state.data.domains : null;
  const open = domains?.find((d) => d.id === openId) || (domains && domains.length === 1 && openId === null ? domains[0] : undefined);

  useEffect(() => {
    if (router.query.add && canManage) {
      setAdding(true);
      router.replace('/domains/', undefined, { shallow: true });
    }
  }, [router.query.add, canManage]);

  const verify = async (d: Domain) => {
    setBusyId(d.id);
    try {
      const updated = await api<Domain>('POST', `/api/domains/${d.id}/verify`);
      toast(updated.status === 'verified' ? `${updated.name} is verified.` : `${updated.name} is not verified yet.`, updated.status === 'verified' ? 'success' : 'warn');
      reload();
      refreshMe();
    } catch (err) {
      toast(describeError(err), 'error');
    } finally {
      setBusyId(null);
    }
  };

  const remove = async (d: Domain) => {
    if (!(await confirm('Remove identity', `Remove ${d.name}? It is also deleted from Amazon SES and emails can no longer be sent from it.`, 'Remove', true))) return;
    try {
      await api('DELETE', `/api/domains/${d.id}`);
      toast('Identity removed.', 'success');
      if (openId === d.id) setOpenId(null);
      reload();
      refreshMe();
    } catch (err) {
      toast(describeError(err), 'error');
    }
  };

  const columns = useMemo<Columns<Domain>>(() => [
    {
      accessorKey: 'name', header: 'Identity',
      cell: ({ row }) => {
        const Icon = row.original.kind === 'domain' ? Globe : Mail;
        return (
          <div className="flex items-center gap-3">
            <span className={clsx('grid size-8 place-items-center rounded-lg', row.original.status === 'verified' ? 'bg-success-soft text-success' : 'bg-warning-soft text-warning')}><Icon className="size-4" /></span>
            <Strong sub={row.original.kind === 'domain' ? 'Domain' : 'Email address'}>{row.original.name}</Strong>
          </div>
        );
      },
    },
    { accessorKey: 'status', header: 'Status', cell: ({ getValue }) => <Badge value={getValue()} /> },
    { accessorKey: 'last_checked_at', header: 'Last checked', cell: ({ getValue }) => <span className="text-muted">{timeAgo(getValue())}</span> },
    { accessorKey: 'created_at', header: 'Added', cell: ({ getValue }) => <span className="text-muted">{formatDate(getValue())}</span> },
    {
      id: 'actions', header: '', meta: { className: 'text-right' },
      cell: ({ row }) => (
        <div className="flex justify-end gap-1">
          {row.original.status !== 'verified' ? (
            <Button size="xs" variant="ghost" icon={RefreshCw} loading={busyId === row.original.id} onClick={(e) => { e.stopPropagation(); verify(row.original); }}>Verify</Button>
          ) : null}
          {canManage ? <Button size="xs" variant="ghost" aria-label={`Remove ${row.original.name}`} className="text-muted hover:text-danger" icon={Trash2} onClick={(e) => { e.stopPropagation(); remove(row.original); }} /> : null}
        </div>
      ),
    },
  ], [busyId, openId, canManage]);

  const addButton = canManage ? <Button variant="primary" icon={Plus} onClick={() => setAdding(true)}>Add identity</Button> : null;
  return (
    <>
      <PageHeader title="Domains" description="Verified domains and addresses you can send from. Pending identities are rechecked every minute." actions={addButton} />
      <div className="flex flex-col gap-5">
        <Card flush>
          <DataTable
            columns={columns}
            data={domains}
            error={state.error}
            noun="identities"
            onRowClick={(d) => setOpenId(d.id === open?.id ? '' : d.id)}
            empty={{ title: 'No sending identities yet', hint: 'Add a domain, or a single email address for sandbox testing.', action: addButton }}
          />
        </Card>
        {open ? (
          <Card
            title={`DNS records · ${open.name}`}
            description={open.ses_status ? `Amazon SES status: ${humanize(open.ses_status).toLowerCase()}` : undefined}
            flush
            actions={open.status !== 'verified' ? <Button size="sm" icon={RefreshCw} loading={busyId === open.id} onClick={() => verify(open)}>Verify now</Button> : <Badge value="verified" />}
          >
            <Records domain={open} />
          </Card>
        ) : null}
      </div>
      {adding ? <AddIdentity onClose={() => setAdding(false)} onAdded={(d) => { setAdding(false); setOpenId(d.id); toast(`${d.name} added.`, 'success'); reload(); refreshMe(); }} /> : null}
    </>
  );
}
