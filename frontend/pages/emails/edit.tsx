import { Eye, Monitor, Save, Send, Smartphone, Trash2, Upload, Users } from 'lucide-react';
import { useRouter } from 'next/router';
import { useEffect, useMemo, useState } from 'react';
import ContactPicker from '../../components/ContactPicker';
import ImportForm from '../../components/ImportForm';
import { Modal, useConfirm } from '../../components/Modal';
import TagSelect from '../../components/TagSelect';
import { Button, Card, EmailFrame, Empty, Field, Input, Loading, PageHeader, Select, Tabs, Textarea } from '../../components/ui';
import { api, ApiError, describeError } from '../../lib/api';
import { plural } from '../../lib/format';
import { useMe, useSession } from '../../lib/session';
import { useToast } from '../../lib/toast';
import type { Audience, Campaign, Email, Tag, Topic } from '../../lib/types';
import { useDebounced } from '../../lib/useLoad';

const sample: Record<string, string> = { first_name: 'Ada', email: 'ada@example.com' };

const starterHTML = `<html>
  <body style="font-family: -apple-system, Segoe UI, sans-serif; color: #18181b; padding: 24px;">
    <h1 style="font-size: 22px;">Hi {{first_name}},</h1>
    <p>Here is what is new this month.</p>
    <p><a href="https://forke.space">Read more on forke.space</a></p>
  </body>
</html>`;

