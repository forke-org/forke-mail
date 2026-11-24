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
import { ArrowLeft, ArrowRight, Bot, Check, Globe, KeyRound, Loader2, PartyPopper, Send, Wrench, X } from 'lucide-react';
import { useRouter } from 'next/router';
import { FormEvent, ReactNode, useCallback, useEffect, useState } from 'react';
import Logo from '../components/Logo';
import { agentApps, ConnectApps, PromptList, type AgentApp, type PromptCard } from '../components/Agents';
import { SendSnippet } from '../components/QuickStart';
import { Button, ButtonLink, Callout, CopyButton, Field, FormError, Input } from '../components/ui';
import { api, describeError } from '../lib/api';
import { useMe, useSession } from '../lib/session';
import type { Domain } from '../lib/types';

type Path = 'ai' | 'manual';
type Agents = { connections: { id: string; app: string }[]; prompts: PromptCard[]; mcp_url: string };

const flows: Record<Path, string[]> = {
  ai: ['Connect your app', 'Ask it to set things up', 'Done'],
  manual: ['Add your domain', 'Create an API key', 'Send your first email', 'Done'],
};

function Shell({ step, path, children, onBack, onSkip }: { step: number; path: Path | null; children: ReactNode; onBack?: () => void; onSkip: () => void }) {
  const labels = path ? flows[path] : [];
  return (
    <div className="relative flex min-h-[100dvh] flex-col bg-bg">
      <div aria-hidden className="pointer-events-none fixed inset-x-0 top-0 h-[420px] bg-[radial-gradient(ellipse_60%_100%_at_50%_0%,rgba(255,122,0,0.10),transparent_70%)]" />
      <header className="relative z-10 flex h-16 items-center justify-between px-5 sm:px-8">
        <Logo />
        <button type="button" onClick={onSkip} className="inline-flex h-8 items-center gap-1.5 rounded-[6px] px-3 text-[13px] text-muted transition-colors hover:bg-surface-2 hover:text-ink">Skip setup<X className="size-3.5" /></button>
      </header>
      <main className="relative z-10 flex flex-1 flex-col items-center justify-center px-5 pt-4 pb-16 sm:px-8">
        <div className="flex w-full max-w-[760px] flex-col gap-8">
          {labels.length ? (
            <div className="flex flex-col gap-4">
              <div className="flex h-6 items-center justify-center">
                {onBack ? <button type="button" onClick={onBack} className="inline-flex items-center gap-1.5 text-[13px] text-muted transition-colors hover:text-ink"><ArrowLeft className="size-3.5" />Back</button> : null}
              </div>
              <ol className="mx-auto flex w-full max-w-[560px] items-center gap-2">
                {labels.map((label, i) => (
                  <li key={label} className="flex flex-1 flex-col items-center gap-2 text-center">
                    <span className={clsx('h-1 w-full rounded-full transition-colors duration-500', i <= step ? 'bg-accent' : 'bg-surface-3')} />
                    <span className={clsx('hidden text-[11px] font-medium tracking-wide uppercase transition-colors sm:block', i === step ? 'text-ink' : 'text-faint')}>{label}</span>
                  </li>
                ))}
              </ol>
            </div>
          ) : null}
          <div key={`${path}-${step}`} className="animate-[fade-up_0.35s_ease_both]">{children}</div>
        </div>
      </main>
    </div>
  );
}

function Heading({ eyebrow, title, children }: { eyebrow: string; title: string; children?: ReactNode }) {
  return (
    <div className="mb-8 text-center">
      <p className="mb-2 font-mono text-[11px] tracking-[0.08em] text-accent uppercase">{eyebrow}</p>
      <h1 className="text-[28px] leading-tight font-medium tracking-[-0.03em] text-ink sm:text-[32px]">{title}</h1>
      {children ? <p className="mx-auto mt-2 max-w-[560px] text-sm text-muted">{children}</p> : null}
    </div>
  );
}

