import { CalendarClock, Mail, Pencil, Plus, Send } from 'lucide-react';
import { useRouter } from 'next/router';
import { useMemo, useState } from 'react';
import CampaignForm from '../../components/CampaignForm';
import DataTable, { Columns, Strong } from '../../components/DataTable';
import { Badge, Button, ButtonLink, Card, Empty, Loading, PageHeader, Progress, Stat } from '../../components/ui';
import { api } from '../../lib/api';
import { formatDate, number, timeAgo } from '../../lib/format';
import type { Campaign, Email } from '../../lib/types';
import { never, useLoad } from '../../lib/useLoad';

export default function CampaignView() {
  const router = useRouter();
  const id = router.query.id as string | undefined;
  const [editing, setEditing] = useState(false);
  const [state, reload] = useLoad(() => (id ? api<{ campaign: Campaign; emails: Email[] }>('GET', `/api/campaigns/${id}`) : never<{ campaign: Campaign; emails: Email[] }>()), [id]);

  const columns = useMemo<Columns<Email>>(() => [
    { accessorKey: 'subject', header: 'Email', cell: ({ row }) => <Strong sub={row.original.topic ? `Topic: ${row.original.topic}` : undefined}>{row.original.subject || 'Untitled draft'}</Strong> },
    { accessorKey: 'status', header: 'Status', cell: ({ getValue }) => <Badge value={getValue()} /> },
    { id: 'progress', header: 'Progress', enableSorting: false, cell: ({ row }) => (row.original.status === 'draft' ? <span className="text-faint">—</span> : <Progress percent={row.original.recipients ? (row.original.processed / row.original.recipients) * 100 : 0} />) },
    { accessorKey: 'recipients', header: 'Recipients', meta: { className: 'text-right' }, cell: ({ getValue }) => <span className="tabular-nums">{number(getValue())}</span> },
    { accessorKey: 'created_at', header: 'Created', cell: ({ getValue }) => <span className="text-muted">{formatDate(getValue())}</span> },
  ], []);

  if (state.error) return <Card><Empty title="Campaign not found" hint={state.error} /></Card>;
  if (!state.data) return <Loading />;
  const { campaign, emails } = state.data;
  const newEmail = <ButtonLink variant="primary" icon={Plus} href={`/emails/edit/?campaign=${campaign.id}`}>New email</ButtonLink>;

  return (
    <>
      <PageHeader
        back={{ href: '/campaigns/', label: 'Broadcasts' }}
        title={campaign.name}
        description={campaign.description || 'No description'}
        actions={<><Button icon={Pencil} onClick={() => setEditing(true)}>Edit</Button>{newEmail}</>}
      />
      <div className="mb-6 grid grid-cols-1 gap-px overflow-hidden rounded-[8px] bg-line shadow-card sm:grid-cols-3">
        <Stat icon={Mail} tone="accent" label="Emails" value={number(campaign.emails)} />
        <Stat icon={Send} tone="green" label="Recipients" value={number(campaign.sent)} hint="Across all emails" />
        <Stat icon={CalendarClock} tone="blue" label="Last email" value={campaign.last_email_at ? timeAgo(campaign.last_email_at) : '—'} hint={campaign.last_email_at ? formatDate(campaign.last_email_at) : undefined} />
      </div>
      <Card title="Emails" flush>
        <DataTable
          columns={columns}
          data={emails}
          noun="emails"
          onRowClick={(e) => router.push(e.status === 'draft' ? `/emails/edit/?id=${e.id}` : `/emails/view/?id=${e.id}`)}
          empty={{ title: 'No emails in this campaign', hint: 'Write an email, pick who receives it, and send.', action: newEmail }}
        />
      </Card>
      {editing ? <CampaignForm campaign={campaign} onClose={() => setEditing(false)} onSaved={() => { setEditing(false); reload(); }} /> : null}
    </>
  );
}
