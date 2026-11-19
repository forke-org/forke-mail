import clsx from 'clsx';
import { Check, ChevronsUpDown, Plus } from 'lucide-react';
import { FormEvent, useEffect, useRef, useState } from 'react';
import { api, describeError } from '../lib/api';
import { useMe } from '../lib/session';
import { FormActions, Modal } from './Modal';
import { Badge, Button, Field, FormError, Input } from './ui';

interface Project { id: string; name: string; role: string; plan: string; members: number; current: boolean }

function NewProject({ onClose }: { onClose: () => void }) {
  const [name, setName] = useState('');
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  const submit = async (e: FormEvent) => {
    e.preventDefault();
    setBusy(true);
    setError('');
    try {
      await api('POST', '/api/projects', { name: name.trim() });
      window.location.href = '/';
    } catch (err) {
      setError(describeError(err));
      setBusy(false);
    }
  };
  return (
    <Modal title="New project" description="A separate workspace with its own domains, contacts, keys and team. You can switch between projects any time." onClose={onClose}>
      <form className="flex flex-col gap-4" onSubmit={submit}>
        <Field label="Project name"><Input required autoFocus maxLength={100} value={name} onChange={(e) => setName(e.target.value)} /></Field>
        <FormError>{error}</FormError>
        <FormActions>
          <Button onClick={onClose}>Cancel</Button>
          <Button variant="primary" type="submit" icon={Plus} loading={busy}>Create project</Button>
        </FormActions>
      </form>
    </Modal>
  );
}

export default function ProjectSwitcher() {
  const me = useMe();
  const [open, setOpen] = useState(false);
  const [creating, setCreating] = useState(false);
  const [projects, setProjects] = useState<Project[] | null>(null);
  const [max, setMax] = useState(0);
  const ref = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!me.user) return;
    api<{ projects: Project[]; max_projects: number }>('GET', '/api/projects').then((d) => { setProjects(d.projects); setMax(d.max_projects); }, () => setProjects([]));
  }, [me.user, me.workspace.id]);

  useEffect(() => {
    if (!open) return undefined;
    const close = (e: MouseEvent) => { if (!ref.current?.contains(e.target as Node)) setOpen(false); };
    document.addEventListener('mousedown', close);
    return () => document.removeEventListener('mousedown', close);
  }, [open]);

  if (!me.user) return null;
  const owned = (projects || []).filter((p) => p.role === 'super_admin').length;

  const pick = async (p: Project) => {
    if (p.current) { setOpen(false); return; }
    await api('POST', '/api/projects/switch', { id: p.id });
    window.location.href = '/';
  };

  return (
    <div className="relative px-3 pt-3" ref={ref}>
      <button type="button" aria-expanded={open} onClick={() => setOpen((o) => !o)}
        className={clsx('flex h-10 w-full cursor-pointer items-center gap-2.5 rounded-[6px] bg-surface px-2 text-left shadow-[inset_0_0_0_1px_var(--line)] transition-colors hover:shadow-[inset_0_0_0_1px_var(--line-strong)]', open && 'shadow-[inset_0_0_0_1px_var(--line-strong)]')}>
        <span className="grid size-6 shrink-0 place-items-center rounded-[4px] bg-surface-3 text-[11px] font-semibold text-ink">{me.workspace.name.slice(0, 1).toUpperCase()}</span>
        <span className="min-w-0 flex-1 truncate text-[13px] font-medium text-ink">{me.workspace.name}</span>
        <ChevronsUpDown className="size-3.5 shrink-0 text-muted" />
      </button>
      {open ? (
        <div className="absolute right-3 left-3 top-full z-50 mt-1.5 animate-pop overflow-hidden rounded-[8px] bg-surface shadow-pop">
          <div className="px-3 pt-2.5 pb-1.5 font-mono text-[10.5px] tracking-[0.08em] text-faint uppercase">Projects</div>
          <div className="max-h-72 overflow-y-auto p-1">
            {(projects || []).map((p) => (
              <button key={p.id} type="button" onClick={() => pick(p)}
                className="flex w-full cursor-pointer items-center gap-2.5 rounded-[5px] px-2 py-2 text-left hover:bg-surface-3">
                <span className="grid size-6 shrink-0 place-items-center rounded-[4px] bg-surface-3 text-[11px] font-semibold text-ink">{p.name.slice(0, 1).toUpperCase()}</span>
                <span className="min-w-0 flex-1">
                  <span className="block truncate text-[13px] text-ink">{p.name}</span>
                  <span className="block text-[11px] text-muted">{p.members} {p.members === 1 ? 'member' : 'members'} · {p.plan}</span>
                </span>
                <Badge value={p.role} />
                {p.current ? <Check className="size-3.5 shrink-0 text-accent" /> : <span className="size-3.5 shrink-0" />}
              </button>
            ))}
          </div>
          <div className="border-t border-line p-1">
            <button type="button" disabled={owned >= max} onClick={() => { setOpen(false); setCreating(true); }}
              className="flex w-full cursor-pointer items-center gap-2 rounded-[5px] px-2 py-2 text-[13px] text-ink-2 hover:bg-surface-3 hover:text-ink disabled:cursor-not-allowed disabled:opacity-50">
              <Plus className="size-4" />New project
              <span className="ml-auto text-[11px] text-faint">{owned} / {max}</span>
            </button>
          </div>
        </div>
      ) : null}
      {creating ? <NewProject onClose={() => setCreating(false)} /> : null}
    </div>
  );
}
