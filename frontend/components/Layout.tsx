import { AsciiReveal } from 'ascii-reveal';
import clsx from 'clsx';
import { ChevronsUpDown, LogOut, Menu, Search, Settings, X } from 'lucide-react';
import Link from 'next/link';
import { useRouter } from 'next/router';
import { ReactNode, useEffect, useRef, useState } from 'react';
import { compact, number } from '../lib/format';
import { useSession, useMe } from '../lib/session';
import CommandMenu from './CommandMenu';
import ScrollArea from './ScrollArea';
import Logo from './Logo';
import { currentItem, isActive, navGroups } from './nav';
import { Badge, Button, Callout, Kbd, Meter } from './ui';

const workspacePauseReasons: Record<string, string> = {
  bounce_rate_exceeded: 'too many of your emails hard bounced',
  complaint_rate_exceeded: 'too many recipients marked your emails as spam',
};

function initials(name: string) {
  return name.split(/\s+/).map((p) => p[0]).join('').slice(0, 2).toUpperCase();
}

function Avatar({ name, className }: { name: string; className?: string }) {
  return (
    <span className={clsx('grid shrink-0 place-items-center rounded-[6px] bg-gradient-to-br from-[#ffb25c] to-accent font-semibold text-[#1a0a00]', className)}>
      {initials(name)}
    </span>
  );
}

function Usage() {
  const me = useMe();
  const { trial, showUpgrade } = useSession();
  const used = trial ? me.quota.trial_used : me.quota.sent_this_month;
  const limit = trial ? me.quota.trial_limit : me.quota.monthly_limit;
  return (
    <div className="px-1">
      <div className="mb-2 flex items-center justify-between text-xs">
        <span className="font-mono text-[10.5px] tracking-[0.08em] text-faint uppercase">{trial ? 'Trial sends' : 'This month'}</span>
        <span className="text-muted tabular-nums">{trial ? `${number(used)} / ${number(limit)}` : `${compact(used)}${limit > 0 ? ` / ${compact(limit)}` : ''}`}</span>
      </div>
      {limit > 0 ? <Meter value={used} max={limit} /> : <div className="text-[11px] text-muted">No monthly cap</div>}
      {trial ? <Button size="xs" variant="primary" className="mt-3 w-full" onClick={() => showUpgrade()}>Upgrade plan</Button> : null}
    </div>
  );
}

function AccountMenu({ onNavigate }: { onNavigate: () => void }) {
  const me = useMe();
  const { trial, logout } = useSession();
  const router = useRouter();
  const [open, setOpen] = useState(false);
  const ref = useRef<HTMLDivElement>(null);
  const name = me.user?.name || me.workspace.name;
  useEffect(() => {
    if (!open) return undefined;
    const close = (e: MouseEvent) => { if (!ref.current?.contains(e.target as Node)) setOpen(false); };
    const esc = (e: KeyboardEvent) => { if (e.key === 'Escape') setOpen(false); };
    document.addEventListener('mousedown', close);
    document.addEventListener('keydown', esc);
    return () => { document.removeEventListener('mousedown', close); document.removeEventListener('keydown', esc); };
  }, [open]);
  return (
    <div className="relative" ref={ref}>
      {open ? (
        <div className="absolute right-0 bottom-full left-0 z-50 mb-2 animate-pop overflow-hidden rounded-[8px] bg-surface shadow-pop">
          <div className="flex items-center gap-3 border-b border-line bg-surface-2 px-3 py-3">
            <Avatar name={name} className="size-9 text-xs" />
            <div className="min-w-0">
              <div className="truncate text-[13px] font-medium text-ink">{name}</div>
              <div className="truncate text-xs text-muted">{me.user?.email || 'API key session'}</div>
            </div>
          </div>
          <div className="flex flex-col gap-2 border-b border-line px-3 py-3 text-xs">
            <div className="flex items-center justify-between gap-2"><span className="text-muted">Workspace</span><span className="truncate font-medium text-ink">{me.workspace.name}</span></div>
            <div className="flex items-center justify-between gap-2"><span className="text-muted">Plan</span><Badge value={trial ? 'trial' : 'paid'} /></div>
            {me.user ? <div className="flex items-center justify-between gap-2"><span className="text-muted">Role</span><Badge value={me.user.role} /></div> : null}
          </div>
          <div className="p-1">
            <Link href="/settings/" onClick={() => { setOpen(false); onNavigate(); }} className="flex items-center gap-2 rounded-[5px] px-2.5 py-2 text-[13px] text-ink-2 hover:bg-surface-3 hover:text-ink">
              <Settings className="size-4" />Settings
            </Link>
            <button
              type="button"
              onClick={async () => { await logout(); router.push('/login/'); }}
              className="flex w-full cursor-pointer items-center gap-2 rounded-[5px] px-2.5 py-2 text-[13px] text-ink-2 hover:bg-surface-3 hover:text-ink"
            >
              <LogOut className="size-4" />Log out
            </button>
          </div>
        </div>
      ) : null}
      <button
        type="button"
        aria-expanded={open}
        onClick={() => setOpen((o) => !o)}
        className={clsx('flex w-full cursor-pointer items-center gap-2.5 rounded-[6px] p-1.5 text-left transition-colors hover:bg-surface-3', open && 'bg-surface-3')}
      >
        <Avatar name={name} className="size-8 text-[11px]" />
        <span className="min-w-0 flex-1">
          <span className="block truncate text-[13px] font-medium text-ink">{name}</span>
          <span className="block truncate text-[11.5px] text-muted">{me.workspace.name} · {trial ? 'Trial' : 'Paid'}</span>
        </span>
        <ChevronsUpDown className="size-3.5 shrink-0 text-muted" />
      </button>
    </div>
  );
}

