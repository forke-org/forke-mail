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
import { ArrowRight, Check, Code2, Megaphone, ShieldCheck, Users, KeyRound, Send } from 'lucide-react';
import { useEffect, useState } from 'react';
import { ButtonLink, Card, CopyButton, Meter, Tabs } from './ui';

export type Goal = 'transactional' | 'campaigns';

type Step = { icon: typeof Send; label: string; hint: string; done: boolean; href: string; cta: string; required?: boolean };

const goals: { id: Goal; icon: typeof Send; title: string; time: string; body: string }[] = [
  { id: 'transactional', icon: Code2, title: 'Send transactional email', time: '~1 minute', body: 'Wire Forke Mail into your app for sign-in codes, password resets, receipts and notifications.' },
  { id: 'campaigns', icon: Megaphone, title: 'Run email campaigns', time: '~5 minutes', body: 'Import contacts, write a broadcast and send it to your audience with unsubscribe handled for you.' },
];

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

export function GoalPicker({ onPick }: { onPick: (g: Goal) => void }) {
  return (
    <div className="mb-6">
      <div className="mb-4">
        <h2 className="text-lg font-medium tracking-[-0.02em] text-ink">How do you plan to use Forke Mail?</h2>
        <p className="mt-1 text-sm text-muted">Pick the closest fit. You can do both once you&apos;re set up.</p>
      </div>
      <div className="grid gap-4 md:grid-cols-2">
        {goals.map((g) => (
          <button key={g.id} type="button" onClick={() => onPick(g.id)}
            className="group flex flex-col rounded-[8px] bg-surface p-5 text-left shadow-card transition-colors hover:bg-surface-2">
            <div className="flex items-start justify-between">
              <span className="grid size-10 place-items-center rounded-[6px] bg-surface-2 text-ink shadow-[inset_0_0_0_1px_var(--line)]"><g.icon className="size-4" /></span>
              <span className="text-xs text-faint">{g.time}</span>
            </div>
            <div className="mt-5 text-[15px] font-medium text-ink">{g.title}</div>
            <p className="mt-1.5 text-[13px] leading-relaxed text-muted">{g.body}</p>
            <span className="mt-5 inline-flex items-center gap-1.5 text-[13px] font-medium text-ink group-hover:text-accent">Start here<ArrowRight className="size-3.5" /></span>
          </button>
        ))}
      </div>
    </div>
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
