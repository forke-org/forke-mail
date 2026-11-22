import { Save, Trash2 } from 'lucide-react';
import { useRouter } from 'next/router';
import { FormEvent, useEffect, useMemo, useState } from 'react';
import { SendSnippetTemplate } from '../../components/TemplateSnippet';
import { Button, Card, EmailFrame, Field, FormError, Input, Loading, PageHeader, Select, Tabs, Textarea } from '../../components/ui';
import { api, describeError } from '../../lib/api';
import { useToast } from '../../lib/toast';
import type { EmailTemplate } from '../../lib/types';

const starter = `<div style="font-family:-apple-system,Segoe UI,Helvetica,Arial,sans-serif;font-size:15px;line-height:1.6;color:#111;max-width:520px">
  <p>Hi {{name}},</p>
  <p>Your sign-in code is:</p>
  <p style="font-size:26px;font-weight:600;letter-spacing:4px">{{code}}</p>
  <p style="color:#666;font-size:13px">It expires in 15 minutes. If this wasn't you, ignore this email.</p>
</div>`;

const placeholder = /\{\{\{\s*([A-Za-z_][A-Za-z0-9_.]{0,63})\s*\}\}\}|\{\{\s*([A-Za-z_][A-Za-z0-9_.]{0,63})\s*\}\}/g;

function variablesOf(...parts: string[]) {
  const out = new Set<string>();
  for (const p of parts) for (const m of p.matchAll(placeholder)) out.add(m[1] || m[2]);
  return [...out].sort();
}

function fill(s: string, data: Record<string, string>, escape: boolean) {
  return s.replace(placeholder, (match, raw: string | undefined, plain: string | undefined) => {
    const key = raw || plain || '';
    if (!(key in data) || data[key] === '') return match;
    const v = data[key];
    return escape && !raw ? v.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;') : v;
  });
}

export default function TemplateEditor() {
  const router = useRouter();
  const toast = useToast();
  const id = typeof router.query.id === 'string' ? router.query.id : '';
  const [form, setForm] = useState({ name: 'Sign-in code', kind: 'transactional', subject: 'Your code is {{code}}', html: starter, text: '' });
  const [sample, setSample] = useState<Record<string, string>>({ name: 'Ada', code: '482913' });
  const [view, setView] = useState<'preview' | 'api'>('preview');
  const [loaded, setLoaded] = useState(false);
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    if (!router.isReady) return;
    if (!id) { setLoaded(true); return; }
    api<EmailTemplate>('GET', `/api/templates/${id}`).then((t) => {
      setForm({ name: t.name, kind: t.kind, subject: t.subject, html: t.html, text: t.text });
      setSample({});
      setLoaded(true);
    }, (err) => { setError(describeError(err)); setLoaded(true); });
  }, [router.isReady, id]);

  const vars = useMemo(() => variablesOf(form.subject, form.html, form.text), [form.subject, form.html, form.text]);
  const set = (key: keyof typeof form) => (e: React.ChangeEvent<HTMLInputElement | HTMLTextAreaElement>) => setForm((f) => ({ ...f, [key]: e.target.value }));

  const save = async (e: FormEvent) => {
    e.preventDefault();
    setBusy(true);
    setError('');
    try {
      const t = await api<EmailTemplate>(id ? 'PUT' : 'POST', id ? `/api/templates/${id}` : '/api/templates', form);
      toast('Template saved.', 'success');
      if (!id) router.replace(`/templates/edit/?id=${t.id}`);
    } catch (err) {
      setError(describeError(err));
    } finally {
      setBusy(false);
    }
  };

  const remove = async () => {
    try {
      await api('DELETE', `/api/templates/${id}`);
      toast('Template deleted.', 'success');
      router.push('/templates/');
    } catch (err) {
      setError(describeError(err));
    }
  };

  if (!loaded) return <Loading />;

  return (
    <form onSubmit={save}>
      <PageHeader title={id ? form.name || 'Template' : 'New template'} back={{ href: '/templates/', label: 'Templates' }}
        actions={<>
          {id ? <Button icon={Trash2} onClick={remove}>Delete</Button> : null}
          <Button variant="primary" type="submit" icon={Save} loading={busy}>Save template</Button>
        </>} />
      <FormError>{error}</FormError>
      <div className="grid gap-5 xl:grid-cols-[minmax(0,1fr)_minmax(0,1fr)]">
        <Card title="Content">
          <div className="flex flex-col gap-4">
            <div className="grid gap-4 sm:grid-cols-[minmax(0,1fr)_180px]">
              <Field label="Name"><Input required maxLength={100} value={form.name} onChange={set('name')} /></Field>
              <Field label="Type">
                <Select value={form.kind} onChange={(e) => setForm((f) => ({ ...f, kind: e.target.value }))} aria-label="Type">
                  <option value="transactional">Transactional</option>
                  <option value="marketing">Marketing</option>
                </Select>
              </Field>
            </div>
            <Field label="Subject" hint="Variables work here too, like {{name}}. In the HTML, {{{block}}} with three braces inserts HTML you send as-is, such as a list of posts."><Input required maxLength={300} value={form.subject} onChange={set('subject')} /></Field>
            <Field label="HTML"><Textarea code rows={16} value={form.html} onChange={set('html')} /></Field>
            <Field label="Plain text" hint="Optional. Shown by mail apps that can't display HTML."><Textarea code rows={4} value={form.text} onChange={set('text')} /></Field>
          </div>
        </Card>
        <div className="flex flex-col gap-5">
          <Card title="Variables" description={vars.length ? 'Sample values for the preview. Send real values with "data" in the API.' : 'Add {{variable}} anywhere to fill it per send.'}>
            {vars.length ? (
              <div className="grid gap-3 sm:grid-cols-2">
                {vars.map((v) => (
                  <Field key={v} label={<span className="font-mono">{`{{${v}}}`}</span>}>
                    <Input value={sample[v] || ''} onChange={(e) => setSample((s) => ({ ...s, [v]: e.target.value }))} />
                  </Field>
                ))}
              </div>
            ) : null}
          </Card>
          <Card title={view === 'preview' ? fill(form.subject, sample, false) || 'Preview' : 'Send it'} actions={<Tabs value={view} onChange={setView} options={[['preview', 'Preview'], ['api', 'API']]} />}>
            {view === 'preview' ? (
              <div className="dot-grid overflow-hidden rounded-xl bg-surface-2 p-3 shadow-[inset_0_0_0_1px_var(--line)]">
                <div className="overflow-hidden rounded-lg shadow-card"><EmailFrame html={fill(form.html, sample, true)} className="h-[480px]" /></div>
              </div>
            ) : id ? <SendSnippetTemplate id={id} vars={vars} sample={sample} /> : <p className="text-[13px] text-muted">Save the template to get its ID.</p>}
          </Card>
        </div>
      </div>
    </form>
  );
}
