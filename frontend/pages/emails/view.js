import Link from 'next/link';
import { useRouter } from 'next/router';
import { useEffect, useMemo, useState } from 'react';
import DataTable from '../../components/DataTable';
import { Badge, Card, EmailFrame, Empty, Loading, PageHead, Progress, Stat } from '../../components/ui';
import { api, describeError } from '../../lib/api';
import { formatDate, number } from '../../lib/format';
import { useEmailWatch } from '../../lib/useEmailWatch';
import { useLoad } from '../../lib/useLoad';
import { useSession } from '../../lib/session';
import { useToast } from '../../lib/toast';

const pauseReasons = {
  manual: 'Sending is paused.',
  trial_quota_exhausted: 'Sending stopped because your trial send limit is used up.',
  daily_limit_reached: 'Sending paused because the daily send limit was reached. Resume tomorrow.',
  ses_sending_paused: 'Amazon SES has paused sending for the account. Resume once sending is re-enabled in SES.',
  identity_not_verified: 'Sending stopped because the from domain or address is not verified in Amazon SES. Verify it on the Domains page, then resume.',
  reputation: 'Sending stopped because this workspace exceeded its bounce or complaint rate limit.',
};

const noResume = ['trial_quota_exhausted', 'reputation'];

const statCards = [
  ['sent', 'Sent'], ['delivered', 'Delivered'], ['bounced', 'Bounced'], ['complained', 'Complained'],
  ['unsubscribed', 'Unsubscribed'], ['failed', 'Failed'], ['skipped', 'Skipped'], ['pending', 'Pending'],
];

function outcome(r) {
  if (r.complained_at) return 'complained';
  if (r.bounced_at) return 'bounced';
  if (r.unsubscribed_at) return 'unsubscribed';
  if (r.delivered_at) return 'delivered';
  return '';
}

function Recipients({ id, version }) {
  const [state, reload] = useLoad(() => api('GET', `/api/emails/${id}/recipients?limit=200`), [id]);
  useEffect(() => { if (version) reload(); }, [version]);
  const columns = useMemo(() => [
    { accessorKey: 'email', header: 'Recipient', cell: ({ getValue }) => <span className="cell-strong">{getValue()}</span> },
    { accessorKey: 'status', header: 'Status', cell: ({ getValue }) => <Badge value={getValue()} /> },
    { accessorKey: 'attempts', header: 'Attempts', meta: { className: 'num' } },
    { accessorKey: 'sent_at', header: 'Sent', cell: ({ getValue }) => formatDate(getValue()) },
    { id: 'outcome', header: 'Outcome', accessorFn: outcome, cell: ({ getValue }) => (getValue() ? <Badge value={getValue()} /> : '—') },
    { accessorKey: 'last_error', header: 'Note', cell: ({ getValue }) => (getValue() ? <span className="small muted truncate">{getValue().replace(/_/g, ' ')}</span> : '—') },
  ], []);
  return (
    <Card title="Recipients" flush>
      <DataTable columns={columns} data={state.data ? state.data.recipients : null} error={state.error} empty={{ title: 'No recipients yet' }} />
    </Card>
  );
}

function EmailReport({ email }) {
  const toast = useToast();
  const { showUpgrade } = useSession();
  const [report, setReport] = useState(null);
  const [version, setVersion] = useState(0);
  const [restart, setRestart] = useState(0);
  const status = report ? report.status : email.status;
  const pauseReason = report ? report.pause_reason : email.pause_reason;

  useEmailWatch(email.id, (r) => {
    setReport(r);
    setVersion((v) => v + 1);
  }, restart);

  const control = async (action) => {
    try {
      await api('POST', `/api/emails/${email.id}/${action}`);
      toast(action === 'pause' ? 'Sending paused.' : 'Sending resumed.', 'success');
      setRestart((k) => k + 1);
    } catch (err) {
      toast(describeError(err), 'error');
    }
  };

  return (
    <>
      <PageHead
        back={<Link className="back-link" href={`/campaigns/view/?id=${email.campaign_id}`}>← {email.campaign_name}</Link>}
        title={email.subject}
        subtitle={`From ${email.from_name ? `${email.from_name} <${email.from_address}>` : email.from_address}${email.started_at ? ` · Started ${formatDate(email.started_at)}` : ''}`}
      >
        <Badge value={status} />
        {status === 'sending' ? <button className="btn" type="button" onClick={() => control('pause')}>Pause</button> : null}
        {status === 'paused' && !noResume.includes(pauseReason) ? <button className="btn btn-primary" type="button" onClick={() => control('resume')}>Resume</button> : null}
        {status === 'paused' && pauseReason === 'trial_quota_exhausted' ? <button className="btn btn-primary" type="button" onClick={() => showUpgrade()}>Upgrade</button> : null}
      </PageHead>
      {status === 'paused' ? <div className="callout">{pauseReasons[pauseReason] || 'Paused.'}</div> : null}
      <Card>
        <div className="card-body">
          {report ? (
            <>
              <div className="row-between">
                <span className="muted">{number(report.total - report.counts.pending)} of {number(report.total)} processed</span>
                <span className="small muted">{status === 'done' && email.finished_at ? `Finished ${formatDate(email.finished_at)}` : null}</span>
              </div>
              <Progress percent={report.progress} />
            </>
          ) : <Loading />}
        </div>
      </Card>
      <div className="grid grid-4">
        {statCards.map(([key, label]) => <Stat key={key} label={label} value={report ? number(report.counts[key]) : '—'} />)}
      </div>
      <Recipients id={email.id} version={version} />
      <Card title="Content" flush>
        <EmailFrame html={email.html || ''} />
      </Card>
    </>
  );
}

export default function EmailView() {
  const router = useRouter();
  const id = router.query.id;
  const [state] = useLoad(() => (id ? api('GET', `/api/emails/${id}`) : new Promise(() => {})), [id]);
  useEffect(() => {
    if (state.data && state.data.status === 'draft') router.replace(`/emails/edit/?id=${state.data.id}`);
  }, [state.data, router]);
  if (state.error) return <Card><Empty title="Email not found" hint={state.error} /></Card>;
  if (!state.data || state.data.status === 'draft') return <Loading />;
  return <EmailReport key={state.data.id} email={state.data} />;
}
