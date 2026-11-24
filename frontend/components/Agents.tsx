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
import type React from 'react';
import { BarChart3, Check, Copy, Globe, KeyRound, LifeBuoy, Mail, Newspaper, Repeat, Rocket, Truck, type LucideIcon } from 'lucide-react';
import { useState } from 'react';
import { CopyButton, Tabs } from './ui';
import { AppLogo } from './AppLogos';

export type AgentApp = 'claude' | 'chatgpt' | 'code' | 'cursor' | 'cli';
export type PromptCard = { name: string; title: string; description: string; text: string };

export const agentApps: [AgentApp, string][] = [['claude', 'Claude'], ['chatgpt', 'ChatGPT'], ['code', 'Claude Code'], ['cursor', 'Cursor & others'], ['cli', 'CLI']];

export function CodeLine({ value }: { value: string }) {
  return (
    <div className="flex items-start gap-2">
      <pre className="min-w-0 flex-1 overflow-x-auto rounded-[6px] bg-surface-2 px-3 py-2.5 font-mono text-[12.5px] leading-relaxed text-ink shadow-[inset_0_0_0_1px_var(--line)]">{value}</pre>
      <CopyButton value={value} boxed />
    </div>
  );
}

export function connectSteps(url: string): Record<AgentApp, { text: string; code: string }> {
  return {
    claude: { text: 'In Claude (web or desktop) open Settings → Connectors → Add custom connector, paste this URL, then sign in and pick a project.', code: url },
    chatgpt: { text: 'In ChatGPT open Settings → Connectors → Create, choose OAuth, paste this URL, then sign in and pick a project.', code: url },
    code: { text: 'Run this in your terminal, then type /mcp in Claude Code to sign in.', code: `claude mcp add --transport http forke-mail ${url}` },
    cursor: { text: 'Cursor, Windsurf, VS Code, Antigravity and most other apps: add this to the MCP settings. The app opens a browser window to sign in.', code: JSON.stringify({ mcpServers: { 'forke-mail': { url } } }, null, 2) },
    cli: { text: 'Install the CLI, sign in once, then point any app that runs local servers at it.', code: `curl -fsSL https://mail.forke.space/install.sh | sh\nforke-mail login\n\n{"mcpServers": {"forke-mail": {"command": "forke-mail", "args": ["mcp"]}}}` },
  };
}

export function ConnectApps({ url, app, onApp }: { url: string; app?: AgentApp; onApp?: (a: AgentApp) => void }) {
  const [own, setOwn] = useState<AgentApp>('claude');
  const current = app ?? own;
  const s = connectSteps(url)[current];
  return (
    <div className="flex flex-col gap-3">
      <Tabs value={current} onChange={(a) => (onApp ? onApp(a) : setOwn(a))} className="max-w-full self-start overflow-x-auto" options={agentApps.map(([v, label]) => [v, <span key={v} className="inline-flex items-center gap-1.5 whitespace-nowrap"><AppLogo app={v} />{label}</span>] as [AgentApp, React.ReactNode])} />
      <p className="text-[13px] text-muted">{s.text}</p>
      <CodeLine value={s.code} />
    </div>
  );
}

const promptIcons: Record<string, [LucideIcon, string]> = {
  'setup-app': [Rocket, 'Start here'],
  'welcome-email': [Mail, 'Template'],
  'auth-emails': [KeyRound, 'Templates'],
  'welcome-series': [Repeat, 'Workflow'],
  newsletter: [Newspaper, 'Broadcast'],
  'verify-domain': [Globe, 'Domain'],
  'debug-delivery': [LifeBuoy, 'Debug'],
  migrate: [Truck, 'Migrate'],
  'weekly-report': [BarChart3, 'Report'],
};

export function PromptList({ prompts, highlight, wide }: { prompts: PromptCard[]; highlight?: string; wide?: boolean }) {
  const [copied, setCopied] = useState('');
  const copy = async (p: PromptCard) => {
    try {
      await navigator.clipboard.writeText(p.text);
      setCopied(p.name);
      setTimeout(() => setCopied(''), 1500);
    } catch {
      setCopied('');
    }
  };
  return (
    <div className={clsx('grid gap-3 md:grid-cols-2', wide && 'xl:grid-cols-3')}>
      {prompts.map((p, i) => {
        const [Icon, tag] = promptIcons[p.name] || [Mail, 'Prompt'];
        const done = copied === p.name;
        const lead = p.name === highlight;
        return (
          <button key={p.name} type="button" onClick={() => copy(p)} style={{ animationDelay: `${i * 40}ms` }}
            className={clsx('group relative flex animate-[fade-up_0.4s_ease_both] flex-col overflow-hidden rounded-[10px] p-4 text-left transition-[background-color,box-shadow,transform] duration-200 hover:-translate-y-0.5 active:translate-y-0 active:scale-[0.99]',
              lead ? 'bg-accent-soft shadow-[inset_0_0_0_1px_var(--accent-line)]' : 'bg-surface-2 shadow-[inset_0_0_0_1px_var(--line)] hover:bg-surface-3 hover:shadow-[inset_0_0_0_1px_var(--line-strong)]')}>
            <span className="flex items-start justify-between gap-3">
              <span className={clsx('grid size-9 shrink-0 place-items-center rounded-[8px] transition-colors', lead ? 'bg-accent text-white' : 'bg-surface-3 text-ink-2 group-hover:bg-accent-soft group-hover:text-accent')}>
                <Icon className="size-[18px]" />
              </span>
              <span className={clsx('rounded-full px-2 py-0.5 font-mono text-[10px] tracking-[0.06em] uppercase', lead ? 'bg-accent/15 text-accent' : 'bg-surface-3 text-faint')}>{lead ? 'Start here' : tag}</span>
            </span>
            <span className="mt-3.5 text-[14px] font-medium text-ink">{p.title}</span>
            <span className="mt-1 text-[12.5px] leading-relaxed text-muted">{p.description}</span>
            <span className="mt-3.5 block rounded-[6px] bg-bg/60 px-3 py-2.5 shadow-[inset_0_0_0_1px_var(--line)]">
              <span className="block max-h-[54px] overflow-hidden font-mono text-[11px] leading-relaxed text-ink-2 [mask-image:linear-gradient(to_bottom,black_45%,transparent)]">{p.text}</span>
            </span>
            <span className="mt-3.5 flex items-center justify-between gap-2 text-xs">
              <code className="truncate font-mono text-faint">/{p.name}</code>
              <span className={clsx('inline-flex items-center gap-1 font-medium transition-colors', done ? 'text-success' : 'text-muted group-hover:text-ink')}>
                {done ? <><Check className="size-3.5 animate-[pop_0.25s_ease_both]" />Copied</> : <><Copy className="size-3.5" />Copy prompt</>}
              </span>
            </span>
          </button>
        );
      })}
    </div>
  );
}
