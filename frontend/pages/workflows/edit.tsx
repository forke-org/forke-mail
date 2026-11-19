import { ArrowDown, Clock, Mail, Plus, Save, Trash2, X, Zap } from 'lucide-react';
import { useRouter } from 'next/router';
import { FormEvent, useEffect, useMemo, useState } from 'react';
import DataTable, { Columns } from '../../components/DataTable';
import { Badge, Button, Card, Checkbox, CopyButton, Field, FormError, Input, Loading, PageHeader, Select } from '../../components/ui';
import { api, describeError } from '../../lib/api';
import { formatDate } from '../../lib/format';
import { useMe } from '../../lib/session';
import { useToast } from '../../lib/toast';
import type { DataUsage, EmailTemplate, Workflow, WorkflowStep } from '../../lib/types';
import { useLoad } from '../../lib/useLoad';

type Run = { id: string; email: string; step: number; status: string; next_run_at: string; last_error: string | null; created_at: string };

const units: [string, number][] = [['minutes', 1], ['hours', 60], ['days', 1440]];

function waitParts(minutes: number): [number, number] {
  for (const [, m] of [...units].reverse()) if (minutes % m === 0) return [minutes / m, m];
  return [minutes, 1];
}

export default function WorkflowEditor() {
  const router = useRouter();
  const toast = useToast();
  const me = useMe();
  const id = typeof router.query.id === 'string' ? router.query.id : '';
  const domain = me.domains.find((d) => !d.includes('@')) || '';
  const defaultFrom = me.domains.find((d) => d.includes('@')) || (domain ? `hello@${domain}` : '');
  const [wf, setWf] = useState<Pick<Workflow, 'name' | 'trigger_event' | 'steps' | 'enabled' | 'allow_reentry'>>({ name: 'Welcome series', trigger_event: 'user.signed-up', steps: [], enabled: false, allow_reentry: false });
  const [loaded, setLoaded] = useState(false);
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  const [meta] = useLoad(() => Promise.all([api<{ templates: EmailTemplate[] }>('GET', '/api/templates'), api<{ events: DataUsage[] }>('GET', '/api/data')]), []);
  const templates = meta.data ? meta.data[0].templates : [];
  const events = meta.data ? meta.data[1].events : [];
  const [runs, reloadRuns] = useLoad(() => (id ? api<{ runs: Run[] }>('GET', `/api/workflows/${id}/runs`) : Promise.resolve({ runs: [] as Run[] })), [id]);

  useEffect(() => {
    if (!router.isReady) return;
    if (!id) { setLoaded(true); return; }
    api<Workflow>('GET', `/api/workflows/${id}`).then((w) => { setWf(w); setLoaded(true); }, (err) => { setError(describeError(err)); setLoaded(true); });
  }, [router.isReady, id]);

  const setStep = (i: number, s: WorkflowStep) => setWf((w) => ({ ...w, steps: w.steps.map((x, j) => (j === i ? s : x)) }));
  const removeStep = (i: number) => setWf((w) => ({ ...w, steps: w.steps.filter((_, j) => j !== i) }));
  const addStep = (type: 'wait' | 'send') => setWf((w) => ({ ...w, steps: [...w.steps, type === 'wait' ? { type, minutes: 1440 } : { type, template_id: templates[0]?.id || '', from_address: defaultFrom, from_name: me.workspace.name }] }));

  const save = async (e: FormEvent) => {
    e.preventDefault();
    setBusy(true);
    setError('');
    try {
      const saved = await api<Workflow>(id ? 'PUT' : 'POST', id ? `/api/workflows/${id}` : '/api/workflows', wf);
      toast(saved.enabled ? 'Workflow is live.' : 'Workflow saved. Turn it on when you are ready.', 'success');
      if (!id) router.replace(`/workflows/edit/?id=${saved.id}`); else reloadRuns();
    } catch (err) { setError(describeError(err)); } finally { setBusy(false); }
  };
  const remove = async () => {
    try { await api('DELETE', `/api/workflows/${id}`); router.push('/workflows/'); } catch (err) { setError(describeError(err)); }
  };

  const snippet = `curl -X POST ${typeof window === 'undefined' ? 'https://mail.forke.space' : window.location.origin}/api/events \\
  -H "Authorization: Bearer $FORKE_MAIL_API_KEY" \\
  -H "Content-Type: application/json" \\
  -d '{"email":"user@example.com","event":"${wf.trigger_event}","data":{"plan":"pro"}}'`;

  const runColumns = useMemo<Columns<Run>>(() => [
    { accessorKey: 'email', header: 'Contact' },
    { accessorKey: 'status', header: 'Status', cell: ({ getValue }) => <Badge value={getValue()} /> },
    { accessorKey: 'step', header: 'Step', cell: ({ getValue }) => <span className="tabular-nums">{Number(getValue()) + 1}</span> },
    { accessorKey: 'last_error', header: 'Note', cell: ({ getValue }) => (getValue() ? <span className="font-mono text-xs text-muted">{getValue()}</span> : <span className="text-faint">-</span>) },
    { accessorKey: 'created_at', header: 'Started', cell: ({ getValue }) => <span className="text-muted">{formatDate(getValue())}</span> },
  ], []);

  if (!loaded) return <Loading />;
  return (
    <form onSubmit={save}>
      <PageHeader title={id ? wf.name : 'New workflow'} back={{ href: '/workflows/', label: 'Workflows' }} meta={id ? <Badge value={wf.enabled ? 'live' : 'off'} tone={wf.enabled ? 'green' : 'neutral'} /> : null}
        actions={<>{id ? <Button icon={Trash2} onClick={remove}>Delete</Button> : null}<Button variant="primary" type="submit" icon={Save} loading={busy}>Save workflow</Button></>} />
      <FormError>{error}</FormError>
      <div className="grid gap-5 xl:grid-cols-[minmax(0,1.4fr)_minmax(0,1fr)]">
        <div className="flex flex-col gap-5">
          <Card title="Trigger" icon={Zap}>
            <div className="flex flex-col gap-4">
              <div className="grid gap-4 sm:grid-cols-2">
                <Field label="Name"><Input required maxLength={100} value={wf.name} onChange={(e) => setWf({ ...wf, name: e.target.value })} /></Field>
                <Field label="When this event happens"><Input required list="workflow-events" className="font-mono" value={wf.trigger_event} onChange={(e) => setWf({ ...wf, trigger_event: e.target.value.trim() })} /></Field>
              </div>
              <datalist id="workflow-events">{events.map((ev) => <option key={ev.name} value={ev.name} />)}</datalist>
              <div className="flex flex-wrap gap-5">
                <Checkbox label="Live" checked={wf.enabled} onChange={(e) => setWf({ ...wf, enabled: e.target.checked })} />
                <Checkbox label="Let a contact go through it more than once" checked={wf.allow_reentry} onChange={(e) => setWf({ ...wf, allow_reentry: e.target.checked })} />
              </div>
            </div>
          </Card>
          <Card title="Steps" description="Run top to bottom for each contact. Unsubscribed or bounced contacts stop before a send.">
            <div className="flex flex-col items-stretch">
              {wf.steps.map((s, i) => (
                <div key={i} className="flex flex-col items-stretch">
                  {i > 0 ? <ArrowDown className="mx-auto my-1.5 size-4 text-faint" /> : null}
                  <div className="flex items-start gap-3 rounded-[6px] bg-surface-2 p-3 shadow-[inset_0_0_0_1px_var(--line)]">
                    <span className="mt-1.5 grid size-7 shrink-0 place-items-center rounded-[5px] bg-surface-3 text-ink-2">{s.type === 'wait' ? <Clock className="size-3.5" /> : <Mail className="size-3.5" />}</span>
                    {s.type === 'wait' ? (() => {
                      const [n, unit] = waitParts(s.minutes || 60);
                      return (
                        <div className="flex flex-1 flex-wrap items-center gap-2 pt-0.5">
                          <span className="text-[13px] text-ink">Wait</span>
                          <Input type="number" min={1} className="w-24" value={n} onChange={(e) => setStep(i, { type: 'wait', minutes: Math.max(1, Number(e.target.value)) * unit })} />
                          <Select value={String(unit)} onChange={(e) => setStep(i, { type: 'wait', minutes: n * Number(e.target.value) })} aria-label="Unit" className="w-32">
                            {units.map(([l, m]) => <option key={l} value={String(m)}>{l}</option>)}
                          </Select>
                        </div>
                      );
                    })() : (
                      <div className="grid flex-1 gap-2 sm:grid-cols-2">
                        <Field label="Template" className="sm:col-span-2">
                          <Select value={s.template_id || ''} onChange={(e) => setStep(i, { ...s, template_id: e.target.value })} aria-label="Template">
                            <option value="">Pick a template</option>
                            {templates.map((t) => <option key={t.id} value={t.id}>{t.name}</option>)}
                          </Select>
                        </Field>
                        <Field label="From address"><Input type="email" required value={s.from_address || ''} onChange={(e) => setStep(i, { ...s, from_address: e.target.value })} /></Field>
                        <Field label="From name"><Input value={s.from_name || ''} onChange={(e) => setStep(i, { ...s, from_name: e.target.value })} /></Field>
                      </div>
                    )}
                    <Button size="sm" variant="ghost" icon={X} aria-label="Remove step" onClick={() => removeStep(i)} />
                  </div>
                </div>
              ))}
              {!wf.steps.length ? <p className="text-[13px] text-muted">No steps yet. Add a wait or an email.</p> : null}
              <div className="mt-3 flex gap-2">
                <Button size="sm" icon={Plus} onClick={() => addStep('send')} disabled={!templates.length}>Send email</Button>
                <Button size="sm" icon={Clock} onClick={() => addStep('wait')}>Wait</Button>
                {!templates.length ? <span className="self-center text-xs text-muted">Create a template first to send email.</span> : null}
              </div>
              <p className="mt-3 text-xs text-muted">Templates get <span className="font-mono">{'{{email}}'}</span>, <span className="font-mono">{'{{first_name}}'}</span>, <span className="font-mono">{'{{last_name}}'}</span>, the contact&apos;s custom fields and the event&apos;s data.</p>
            </div>
          </Card>
        </div>
        <div className="flex flex-col gap-5">
          <Card title="Fire the event from your app">
            <div className="overflow-hidden rounded-[6px] bg-surface-2 shadow-[inset_0_0_0_1px_var(--line)]">
              <div className="flex items-center justify-between gap-3 border-b border-line px-3 py-2"><span className="font-mono text-xs text-muted">POST /api/events</span><CopyButton value={snippet} label="Copy" /></div>
              <pre className="overflow-x-auto p-4 font-mono text-[12px] leading-relaxed text-ink-2">{snippet}</pre>
            </div>
          </Card>
          {id ? (
            <Card title="Recent runs" flush>
              <DataTable columns={runColumns} data={runs.data ? runs.data.runs : null} error={runs.error} noun="runs" getRowId={(r) => r.id}
                empty={{ title: 'No runs yet', hint: 'Runs appear when the trigger event arrives for a contact.' }} />
            </Card>
          ) : null}
        </div>
      </div>
    </form>
  );
}
