import clsx from 'clsx';
import { BookOpen, ChevronsUpDown, LogOut, Menu, Search, X } from 'lucide-react';
import Link from 'next/link';
import { useRouter } from 'next/router';
import { ReactNode, useEffect, useRef, useState } from 'react';
import { compact, number } from '../lib/format';
import { useSession, useMe } from '../lib/session';
import CommandMenu from './CommandMenu';
import Logo from './Logo';
import { currentItem, isActive, navGroups } from './nav';
import ThemeToggle from './ThemeToggle';
import { Badge, Button, Callout, Kbd, Meter } from './ui';

const workspacePauseReasons: Record<string, string> = {
  bounce_rate_exceeded: 'too many of your emails hard bounced',
  complaint_rate_exceeded: 'too many recipients marked your emails as spam',
};

function initials(name: string) {
  return name.split(/\s+/).map((p) => p[0]).join('').slice(0, 2).toUpperCase();
}

function WorkspaceSwitcher() {
  const me = useMe();
  const { trial, logout } = useSession();
  const router = useRouter();
  const [open, setOpen] = useState(false);
  const ref = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (!open) return undefined;
    const close = (e: MouseEvent) => { if (!ref.current?.contains(e.target as Node)) setOpen(false); };
    document.addEventListener('mousedown', close);
    return () => document.removeEventListener('mousedown', close);
  }, [open]);
  return (
    <div className="relative" ref={ref}>
      <button
        type="button"
        onClick={() => setOpen((o) => !o)}
        className="flex w-full cursor-pointer items-center gap-2.5 rounded-lg px-2 py-1.5 text-left transition-colors hover:bg-surface-3"
      >
        <span className="grid size-7 shrink-0 place-items-center rounded-md bg-gradient-to-br from-accent to-[#ff4d00] text-[11px] font-semibold text-white">{initials(me.workspace.name)}</span>
        <span className="min-w-0 flex-1">
          <span className="block truncate text-[13px] font-medium text-ink">{me.workspace.name}</span>
          <span className="block truncate text-[11px] text-muted">{trial ? 'Trial workspace' : 'Paid workspace'}</span>
        </span>
        <ChevronsUpDown className="size-3.5 text-muted" />
      </button>
      {open ? (
        <div className="absolute top-full right-0 left-0 z-50 mt-1 animate-pop rounded-xl bg-surface p-1.5 shadow-pop">
          {me.user ? (
            <div className="px-2.5 py-2">
              <div className="truncate text-[13px] font-medium text-ink">{me.user.name}</div>
              <div className="truncate text-xs text-muted">{me.user.email}</div>
              <div className="mt-2"><Badge value={me.user.role} /></div>
            </div>
          ) : null}
          <div className="my-1 h-px bg-line" />
          <button
            type="button"
            onClick={async () => { await logout(); router.push('/login/'); }}
            className="flex w-full cursor-pointer items-center gap-2 rounded-md px-2.5 py-2 text-[13px] text-ink-2 hover:bg-surface-3 hover:text-ink"
          >
            <LogOut className="size-4" />Log out
          </button>
        </div>
      ) : null}
    </div>
  );
}

function UsageCard() {
  const me = useMe();
  const { trial, showUpgrade } = useSession();
  if (trial) {
    const { trial_used: used, trial_limit: limit } = me.quota;
    return (
      <div className="rounded-xl border border-line bg-surface p-3">
        <div className="mb-2 flex items-center justify-between text-xs"><span className="font-medium text-ink-2">Trial sends</span><span className="text-muted tabular-nums">{number(used)} / {number(limit)}</span></div>
        <Meter value={used} max={limit} />
        <Button size="xs" variant="primary" className="mt-3 w-full" onClick={() => showUpgrade()}>Upgrade</Button>
      </div>
    );
  }
  const { sent_this_month: sent, monthly_limit: limit } = me.quota;
  return (
    <Link href="/settings/" className="block rounded-xl border border-line bg-surface p-3 transition-colors hover:border-line-strong">
      <div className="mb-2 flex items-center justify-between text-xs">
        <span className="font-medium text-ink-2">This month</span>
        <span className="text-muted tabular-nums">{compact(sent)}{limit > 0 ? ` / ${compact(limit)}` : ''}</span>
      </div>
      {limit > 0 ? <Meter value={sent} max={limit} /> : <div className="text-[11px] text-muted">No monthly cap</div>}
    </Link>
  );
}

function Sidebar({ onNavigate, onSearch }: { onNavigate: () => void; onSearch: () => void }) {
  const router = useRouter();
  const { superAdmin } = useSession();
  return (
    <div className="flex h-full flex-col gap-3 px-3 py-4">
      <div className="flex items-center justify-between px-2 pb-1">
        <Link href="/" onClick={onNavigate}><Logo /></Link>
      </div>
      <WorkspaceSwitcher />
      <button
        type="button"
        onClick={onSearch}
        className="flex h-8 w-full cursor-pointer items-center gap-2 rounded-lg border border-line bg-surface px-2.5 text-[13px] text-muted transition-colors hover:border-line-strong hover:text-ink-2"
      >
        <Search className="size-3.5" />
        <span className="flex-1 text-left">Quick search…</span>
        <Kbd>⌘K</Kbd>
      </button>
      <nav className="no-scrollbar -mx-1 flex-1 overflow-y-auto px-1">
        {navGroups.map((group) => {
          const items = group.items.filter((i) => !i.superAdmin || superAdmin);
          if (!items.length) return null;
          return (
            <div key={group.label || 'root'} className="mb-3">
              {group.label ? <div className="px-2.5 pt-2 pb-1.5 text-[11px] font-medium tracking-wide text-faint uppercase">{group.label}</div> : null}
              <div className="flex flex-col gap-px">
                {items.map((item) => {
                  const active = isActive(router.pathname, item);
                  const Icon = item.icon;
                  return (
                    <Link
                      key={item.href}
                      href={item.href}
                      onClick={onNavigate}
                      className={clsx(
                        'group relative flex h-8 items-center gap-2.5 rounded-lg px-2.5 text-[13.5px] transition-colors',
                        active ? 'bg-accent-soft font-medium text-ink' : 'text-ink-2 hover:bg-surface-3 hover:text-ink',
                      )}
                    >
                      {active ? <span className="absolute top-1.5 bottom-1.5 left-0 w-[3px] rounded-r-full bg-accent" /> : null}
                      <Icon className={clsx('size-4', active ? 'text-accent' : 'text-muted group-hover:text-ink-2')} />
                      {item.label}
                    </Link>
                  );
                })}
              </div>
            </div>
          );
        })}
      </nav>
      <UsageCard />
    </div>
  );
}

