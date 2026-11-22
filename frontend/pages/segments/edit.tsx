import { Plus, Save, Trash2, X } from 'lucide-react';
import { useRouter } from 'next/router';
import { FormEvent, useEffect, useState } from 'react';
import { Badge, Button, Card, Field, FormError, Input, Loading, PageHeader, Select, Tabs } from '../../components/ui';
import { api, describeError } from '../../lib/api';
import { number } from '../../lib/format';
import { baseFields, kindOf, needsDays, needsValue, ops } from '../../lib/segmentFields';
import { useToast } from '../../lib/toast';
import type { DataUsage, Segment, SegmentRule, Tag } from '../../lib/types';
import { useDebounced, useLoad } from '../../lib/useLoad';

type Preview = { size: number; sample: { id: string; email: string; first_name: string; last_name: string; status: string }[] };

function RuleRow({ rule, fields, tags, events, onChange, onRemove }: {
  rule: SegmentRule; fields: { value: string; label: string }[]; tags: Tag[]; events: DataUsage[]; onChange: (r: SegmentRule) => void; onRemove: () => void;
}) {
  const kind = kindOf(rule.field);
  const setField = (field: string) => {
    const k = kindOf(field);
    onChange({ field, op: ops[k][0][0], value: k === 'status' ? 'active' : '', days: k === 'date' ? 30 : undefined });
  };
  return (
    <div className="grid gap-2 rounded-[6px] bg-surface-2 p-3 shadow-[inset_0_0_0_1px_var(--line)] sm:grid-cols-[minmax(0,1fr)_minmax(0,1fr)_minmax(0,1.3fr)_auto] sm:items-center">
      <Select value={rule.field} onChange={(e) => setField(e.target.value)} aria-label="Field">
        {fields.map((f) => <option key={f.value} value={f.value}>{f.label}</option>)}
      </Select>
      <Select value={rule.op} onChange={(e) => onChange({ ...rule, op: e.target.value })} aria-label="Condition">
        {ops[kind].map(([v, l]) => <option key={v} value={v}>{l}</option>)}
      </Select>
      <div className="flex items-center gap-2">
        {needsValue(rule.op) ? (
          kind === 'status' ? (
            <Select value={rule.value} onChange={(e) => onChange({ ...rule, value: e.target.value })} aria-label="Status">
              {['active', 'unsubscribed', 'bounced', 'complained', 'invalid'].map((s) => <option key={s} value={s}>{s}</option>)}
            </Select>
          ) : kind === 'tag' ? (
            <Select value={rule.value} onChange={(e) => onChange({ ...rule, value: e.target.value })} aria-label="Tag">
              <option value="">Pick a tag</option>
              {tags.map((t) => <option key={t.id} value={t.name}>{t.name}</option>)}
            </Select>
          ) : (
            <Input type={kind === 'date' ? 'date' : 'text'} list={kind === 'event' ? 'segment-events' : undefined} placeholder={kind === 'event' ? 'user.signed-up' : 'value'}
              value={rule.value} onChange={(e) => onChange({ ...rule, value: e.target.value })} />
          )
        ) : null}
        {needsDays(kind, rule.op) ? (
          <div className="flex items-center gap-1.5">
            <Input type="number" min={kind === 'event' ? 0 : 1} max={3650} className="w-20" value={rule.days ?? ''} placeholder={kind === 'event' ? 'any' : ''}
              onChange={(e) => onChange({ ...rule, days: e.target.value === '' ? undefined : Number(e.target.value) })} />
            <span className="shrink-0 text-xs text-muted">{kind === 'event' ? 'days (blank = ever)' : 'days'}</span>
          </div>
        ) : null}
      </div>
      <Button size="sm" variant="ghost" icon={X} aria-label="Remove rule" onClick={onRemove} />
      {kind === 'event' ? <datalist id="segment-events">{events.map((e) => <option key={e.name} value={e.name} />)}</datalist> : null}
    </div>
  );
}

