import clsx from 'clsx';
import { Check, Gauge, Megaphone, Send, Users, Zap } from 'lucide-react';
import { useRouter } from 'next/router';
import { useMemo } from 'react';
import DataTable, { Columns, Strong } from '../components/DataTable';
import { Badge, ButtonLink, Card, Meter, PageHeader, Skeleton, Stat } from '../components/ui';
import { api } from '../lib/api';
import { formatDate, number, percent } from '../lib/format';
import { useMe, useSession } from '../lib/session';
import type { ApiKey, Dashboard } from '../lib/types';
import { useLoad } from '../lib/useLoad';

const sendingModes = {
  log: { label: 'Log sender', tone: 'blue' as const },
  sandbox: { label: 'SES sandbox', tone: 'amber' as const },
  production: { label: 'SES production', tone: 'green' as const },
};

const breakdown: [string, string, string][] = [
  ['delivered', 'Delivered', 'bg-success'],
  ['opened', 'Opened', 'bg-accent'],
  ['clicked', 'Clicked', 'bg-[#ffa347]'],
  ['unsubscribed', 'Unsubscribed', 'bg-warning'],
  ['bounced', 'Bounced', 'bg-danger'],
  ['complained', 'Complained', 'bg-danger'],
];

type Step = { label: string; hint: string; done: boolean; href: string; cta: string };

function Checklist({ steps }: { steps: Step[] }) {
  const done = steps.filter((s) => s.done).length;
  if (done === steps.length) return null;
  return (
    <Card
      title="Get started"
      description={`${done} of ${steps.length} complete`}
      actions={<Meter value={done} max={steps.length} tone="accent" className="w-28" />}
      flush
      className="mb-6"
    >
      <ol className="divide-y divide-line">
        {steps.map((s, i) => (
          <li key={s.label} className="flex items-center gap-4 px-5 py-3.5">
            <span className={clsx('grid size-7 shrink-0 place-items-center rounded-full text-xs font-semibold', s.done ? 'bg-success-soft text-success' : 'bg-surface-3 text-ink-2')}>
              {s.done ? <Check className="size-3.5" /> : i + 1}
            </span>
            <div className="min-w-0 flex-1">
              <div className={clsx('text-sm font-medium', s.done ? 'text-muted line-through' : 'text-ink')}>{s.label}</div>
              <div className="text-xs text-muted">{s.hint}</div>
            </div>
            {s.done ? null : <ButtonLink size="sm" href={s.href}>{s.cta}</ButtonLink>}
          </li>
        ))}
      </ol>
    </Card>
  );
}

