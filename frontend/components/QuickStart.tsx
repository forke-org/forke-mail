/**
 * @fileoverview Forke Platform
 * @copyright (c) 2026 Forke Inc. (https://www.forke.space/)
 *
 * Source-Available License (Non-Commercial / Fair Source).
 * This source code is open for inspection, learning, and personal development.
 * Commercial use, hosting, or resale as a paid service without an explicit
 * commercial license from Forke Inc. is strictly prohibited.
 */

import clsx from 'clsx';
import { ArrowRight, Plug, Check, Code2, Megaphone, ShieldCheck, Users, KeyRound, Send } from 'lucide-react';
import Link from 'next/link';
import { useEffect, useState } from 'react';
import { ButtonLink, Card, CopyButton, Meter, Tabs } from './ui';

export type Goal = 'transactional' | 'campaigns';

type Step = { icon: typeof Send; label: string; hint: string; done: boolean; href: string; cta: string; required?: boolean };

const storageKey = (ws: string) => `forke-mail-goal-${ws}`;

export function useGoal(workspaceID: string): [Goal | null, (g: Goal | null) => void] {
  const [goal, setGoal] = useState<Goal | null>(null);
  useEffect(() => {
    const saved = window.localStorage.getItem(storageKey(workspaceID));
    if (saved === 'transactional' || saved === 'campaigns') setGoal(saved);
  }, [workspaceID]);
  const update = (g: Goal | null) => {
    if (g) window.localStorage.setItem(storageKey(workspaceID), g);
    else window.localStorage.removeItem(storageKey(workspaceID));
    setGoal(g);
  };
  return [goal, update];
}

export function SetupBanner({ onDismiss }: { onDismiss: () => void }) {
  const choices: { path: 'ai' | 'manual'; icon: typeof Send; title: string; body: string }[] = [
    { path: 'ai', icon: Plug, title: 'With an AI agent', body: 'Connect Claude, ChatGPT or Cursor and let it set up email for you.' },
    { path: 'manual', icon: Code2, title: 'Step by step myself', body: 'Domain, test key and your first email with copy-paste code.' },
  ];
  return (
    <section className="mb-6 overflow-hidden rounded-[10px] bg-surface shadow-card">
      <div className="flex items-start justify-between gap-4 border-b border-line px-5 py-4">
        <div>
          <h2 className="text-[15px] font-medium tracking-[-0.01em] text-ink">Get set up in about 5 minutes</h2>
          <p className="mt-0.5 text-[13px] text-muted">Pick how you&apos;d like to work. You can switch any time.</p>
        </div>
        <button type="button" onClick={onDismiss} className="text-xs text-muted hover:text-ink">Hide</button>
      </div>
      <div className="grid gap-px bg-line md:grid-cols-2">
        {choices.map((c) => (
          <Link key={c.path} href={`/setup/?path=${c.path}`} className="group flex items-start gap-4 bg-surface px-5 py-4 transition-colors hover:bg-surface-2">
            <span className="grid size-10 shrink-0 place-items-center rounded-[8px] bg-accent-soft text-accent shadow-[inset_0_0_0_1px_var(--accent-line)]"><c.icon className="size-4" /></span>
            <span className="min-w-0 flex-1">
              <span className="block text-sm font-medium text-ink">{c.title}</span>
              <span className="mt-0.5 block text-xs text-muted">{c.body}</span>
            </span>
            <ArrowRight className="mt-1 size-4 text-muted transition-transform group-hover:translate-x-0.5 group-hover:text-accent" />
          </Link>
        ))}
      </div>
    </section>
  );
}

type Lang = 'curl' | 'node' | 'python';

function snippet(lang: Lang, origin: string, from: string, to: string) {
  const body = { from_address: from, to, subject: 'Hello from Forke Mail', html: '<p>Your first email is live.</p>' };
  if (lang === 'curl') {
    return `curl -X POST ${origin}/api/transactional \\
  -H "Authorization: Bearer $FORKE_MAIL_API_KEY" \\
  -H "Content-Type: application/json" \\
  -d '${JSON.stringify(body)}'`;
  }
  if (lang === 'node') {
    return `await fetch('${origin}/api/transactional', {
  method: 'POST',
  headers: {
    Authorization: \`Bearer \${process.env.FORKE_MAIL_API_KEY}\`,
    'Content-Type': 'application/json',
  },
  body: JSON.stringify(${JSON.stringify(body, null, 2).replace(/\n/g, '\n  ')}),
});`;
  }
  return `import os, requests

requests.post(
    "${origin}/api/transactional",
    headers={"Authorization": f"Bearer {os.environ['FORKE_MAIL_API_KEY']}"},
    json=${JSON.stringify(body)},
)`;
}

