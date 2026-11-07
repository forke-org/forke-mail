import { useMemo, useState } from 'react';
import DataTable from '../components/DataTable';
import Icon from '../components/Icon';
import { Modal, useConfirm } from '../components/Modal';
import { Badge, Card, CopyButton, Field, PageHead, Tabs } from '../components/ui';
import { api, describeError } from '../lib/api';
import { formatDate, timeAgo } from '../lib/format';
import { useLoad } from '../lib/useLoad';
import { useSession } from '../lib/session';
import { useToast } from '../lib/toast';

function AddIdentity({ onClose, onAdded }) {
  const [kind, setKind] = useState('domain');
  const [name, setName] = useState('');
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  const submit = async (e) => {
    e.preventDefault();
    setBusy(true);
    setError('');
    try {
      onAdded(await api('POST', '/api/domains', { name: name.trim(), kind }));
    } catch (err) {
      setError(describeError(err));
      setBusy(false);
    }
  };
  return (
    <Modal title="Add sending identity" onClose={onClose}>
      <form className="stack-sm" onSubmit={submit}>
        <Tabs value={kind} onChange={setKind} options={[['domain', 'Domain'], ['email', 'Single email address']]} />
        <Field
          label={kind === 'domain' ? 'Domain' : 'Email address'}
          hint={kind === 'domain'
            ? 'Send from any address on this domain once its DNS records are in place.'
            : 'For sandbox testing. Amazon SES emails this address a link that must be clicked to verify it.'}
        >
          <input type="text" required autoFocus value={name} onChange={(e) => setName(e.target.value)} placeholder={kind === 'domain' ? 'mail.example.com' : 'you@example.com'} />
        </Field>
        {error ? <p className="form-error">{error}</p> : null}
        <div className="actions actions-end">
          <button className="btn" type="button" onClick={onClose}>Cancel</button>
          <button className="btn btn-primary" type="submit" disabled={busy}>{busy ? 'Adding…' : 'Add'}</button>
        </div>
      </form>
    </Modal>
  );
}

function Records({ domain }) {
  const columns = useMemo(() => [
    { accessorKey: 'type', header: 'Type', enableSorting: false, cell: ({ getValue }) => <code>{getValue()}</code> },
    { accessorKey: 'name', header: 'Name', enableSorting: false, cell: ({ getValue }) => <span className="record-value"><code>{getValue()}</code><CopyButton value={getValue()} /></span> },
    { accessorKey: 'value', header: 'Value', enableSorting: false, cell: ({ getValue }) => <span className="record-value"><code>{getValue()}</code><CopyButton value={getValue()} /></span> },
  ], []);
  if (domain.kind === 'email') {
    return <p className="small muted card-body">{domain.status === 'verified' ? 'This address is verified.' : `Open the verification email Amazon SES sent to ${domain.name} and click the link, then press Verify.`}</p>;
  }
  return (
    <>
      <p className="small muted card-body">Add these records at your DNS provider. The three CNAME records are required for DKIM; SPF and DMARC are recommended.</p>
      <DataTable columns={columns} data={domain.records} getRowId={(r) => `${r.type}-${r.name}`} />
    </>
  );
}

export default function Domains() {
  const toast = useToast();
  const confirm = useConfirm();
  const { refreshMe } = useSession();
  const [adding, setAdding] = useState(false);
  const [openId, setOpenId] = useState(null);
  const [busyId, setBusyId] = useState(null);
  const [state, reload] = useLoad(() => api('GET', '/api/domains'), []);
  const domains = state.data ? state.data.domains : null;
  const open = domains && domains.find((d) => d.id === openId);

  const verify = async (d) => {
    setBusyId(d.id);
    try {
      const updated = await api('POST', `/api/domains/${d.id}/verify`);
      toast(updated.status === 'verified' ? `${updated.name} is verified.` : `${updated.name} is not verified yet.`, updated.status === 'verified' ? 'success' : 'warn');
      reload();
      refreshMe();
    } catch (err) {
      toast(describeError(err), 'error');
    } finally {
      setBusyId(null);
    }
  };

  const remove = async (d) => {
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

  const columns = useMemo(() => [
    { accessorKey: 'name', header: 'Identity', cell: ({ getValue }) => <span className="cell-strong">{getValue()}</span> },
    { accessorKey: 'kind', header: 'Type', cell: ({ getValue }) => (getValue() === 'domain' ? 'Domain' : 'Email address') },
    { accessorKey: 'status', header: 'Status', cell: ({ getValue }) => <Badge value={getValue()} /> },
    { accessorKey: 'last_checked_at', header: 'Last checked', cell: ({ getValue }) => timeAgo(getValue()) },
    { accessorKey: 'created_at', header: 'Added', cell: ({ getValue }) => formatDate(getValue()) },
    {
      id: 'actions', header: '', meta: { className: 'col-actions' },
      cell: ({ row }) => (
        <div className="actions actions-end">
          {row.original.status !== 'verified' ? (
            <button className="btn btn-ghost btn-sm" type="button" disabled={busyId === row.original.id} onClick={(e) => { e.stopPropagation(); verify(row.original); }}>
              {busyId === row.original.id ? 'Checking…' : 'Verify'}
            </button>
          ) : null}
          <button className="btn btn-ghost btn-sm btn-danger" type="button" onClick={(e) => { e.stopPropagation(); remove(row.original); }}>Remove</button>
        </div>
      ),
    },
  ], [busyId, openId]);

  const addButton = <button className="btn btn-primary" type="button" onClick={() => setAdding(true)}><Icon name="plus" />Add identity</button>;
  return (
    <>
      <PageHead title="Domains" subtitle="Verified domains and addresses you can send from. Pending identities are rechecked every minute.">{addButton}</PageHead>
      <Card flush>
        <DataTable
          columns={columns}
          data={domains}
          error={state.error}
          onRowClick={(d) => setOpenId(d.id === openId ? null : d.id)}
          empty={{ title: 'No sending identities yet', hint: 'Add a domain, or a single email address for sandbox testing.', action: addButton }}
        />
      </Card>
      {open ? (
        <Card title={`DNS records for ${open.name}`} description={open.ses_status ? `Amazon SES status: ${open.ses_status.replace(/_/g, ' ').toLowerCase()}` : null} flush
          actions={open.status !== 'verified' ? <button className="btn btn-sm" type="button" onClick={() => verify(open)}>Verify now</button> : <Badge value="verified" />}>
          <Records domain={open} />
        </Card>
      ) : null}
      {adding ? <AddIdentity onClose={() => setAdding(false)} onAdded={(d) => { setAdding(false); setOpenId(d.id); toast(`${d.name} added.`, 'success'); reload(); refreshMe(); }} /> : null}
    </>
  );
}