export default function Overview() {
  const router = useRouter();
  const me = useMe();
  const { trial, superAdmin } = useSession();
  const [state] = useLoad(() => Promise.all([
    api<Dashboard>('GET', '/api/dashboard'),
    superAdmin ? api<{ keys: ApiKey[] }>('GET', '/api/keys') : Promise.resolve(null),
  ]), [superAdmin]);
  const [d, keys] = state.data || [null, null];

  const columns = useMemo<Columns<Dashboard['recent_emails'][number]>>(() => [
    { accessorKey: 'subject', header: 'Email', cell: ({ row }) => <Strong sub={row.original.campaign_name}>{row.original.subject}</Strong> },
    { accessorKey: 'status', header: 'Status', cell: ({ getValue }) => <Badge value={getValue()} /> },
    { accessorKey: 'recipients', header: 'Recipients', meta: { className: 'text-right' }, cell: ({ getValue }) => <span className="tabular-nums">{number(getValue())}</span> },
    { accessorKey: 'created_at', header: 'Created', cell: ({ getValue }) => <span className="text-muted">{formatDate(getValue())}</span> },
  ], []);

  const steps: Step[] = d ? [
    { label: 'Verify a sending domain', hint: 'Add a domain and publish its DNS records.', done: me.domains.length > 0, href: '/domains/', cta: 'Add domain' },
    { label: 'Create a campaign', hint: 'Campaigns group related broadcasts, like the changelog.', done: d.campaigns > 0, href: '/campaigns/', cta: 'Create' },
    { label: 'Add contacts', hint: 'Import a CSV and tag contacts to target them later.', done: d.contacts > 0, href: '/contacts/', cta: 'Import' },
    { label: 'Send your first broadcast', hint: 'Write an email inside a campaign and send it.', done: d.emails_sent > 0, href: '/campaigns/', cta: 'Write' },
    ...(superAdmin && keys ? [{ label: 'Create an API key', hint: 'Send from Forke apps with the REST API.', done: keys.keys.length > 0, href: '/keys/', cta: 'Create' }] : []),
  ] : [];

  const events = d?.events_last_7_days || {};
  const delivered = events.delivered || 0;
  const maxEvent = Math.max(1, ...breakdown.map(([k]) => events[k] || 0));
  const { sent_this_month: month, monthly_limit: monthLimit } = me.quota;
  const first = me.user ? me.user.name.split(' ')[0] : me.workspace.name;

  return (
    <>
      <PageHeader
        title={`Welcome back, ${first}`}
        description={`${me.workspace.name} · ${trial ? 'Trial' : 'Paid'} workspace`}
        meta={d ? <Badge value={sendingModes[d.sending_mode].label} tone={sendingModes[d.sending_mode].tone} /> : null}
        actions={(
          <>
            <ButtonLink href="/transactional/?test=1" icon={Zap}>Send test</ButtonLink>
            <ButtonLink href="/campaigns/" variant="primary" icon={Megaphone}>New broadcast</ButtonLink>
          </>
        )}
      />
      {d ? <Checklist steps={steps} /> : null}
      <div className="mb-6 grid grid-cols-2 gap-3 lg:grid-cols-4">
        <Stat icon={Users} tone="accent" label="Contacts" value={d ? number(d.contacts) : <Skeleton className="h-8 w-16" />} hint={d ? `${number(d.active_contacts)} active` : ' '} />
        <Stat icon={Send} tone="blue" label="Sent today" value={d ? number(d.sent_today) : <Skeleton className="h-8 w-16" />} hint={d ? `${number(d.sent_last_7_days)} in the last 7 days` : ' '} />
        <Stat icon={Megaphone} tone="green" label="Delivery rate" value={d ? percent(delivered, d.sent_last_7_days) : <Skeleton className="h-8 w-16" />} hint="Last 7 days" />
        {trial
          ? <Stat icon={Gauge} tone="amber" label="Trial sends left" value={number(Math.max(0, me.quota.trial_limit - me.quota.trial_used))} hint={`of ${number(me.quota.trial_limit)}`} />
          : <Stat icon={Gauge} tone="amber" label="Sent this month" value={number(month)} hint={monthLimit > 0 ? `of ${number(monthLimit)} cap` : 'No monthly cap'}>{monthLimit > 0 ? <Meter value={month} max={monthLimit} className="mt-2" /> : null}</Stat>}
      </div>
      <div className="mb-6 grid gap-4 lg:grid-cols-[minmax(0,1.4fr)_minmax(0,1fr)]">
        <Card title="Engagement" description="Events recorded in the last 7 days">
          <div className="flex flex-col gap-3.5">
            {breakdown.map(([key, label, color]) => {
              const n = events[key] || 0;
              return (
                <div key={key} className="grid grid-cols-[110px_minmax(0,1fr)_64px] items-center gap-3 text-[13px]">
                  <span className="text-ink-2">{label}</span>
                  <div className="h-2 overflow-hidden rounded-full bg-surface-3">
                    <div className={clsx('h-full rounded-full transition-[width] duration-700', color)} style={{ width: d ? `${Math.max(n ? 2 : 0, (n / maxEvent) * 100)}%` : '0%' }} />
                  </div>
                  <span className="text-right text-ink tabular-nums">{d ? number(n) : '—'}</span>
                </div>
              );
            })}
          </div>
        </Card>
        <Card title="Sending limits" description="How fast and how much this workspace can send">
          <div className="flex flex-col">
            <div className="flex items-center justify-between border-b border-line pb-3 text-[13px]"><span className="text-muted">Send rate</span><span className="text-ink tabular-nums">{me.rate_limits.send_per_second} / second</span></div>
            <div className="flex items-center justify-between border-b border-line py-3 text-[13px]"><span className="text-muted">Daily limit</span><span className="text-ink tabular-nums">{me.quota.daily_limit > 0 ? number(me.quota.daily_limit) : 'Unlimited'}</span></div>
            <div className="flex items-center justify-between border-b border-line py-3 text-[13px]"><span className="text-muted">Monthly cap</span><span className="text-ink tabular-nums">{monthLimit > 0 ? number(monthLimit) : 'None'}</span></div>
            <div className="flex items-center justify-between pt-3 text-[13px]"><span className="text-muted">Verified identities</span><span className="text-ink tabular-nums">{me.domains.length}</span></div>
          </div>
        </Card>
      </div>
      <Card title="Recent broadcasts" flush actions={<ButtonLink size="sm" variant="ghost" href="/campaigns/">View all</ButtonLink>}>
        <DataTable
          columns={columns}
          data={d ? d.recent_emails : null}
          error={state.error}
          noun="emails"
          onRowClick={(e) => router.push(e.status === 'draft' ? `/emails/edit/?id=${e.id}` : `/emails/view/?id=${e.id}`)}
          empty={{ title: 'No broadcasts yet', hint: 'Create a campaign and write your first email.' }}
        />
      </Card>
    </>
  );
}