function GroupRow({ rule, fields, tags, events, onChange, onRemove }: {
  rule: SegmentRule; fields: { value: string; label: string }[]; tags: Tag[]; events: DataUsage[]; onChange: (r: SegmentRule) => void; onRemove: () => void;
}) {
  const rules = rule.rules || [];
  const set = (next: SegmentRule[]) => onChange({ ...rule, rules: next });
  return (
    <div className="flex flex-col gap-2 rounded-[6px] p-3 shadow-[inset_0_0_0_1px_var(--line-strong)]">
      <div className="flex items-center justify-between gap-2">
        <div className="flex items-center gap-2 text-[13px] text-ink-2">
          Matches
          <Tabs value={rule.op as 'any' | 'all'} onChange={(op) => onChange({ ...rule, op })} options={[['any', 'any of'], ['all', 'all of']]} />
          these
        </div>
        <Button size="sm" variant="ghost" icon={X} aria-label="Remove group" onClick={onRemove} />
      </div>
      {rules.map((r, i) => (
        <RuleRow key={i} rule={r} fields={fields} tags={tags} events={events}
          onChange={(next) => set(rules.map((x, j) => (j === i ? next : x)))} onRemove={() => set(rules.filter((_, j) => j !== i))} />
      ))}
      <div><Button size="sm" icon={Plus} onClick={() => set([...rules, { field: 'email', op: 'contains', value: '' }])}>Add rule to group</Button></div>
    </div>
  );
}