const escapeHTML = (s: string) => s.replace(/[&<>"']/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' })[c] as string);
const merge = (s: string, escape: boolean) => s.replace(/\{\{(first_name|email)\}\}/g, (_, k: string) => (escape ? escapeHTML(sample[k]) : sample[k]));

function logRequest(method: string, url: string, body: unknown = null) {
  console.log('[forke] request', JSON.parse(JSON.stringify({ method, url, body })));
}

function splitAddress(address: string | undefined, identities: string[]) {
  if (address && identities.includes(address)) return { local: 'hello', identity: address };
  const at = (address || '').lastIndexOf('@');
  if (!address || at < 0) return { local: 'hello', identity: identities[0] || '' };
  return { local: address.slice(0, at), identity: address.slice(at + 1) };
}

function fromAddress(local: string, identity: string) {
  return identity.includes('@') ? identity : `${local.trim()}@${identity}`.toLowerCase();
}

function audienceKey(a: Audience) {
  return JSON.stringify([a.all, [...a.tag_ids].sort(), [...a.contact_ids].sort()]);
}

type Form = { subject: string; from_name: string; from_local: string; from_identity: string; reply_to: string; topic: string; html: string; text: string };

function Editor({ campaign, existing, tags: initialTags, topics }: { campaign: Campaign; existing: Email | null; tags: Tag[]; topics: Topic[] }) {
  const router = useRouter();
  const toast = useToast();
  const confirm = useConfirm();
  const me = useMe();
  const { refreshMe } = useSession();
  const identities = me.domains;
  const [email, setEmail] = useState<Email | null>(existing);
  const [tags, setTags] = useState(initialTags);
  const [form, setForm] = useState<Form>(() => {
    const { local, identity } = splitAddress(existing?.from_address, identities);
    return {
      subject: existing?.subject || '',
      from_name: existing?.from_name || me.workspace.name,
      from_local: local,
      from_identity: identity,
      reply_to: existing?.reply_to || '',
      topic: existing?.topic || '',
      html: existing ? existing.html : starterHTML,
      text: existing?.text || '',
    };
  });
  const [audience, setAudience] = useState<Audience>(() => ({
    all: existing ? existing.audience.all : true,
    tag_ids: existing?.audience.tag_ids || [],
    contact_ids: existing?.audience.contact_ids || [],
  }));
  const [busy, setBusy] = useState('');
  const [modal, setModal] = useState<'pick' | 'import' | null>(null);
  const [device, setDevice] = useState<'desktop' | 'mobile'>('desktop');
  const set = (key: keyof Form) => (e: React.ChangeEvent<HTMLInputElement | HTMLTextAreaElement | HTMLSelectElement>) => setForm((f) => ({ ...f, [key]: e.target.value }));
  const preview = useDebounced(form.html, 150);
  const previewDoc = useMemo(() => merge(preview, true), [preview]);
  const savedAudience = email ? audienceKey(email.audience) : null;
  const audienceDirty = savedAudience !== audienceKey(audience);
  const identityOptions = identities.includes(form.from_identity) || !form.from_identity ? identities : [form.from_identity, ...identities];
  const singleAddress = form.from_identity.includes('@');

  const payload = () => ({
    subject: form.subject.trim(),
    from_name: form.from_name.trim(),
    from_address: fromAddress(form.from_local, form.from_identity),
    reply_to: form.reply_to.trim(),
    topic: form.topic,
    html: form.html,
    text: form.text,
    audience,
  });

  const save = async () => {
    const body = payload();
    let saved: Email;
    if (email) {
      logRequest('PATCH', `/api/emails/${email.id}`, body);
      saved = await api<Email>('PATCH', `/api/emails/${email.id}`, body);
    } else {
      logRequest('POST', `/api/campaigns/${campaign.id}/emails`, body);
      saved = await api<Email>('POST', `/api/campaigns/${campaign.id}/emails`, body);
      router.replace(`/emails/edit/?id=${saved.id}`, undefined, { shallow: true });
    }
    setEmail(saved);
    return saved;
  };

  const run = async (label: string, fn: () => Promise<void>) => {
    setBusy(label);
    try {
      await fn();
    } catch (err) {
      if (!(err instanceof ApiError && err.status === 402)) toast(describeError(err), 'error', 6000);
    } finally {
      setBusy('');
    }
  };

  const openImport = () => run('save', async () => {
    await save();
    setModal('import');
  });

  const afterImport = async () => {
    if (!email) return;
    const [fresh, tagList] = await Promise.all([api<Email>('GET', `/api/emails/${email.id}`), api<{ tags: Tag[] }>('GET', '/api/tags')]);
    setEmail(fresh);
    setTags(tagList.tags);
    setAudience((a) => ({ ...a, contact_ids: fresh.audience.contact_ids }));
  };

  const send = () => run('send', async () => {
    const saved = await save();
    const topic = topics.find((t) => t.key === saved.topic);
    const message = `Send "${saved.subject}" to ${plural(saved.audience.size || 0, 'contact')}${topic ? ` subscribed to ${topic.label}` : ''} now? This can't be undone.`;
    if (!(await confirm('Send broadcast', message, 'Send now'))) return;
    const url = `/api/emails/${saved.id}/send`;
    logRequest('POST', url);
    const res = await api<{ recipients: number }>('POST', url);
    toast(`Sending to ${plural(res.recipients, 'contact')}.`, 'success');
    refreshMe();
    router.push(`/emails/view/?id=${saved.id}`);
  });

  const remove = async () => {
    if (!email || !(await confirm('Delete draft', 'Delete this draft? This cannot be undone.', 'Delete', true))) return;
    run('delete', async () => {
      await api('DELETE', `/api/emails/${email.id}`);
      toast('Draft deleted.', 'success');
      router.push(`/campaigns/view/?id=${campaign.id}`);
    });
  };

  const selectedCount = audience.contact_ids.length;
  return (
    <>
      <PageHeader
        back={{ href: `/campaigns/view/?id=${campaign.id}`, label: campaign.name }}
        title={email ? 'Edit email' : 'New email'}
        description={<>Use <code className="rounded bg-surface-3 px-1 font-mono text-xs">{'{{first_name}}'}</code> and <code className="rounded bg-surface-3 px-1 font-mono text-xs">{'{{email}}'}</code> to personalize. An unsubscribe link is added automatically.</>}
        actions={(
          <>
            {email ? <Button variant="danger" icon={Trash2} disabled={!!busy} onClick={remove}>Delete</Button> : null}
            <Button icon={Save} loading={busy === 'save'} disabled={!!busy} onClick={() => run('save', async () => { await save(); toast('Draft saved.', 'success'); })}>Save draft</Button>
            <Button variant="primary" icon={Send} loading={busy === 'send'} disabled={!!busy} onClick={send}>Send</Button>
          </>
        )}
      />
      <div className="grid items-start gap-5 xl:grid-cols-[minmax(0,1fr)_minmax(0,1fr)]">
        <div className="flex flex-col gap-5">
          <Card title="Sender">
            <div className="flex flex-col gap-4">
              <div className="grid gap-4 sm:grid-cols-2">
                <Field label="From name"><Input maxLength={100} value={form.from_name} onChange={set('from_name')} /></Field>
                <Field label="From address">
                  <div className="flex items-stretch">
                    {singleAddress ? null : (
                      <>
                        <Input className="w-auto min-w-0 flex-1 rounded-r-none" value={form.from_local} onChange={set('from_local')} aria-label="Address name" />
                        <span className="grid place-items-center border-y border-line-strong bg-surface-2 px-2 text-sm text-muted">@</span>
                      </>
                    )}
                    <Select className={singleAddress ? 'w-full' : 'min-w-0 flex-[1.6] [&_select]:rounded-l-none'} value={form.from_identity} onChange={set('from_identity')} aria-label="Sending identity">
                      {identityOptions.length ? identityOptions.map((d) => <option key={d} value={d}>{d}</option>) : <option value="">No verified domains</option>}
                    </Select>
                  </div>
                </Field>
              </div>
              <div className="grid gap-4 sm:grid-cols-2">
                <Field label="Reply-to" hint="Optional. Replies go here instead of the from address."><Input type="email" placeholder="team@forke.space" value={form.reply_to} onChange={set('reply_to')} /></Field>
                <Field label="Topic" hint="Contacts who opted out of this topic are skipped.">
                  <Select value={form.topic} onChange={set('topic')}>
                    <option value="">No topic (everyone)</option>
                    {topics.map((t) => <option key={t.key} value={t.key}>{t.label}</option>)}
                  </Select>
                </Field>
              </div>
              <Field label="Subject"><Input maxLength={300} placeholder="What's new at Forke, {{first_name}}" value={form.subject} onChange={set('subject')} /></Field>
            </div>
          </Card>
          <Card
            title="Audience"
            icon={Users}
            description={email && !audienceDirty ? `Sends to ${plural(email.audience.size || 0, 'active contact')}.` : 'Save to see how many contacts this reaches.'}
          >
            <div className="flex flex-col gap-4">
              <Tabs value={audience.all ? 'all' : 'some'} onChange={(v) => setAudience((a) => ({ ...a, all: v === 'all' }))} options={[['all', 'All contacts'], ['some', 'Choose audience']]} />
              {audience.all ? (
                <p className="text-[13px] leading-relaxed text-muted">Every active contact receives this email. Unsubscribed, paused and bounced contacts are always skipped.</p>
              ) : (
                <>
                  <div className="flex flex-col gap-2">
                    <span className="text-[13px] font-medium text-ink-2">Contacts with any of these tags</span>
                    <TagSelect tags={tags} selected={audience.tag_ids} onChange={(tag_ids) => setAudience((a) => ({ ...a, tag_ids }))} />
                  </div>
                  <div className="flex flex-col gap-2">
                    <span className="text-[13px] font-medium text-ink-2">Plus individual contacts</span>
                    <div className="flex flex-col gap-3 rounded-lg bg-surface-2 px-3 py-2.5 shadow-[inset_0_0_0_1px_var(--line)] sm:flex-row sm:items-center sm:justify-between">
                      <span className="text-[13px] text-muted">{selectedCount ? `${plural(selectedCount, 'contact')} selected` : 'No individual contacts selected'}</span>
                      <div className="flex gap-2">
                        <Button size="sm" onClick={() => setModal('pick')}>Choose contacts</Button>
                        <Button size="sm" icon={Upload} disabled={!!busy} onClick={openImport}>Import new</Button>
                      </div>
                    </div>
                  </div>
                </>
              )}
            </div>
          </Card>
          <Card title="Content">
            <div className="flex flex-col gap-4">
              <Field label="HTML"><Textarea code rows={16} value={form.html} onChange={set('html')} spellCheck={false} /></Field>
              <Field label="Plain text" hint="Optional. Generated from the HTML when left empty."><Textarea code rows={5} value={form.text} onChange={set('text')} /></Field>
            </div>
          </Card>
        </div>
        <section className="overflow-hidden rounded-xl bg-surface shadow-card xl:sticky xl:top-20">
          <div className="flex items-center justify-between gap-3 border-b border-line px-5 py-3">
            <div className="flex items-center gap-2 text-sm font-semibold text-ink"><Eye className="size-4 text-muted" />Preview</div>
            <Tabs value={device} onChange={setDevice} options={[['desktop', <Monitor key="d" className="size-3.5" />], ['mobile', <Smartphone key="m" className="size-3.5" />]]} />
          </div>
          <div className="border-b border-line bg-surface-2 px-5 py-3">
            <div className="truncate text-sm font-medium text-ink">{merge(form.subject, false) || 'No subject'}</div>
            <div className="mt-0.5 truncate text-xs text-muted">{`${form.from_name || 'Sender'} <${fromAddress(form.from_local, form.from_identity)}>`}</div>
          </div>
          <div className="dot-grid flex justify-center bg-surface-2 p-4">
            <div className={device === 'mobile' ? 'w-[375px] max-w-full overflow-hidden rounded-[22px] shadow-pop' : 'w-full overflow-hidden rounded-lg shadow-card'}>
              <EmailFrame html={previewDoc} className={device === 'mobile' ? 'h-[620px]' : 'h-[600px]'} />
            </div>
          </div>
        </section>
      </div>
      {modal === 'pick' ? (
        <ContactPicker
          selected={audience.contact_ids}
          onClose={() => setModal(null)}
          onSave={(contact_ids) => { setAudience((a) => ({ ...a, contact_ids })); setModal(null); }}
        />
      ) : null}
      {modal === 'import' && email ? (
        <Modal title="Import contacts" description="New contacts are saved to your contact list and added to this email." onClose={() => setModal(null)}>
          <ImportForm emailId={email.id} onDone={afterImport} onCancel={() => setModal(null)} />
        </Modal>
      ) : null}
    </>
  );
}

type Loaded = { ready: false } | { ready: true; error: string } | { ready: true; error: ''; campaign: Campaign; existing: Email | null; tags: Tag[]; topics: Topic[] };

export default function EditEmail() {
  const router = useRouter();
  const [state, setState] = useState<Loaded>({ ready: false });
  const { id, campaign: campaignId } = router.query as { id?: string; campaign?: string };

  useEffect(() => {
    if (!router.isReady) return;
    (async () => {
      try {
        const existing = id ? await api<Email>('GET', `/api/emails/${id}`) : null;
        if (existing && existing.status !== 'draft') {
          router.replace(`/emails/view/?id=${existing.id}`);
          return;
        }
        const cid = existing ? existing.campaign_id : campaignId;
        if (!cid) throw new Error('missing');
        const [{ campaign }, { tags }, { topics }] = await Promise.all([
          api<{ campaign: Campaign }>('GET', `/api/campaigns/${cid}`),
          api<{ tags: Tag[] }>('GET', '/api/tags'),
          api<{ topics: Topic[] }>('GET', '/api/topics'),
        ]);
        setState({ ready: true, error: '', campaign, existing, tags, topics });
      } catch (err) {
        setState({ ready: true, error: describeError(err) });
      }
    })();
  }, [router.isReady]);

  if (!state.ready) return <Loading />;
  if (state.error || !('campaign' in state)) return <Card><Empty title="Could not open this email" hint={state.error} /></Card>;
  return <Editor campaign={state.campaign} existing={state.existing} tags={state.tags} topics={state.topics} />;
}