export function SendSnippet({ from, to }: { from: string; to: string }) {
  const [lang, setLang] = useState<Lang>('curl');
  const [origin, setOrigin] = useState('https://mail.forke.space');
  useEffect(() => setOrigin(window.location.origin), []);
  const code = snippet(lang, origin, from, to);
  return (
    <div className="overflow-hidden rounded-[6px] bg-surface-2 shadow-[inset_0_0_0_1px_var(--line)]">
      <div className="flex items-center justify-between gap-3 border-b border-line px-3 py-2">
        <Tabs value={lang} onChange={setLang} options={[['curl', 'cURL'], ['node', 'Node.js'], ['python', 'Python']]} />
        <CopyButton value={code} label="Copy" />
      </div>
      <pre className="overflow-x-auto p-4 font-mono text-[12px] leading-relaxed text-ink-2">{code}</pre>
    </div>
  );
}

export function QuickStart({ goal, domains, hasKey, canKeys, contacts, sent, campaigns, email, onChange }: {
  goal: Goal; domains: string[]; hasKey: boolean; canKeys: boolean; contacts: number; sent: number; campaigns: number; email: string; onChange: () => void;
}) {
  const domain = domains.find((d) => !d.includes('@')) || '';
  const from = domains.find((d) => d.includes('@')) || (domain ? `hello@${domain}` : 'hello@yourdomain.com');
  const steps: Step[] = goal === 'transactional'
    ? [
      { icon: ShieldCheck, label: 'Verify your sending domain', hint: 'Two DKIM records and you can send from any address on it.', done: domains.length > 0, href: '/domains/', cta: 'Verify domain', required: true },
      ...(canKeys ? [{ icon: KeyRound, label: 'Create an API key', hint: 'Keep it on your server. Never put it in browser code.', done: hasKey, href: '/keys/', cta: 'Create key' }] : []),
      { icon: Send, label: 'Send your first email', hint: 'One API call. Pick your language and drop it into your app.', done: sent > 0, href: '/transactional/?test=1', cta: 'Send test' },
    ]
    : [
      { icon: ShieldCheck, label: 'Verify your sending domain', hint: 'Proves you own the address your campaigns come from.', done: domains.length > 0, href: '/domains/', cta: 'Verify domain', required: true },
      { icon: Users, label: 'Add your contacts', hint: 'Import a CSV or add them through the API.', done: contacts > 0, href: '/contacts/', cta: 'Add contacts' },
      { icon: Megaphone, label: 'Draft your first campaign', hint: 'Write it, preview it, send a test, then send it.', done: campaigns > 0 && sent > 0, href: '/campaigns/', cta: 'Draft campaign' },
    ];
  const done = steps.filter((s) => s.done).length;
  if (done === steps.length) return null;
  return (
    <Card
      title={goal === 'transactional' ? 'Send your first email' : 'Set up for campaigns'}
      description={`${done} of ${steps.length} done`}
      actions={<div className="flex items-center gap-3"><Meter value={done} max={steps.length} tone="accent" className="w-24" /><button type="button" onClick={onChange} className="text-xs text-muted hover:text-ink">Change goal</button></div>}
      flush
      className="mb-6"
    >
      <ol className="divide-y divide-line">
        {steps.map((s, i) => (
          <li key={s.label} className="px-5 py-4">
            <div className="flex items-center gap-4">
              <span className={clsx('grid size-7 shrink-0 place-items-center rounded-full text-xs font-semibold', s.done ? 'bg-success-soft text-success' : 'bg-surface-3 text-ink-2')}>
                {s.done ? <Check className="size-3.5" /> : i + 1}
              </span>
              <span className="grid size-9 shrink-0 place-items-center rounded-[6px] bg-surface-2 text-ink-2 shadow-[inset_0_0_0_1px_var(--line)]"><s.icon className="size-4" /></span>
              <div className="min-w-0 flex-1">
                <div className={clsx('flex items-center gap-2 text-sm font-medium', s.done ? 'text-muted line-through' : 'text-ink')}>
                  {s.label}
                  {s.required && !s.done ? <span className="rounded-[3px] bg-ink px-1.5 py-0.5 text-[10px] font-semibold tracking-wide text-surface uppercase no-underline">Required</span> : null}
                </div>
                <div className="text-xs text-muted">{s.hint}</div>
              </div>
              {s.done ? null : <ButtonLink size="sm" href={s.href}>{s.cta}<ArrowRight /></ButtonLink>}
            </div>
            {goal === 'transactional' && s.icon === Send && !s.done ? <div className="mt-4 sm:pl-[92px]"><SendSnippet from={from} to={email} /></div> : null}
          </li>
        ))}
      </ol>
    </Card>
  );
}
