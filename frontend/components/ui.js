import { useState } from 'react';

const badgeTones = {
  active: 'green', sent: 'green', delivered: 'green', done: 'green', verified: 'green', paid: 'accent',
  sending: 'blue', queued: 'blue', scheduled: 'blue', trial: 'amber', pending: 'amber', delayed: 'amber', soft_bounced: 'amber',
  paused: 'amber', skipped: 'amber', unsubscribed: 'amber',
  bounced: 'red', complained: 'red', failed: 'red', invalid: 'red', revoked: 'red', rejected: 'red', sending_paused: 'red',
  full_access: 'accent', sending_access: 'blue',
};

const badgeLabels = { done: 'sent', full_access: 'full access', sending_access: 'sending access', soft_bounced: 'soft bounce' };

export function Badge({ value, tone }) {
  const t = tone || badgeTones[value];
  return <span className={t ? `badge badge-${t}` : 'badge'}><span className="badge-dot" />{badgeLabels[value] || String(value).replace(/_/g, ' ')}</span>;
}

export function Progress({ percent }) {
  const value = Math.max(0, Math.min(100, Number(percent) || 0));
  return (
    <div className="progress">
      <div className="meter"><div className="meter-fill" style={{ width: `${value}%` }} /></div>
      <span className="progress-label">{Math.round(value)}%</span>
    </div>
  );
}

export function PageHead({ title, subtitle, back, children }) {
  return (
    <div className="page-head">
      <div>
        {back}
        <h1>{title}</h1>
        {subtitle ? <p className="muted">{subtitle}</p> : null}
      </div>
      {children ? <div className="actions">{children}</div> : null}
    </div>
  );
}

export function Card({ title, description, actions, children, flush }) {
  return (
    <section className={`card${flush ? ' card-flush' : ''}`}>
      {title ? (
        <div className="card-head">
          <div>
            <h2>{title}</h2>
            {description ? <p className="muted small">{description}</p> : null}
          </div>
          {actions ? <div className="actions">{actions}</div> : null}
        </div>
      ) : null}
      {children}
    </section>
  );
}

export function Field({ label, hint, children }) {
  return (
    <label className="field">
      <span>{label}</span>
      {children}
      {hint ? <span className="field-hint">{hint}</span> : null}
    </label>
  );
}

export function Loading({ label = 'Loading…' }) {
  return <div className="loading"><span className="spinner" />{label}</div>;
}

export function Empty({ title, hint, action }) {
  return (
    <div className="empty">
      <div className="empty-title">{title}</div>
      {hint ? <p className="muted">{hint}</p> : null}
      {action || null}
    </div>
  );
}

export function Stat({ label, value, hint }) {
  return (
    <div className="card stat">
      <span className="stat-label">{label}</span>
      <span className="stat-value">{value}</span>
      {hint ? <span className="small muted">{hint}</span> : null}
    </div>
  );
}

export function EmailFrame({ html }) {
  return <iframe className="preview-frame" sandbox="" referrerPolicy="no-referrer" title="Email preview" srcDoc={html} />;
}

export function Tabs({ value, options, onChange }) {
  return (
    <div className="tabs" role="tablist">
      {options.map(([v, label]) => (
        <button key={v} type="button" role="tab" aria-selected={v === value} className={`tab${v === value ? ' active' : ''}`} onClick={() => onChange(v)}>{label}</button>
      ))}
    </div>
  );
}

export function CopyButton({ value }) {
  const [copied, setCopied] = useState(false);
  const copy = async () => {
    try {
      await navigator.clipboard.writeText(value);
      setCopied(true);
      setTimeout(() => setCopied(false), 1500);
    } catch {
      setCopied(false);
    }
  };
  return <button className="btn btn-ghost btn-sm" type="button" onClick={copy}>{copied ? 'Copied' : 'Copy'}</button>;
}
