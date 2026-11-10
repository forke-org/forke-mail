import { CalendarClock, CreditCard, Globe, HeartPulse, UserRound } from 'lucide-react';
import { FormEvent, useEffect, useState } from 'react';
import { Badge, Button, ButtonLink, Callout, Card, Checkbox, DefinitionRow, EmailFrame, Field, FormError, Input, Loading, Meter, PageHeader, Tabs, Textarea } from '../components/ui';
import { api, describeError } from '../lib/api';
import { number } from '../lib/format';
import { useMe, useSession } from '../lib/session';
import { useToast } from '../lib/toast';
import type { Reengagement } from '../lib/types';
import { useLoad } from '../lib/useLoad';

const reengagementStarter = `<p>Hi {{first_name}},</p>
<p>We haven't heard from you in a while. Do you still want to hear from us?</p>
<p><a href="https://example.com">Yes, keep me subscribed</a></p>
<p>If we don't hear back, we'll stop emailing you in a few weeks.</p>`;

function General() {
  const me = useMe();
  const { trial } = useSession();
  const { sent_this_month: month, monthly_limit: monthLimit, sent_today: today, daily_limit: daily } = me.quota;
  return (
    <div className="flex flex-col gap-5">
      <Card title="Plan and usage" icon={CreditCard} actions={<Badge value={me.workspace.plan} />}>
        <div className="grid gap-5 md:grid-cols-3">
          <div>
            <div className="text-[13px] text-muted">{trial ? 'Trial sends' : 'Sent this month'}</div>
            <div className="mt-1 text-2xl font-semibold text-ink tabular-nums">{number(trial ? me.quota.trial_used : month)}</div>
            <div className="mb-2 text-xs text-muted">{trial ? `of ${number(me.quota.trial_limit)}` : monthLimit > 0 ? `of ${number(monthLimit)} monthly cap` : 'No monthly cap'}</div>
            {trial ? <Meter value={me.quota.trial_used} max={me.quota.trial_limit} /> : monthLimit > 0 ? <Meter value={month} max={monthLimit} /> : null}
          </div>
          <div>
            <div className="text-[13px] text-muted">Sent today</div>
            <div className="mt-1 text-2xl font-semibold text-ink tabular-nums">{number(today)}</div>
            <div className="mb-2 text-xs text-muted">{daily > 0 ? `of ${number(daily)} daily limit` : 'No daily limit'}</div>
            {daily > 0 ? <Meter value={today} max={daily} /> : null}
          </div>
          <div>
            <div className="text-[13px] text-muted">Send rate</div>
            <div className="mt-1 text-2xl font-semibold text-ink tabular-nums">{me.rate_limits.send_per_second}<span className="text-sm font-normal text-muted"> / s</span></div>
            <div className="text-xs text-muted">API {me.rate_limits.api_per_second} requests / s</div>
          </div>
        </div>
        {monthLimit > 0 ? <p className="mt-5 border-t border-line pt-4 text-xs leading-relaxed text-muted">The monthly cap comes with your plan. Broadcasts that would not fit are refused before they start; transactional mail stops at the cap. It resets on the 1st.</p> : null}
      </Card>
      <Card title="Sending identities" icon={Globe} description="Send from any address on a verified domain, or from a verified address." actions={<ButtonLink size="sm" href="/domains/">Manage</ButtonLink>}>
        {me.domains.length ? me.domains.map((d) => (
          <DefinitionRow key={d} label={<code className="font-mono text-[13px] text-ink">{d}</code>}><Badge value="verified" /></DefinitionRow>
        )) : <p className="text-[13px] text-muted">No verified identities yet. Add one on the Domains page.</p>}
      </Card>
      {me.user ? (
        <Card title="Account" icon={UserRound}>
          <DefinitionRow label="Name">{me.user.name}</DefinitionRow>
          <DefinitionRow label="Email">{me.user.email}</DefinitionRow>
          <DefinitionRow label="Role"><Badge value={me.user.role} /></DefinitionRow>
          <DefinitionRow label="Workspace">{me.workspace.name}</DefinitionRow>
          <p className="mt-3 text-xs text-muted">Names, passwords and roles are managed by your workspace owner.</p>
        </Card>
      ) : null}
    </div>
  );
}

