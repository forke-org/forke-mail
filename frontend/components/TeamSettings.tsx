/**
 * @fileoverview Forke Platform
 * @copyright (c) 2026 Forke Inc. (https://www.forke.space/)
 *
 * Source-Available License (Non-Commercial / Fair Source).
 * This source code is open for inspection, learning, and personal development.
 * Commercial use, hosting, or resale as a paid service without an explicit
 * commercial license from Forke Inc. is strictly prohibited.
 */

import { MailPlus, UserMinus, Users, X } from 'lucide-react';
import { FormEvent, useState } from 'react';
import { api, describeError } from '../lib/api';
import { formatDate } from '../lib/format';
import { useSession } from '../lib/session';
import { useToast } from '../lib/toast';
import { useLoad } from '../lib/useLoad';
import { FormActions, Modal } from './Modal';
import { Badge, Button, Callout, Card, Field, FormError, Input, Loading, Select } from './ui';

interface Member { id: string; name: string; email: string; username: string | null; role: 'super_admin' | 'admin'; you: boolean; created_at: string }
interface Invite { id: string; email: string; role: 'super_admin' | 'admin'; expires_at: string; created_at: string }
interface Team { members: Member[]; invites: Invite[]; managed_by_forke: boolean }

function InviteModal({ onClose, onSent }: { onClose: () => void; onSent: () => void }) {
  const toast = useToast();
  const [email, setEmail] = useState('');
  const [role, setRole] = useState<'admin' | 'super_admin'>('admin');
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  const submit = async (e: FormEvent) => {
    e.preventDefault();
    setBusy(true);
    setError('');
    try {
      await api('POST', '/api/team/invites', { email: email.trim(), role });
      toast(`Invite sent to ${email.trim()}.`, 'success');
      onSent();
    } catch (err) {
      setError(describeError(err));
      setBusy(false);
    }
  };
  return (
    <Modal title="Invite a teammate" description="They get an email with a link to choose a username and password. The link works for 7 days." onClose={onClose}>
      <form className="flex flex-col gap-4" onSubmit={submit}>
        <Field label="Email"><Input type="email" required autoFocus value={email} onChange={(e) => setEmail(e.target.value)} /></Field>
        <Field label="Role" hint="Owners can also manage API keys, domains, webhooks and the team. Admins do everything else.">
          <Select value={role} onChange={(e) => setRole(e.target.value as 'admin' | 'super_admin')} aria-label="Role">
            <option value="admin">Admin</option>
            <option value="super_admin">Owner</option>
          </Select>
        </Field>
        <FormError>{error}</FormError>
        <FormActions>
          <Button onClick={onClose}>Cancel</Button>
          <Button variant="primary" type="submit" icon={MailPlus} loading={busy}>Send invite</Button>
        </FormActions>
      </form>
    </Modal>
  );
}

export default function TeamSettings() {
  const { superAdmin } = useSession();
  const toast = useToast();
  const [state, reload] = useLoad(() => api<Team>('GET', '/api/team'), []);
  const [inviting, setInviting] = useState(false);
  if (!state.data) return state.error ? <FormError>{state.error}</FormError> : <Loading />;
  const { members, invites, managed_by_forke: managed } = state.data;
  const canInvite = superAdmin && !managed;

  const act = async (fn: () => Promise<unknown>, ok: string) => {
    try { await fn(); toast(ok, 'success'); reload(); } catch (err) { toast(describeError(err), 'error'); }
  };

  return (
    <div className="flex flex-col gap-5">
      {managed ? (
        <Callout tone="accent" title="This is Forke's own team">Everyone with a Forke super admin or admin account is a member automatically. Manage people from the Forke admin panel.</Callout>
      ) : null}
      <Card title="Team members" icon={Users} description={`${members.length} ${members.length === 1 ? 'person' : 'people'} in this workspace.`}
        actions={canInvite ? <Button size="sm" variant="primary" icon={MailPlus} onClick={() => setInviting(true)}>Invite</Button> : null}>
        <div className="divide-y divide-line">
          {members.map((m) => (
            <div key={m.id} className="flex items-center justify-between gap-3 py-3 first:pt-0 last:pb-0">
              <div className="min-w-0">
                <div className="flex items-center gap-2 text-[13px] text-ink">
                  <span className="truncate">{m.name}</span>
                  {m.username ? <span className="font-mono text-xs text-accent">@{m.username}</span> : null}
                  {m.you ? <Badge value="you" /> : null}
                </div>
                <div className="truncate text-xs text-muted">{m.email} · joined {formatDate(m.created_at)}</div>
              </div>
              <div className="flex items-center gap-2">
                <Badge value={m.role} />
                {canInvite && !m.you ? (
                  <>
                    <Button size="sm" onClick={() => act(() => api('PATCH', `/api/team/members/${m.id}`, { role: m.role === 'super_admin' ? 'admin' : 'super_admin' }), m.role === 'super_admin' ? `${m.name} is now an admin.` : `${m.name} is now an owner.`)}>
                      {m.role === 'super_admin' ? 'Make admin' : 'Make owner'}
                    </Button>
                    <Button size="sm" icon={UserMinus} onClick={() => act(() => api('DELETE', `/api/team/members/${m.id}`), `${m.name} removed.`)}>Remove</Button>
                  </>
                ) : null}
              </div>
            </div>
          ))}
        </div>
      </Card>
      {invites.length ? (
        <Card title="Pending invites" icon={MailPlus}>
          <div className="divide-y divide-line">
            {invites.map((i) => (
              <div key={i.id} className="flex items-center justify-between gap-3 py-3 first:pt-0 last:pb-0">
                <div className="min-w-0">
                  <div className="truncate text-[13px] text-ink">{i.email}</div>
                  <div className="text-xs text-muted">expires {formatDate(i.expires_at)}</div>
                </div>
                <div className="flex items-center gap-2">
                  <Badge value={i.role} />
                  {canInvite ? <Button size="sm" icon={X} onClick={() => act(() => api('DELETE', `/api/team/invites/${i.id}`), 'Invite cancelled.')}>Cancel</Button> : null}
                </div>
              </div>
            ))}
          </div>
        </Card>
      ) : null}
      {inviting ? <InviteModal onClose={() => setInviting(false)} onSent={() => { setInviting(false); reload(); }} /> : null}
    </div>
  );
}
