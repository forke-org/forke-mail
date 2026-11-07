import Link from 'next/link';
import { useRouter } from 'next/router';
import { useEffect, useMemo, useState } from 'react';
import ContactPicker from '../../components/ContactPicker';
import ImportForm from '../../components/ImportForm';
import Icon from '../../components/Icon';
import { Modal, useConfirm } from '../../components/Modal';
import TagSelect from '../../components/TagSelect';
import { Card, EmailFrame, Empty, Field, Loading, PageHead, Tabs } from '../../components/ui';
import { api, ApiError, describeError } from '../../lib/api';
import { plural } from '../../lib/format';
import { useSession } from '../../lib/session';
import { useToast } from '../../lib/toast';

const sample = { first_name: 'Ada', email: 'ada@example.com' };

const starterHTML = `<html>
  <body style="font-family: -apple-system, Segoe UI, sans-serif; color: #18181b; padding: 24px;">
    <h1 style="font-size: 22px;">Hi {{first_name}},</h1>
    <p>Here is what is new this month.</p>
  </body>
</html>`;

const escapeHTML = (s) => s.replace(/[&<>"']/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' })[c]);
const merge = (s, escape) => s.replace(/\{\{(first_name|email)\}\}/g, (_, k) => (escape ? escapeHTML(sample[k]) : sample[k]));

function logRequest(method, url, body = null) {
  console.log('[forke] request', JSON.parse(JSON.stringify({ method, url, body })));
}

function splitAddress(address, identities) {
  if (identities.includes(address)) return { local: 'hello', identity: address };
  const at = (address || '').lastIndexOf('@');
  if (at < 0) return { local: 'hello', identity: identities[0] || '' };
  return { local: address.slice(0, at), identity: address.slice(at + 1) };
}

function fromAddress(local, identity) {
  return identity.includes('@') ? identity : `${local.trim()}@${identity}`.toLowerCase();
}

function useDebounced(value, ms) {
  const [v, setV] = useState(value);
  useEffect(() => {
    const t = setTimeout(() => setV(value), ms);
    return () => clearTimeout(t);
  }, [value, ms]);
  return v;
}

function audienceKey(a) {
  return JSON.stringify([a.all, [...a.tag_ids].sort(), [...a.contact_ids].sort()]);
}

function Editor({ campaign, existing, tags: initialTags }) {
  const router = useRouter();
  const toast = useToast();
  const confirm = useConfirm();
  const { me, refreshMe } = useSession();
  const identities = me.domains;
  const [email, setEmail] = useState(existing);
  const [tags, setTags] = useState(initialTags);
  const [form, setForm] = useState(() => {
    const { local, identity } = splitAddress(existing?.from_address, identities);
    return {
      subject: existing?.subject || '',
      from_name: existing?.from_name || me.workspace.name,
      from_local: local,
      from_identity: identity,
      html: existing ? existing.html : starterHTML,
      text: existing?.text || '',
    };
  });
  const [audience, setAudience] = useState(() => ({
    all: existing ? existing.audience.all : true,
    tag_ids: existing?.audience.tag_ids || [],
    contact_ids: existing?.audience.contact_ids || [],
  }));
  const [busy, setBusy] = useState('');
  const [modal, setModal] = useState(null);
  const set = (key) => (e) => setForm((f) => ({ ...f, [key]: e.target.value }));
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
    html: form.html,
    text: form.text,
    audience,
  });

  const save = async () => {
    const body = payload();
    let saved;
    if (email) {
      logRequest('PATCH', `/api/emails/${email.id}`, body);
      saved = await api('PATCH', `/api/emails/${email.id}`, body);
    } else {
      logRequest('POST', `/api/campaigns/${campaign.id}/emails`, body);
      saved = await api('POST', `/api/campaigns/${campaign.id}/emails`, body);
      router.replace(`/emails/edit/?id=${saved.id}`, undefined, { shallow: true });
    }
    setEmail(saved);
    return saved;
  };

  const run = async (label, fn) => {
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
    const [fresh, tagList] = await Promise.all([api('GET', `/api/emails/${email.id}`), api('GET', '/api/tags')]);
    setEmail(fresh);
    setTags(tagList.tags);
    setAudience((a) => ({ ...a, contact_ids: fresh.audience.contact_ids }));
  };

  const send = () => run('send', async () => {
    const saved = await save();
    if (!(await confirm('Send email', `Send "${saved.subject}" to ${plural(saved.audience.size, 'contact')} now? This can't be undone.`, 'Send now'))) return;
    const url = `/api/emails/${saved.id}/send`;
    logRequest('POST', url);
    const res = await api('POST', url);
    toast(`Sending to ${plural(res.recipients, 'contact')}.`, 'success');
    refreshMe();
    router.push(`/emails/view/?id=${saved.id}`);
  });

  const remove = async () => {
    if (!(await confirm('Delete draft', 'Delete this draft? This cannot be undone.', 'Delete', true))) return;
    run('delete', async () => {
      await api('DELETE', `/api/emails/${email.id}`);
      toast('Draft deleted.', 'success');
      router.push(`/campaigns/view/?id=${campaign.id}`);
    });
  };

  const selectedCount = audience.contact_ids.length;
  return (
    <>
      <PageHead
        back={<Link className="back-link" href={`/campaigns/view/?id=${campaign.id}`}>← {campaign.name}</Link>}
        title={email ? 'Edit email' : 'New email'}
        subtitle="Use {{first_name}} and {{email}} to personalize. An unsubscribe link is added automatically."
      >
        {email ? <button className="btn btn-ghost btn-danger" type="button" disabled={!!busy} onClick={remove}>Delete</button> : null}
        <button className="btn" type="button" disabled={!!busy} onClick={() => run('save', async () => { await save(); toast('Draft saved.', 'success'); })}>{busy === 'save' ? 'Saving…' : 'Save draft'}</button>
        <button className="btn btn-primary" type="button" disabled={!!busy} onClick={send}>{busy === 'send' ? 'Sending…' : 'Send'}</button>
      </PageHead>
      <div className="compose">
        <div className="stack">
          <Card title="Sender">
            <div className="card-body">
              <div className="grid grid-2">
                <Field label="From name"><input type="text" maxLength={100} value={form.from_name} onChange={set('from_name')} /></Field>
                <Field label="From address">
                  <div className="input-group">
                    {singleAddress ? null : (
                      <>
                        <input type="text" value={form.from_local} onChange={set('from_local')} aria-label="Address name" />
                        <span className="input-addon">@</span>
                      </>
                    )}
                    <select value={form.from_identity} onChange={set('from_identity')} aria-label="Sending identity">
                      {identityOptions.map((d) => <option key={d} value={d}>{d}</option>)}
                    </select>
                  </div>
                </Field>
              </div>
              <Field label="Subject"><input type="text" maxLength={300} placeholder="What's new at Forke, {{first_name}}" value={form.subject} onChange={set('subject')} /></Field>
            </div>
          </Card>
          <Card
            title="Audience"
            description={email && !audienceDirty ? `Sends to ${plural(email.audience.size, 'active contact')}.` : 'Save to see how many contacts this reaches.'}
          >
            <div className="card-body">
              <Tabs value={audience.all ? 'all' : 'some'} onChange={(v) => setAudience((a) => ({ ...a, all: v === 'all' }))} options={[['all', 'All contacts'], ['some', 'Choose audience']]} />
              {audience.all ? (
                <p className="small muted">Every active contact in your workspace receives this email. Unsubscribed and bounced contacts are always skipped.</p>
              ) : (
                <>
                  <div className="stack-xs">
                    <span className="label">Contacts with any of these tags</span>
                    <TagSelect tags={tags} selected={audience.tag_ids} onChange={(tag_ids) => setAudience((a) => ({ ...a, tag_ids }))} />
                  </div>
                  <div className="stack-xs">
                    <span className="label">Plus individual contacts</span>
                    <div className="audience-row">
                      <span className="muted">{selectedCount ? `${plural(selectedCount, 'contact')} selected` : 'No individual contacts selected'}</span>
                      <div className="actions">
                        <button className="btn btn-sm" type="button" onClick={() => setModal('pick')}>Choose contacts</button>
                        <button className="btn btn-sm" type="button" disabled={!!busy} onClick={openImport}><Icon name="upload" />Import new</button>
                      </div>
                    </div>
                  </div>
                </>
              )}
            </div>
          </Card>
          <Card title="Content">
            <div className="card-body">
              <Field label="HTML"><textarea className="code" rows={14} value={form.html} onChange={set('html')} /></Field>
              <Field label="Plain text" hint="Shown by email clients that don't render HTML."><textarea className="code" rows={5} value={form.text} onChange={set('text')} /></Field>
            </div>
          </Card>
        </div>
        <section className="card preview">
          <div className="card-head"><h2>Preview</h2><span className="badge">Scripts disabled</span></div>
          <div className="preview-meta">
            <strong>{merge(form.subject, false) || 'No subject'}</strong>
            <span className="muted">{`${form.from_name || 'Sender'} <${fromAddress(form.from_local, form.from_identity)}>`}</span>
          </div>
          <EmailFrame html={previewDoc} />
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

export default function EditEmail() {
  const router = useRouter();
  const [state, setState] = useState({ ready: false, error: '' });
  const { id, campaign: campaignId } = router.query;

  useEffect(() => {
    if (!router.isReady) return;
    (async () => {
      try {
        const existing = id ? await api('GET', `/api/emails/${id}`) : null;
        if (existing && existing.status !== 'draft') {
          router.replace(`/emails/view/?id=${existing.id}`);
          return;
        }
        const cid = existing ? existing.campaign_id : campaignId;
        if (!cid) throw new Error('missing');
        const [{ campaign }, { tags }] = await Promise.all([api('GET', `/api/campaigns/${cid}`), api('GET', '/api/tags')]);
        setState({ ready: true, error: '', campaign, existing, tags });
      } catch (err) {
        setState({ ready: true, error: describeError(err) });
      }
    })();
  }, [router.isReady]);

  if (!state.ready) return <Loading />;
  if (state.error) return <Card><Empty title="Could not open this email" hint={state.error} /></Card>;
  return <Editor campaign={state.campaign} existing={state.existing} tags={state.tags} />;
}
