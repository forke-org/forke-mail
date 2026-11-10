import clsx from 'clsx';
import { Pause, Play } from 'lucide-react';
import { useRouter } from 'next/router';
import { useEffect, useMemo, useState } from 'react';
import DataTable, { Columns } from '../../components/DataTable';
import { Badge, Button, Callout, Card, EmailFrame, Empty, Loading, Meter, PageHeader, Skeleton } from '../../components/ui';
import { api, describeError } from '../../lib/api';
import { formatDate, humanize, number, percent } from '../../lib/format';
import { useSession } from '../../lib/session';
import { useToast } from '../../lib/toast';
import { outcome } from '../../lib/transactional';
import type { Email, Recipient, Report } from '../../lib/types';
import { useEmailWatch } from '../../lib/useEmailWatch';
import { never, useLoad } from '../../lib/useLoad';

const pauseReasons: Record<string, string> = {
  manual: 'Sending is paused.',
  trial_quota_exhausted: 'Sending stopped because your trial send limit is used up.',
  daily_limit_reached: 'Sending paused because the daily send limit was reached. Resume tomorrow.',
  monthly_limit_reached: 'Sending paused because the monthly sending cap was reached. Resume on the 1st, or raise the cap.',
  ses_sending_paused: 'Our email provider paused sending. Resume once sending is re-enabled.',
  identity_not_verified: 'Sending stopped because the from domain or address is not verified. Verify it on the Domains page, then resume.',
  reputation: 'Sending stopped because this workspace exceeded its bounce or complaint rate limit.',
};

const noResume = ['trial_quota_exhausted', 'reputation'];

type CountKey = keyof Report['counts'];

const funnel: { key: CountKey; label: string; base?: CountKey; tone: string }[] = [
  { key: 'sent', label: 'Sent', tone: 'text-ink' },
  { key: 'delivered', label: 'Delivered', base: 'sent', tone: 'text-success' },
  { key: 'opened', label: 'Opened', base: 'delivered', tone: 'text-accent-ink' },
  { key: 'clicked', label: 'Clicked', base: 'delivered', tone: 'text-accent-ink' },
];

const problems: { key: CountKey; label: string }[] = [
  { key: 'unsubscribed', label: 'Unsubscribed' },
  { key: 'bounced', label: 'Bounced' },
  { key: 'complained', label: 'Complained' },
  { key: 'failed', label: 'Failed' },
  { key: 'skipped', label: 'Skipped' },
  { key: 'pending', label: 'Pending' },
];

function Recipients({ id, version }: { id: string; version: number }) {
  const [state, reload] = useLoad(() => api<{ recipients: Recipient[] }>('GET', `/api/emails/${id}/recipients?limit=200`), [id]);
  useEffect(() => { if (version) reload(); }, [version]);
  const columns = useMemo<Columns<Recipient>>(() => [
    { accessorKey: 'email', header: 'Recipient', cell: ({ getValue }) => <span className="font-medium text-ink">{getValue()}</span> },
    { accessorKey: 'status', header: 'Status', cell: ({ getValue }) => <Badge value={getValue()} /> },
    { id: 'outcome', header: 'Outcome', accessorFn: outcome, cell: ({ getValue }) => (getValue() ? <Badge value={getValue()} /> : <span className="text-faint">—</span>) },
    { accessorKey: 'attempts', header: 'Attempts', meta: { className: 'text-right' }, cell: ({ getValue }) => <span className="tabular-nums">{getValue()}</span> },
    { accessorKey: 'sent_at', header: 'Sent', cell: ({ getValue }) => <span className="text-muted">{formatDate(getValue())}</span> },
    { accessorKey: 'last_error', header: 'Note', cell: ({ getValue }) => (getValue() ? <span className="block max-w-xs truncate text-xs text-muted">{humanize(getValue())}</span> : <span className="text-faint">—</span>) },
  ], []);
  return (
    <Card title="Recipients" description="The latest 200 recipients" flush>
      <DataTable columns={columns} data={state.data ? state.data.recipients : null} error={state.error} noun="recipients" empty={{ title: 'No recipients yet' }} />
    </Card>
  );
}

