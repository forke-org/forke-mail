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
import { Check, Copy } from 'lucide-react';
import { useState } from 'react';
import { CopyButton, Tabs } from './ui';

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
      <Tabs value={current} onChange={(a) => (onApp ? onApp(a) : setOwn(a))} className="self-start" options={agentApps} />
      <p className="text-[13px] text-muted">{s.text}</p>
      <CodeLine value={s.code} />
    </div>
  );
}

export function PromptList({ prompts, highlight }: { prompts: PromptCard[]; highlight?: string }) {
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
    <div className="grid gap-3 md:grid-cols-2">
      {prompts.map((p) => (
        <button key={p.name} type="button" onClick={() => copy(p)}
          className={clsx('group flex flex-col gap-1.5 rounded-[8px] p-4 text-left transition-[background-color,box-shadow,transform] duration-150 active:scale-[0.99]',
            p.name === highlight ? 'bg-accent-soft shadow-[inset_0_0_0_1px_var(--accent-line)]' : 'bg-surface-2 shadow-[inset_0_0_0_1px_var(--line)] hover:bg-surface-3')}>
          <span className="flex items-center justify-between gap-3">
            <span className="text-sm font-medium text-ink">{p.title}</span>
            <span className={clsx('inline-flex items-center gap-1 text-xs', copied === p.name ? 'text-success' : 'text-muted group-hover:text-ink')}>
              {copied === p.name ? <><Check className="size-3.5" />Copied</> : <><Copy className="size-3.5" />Copy</>}
            </span>
          </span>
          <span className="text-xs text-muted">{p.description}</span>
          <span className="mt-1 line-clamp-3 font-mono text-[11.5px] leading-relaxed text-ink-2">{p.text}</span>
        </button>
      ))}
    </div>
  );
}
