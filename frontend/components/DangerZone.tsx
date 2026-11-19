import { AlertTriangle, DoorOpen, Eraser, Pencil, Trash2 } from 'lucide-react';
import { FormEvent, ReactNode, useState } from 'react';
import { api, describeError } from '../lib/api';
import { useMe, useSession } from '../lib/session';
import { useToast } from '../lib/toast';
import { FormActions, Modal } from './Modal';
import { Button, Card, Field, FormError, Input } from './ui';

function ConfirmByTyping({ title, body, word, label, onConfirm, onClose }: { title: string; body: ReactNode; word: string; label: string; onConfirm: () => Promise<void>; onClose: () => void }) {
  const [typed, setTyped] = useState('');
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  const submit = async (e: FormEvent) => {
    e.preventDefault();
    setBusy(true);
    setError('');
    try { await onConfirm(); } catch (err) { setError(describeError(err)); setBusy(false); }
  };
  return (
    <Modal title={title} onClose={onClose}>
      <form className="flex flex-col gap-4" onSubmit={submit}>
        <div className="text-[13px] leading-relaxed text-muted">{body}</div>
        <Field label={<>Type <span className="font-mono text-ink">{word}</span> to confirm</>}><Input autoFocus value={typed} onChange={(e) => setTyped(e.target.value)} /></Field>
        <FormError>{error}</FormError>
        <FormActions>
          <Button onClick={onClose}>Cancel</Button>
          <Button variant="danger" type="submit" loading={busy} disabled={typed.trim() !== word}>{label}</Button>
        </FormActions>
      </form>
    </Modal>
  );
}

function Row({ icon: Icon, title, body, action }: { icon: typeof Trash2; title: string; body: ReactNode; action: ReactNode }) {
  return (
    <div className="flex flex-col gap-3 py-4 first:pt-0 last:pb-0 sm:flex-row sm:items-center sm:justify-between">
      <div className="min-w-0">
        <div className="flex items-center gap-2 text-[13.5px] font-medium text-ink"><Icon className="size-4 text-danger" />{title}</div>
        <div className="mt-1 text-xs leading-relaxed text-muted">{body}</div>
      </div>
      <div className="shrink-0">{action}</div>
    </div>
  );
}

export function ProjectCard() {
  const me = useMe();
  const { superAdmin, refreshMe } = useSession();
  const toast = useToast();
  const [name, setName] = useState(me.workspace.name);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const save = async (e: FormEvent) => {
    e.preventDefault();
    setBusy(true);
    setError('');
    try { await api('PATCH', '/api/projects/current', { name: name.trim() }); await refreshMe(); toast('Project renamed.', 'success'); } catch (err) { setError(describeError(err)); } finally { setBusy(false); }
  };
  if (!superAdmin || me.workspace.platform) return null;
  return (
    <Card title="Project" icon={Pencil} description="The name your team sees in the project menu.">
      <form className="flex flex-col gap-3 sm:flex-row sm:items-end" onSubmit={save}>
        <Field label="Project name" className="flex-1"><Input required maxLength={100} value={name} onChange={(e) => setName(e.target.value)} /></Field>
        <Button type="submit" loading={busy} disabled={name.trim() === me.workspace.name}>Save</Button>
      </form>
      <FormError>{error}</FormError>
    </Card>
  );
}

export default function DangerZone() {
  const me = useMe();
  const { superAdmin } = useSession();
  const toast = useToast();
  const [open, setOpen] = useState<'reset' | 'delete' | 'leave' | null>(null);
  if (!me.user || me.workspace.platform) return null;
  const name = me.workspace.name;
  const go = () => { window.location.href = '/'; };
  return (
    <Card title="Danger zone" icon={AlertTriangle} description="Actions here can't be undone." className="shadow-[0_0_0_1px_color-mix(in_srgb,var(--danger)_35%,transparent)]">
      <div className="divide-y divide-line">
        {superAdmin ? (
          <Row icon={Eraser} title="Reset project data" body="Deletes contacts, broadcasts, transactional history, templates, topics and events. Keeps API keys, domains, webhooks, the team and the suppression list."
            action={<Button onClick={() => setOpen('reset')}>Reset data</Button>} />
        ) : null}
        <Row icon={DoorOpen} title="Leave project" body="You lose access to this project. Your account and other projects stay. An owner has to invite you back."
          action={<Button onClick={() => setOpen('leave')}>Leave</Button>} />
        {superAdmin ? (
          <Row icon={Trash2} title="Delete project" body="Deletes this project, all of its data and its domains for everyone. Team members whose only project this is lose their account."
            action={<Button variant="danger" onClick={() => setOpen('delete')}>Delete project</Button>} />
        ) : null}
      </div>
      {open === 'reset' ? (
        <ConfirmByTyping title="Reset project data" word={name} label="Reset data" onClose={() => setOpen(null)}
          body="Every contact, broadcast, transactional message, template, topic and event in this project will be deleted."
          onConfirm={async () => { await api('POST', '/api/projects/current/reset', { confirm: name }); toast('Project data reset.', 'success'); go(); }} />
      ) : null}
      {open === 'delete' ? (
        <ConfirmByTyping title="Delete project" word={name} label="Delete project" onClose={() => setOpen(null)}
          body={<>This deletes <span className="text-ink">{name}</span> and everything in it, including its sending domains. You&apos;ll move to another project.</>}
          onConfirm={async () => { await api('POST', '/api/projects/current/delete', { confirm: name }); go(); }} />
      ) : null}
      {open === 'leave' ? (
        <ConfirmByTyping title="Leave project" word={name} label="Leave project" onClose={() => setOpen(null)}
          body="You'll lose access right away and move to another project."
          onConfirm={async () => { await api('POST', '/api/projects/current/leave', {}); go(); }} />
      ) : null}
    </Card>
  );
}