function EmailReport({ email }: { email: Email }) {
  const toast = useToast();
  const { showUpgrade } = useSession();
  const [report, setReport] = useState<Report | null>(null);
  const [version, setVersion] = useState(0);
  const [restart, setRestart] = useState(0);
  const status = report ? report.status : email.status;
  const pauseReason = report ? report.pause_reason : email.pause_reason;

  useEmailWatch(email.id, (r) => {
    setReport(r);
    setVersion((v) => v + 1);
  }, restart);

  const control = async (action: 'pause' | 'resume') => {
    try {
      await api('POST', `/api/emails/${email.id}/${action}`);
      toast(action === 'pause' ? 'Sending paused.' : 'Sending resumed.', 'success');
      setRestart((k) => k + 1);
    } catch (err) {
      toast(describeError(err), 'error');
    }
  };

  const c = report?.counts;
  const processed = report ? report.total - report.counts.pending : 0;

  return (
    <>
      <PageHeader
        back={{ href: `/campaigns/view/?id=${email.campaign_id}`, label: email.campaign_name }}
        title={email.subject}
        meta={<Badge value={status} />}
        description={`From ${email.from_name ? `${email.from_name} <${email.from_address}>` : email.from_address}${email.topic ? ` · Topic ${email.topic}` : ''}${email.started_at ? ` · Started ${formatDate(email.started_at)}` : ''}`}
        actions={(
          <>
            {status === 'sending' ? <Button icon={Pause} onClick={() => control('pause')}>Pause</Button> : null}
            {status === 'paused' && !noResume.includes(pauseReason || '') ? <Button variant="primary" icon={Play} onClick={() => control('resume')}>Resume</Button> : null}
            {status === 'paused' && pauseReason === 'trial_quota_exhausted' ? <Button variant="primary" onClick={() => showUpgrade()}>Upgrade</Button> : null}
          </>
        )}
      />
      {status === 'paused' ? <div className="mb-5"><Callout tone="warn">{pauseReasons[pauseReason || ''] || 'Paused.'}</Callout></div> : null}
      <Card className="mb-5">
        {report ? (
          <div className="flex flex-col gap-3">
            <div className="flex flex-wrap items-baseline justify-between gap-2">
              <div className="text-sm text-ink-2"><span className="text-2xl font-semibold text-ink tabular-nums">{number(processed)}</span> of {number(report.total)} processed</div>
              <span className="text-xs text-muted">{status === 'done' && email.finished_at ? `Finished ${formatDate(email.finished_at)}` : status === 'sending' ? 'Sending now…' : null}</span>
            </div>
            <Meter value={report.progress} max={100} tone="accent" className={clsx('h-2', status === 'sending' && '[&>div]:animate-pulse')} />
          </div>
        ) : <Skeleton className="h-14" />}
      </Card>
      <div className="mb-5 grid grid-cols-2 gap-3 lg:grid-cols-4">
        {funnel.map(({ key, label, base, tone }) => (
          <div key={key} className="rounded-xl bg-surface p-4 shadow-card">
            <div className="text-[13px] text-muted">{label}</div>
            <div className={clsx('mt-1 text-2xl font-semibold tracking-tight tabular-nums', tone)}>{c ? number(c[key]) : '—'}</div>
            <div className="mt-0.5 text-xs text-muted">{c && base ? `${percent(c[key], c[base])} of ${base}` : ' '}</div>
          </div>
        ))}
      </div>
      <div className="mb-6 grid grid-cols-3 overflow-hidden rounded-xl bg-surface shadow-card sm:grid-cols-6">
        {problems.map(({ key, label }) => (
          <div key={key} className="border-r border-b border-line px-4 py-3 last:border-r-0 sm:border-b-0">
            <div className="text-xs text-muted">{label}</div>
            <div className="mt-0.5 text-lg font-semibold text-ink tabular-nums">{c ? number(c[key]) : '—'}</div>
          </div>
        ))}
      </div>
      <div className="flex flex-col gap-5">
        <Recipients id={email.id} version={version} />
        <Card title="Content" flush>
          <div className="dot-grid bg-surface-2 p-4"><div className="overflow-hidden rounded-lg shadow-card"><EmailFrame html={email.html || ''} /></div></div>
        </Card>
      </div>
    </>
  );
}

export default function EmailView() {
  const router = useRouter();
  const id = router.query.id as string | undefined;
  const [state] = useLoad(() => (id ? api<Email>('GET', `/api/emails/${id}`) : never<Email>()), [id]);
  useEffect(() => {
    if (state.data && state.data.status === 'draft') router.replace(`/emails/edit/?id=${state.data.id}`);
  }, [state.data, router]);
  if (state.error) return <Card><Empty title="Email not found" hint={state.error} /></Card>;
  if (!state.data || state.data.status === 'draft') return <Loading />;
  return <EmailReport key={state.data.id} email={state.data} />;
}