function ReengagementSettings() {
  const toast = useToast();
  const me = useMe();
  const { canManage } = useSession();
  const [state] = useLoad(() => api<Reengagement>('GET', '/api/settings/reengagement'), []);
  const [form, setForm] = useState<Reengagement | null>(null);
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    if (!state.data) return;
    const d = state.data;
    const domain = me.domains.find((x) => !x.includes('@')) || '';
    setForm({
      ...d,
      from_name: d.from_name || me.workspace.name,
      from_address: d.from_address || (domain ? `hello@${domain}` : ''),
      subject: d.subject || 'Still want to hear from us?',
      html: d.html || reengagementStarter,
    });
  }, [state.data]);

  if (state.error) return <Card><p className="text-sm text-danger">{state.error}</p></Card>;
  if (!form) return <Loading />;

  const set = <K extends keyof Reengagement>(key: K, value: Reengagement[K]) => setForm((f) => (f ? { ...f, [key]: value } : f));
  const submit = async (e: FormEvent) => {
    e.preventDefault();
    setBusy(true);
    setError('');
    try {
      const saved = await api<Reengagement>('PUT', '/api/settings/reengagement', form);
      setForm(saved);
      toast(saved.enabled ? 'Re-engagement is on.' : 'Re-engagement settings saved.', 'success');
    } catch (err) {
      setError(describeError(err));
    } finally {
      setBusy(false);
    }
  };

  return (
    <form onSubmit={submit} className="flex flex-col gap-5">
      <Callout tone="accent" title="How re-engagement works">
        Contacts who received at least {form.min_broadcasts} broadcasts but haven&apos;t opened or clicked anything in {form.inactive_days} days get this email once.
        If they don&apos;t click within {form.grace_days} days, they are suppressed as inactive. This protects your sender reputation.
      </Callout>
      <Card title="Schedule" icon={CalendarClock} actions={<Checkbox label="Enabled" checked={form.enabled} disabled={!canManage} onChange={(e) => set('enabled', e.target.checked)} />}>
        <div className="grid gap-4 sm:grid-cols-3">
          <Field label="Inactive for (days)" hint="30 to 730"><Input type="number" min={30} max={730} value={form.inactive_days} disabled={!canManage} onChange={(e) => set('inactive_days', Number(e.target.value))} /></Field>
          <Field label="Grace period (days)" hint="7 to 90"><Input type="number" min={7} max={90} value={form.grace_days} disabled={!canManage} onChange={(e) => set('grace_days', Number(e.target.value))} /></Field>
          <Field label="Minimum broadcasts" hint="1 to 50"><Input type="number" min={1} max={50} value={form.min_broadcasts} disabled={!canManage} onChange={(e) => set('min_broadcasts', Number(e.target.value))} /></Field>
        </div>
      </Card>
      <Card title="Email" icon={HeartPulse}>
        <div className="grid gap-5 xl:grid-cols-2">
          <div className="flex flex-col gap-4">
            <div className="grid gap-4 sm:grid-cols-2">
              <Field label="From name"><Input value={form.from_name} disabled={!canManage} onChange={(e) => set('from_name', e.target.value)} /></Field>
              <Field label="From address"><Input type="email" required value={form.from_address} disabled={!canManage} onChange={(e) => set('from_address', e.target.value)} /></Field>
            </div>
            <Field label="Reply-to" hint="Optional."><Input type="email" value={form.reply_to} disabled={!canManage} onChange={(e) => set('reply_to', e.target.value)} /></Field>
            <Field label="Subject"><Input required value={form.subject} disabled={!canManage} onChange={(e) => set('subject', e.target.value)} /></Field>
            <Field label="HTML" hint="Must contain at least one https link. A click on any link keeps the contact subscribed."><Textarea code rows={10} value={form.html} disabled={!canManage} onChange={(e) => set('html', e.target.value)} /></Field>
          </div>
          <div className="dot-grid overflow-hidden rounded-xl bg-surface-2 p-3 shadow-[inset_0_0_0_1px_var(--line)]">
            <div className="overflow-hidden rounded-lg shadow-card"><EmailFrame html={form.html} className="h-[460px]" /></div>
          </div>
        </div>
      </Card>
      <FormError>{error}</FormError>
      {canManage ? (
        <div className="flex justify-end"><Button variant="primary" type="submit" loading={busy}>Save re-engagement</Button></div>
      ) : <p className="text-xs text-muted">Only workspace owners can change re-engagement.</p>}
    </form>
  );
}

export default function Settings() {
  const [tab, setTab] = useState<'general' | 'reengagement'>('general');
  return (
    <>
      <PageHeader title="Settings" description="Plan, usage, sending identities and list hygiene for this workspace." />
      <Tabs className="mb-5" value={tab} onChange={setTab} options={[['general', 'General'], ['reengagement', 'Re-engagement']]} />
      {tab === 'general' ? <General /> : <ReengagementSettings />}
    </>
  );
}