function Choose({ onPick }: { onPick: (p: Path) => void }) {
  const options: { id: Path; icon: typeof Bot; title: string; body: string; time: string; points: string[] }[] = [
    { id: 'ai', icon: Bot, title: 'With an AI agent', time: '~3 minutes', body: 'Connect Claude, ChatGPT, Cursor or your coding agent and let it set up email for you while you watch.',
      points: ['It adds your domain, keys and templates', 'It writes the code in your app', 'It asks you before anything real is sent'] },
    { id: 'manual', icon: Wrench, title: 'Step by step myself', time: '~5 minutes', body: 'Verify your domain, create a key and send your first email with copy-paste code.',
      points: ['Clear DNS records to copy', 'A test key so nothing goes out by accident', 'Code for cURL, Node.js and Python'] },
  ];
  return (
    <>
      <Heading eyebrow="Welcome to Forke Mail" title="Send your first email in about 5 minutes">How would you like to set things up? You can switch any time.</Heading>
      <div className="grid gap-4 md:grid-cols-2">
        {options.map((o, i) => (
          <button key={o.id} type="button" onClick={() => onPick(o.id)} style={{ animationDelay: `${i * 80}ms` }}
            className="group relative flex animate-[fade-up_0.45s_ease_both] flex-col items-center rounded-[10px] bg-surface p-6 text-center shadow-card transition-[background-color,transform,box-shadow] duration-200 hover:-translate-y-0.5 hover:bg-surface-2 active:translate-y-0">
            <span className="absolute top-4 right-4 text-xs text-faint">{o.time}</span>
            <div className="flex items-start justify-center">
              <span className="grid size-11 place-items-center rounded-[8px] bg-accent-soft text-accent shadow-[inset_0_0_0_1px_var(--accent-line)]"><o.icon className="size-5" /></span>
            </div>
            <div className="mt-5 text-base font-medium text-ink">{o.title}</div>
            <p className="mt-1.5 text-[13px] leading-relaxed text-muted">{o.body}</p>
            <ul className="mt-4 flex flex-col items-center gap-1.5">
              {o.points.map((p) => <li key={p} className="flex items-center gap-2 text-[13px] text-ink-2"><Check className="size-3.5 text-success" />{p}</li>)}
            </ul>
            <span className="mt-6 inline-flex items-center gap-1.5 text-[13px] font-medium text-ink transition-colors group-hover:text-accent">Start<ArrowRight className="size-3.5 transition-transform group-hover:translate-x-0.5" /></span>
          </button>
        ))}
      </div>
    </>
  );
}

function ConnectStep({ agents, onNext }: { agents: Agents; onNext: () => void }) {
  const [app, setApp] = useState<AgentApp>('claude');
  const [baseline] = useState(() => new Set(agents.connections.map((c) => c.id)));
  const [connected, setConnected] = useState<string | null>(null);
  useEffect(() => {
    if (connected) return undefined;
    const id = window.setInterval(async () => {
      try {
        const next = await api<Agents>('GET', '/api/agents');
        const fresh = next.connections.find((c) => !baseline.has(c.id));
        if (fresh) setConnected(fresh.app);
      } catch {
        return;
      }
    }, 3000);
    return () => window.clearInterval(id);
  }, [baseline, connected]);
  const appName = agentApps.find(([a]) => a === app)?.[1] || 'your app';
  return (
    <>
      <Heading eyebrow="Step 1" title={`Connect ${appName}`}>Add Forke Mail once. Your app opens a Forke Mail sign-in page where you pick this project and press Allow.</Heading>
      <div className="rounded-[10px] bg-surface p-5 shadow-card"><ConnectApps url={agents.mcp_url} app={app} onApp={setApp} /></div>
      <div className={clsx('mt-5 flex items-center gap-3 rounded-[10px] px-4 py-3.5 text-sm transition-colors duration-500', connected ? 'bg-success-soft text-success' : 'bg-surface-2 text-muted shadow-[inset_0_0_0_1px_var(--line)]')}>
        {connected ? <Check className="size-4 animate-[pop_0.3s_ease_both]" /> : <Loader2 className="size-4 animate-spin" />}
        <span className="flex-1">{connected ? `${connected} is connected.` : 'Waiting for your app to connect… this updates on its own.'}</span>
        <Button variant={connected ? 'primary' : 'ghost'} size="sm" onClick={onNext}>{connected ? 'Continue' : 'I connected it'}<ArrowRight /></Button>
      </div>
    </>
  );
}

function PromptStep({ prompts, onNext }: { prompts: PromptCard[]; onNext: () => void }) {
  return (
    <>
      <Heading eyebrow="Step 2" title="Ask it to set things up">Copy a prompt into your app. In Claude and other MCP apps they also appear in the prompt or slash-command menu. Start with the first one.</Heading>
      <PromptList prompts={prompts} highlight="setup-app" />
      <Callout title="You stay in control">Your agent asks in the chat before sending real email, creating keys or deleting anything, and says clearly when something can&apos;t be undone.</Callout>
      <div className="mt-8 flex justify-center"><Button variant="primary" onClick={onNext}>I&apos;m set<ArrowRight /></Button></div>
    </>
  );
}