export default function SegmentEditor() {
  const router = useRouter();
  const toast = useToast();
  const id = typeof router.query.id === 'string' ? router.query.id : '';
  const [name, setName] = useState('');
  const [match, setMatch] = useState<'all' | 'any'>('all');
  const [rules, setRules] = useState<SegmentRule[]>([{ field: 'last_engaged_at', op: 'within_days', value: '', days: 30 }]);
  const [loaded, setLoaded] = useState(false);
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  const [preview, setPreview] = useState<Preview | null>(null);
  const [previewError, setPreviewError] = useState('');
  const [meta] = useLoad(() => Promise.all([api<{ tags: Tag[] }>('GET', '/api/tags'), api<{ fields: DataUsage[]; events: DataUsage[] }>('GET', '/api/data')]), []);
  const tags = meta.data ? meta.data[0].tags : [];
  const data = meta.data ? meta.data[1] : { fields: [], events: [] };
  const fields = [...baseFields, ...data.fields.map((f) => ({ value: `data.${f.name}`, label: `Field: ${f.name}`, kind: 'data' as const }))];
  rules.flatMap((r) => (r.field === 'group' ? r.rules || [] : [r])).forEach((r) => { if (!fields.some((f) => f.value === r.field)) fields.push({ value: r.field, label: `Field: ${r.field.replace(/^data\./, '')}`, kind: 'data' }); });

  useEffect(() => {
    if (!router.isReady) return;
    if (!id) { setLoaded(true); return; }
    api<Segment>('GET', `/api/segments/${id}`).then((s) => { setName(s.name); setMatch(s.match); setRules(s.rules); setLoaded(true); }, (err) => { setError(describeError(err)); setLoaded(true); });
  }, [router.isReady, id]);

  const query = useDebounced(JSON.stringify({ match, rules }), 400);
  useEffect(() => {
    if (!loaded) return;
    api<Preview>('POST', '/api/segments/preview', JSON.parse(query)).then((p) => { setPreview(p); setPreviewError(''); }, (err) => setPreviewError(describeError(err)));
  }, [query, loaded]);

  const save = async (e: FormEvent) => {
    e.preventDefault();
    setBusy(true);
    setError('');
    try {
      const s = await api<Segment>(id ? 'PUT' : 'POST', id ? `/api/segments/${id}` : '/api/segments', { name, match, rules });
      toast(`Saved. ${number(s.size)} contacts match.`, 'success');
      if (!id) router.replace(`/segments/edit/?id=${s.id}`);
    } catch (err) { setError(describeError(err)); } finally { setBusy(false); }
  };
  const remove = async () => {
    try { await api('DELETE', `/api/segments/${id}`); router.push('/segments/'); } catch (err) { setError(describeError(err)); }
  };
  const [customField, setCustomField] = useState('');

  if (!loaded) return <Loading />;
  return (
    <form onSubmit={save}>
      <PageHeader title={id ? name || 'Segment' : 'New segment'} back={{ href: '/segments/', label: 'Segments' }}
        actions={<>{id ? <Button icon={Trash2} onClick={remove}>Delete</Button> : null}<Button variant="primary" type="submit" icon={Save} loading={busy}>Save segment</Button></>} />
      <FormError>{error}</FormError>
      <div className="grid gap-5 xl:grid-cols-[minmax(0,1.5fr)_minmax(0,1fr)]">
        <Card title="Rules" actions={<Tabs value={match} onChange={setMatch} options={[['all', 'Match all'], ['any', 'Match any']]} />}>
          <div className="flex flex-col gap-4">
            <Field label="Name"><Input required maxLength={100} placeholder="Active in the last 30 days" value={name} onChange={(e) => setName(e.target.value)} /></Field>
            <div className="flex flex-col gap-2">
              {rules.map((r, i) => {
                const change = (next: SegmentRule) => setRules((rs) => rs.map((x, j) => (j === i ? next : x)));
                const remove = () => setRules((rs) => rs.filter((_, j) => j !== i));
                return r.field === 'group'
                  ? <GroupRow key={i} rule={r} fields={fields} tags={tags} events={data.events} onChange={change} onRemove={remove} />
                  : <RuleRow key={i} rule={r} fields={fields} tags={tags} events={data.events} onChange={change} onRemove={remove} />;
              })}
              {!rules.length ? <p className="text-[13px] text-muted">No rules: every contact matches.</p> : null}
            </div>
            <div className="flex flex-wrap items-center gap-2">
              <Button size="sm" icon={Plus} onClick={() => setRules((rs) => [...rs, { field: 'email', op: 'contains', value: '' }])}>Add rule</Button>
              <Button size="sm" icon={Plus} onClick={() => setRules((rs) => [...rs, { field: 'group', op: 'any', value: '', rules: [{ field: 'last_engaged_at', op: 'within_days', value: '', days: 90 }] }])}>Add group</Button>
              <span className="text-xs text-faint">or a custom field:</span>
              <div className="w-40"><Input placeholder="plan" value={customField} onChange={(e) => setCustomField(e.target.value.replace(/[^A-Za-z0-9_]/g, ''))} /></div>
              <Button size="sm" disabled={!customField} onClick={() => { setRules((rs) => [...rs, { field: `data.${customField}`, op: 'equals', value: '' }]); setCustomField(''); }}>Add</Button>
            </div>
          </div>
        </Card>
        <Card title="Matching contacts" actions={preview ? <Badge value={`${number(preview.size)} contacts`} tone="accent" /> : null} description="Updates as you edit. Saved segments refresh before every send.">
          {previewError ? <FormError>{previewError}</FormError> : !preview ? <Loading /> : preview.sample.length ? (
            <div className="divide-y divide-line">
              {preview.sample.map((c) => (
                <div key={c.id} className="flex items-center justify-between gap-3 py-2.5 first:pt-0 last:pb-0">
                  <div className="min-w-0"><div className="truncate text-[13px] text-ink">{c.email}</div><div className="truncate text-xs text-muted">{[c.first_name, c.last_name].filter(Boolean).join(' ') || '-'}</div></div>
                  <Badge value={c.status} />
                </div>
              ))}
              {preview.size > preview.sample.length ? <p className="pt-2.5 text-xs text-muted">and {number(preview.size - preview.sample.length)} more</p> : null}
            </div>
          ) : <p className="text-[13px] text-muted">No contacts match yet.</p>}
        </Card>
      </div>
    </form>
  );
}
