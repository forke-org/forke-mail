import Link from 'next/link';
import { useRouter } from 'next/router';
import { useMemo } from 'react';
import DataTable from '../components/DataTable';
import { Badge, Card, PageHead, Stat } from '../components/ui';
import { api } from '../lib/api';
import { formatDate, number } from '../lib/format';
import { useLoad } from '../lib/useLoad';
import { useSession } from '../lib/session';

const sendingModes = {
  log: { label: 'Log sender', tone: 'blue' },
  sandbox: { label: 'SES sandbox', tone: 'amber' },
  production: { label: 'SES production', tone: 'green' },
};

function Checklist({ steps }) {
  const done = steps.filter((s) => s.done).length;
  if (done === steps.length) return null;
  return (
    <Card title="Get started" description={`${done} of ${steps.length} done`}>
      <ol className="checklist">
        {steps.map((s, i) => (
          <li key={s.label} className={s.done ? 'done' : undefined}>
            <span className="check">{s.done ? '✓' : i + 1}</span>
            <div>
              <div className="cell-strong">{s.label}</div>
              <div className="small muted">{s.hint}</div>
            </div>
            {s.done ? null : <Link className="btn btn-sm" href={s.href}>{s.cta}</Link>}
          </li>
        ))}
      </ol>
    </Card>
  );
}

export default function Overview() {
  const router = useRouter();
  const { me, trial } = useSession();
  const [state] = useLoad(() => Promise.all([api('GET', '/api/dashboard'), api('GET', '/api/keys')]), []);
  const [d, keys] = state.data || [null, null];

  const columns = useMemo(() => [
    { accessorKey: 'subject', header: 'Email', cell: ({ row }) => <div><div className="cell-strong">{row.original.subject}</div><div className="small muted">{row.original.campaign_name}</div></div> },
    { accessorKey: 'status', header: 'Status', cell: ({ getValue }) => <Badge value={getValue()} /> },
    { accessorKey: 'recipients', header: 'Recipients', meta: { className: 'num' }, cell: ({ getValue }) => number(getValue()) },
    { accessorKey: 'created_at', header: 'Created', cell: ({ getValue }) => formatDate(getValue()) },
  ], []);

  const steps = d ? [
    { label: 'Verify a sending domain', hint: 'Add a domain or address and publish its DNS records.', done: me.domains.length > 0, href: '/domains/', cta: 'Add' },
    { label: 'Create a campaign', hint: 'Campaigns group related emails, like "Product updates".', done: d.campaigns > 0, href: '/campaigns/', cta: 'Create' },
    { label: 'Add contacts', hint: 'Import a CSV and tag contacts to target them later.', done: d.contacts > 0, href: '/contacts/', cta: 'Import' },
    { label: 'Send your first email', hint: 'Write an email inside a campaign and send it.', done: d.emails_sent > 0, href: '/campaigns/', cta: 'Write' },
    { label: 'Create an API key', hint: 'Send from your own code with the REST API.', done: keys.keys.length > 0, href: '/keys/', cta: 'Create' },
  ] : [];

  return (
    <>
      <PageHead title={`Welcome, ${me.user.name.split(' ')[0]}`} subtitle={`${me.workspace.name} · ${trial ? 'Trial' : 'Paid'} plan`}>
        {d ? <Badge value={sendingModes[d.sending_mode].label} tone={sendingModes[d.sending_mode].tone} /> : null}
        <Link className="btn btn-primary" href="/campaigns/">New email</Link>
      </PageHead>
      {d ? <Checklist steps={steps} /> : null}
      <div className="grid grid-4">
        <Stat label="Contacts" value={d ? number(d.contacts) : '—'} hint={d ? `${number(d.active_contacts)} active` : null} />
        <Stat label="Campaigns" value={d ? number(d.campaigns) : '—'} />
        <Stat label="Sent today" value={d ? number(d.sent_today) : '—'} />
        {trial
          ? <Stat label="Trial sends left" value={number(Math.max(0, me.quota.trial_limit - me.quota.trial_used))} hint={`of ${number(me.quota.trial_limit)}`} />
          : <Stat label="Sent last 7 days" value={d ? number(d.sent_last_7_days) : '—'} />}
      </div>
      <Card title="Recent emails" flush>
        <DataTable
          columns={columns}
          data={d ? d.recent_emails : null}
          error={state.error}
          onRowClick={(e) => router.push(e.status === 'draft' ? `/emails/edit/?id=${e.id}` : `/emails/view/?id=${e.id}`)}
          empty={{ title: 'No emails yet', hint: 'Create a campaign and write your first email.' }}
        />
      </Card>
    </>
  );
}
