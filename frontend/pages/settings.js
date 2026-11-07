import Link from 'next/link';
import { Badge, Card, PageHead } from '../components/ui';
import { number } from '../lib/format';
import { useSession } from '../lib/session';

const plans = [
  { id: 'trial', name: 'Trial', price: 'Free', features: ['100 emails total', '1 email per second', 'Sending access API keys'] },
  { id: 'paid', name: 'Pro', price: 'Contact us', features: ['Daily sending limits', 'Up to 14 emails per second', 'Full access API keys'] },
];

export default function Settings() {
  const { me, trial } = useSession();
  const used = me.quota.trial_used;
  const limit = me.quota.trial_limit;
  return (
    <>
      <PageHead title="Settings" subtitle="Your plan, usage, sending domains and account." />
      <Card title="Plan and usage" description={trial ? `${number(used)} of ${number(limit)} trial emails used.` : `${number(me.quota.sent_today)} of ${number(me.quota.daily_limit)} emails sent today.`}>
        <div className="card-body">
          {trial ? <div className="meter meter-lg"><div className={`meter-fill${used >= limit ? ' full' : used / limit >= 0.8 ? ' warn' : ''}`} style={{ width: `${limit ? Math.min(100, (used / limit) * 100) : 100}%` }} /></div> : null}
          <div className="plans">
            {plans.map((p) => {
              const current = p.id === me.workspace.plan;
              return (
                <div key={p.id} className={`plan${current ? ' current' : ''}`}>
                  <div className="row-between"><strong>{p.name}</strong>{current ? <Badge value="current" tone="accent" /> : null}</div>
                  <div className="plan-price">{p.price}</div>
                  <ul>{p.features.map((f) => <li key={f}>{f}</li>)}</ul>
                  {!current && p.id === 'paid' ? <a className="btn btn-primary btn-block" href="mailto:sales@forke.space?subject=Upgrade%20Forke%20Mail">Contact sales to upgrade</a> : null}
                </div>
              );
            })}
          </div>
        </div>
      </Card>
      <Card title="Sending identities" description="You can send from any address on a verified domain, or from a verified address." actions={<Link className="btn btn-sm" href="/domains/">Manage</Link>}>
        <div className="card-body">
          {me.domains.map((d) => (
            <div key={d} className="row-between list-row"><code>{d}</code><Badge value="verified" /></div>
          ))}
          {!me.domains.length ? <p className="small muted">No verified identities yet. Add one on the Domains page.</p> : null}
        </div>
      </Card>
      <Card title="Account">
        <div className="card-body">
          <div className="row-between list-row"><span className="muted">Name</span><span>{me.user.name}</span></div>
          <div className="row-between list-row"><span className="muted">Email</span><span>{me.user.email}</span></div>
          <div className="row-between list-row"><span className="muted">Workspace</span><span>{me.workspace.name}</span></div>
        </div>
      </Card>
    </>
  );
}
