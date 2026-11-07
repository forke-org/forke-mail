import Link from 'next/link';
import { useRouter } from 'next/router';
import { useMemo, useState } from 'react';
import CampaignForm from '../../components/CampaignForm';
import DataTable from '../../components/DataTable';
import Icon from '../../components/Icon';
import { Badge, Card, Empty, Loading, PageHead, Progress, Stat } from '../../components/ui';
import { api } from '../../lib/api';
import { formatDate, number } from '../../lib/format';
import { useLoad } from '../../lib/useLoad';

export default function CampaignView() {
  const router = useRouter();
  const id = router.query.id;
  const [editing, setEditing] = useState(false);
  const [state, reload] = useLoad(() => (id ? api('GET', `/api/campaigns/${id}`) : new Promise(() => {})), [id]);

  const columns = useMemo(() => [
    { accessorKey: 'subject', header: 'Email', cell: ({ getValue }) => <span className="cell-strong">{getValue()}</span> },
    { accessorKey: 'status', header: 'Status', cell: ({ getValue }) => <Badge value={getValue()} /> },
    { id: 'progress', header: 'Progress', enableSorting: false, cell: ({ row }) => (row.original.status === 'draft' ? <span className="muted">—</span> : <Progress percent={row.original.recipients ? (row.original.processed / row.original.recipients) * 100 : 0} />) },
    { accessorKey: 'recipients', header: 'Recipients', meta: { className: 'num' }, cell: ({ getValue }) => number(getValue()) },
    { accessorKey: 'created_at', header: 'Created', cell: ({ getValue }) => formatDate(getValue()) },
  ], []);

  if (state.error) return <Card><Empty title="Campaign not found" hint={state.error} /></Card>;
  if (!state.data) return <Loading />;
  const { campaign, emails } = state.data;
  const newEmail = <Link className="btn btn-primary" href={`/emails/edit/?campaign=${campaign.id}`}><Icon name="plus" />New email</Link>;

  return (
    <>
      <PageHead
        back={<Link className="back-link" href="/campaigns/">← Campaigns</Link>}
        title={campaign.name}
        subtitle={campaign.description || 'No description'}
      >
        <button className="btn" type="button" onClick={() => setEditing(true)}>Edit</button>
        {newEmail}
      </PageHead>
      <div className="grid grid-3">
        <Stat label="Emails" value={number(campaign.emails)} />
        <Stat label="Delivered to" value={number(campaign.sent)} hint="Recipients across all emails" />
        <Stat label="Last email" value={campaign.last_email_at ? formatDate(campaign.last_email_at) : '—'} />
      </div>
      <Card title="Emails" flush>
        <DataTable
          columns={columns}
          data={emails}
          onRowClick={(e) => router.push(e.status === 'draft' ? `/emails/edit/?id=${e.id}` : `/emails/view/?id=${e.id}`)}
          empty={{ title: 'No emails in this campaign', hint: 'Write an email, pick who receives it, and send.', action: newEmail }}
        />
      </Card>
      {editing ? <CampaignForm campaign={campaign} onClose={() => setEditing(false)} onSaved={() => { setEditing(false); reload(); }} /> : null}
    </>
  );
}