function Sidebar({ onNavigate, onSearch }: { onNavigate: () => void; onSearch: () => void }) {
  const router = useRouter();
  const { superAdmin } = useSession();
  return (
    <div className="flex h-full flex-col">
      <div className="relative flex h-14 shrink-0 items-center px-5">
        <Link href="/" onClick={onNavigate}><Logo /></Link>
        <span aria-hidden className="rail-x pointer-events-none absolute inset-x-0 bottom-0 h-px" />
      </div>
      <div className="px-3 pt-3">
        <button
          type="button"
          onClick={onSearch}
          className="flex h-8 w-full cursor-pointer items-center gap-2 rounded-[6px] bg-surface px-2.5 text-[13px] text-muted shadow-[inset_0_0_0_1px_var(--line)] transition-colors hover:text-ink-2 hover:shadow-[inset_0_0_0_1px_var(--line-strong)]"
        >
          <Search className="size-3.5" />
          <span className="flex-1 text-left">Search</span>
          <Kbd>⌘K</Kbd>
        </button>
      </div>
      <nav className="no-scrollbar flex-1 overflow-y-auto px-3 py-3">
        {navGroups.map((group) => {
          const items = group.items.filter((i) => !i.superAdmin || superAdmin);
          if (!items.length) return null;
          return (
            <div key={group.label || 'root'} className="mb-4 last:mb-0">
              {group.label ? <div className="px-2.5 pb-1.5 font-mono text-[10.5px] tracking-[0.08em] text-faint uppercase">{group.label}</div> : null}
              <div className="flex flex-col gap-px">
                {items.map((item) => {
                  const active = isActive(router.pathname, item);
                  const Icon = item.icon;
                  return (
                    <Link
                      key={item.href}
                      href={item.href}
                      onClick={onNavigate}
                      aria-current={active ? 'page' : undefined}
                      className={clsx(
                        'group relative flex h-8 items-center gap-2.5 rounded-[6px] px-2.5 text-[13.5px] transition-colors',
                        active ? 'bg-surface-3 font-medium text-ink' : 'text-ink-2 hover:bg-surface hover:text-ink',
                      )}
                    >
                      {active ? <span className="absolute top-2 bottom-2 -left-3 w-[2px] rounded-r-[2px] bg-accent" /> : null}
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
      <div className="relative flex flex-col gap-4 px-3 pt-4 pb-3">
        <span aria-hidden className="rail-x pointer-events-none absolute inset-x-0 top-0 h-px" />
        <Usage />
        <AccountMenu onNavigate={onNavigate} />
      </div>
    </div>
  );
}

function Topbar({ onMenu, onSearch }: { onMenu: () => void; onSearch: () => void }) {
  const router = useRouter();
  const current = currentItem(router.pathname);
  return (
    <div className="relative z-30 flex h-14 shrink-0 items-center gap-2 bg-bg px-4 sm:px-6 lg:px-8">
      <span aria-hidden className="rail-x pointer-events-none absolute inset-x-0 bottom-0 h-px" />
      <button type="button" onClick={onMenu} aria-label="Open menu" className="-ml-1.5 grid size-8 cursor-pointer place-items-center rounded-[6px] text-muted hover:bg-surface-3 hover:text-ink lg:hidden"><Menu className="size-[18px]" /></button>
      <nav aria-label="Breadcrumb" className="ml-1 flex min-w-0 items-center gap-2 text-[13px] lg:ml-0">
        {current?.group ? <><span className="hidden text-muted sm:inline">{current.group}</span><span className="hidden text-faint sm:inline">/</span></> : null}
        {current ? <Link href={current.item.href} className="truncate font-medium text-ink">{current.item.label}</Link> : null}
      </nav>
      <button type="button" onClick={onSearch} aria-label="Search" className="ml-auto grid size-8 cursor-pointer place-items-center rounded-[6px] text-muted hover:bg-surface-3 hover:text-ink lg:hidden"><Search className="size-4" /></button>
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

  useEffect(() => {
    document.body.style.overflow = drawer ? 'hidden' : '';
    return () => { document.body.style.overflow = ''; };
  }, [drawer]);

  const monthFull = me.quota.monthly_limit > 0 && me.quota.sent_this_month >= me.quota.monthly_limit;

  return (
    <div className="h-[100dvh] overflow-hidden">
      <aside className="fixed inset-y-0 left-0 z-40 hidden w-[240px] bg-sidebar lg:block">
        <span aria-hidden className="rail-y pointer-events-none absolute inset-y-0 right-0 w-px" />
        <Sidebar onNavigate={() => {}} onSearch={() => setSearch(true)} />
      </aside>
      {drawer ? (
        <div className="fixed inset-0 z-50 lg:hidden">
          <div className="absolute inset-0 bg-black/60 backdrop-blur-[2px]" onClick={() => setDrawer(false)} />
          <aside className="absolute inset-y-0 left-0 w-[min(280px,85vw)] animate-[fade-up_0.2s_ease] bg-sidebar shadow-pop">
            <button type="button" onClick={() => setDrawer(false)} aria-label="Close menu" className="absolute top-3 right-3 z-10 grid size-8 cursor-pointer place-items-center rounded-[6px] text-muted hover:bg-surface-3"><X className="size-4" /></button>
            <Sidebar onNavigate={() => setDrawer(false)} onSearch={() => { setDrawer(false); setSearch(true); }} />
          </aside>
        </div>
      ) : null}
      <div className="flex h-full flex-col lg:pl-[240px]">
        <Topbar onMenu={() => setDrawer(true)} onSearch={() => setSearch(true)} />
        <ScrollArea className="flex-1">
        <main className="relative mx-auto min-h-[calc(100dvh-56px)] w-full max-w-[1200px] px-4 py-6 sm:px-6 lg:px-8 lg:py-10">
          <span aria-hidden className="rail-y pointer-events-none absolute inset-y-0 left-0 hidden w-px min-[1480px]:block" />
          <span aria-hidden className="rail-y pointer-events-none absolute inset-y-0 right-0 hidden w-px min-[1480px]:block" />
          <span aria-hidden className="rail-square pointer-events-none absolute -top-[7px] -left-[7px] z-40 hidden min-[1480px]:block" />
          <span aria-hidden className="rail-square pointer-events-none absolute -top-[7px] -right-[7px] z-40 hidden min-[1480px]:block" />
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
          <AsciiReveal key={router.pathname} lazy={false}>{children}</AsciiReveal>
        </main>
        </ScrollArea>
      </div>
      <CommandMenu open={search} onClose={() => setSearch(false)} />
    </div>
  );
}