function Topbar({ onMenu }: { onMenu: () => void }) {
  const me = useMe();
  const router = useRouter();
  const current = currentItem(router.pathname);
  return (
    <div className="sticky top-0 z-30 flex h-14 items-center gap-3 border-b border-line bg-bg/80 px-4 backdrop-blur-md sm:px-6">
      <button type="button" onClick={onMenu} aria-label="Open menu" className="-ml-1 grid size-8 cursor-pointer place-items-center rounded-md text-muted hover:bg-surface-3 hover:text-ink lg:hidden"><Menu className="size-5" /></button>
      <nav aria-label="Breadcrumb" className="flex min-w-0 items-center gap-2 text-[13px]">
        <span className="hidden truncate text-muted sm:inline">{me.workspace.name}</span>
        {current?.group ? <><span className="hidden text-faint sm:inline">/</span><span className="hidden text-muted sm:inline">{current.group}</span></> : null}
        {current ? <><span className="hidden text-faint sm:inline">/</span><Link href={current.item.href} className="truncate font-medium text-ink">{current.item.label}</Link></> : null}
      </nav>
      <div className="ml-auto flex items-center gap-1">
        <a href="https://github.com/forke-org/forke-mail#readme" target="_blank" rel="noreferrer" className="hidden h-8 items-center gap-1.5 rounded-md px-2.5 text-[13px] text-muted transition-colors hover:bg-surface-3 hover:text-ink sm:inline-flex"><BookOpen className="size-4" />Docs</a>
        <ThemeToggle />
        {me.user ? <span title={me.user.email} className="ml-1 grid size-8 place-items-center rounded-full bg-surface-3 text-[11px] font-semibold text-ink-2 ring-1 ring-line-strong">{initials(me.user.name)}</span> : null}
      </div>
    </div>
  );
}

export default function Layout({ children }: { children: ReactNode }) {
  const me = useMe();
  const { exhausted, showUpgrade } = useSession();
  const router = useRouter();
  const [drawer, setDrawer] = useState(false);
  const [search, setSearch] = useState(false);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'k') {
        e.preventDefault();
        setSearch((s) => !s);
      }
    };
    document.addEventListener('keydown', onKey);
    return () => document.removeEventListener('keydown', onKey);
  }, []);

  useEffect(() => setDrawer(false), [router.asPath]);

  const monthFull = me.quota.monthly_limit > 0 && me.quota.sent_this_month >= me.quota.monthly_limit;

  return (
    <div className="min-h-screen">
      <aside className="fixed inset-y-0 left-0 z-40 hidden w-[248px] border-r border-line bg-sidebar lg:block">
        <Sidebar onNavigate={() => {}} onSearch={() => setSearch(true)} />
      </aside>
      {drawer ? (
        <div className="fixed inset-0 z-50 lg:hidden">
          <div className="absolute inset-0 bg-black/50" onClick={() => setDrawer(false)} />
          <aside className="absolute inset-y-0 left-0 w-[272px] animate-[fade-up_0.2s_ease] border-r border-line bg-sidebar">
            <button type="button" onClick={() => setDrawer(false)} aria-label="Close menu" className="absolute top-4 right-3 grid size-8 cursor-pointer place-items-center rounded-md text-muted hover:bg-surface-3"><X className="size-4" /></button>
            <Sidebar onNavigate={() => setDrawer(false)} onSearch={() => { setDrawer(false); setSearch(true); }} />
          </aside>
        </div>
      ) : null}
      <div className="lg:pl-[248px]">
        <Topbar onMenu={() => setDrawer(true)} />
        <main className="mx-auto w-full max-w-[1200px] px-4 py-6 sm:px-6 lg:px-8 lg:py-8">
          <div className="mb-5 flex flex-col gap-3 empty:hidden">
            {me.workspace.sending_paused ? (
              <Callout tone="danger" title="Sending is paused for this workspace">
                Because {workspacePauseReasons[me.workspace.pause_reason || ''] || 'of its sending reputation'} in the last 30 days. Clean your contact list, then contact support to resume.
              </Callout>
            ) : null}
            {monthFull ? (
              <Callout tone="warn" title="Monthly sending cap reached">
                {number(me.quota.sent_this_month)} of {number(me.quota.monthly_limit)} emails sent this month. Sending resumes on the 1st.
              </Callout>
            ) : null}
            {exhausted ? (
              <Callout tone="warn" title="Trial limit used up" action={<Button size="sm" onClick={() => showUpgrade()}>Upgrade</Button>}>
                Emails can&apos;t be sent until you upgrade.
              </Callout>
            ) : null}
          </div>
          {children}
        </main>
      </div>
      <CommandMenu open={search} onClose={() => setSearch(false)} />
    </div>
  );
}
