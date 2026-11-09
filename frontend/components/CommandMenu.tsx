import clsx from 'clsx';
import { CornerDownLeft, Globe, Megaphone, Plus, Search, Upload, Zap, type LucideIcon } from 'lucide-react';
import { useRouter } from 'next/router';
import { useEffect, useMemo, useRef, useState } from 'react';
import { useSession } from '../lib/session';
import { navGroups } from './nav';
import { Kbd } from './ui';

interface Command {
  id: string;
  label: string;
  group: string;
  icon: LucideIcon;
  href: string;
  keywords?: string;
}

export default function CommandMenu({ open, onClose }: { open: boolean; onClose: () => void }) {
  const router = useRouter();
  const { superAdmin, canManage } = useSession();
  const [query, setQuery] = useState('');
  const [index, setIndex] = useState(0);
  const inputRef = useRef<HTMLInputElement>(null);

  const commands = useMemo<Command[]>(() => {
    const pages = navGroups.flatMap((g) => g.items.filter((i) => !i.superAdmin || superAdmin).map((i) => ({
      id: i.href, label: i.label, group: 'Go to', icon: i.icon, href: i.href, keywords: i.keywords,
    })));
    const actions: Command[] = [
      { id: 'new-campaign', label: 'New broadcast campaign', group: 'Actions', icon: Megaphone, href: '/campaigns/?new=1', keywords: 'create' },
      { id: 'import', label: 'Import contacts', group: 'Actions', icon: Upload, href: '/contacts/?import=1', keywords: 'csv upload' },
      { id: 'test', label: 'Send a test transactional email', group: 'Actions', icon: Zap, href: '/transactional/?test=1', keywords: 'try' },
    ];
    if (canManage) actions.push({ id: 'domain', label: 'Add a sending domain', group: 'Actions', icon: Globe, href: '/domains/?add=1', keywords: 'dns verify' });
    if (superAdmin) actions.push({ id: 'key', label: 'Create an API key', group: 'Actions', icon: Plus, href: '/keys/?new=1', keywords: 'token' });
    return [...pages, ...actions];
  }, [superAdmin, canManage]);

  const results = useMemo(() => {
    const q = query.trim().toLowerCase();
    if (!q) return commands;
    return commands.filter((c) => `${c.label} ${c.keywords || ''}`.toLowerCase().includes(q));
  }, [commands, query]);

  useEffect(() => {
    if (!open) return;
    setQuery('');
    setIndex(0);
  }, [open]);

  useEffect(() => setIndex(0), [query]);

  if (!open) return null;

  const run = (c: Command | undefined) => {
    if (!c) return;
    onClose();
    router.push(c.href);
  };

  const onKey = (e: React.KeyboardEvent) => {
    if (e.key === 'ArrowDown') { e.preventDefault(); setIndex((i) => Math.min(results.length - 1, i + 1)); }
    if (e.key === 'ArrowUp') { e.preventDefault(); setIndex((i) => Math.max(0, i - 1)); }
    if (e.key === 'Enter') { e.preventDefault(); run(results[index]); }
    if (e.key === 'Escape') onClose();
  };

  let lastGroup = '';
  return (
    <div className="fixed inset-0 z-[65] flex items-start justify-center bg-black/50 p-4 pt-[12vh] backdrop-blur-[2px]" onMouseDown={(e) => { if (e.target === e.currentTarget) onClose(); }}>
      <div className="w-full max-w-xl animate-pop overflow-hidden rounded-2xl bg-surface shadow-pop" role="dialog" aria-modal="true" aria-label="Quick search">
        <div className="flex items-center gap-3 border-b border-line px-4">
          <Search className="size-4 text-muted" />
          <input
            ref={inputRef}
            autoFocus
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            onKeyDown={onKey}
            placeholder="Search pages and actions…"
            className="h-13 flex-1 bg-transparent text-[15px] text-ink outline-none placeholder:text-faint"
          />
          <Kbd>Esc</Kbd>
        </div>
        <div className="max-h-[50vh] overflow-y-auto p-2">
          {results.length === 0 ? <p className="px-3 py-8 text-center text-sm text-muted">No results for “{query}”</p> : null}
          {results.map((c, i) => {
            const header = c.group !== lastGroup ? c.group : null;
            lastGroup = c.group;
            const Icon = c.icon;
            return (
              <div key={c.id}>
                {header ? <div className="px-3 pt-3 pb-1.5 text-[11px] font-medium tracking-wide text-faint uppercase">{header}</div> : null}
                <button
                  type="button"
                  onMouseMove={() => setIndex(i)}
                  onClick={() => run(c)}
                  className={clsx('flex w-full cursor-pointer items-center gap-3 rounded-lg px-3 py-2 text-left text-sm', i === index ? 'bg-accent-soft text-ink' : 'text-ink-2')}
                >
                  <Icon className={clsx('size-4', i === index ? 'text-accent' : 'text-muted')} />
                  <span className="flex-1">{c.label}</span>
                  {i === index ? <CornerDownLeft className="size-3.5 text-muted" /> : null}
                </button>
              </div>
            );
          })}
        </div>
      </div>
    </div>
  );
}