function DomainStep({ onNext }: { onNext: () => void }) {
  const me = useMe();
  const verified = me.domains.find((d) => !d.includes('@'));
  const [name, setName] = useState('');
  const [domain, setDomain] = useState<Domain | null>(null);
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  const check = useCallback(async (d: Domain) => {
    try {
      setDomain(await api<Domain>('POST', `/api/domains/${d.id}/verify`));
    } catch (err) {
      setError(describeError(err));
    }
  }, []);
  useEffect(() => {
    if (!domain || domain.status === 'verified') return undefined;
    const id = window.setInterval(() => check(domain), 15000);
    return () => window.clearInterval(id);
  }, [domain, check]);
  const add = async (e: FormEvent) => {
    e.preventDefault();
    setBusy(true);
    setError('');
    try {
      setDomain(await api<Domain>('POST', '/api/domains', { name: name.trim().toLowerCase(), kind: 'domain' }));
    } catch (err) {
      setError(describeError(err));
    }
    setBusy(false);
  };
  if (verified && !domain) {
    return (
      <>
        <Heading eyebrow="Step 1" title="Your domain is ready">{verified} is verified, so you can send from any address on it.</Heading>
        <div className="flex justify-center"><Button variant="primary" onClick={onNext}>Continue<ArrowRight /></Button></div>
      </>
    );
  }
  return (
    <>
      <Heading eyebrow="Step 1" title="Add the domain you'll send from">Use your main domain, or a subdomain like mail.yourapp.com. Not ready? Skip it: test keys send from any address without delivering anything.</Heading>
      {!domain ? (
        <form onSubmit={add} className="flex flex-col gap-3 rounded-[10px] bg-surface p-5 shadow-card sm:flex-row sm:items-end">
          <Field label="Domain" className="flex-1"><Input required autoFocus placeholder="yourapp.com" value={name} onChange={(e) => setName(e.target.value)} /></Field>
          <Button variant="primary" type="submit" loading={busy}><Globe />Add domain</Button>
        </form>
      ) : (
        <div className="flex flex-col gap-4 rounded-[10px] bg-surface p-5 shadow-card">
          <div className="flex items-center justify-between gap-3">
            <div className="text-sm font-medium text-ink">{domain.name}</div>
            <span className={clsx('inline-flex items-center gap-1.5 text-xs font-medium', domain.status === 'verified' ? 'text-success' : 'text-warning')}>
              {domain.status === 'verified' ? <Check className="size-3.5" /> : <Loader2 className="size-3.5 animate-spin" />}
              {domain.status === 'verified' ? 'Verified' : 'Waiting for DNS · checks every 15 seconds'}
            </span>
          </div>
          <p className="text-[13px] text-muted">Add these records at your DNS provider (Cloudflare, Namecheap, GoDaddy…). Set Cloudflare records to DNS only.</p>
          <div className="overflow-x-auto rounded-[6px] shadow-[inset_0_0_0_1px_var(--line)]">
            <table className="w-full text-left text-[12.5px]">
              <thead className="bg-surface-2 text-muted"><tr><th className="px-3 py-2 font-medium">Type</th><th className="px-3 py-2 font-medium">Name</th><th className="px-3 py-2 font-medium">Value</th></tr></thead>
              <tbody className="divide-y divide-line">
                {domain.records.map((r) => (
                  <tr key={`${r.type}-${r.name}`}>
                    <td className="px-3 py-2 font-mono text-ink-2">{r.type}</td>
                    <td className="px-3 py-2"><span className="flex items-center gap-1.5 font-mono text-ink">{r.name}<CopyButton value={r.name} /></span></td>
                    <td className="px-3 py-2"><span className="flex items-center gap-1.5 font-mono break-all text-ink">{r.value}<CopyButton value={r.value} /></span></td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          <div className="flex justify-end gap-2"><Button onClick={() => check(domain)}>Check now</Button></div>
        </div>
      )}
      <FormError>{error}</FormError>
      <div className="mt-6 flex items-center justify-between">
        <button type="button" onClick={onNext} className="text-[13px] text-muted hover:text-ink">Skip for now and use test mode</button>
        <Button variant="primary" disabled={!domain || domain.status !== 'verified'} onClick={onNext}>Continue<ArrowRight /></Button>
      </div>
    </>
  );
}

function KeyStep({ onKey, onNext }: { onKey: (k: string) => void; onNext: () => void }) {
  const { superAdmin } = useSession();
  const [password, setPassword] = useState('');
  const [key, setKey] = useState('');
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  const create = async (e: FormEvent) => {
    e.preventDefault();
    setBusy(true);
    setError('');
    try {
      const created = await api<{ key: string }>('POST', '/api/keys', { name: 'Setup test key', scope: 'sending_access', mode: 'test', password });
      setKey(created.key);
      onKey(created.key);
    } catch (err) {
      setError(describeError(err));
    }
    setBusy(false);
  };
  if (!superAdmin) {
    return (
      <>
        <Heading eyebrow="Step 2" title="Get an API key">Only project owners can create keys. Ask an owner for a test key, then continue.</Heading>
        <div className="flex justify-center"><Button variant="primary" onClick={onNext}>Continue<ArrowRight /></Button></div>
      </>
    );
  }
  return (
    <>
      <Heading eyebrow="Step 2" title="Create a test API key">Test keys check every request exactly like live keys, but nothing is delivered and no credits are used. Switch to a live key when you launch.</Heading>
      {!key ? (
        <form onSubmit={create} className="flex flex-col gap-3 rounded-[10px] bg-surface p-5 shadow-card sm:flex-row sm:items-end">
          <Field label="Confirm your password" className="flex-1"><Input type="password" required autoFocus autoComplete="current-password" value={password} onChange={(e) => setPassword(e.target.value)} /></Field>
          <Button variant="primary" type="submit" loading={busy}><KeyRound />Create test key</Button>
        </form>
      ) : (
        <div className="flex flex-col gap-3 rounded-[10px] bg-surface p-5 shadow-card">
          <p className="text-[13px] text-muted">Copy it now; it won&apos;t be shown again. Put it in your server&apos;s environment as <code className="font-mono text-ink">FORKE_MAIL_API_KEY</code>.</p>
          <div className="flex items-center gap-2">
            <code className="flex h-9 min-w-0 flex-1 items-center truncate rounded-[6px] bg-surface-2 px-3 font-mono text-[13px] text-ink shadow-[inset_0_0_0_1px_var(--line)]">{key}</code>
            <CopyButton value={key} boxed />
          </div>
        </div>
      )}
      <FormError>{error}</FormError>
      <div className="mt-8 flex justify-center"><Button variant="primary" disabled={!key} onClick={onNext}>Continue<ArrowRight /></Button></div>
    </>
  );
}

function SendStep({ testKey, onNext }: { testKey: string; onNext: () => void }) {
  const me = useMe();
  const to = me.user?.email || 'you@example.com';
  const domain = me.domains.find((d) => !d.includes('@'));
  const from = domain ? `hello@${domain}` : 'hello@example.com';
  const [result, setResult] = useState<{ id: string; status: string } | null>(null);
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  const send = async () => {
    setBusy(true);
    setError('');
    try {
      const res = await fetch('/api/transactional', {
        method: 'POST',
        headers: { Authorization: `Bearer ${testKey}`, 'Content-Type': 'application/json' },
        body: JSON.stringify({ to, from_name: me.workspace.name, from_address: from, subject: 'Hello from Forke Mail', html: '<p>Your first email works.</p>' }),
      });
      const body = await res.json();
      if (!res.ok) throw new Error(body.error || 'send_failed');
      setResult({ id: body.id, status: body.status });
    } catch (err) {
      setError(err instanceof Error ? err.message.replace(/_/g, ' ') : 'Could not send.');
    }
    setBusy(false);
  };
  return (
    <>
      <Heading eyebrow="Step 3" title="Send your first email">This is all your app needs: one request with your key. Try it right here first.</Heading>
      <div className="flex flex-col gap-4 rounded-[10px] bg-surface p-5 shadow-card">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <div className="text-[13px] text-muted">Test send to <span className="text-ink">{to}</span> from <span className="text-ink">{from}</span></div>
          <Button variant="primary" onClick={send} loading={busy} disabled={!testKey || Boolean(result)}><Send />{result ? 'Sent' : 'Send test'}</Button>
        </div>
        {result ? (
          <div className="flex animate-[fade-up_0.3s_ease_both] items-center gap-2 rounded-[6px] bg-success-soft px-3 py-2.5 text-[13px] text-success">
            <Check className="size-4" />Accepted with status &ldquo;{result.status}&rdquo;. Test sends show in Deliveries and are never delivered.
          </div>
        ) : null}
        {!testKey ? <p className="text-[13px] text-muted">No key from the last step. Paste your key into the code below to try it from your app.</p> : null}
        <SendSnippet from={from} to={to} />
      </div>
      <FormError>{error}</FormError>
      <div className="mt-8 flex justify-center"><Button variant="primary" onClick={onNext}>Finish<ArrowRight /></Button></div>
    </>
  );
}

function Done({ path, onSwitch }: { path: Path; onSwitch: () => void }) {
  const next = path === 'ai'
    ? [['/templates/', 'Review the templates your agent made'], ['/deliveries/', 'Watch emails in Deliveries'], ['/connectors/', 'Manage connected apps']]
    : [['/templates/', 'Turn repeated emails into templates'], ['/contacts/', 'Add contacts for broadcasts'], ['/connectors/', 'Connect an AI agent']];
  return (
    <div className="flex flex-col items-center text-center">
      <span className="mb-5 grid size-14 animate-[pop_0.4s_ease_both] place-items-center rounded-full bg-accent-soft text-accent shadow-[0_0_40px_rgba(255,122,0,0.25)]"><PartyPopper className="size-6" /></span>
      <h1 className="text-[26px] font-medium tracking-[-0.03em] text-ink">You&apos;re set up</h1>
      <p className="mt-2 max-w-[460px] text-sm text-muted">Switch to a live key when you&apos;re ready to send real email. Here&apos;s what most people do next.</p>
      <div className="mt-8 grid w-full gap-3 sm:grid-cols-3">
        {next.map(([href, label]) => (
          <ButtonLink key={href} href={href} className="h-auto animate-[fade-up_0.4s_ease_both] justify-between px-4 py-3 text-left">{label}<ArrowRight /></ButtonLink>
        ))}
      </div>
      <div className="mt-8 flex items-center justify-center gap-4">
        <ButtonLink href="/" variant="primary">Go to overview</ButtonLink>
        <button type="button" onClick={onSwitch} className="text-[13px] text-muted hover:text-ink">{path === 'ai' ? 'Also set it up by hand' : 'Also connect an AI agent'}</button>
      </div>
    </div>
  );
}

export default function Setup() {
  const router = useRouter();
  const me = useMe();
  const storage = `forke-mail-setup-${me.workspace.id}`;
  const [path, setPath] = useState<Path | null>(null);
  const [step, setStep] = useState(0);
  const [agents, setAgents] = useState<Agents | null>(null);
  const [testKey, setTestKey] = useState('');

  useEffect(() => {
    const q = router.query.path;
    if (q === 'ai' || q === 'manual') setPath(q);
  }, [router.query.path]);
  useEffect(() => {
    if (path === 'ai' && !agents) api<Agents>('GET', '/api/agents').then(setAgents).catch(() => setAgents({ connections: [], prompts: [], mcp_url: `${window.location.origin}/mcp` }));
  }, [path, agents]);
  useEffect(() => {
    if (path && step === flows[path].length - 1) window.localStorage.setItem(storage, 'done');
  }, [path, step, storage]);
  useEffect(() => {
    window.scrollTo({ top: 0, behavior: 'smooth' });
  }, [path, step]);

  const skip = () => {
    if (window.localStorage.getItem(storage) !== 'done') window.localStorage.setItem(storage, 'hidden');
    router.push('/');
  };
  const next = () => setStep((s) => s + 1);
  const back = () => (step > 0 ? setStep((s) => s - 1) : setPath(null));
  const switchPath = () => {
    setPath(path === 'ai' ? 'manual' : 'ai');
    setStep(0);
  };

  if (!path) return <Shell step={0} path={null} onSkip={skip}><Choose onPick={(p) => { setPath(p); setStep(0); }} /></Shell>;
  const last = flows[path].length - 1;
  let body: ReactNode;
  if (step === last) body = <Done path={path} onSwitch={switchPath} />;
  else if (path === 'ai') {
    if (!agents) body = <div className="flex items-center gap-2 text-sm text-muted"><Loader2 className="size-4 animate-spin" />Loading…</div>;
    else body = step === 0 ? <ConnectStep agents={agents} onNext={next} /> : <PromptStep prompts={agents.prompts} onNext={next} />;
  } else {
    body = step === 0 ? <DomainStep onNext={next} /> : step === 1 ? <KeyStep onKey={setTestKey} onNext={next} /> : <SendStep testKey={testKey} onNext={next} />;
  }
  return <Shell step={step} path={path} onSkip={skip} onBack={step === last ? undefined : back}>{body}</Shell>;
}

