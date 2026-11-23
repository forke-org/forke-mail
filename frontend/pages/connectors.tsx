/**
 * @fileoverview Forke Platform
 * @copyright (c) 2026 Forke Inc. (https://www.forke.space/)
 *
 * Source-Available License (Non-Commercial / Fair Source).
 * This source code is open for inspection, learning, and personal development.
 * Commercial use, hosting, or resale as a paid service without an explicit
 * commercial license from Forke Inc. is strictly prohibited.
 */

import { Bot, Link2, ShieldCheck, Terminal, Unplug } from 'lucide-react';
import { useState } from 'react';
import { useConfirm } from '../components/Modal';
import { Badge, Button, Card, CopyButton, Empty, Loading, PageHeader, Tabs } from '../components/ui';
import { api, describeError } from '../lib/api';
import { timeAgo } from '../lib/format';
import { useToast } from '../lib/toast';
import { useLoad } from '../lib/useLoad';

type Connection = { id: string; app: string; project: string; created_at: string; last_used_at: string | null };
type Approval = { id: string; summary: string; tool: string; requested_by: string; status: string; created_at: string };
type Agents = { connections: Connection[]; approvals: Approval[]; mcp_url: string };
type Client = 'claude' | 'chatgpt' | 'code' | 'cursor' | 'cli';

function Snippet({ value }: { value: string }) {
  return (
    <div className="flex items-start gap-2">
      <pre className="min-w-0 flex-1 overflow-x-auto rounded-[6px] bg-surface-2 px-3 py-2.5 font-mono text-[12.5px] leading-relaxed text-ink shadow-[inset_0_0_0_1px_var(--line)]">{value}</pre>
      <CopyButton value={value} boxed />
    </div>
  );
}

function Setup({ url }: { url: string }) {
  const [client, setClient] = useState<Client>('claude');
  const steps: Record<Client, { text: string; code?: string }> = {
    claude: { text: 'In Claude (web or desktop) open Settings → Connectors → Add custom connector, paste this URL, then sign in and pick a project.', code: url },
    chatgpt: { text: 'In ChatGPT open Settings → Connectors → Create, choose OAuth, paste this URL, then sign in and pick a project.', code: url },
    code: { text: 'Run this in your terminal, then type /mcp in Claude Code to sign in.', code: `claude mcp add --transport http forke-mail ${url}` },
    cursor: { text: 'Cursor, Windsurf, VS Code, Antigravity and most other apps: add this to the MCP settings. The app opens a browser window to sign in.', code: JSON.stringify({ mcpServers: { 'forke-mail': { url } } }, null, 2) },
    cli: { text: 'Prefer the terminal or an app that only runs local servers? Install the CLI, sign in once, then point the app at the command.', code: `forke-mail login\n\n{"mcpServers": {"forke-mail": {"command": "forke-mail", "args": ["mcp"]}}}` },
  };
  const s = steps[client];
  return (
    <Card title="Connect an AI app" icon={Bot} description="Give Claude, ChatGPT, Cursor or any MCP app access to this project. It can then add domains, write templates, send test emails and build email into your app for you.">
      <div className="flex flex-col gap-3">
        <Tabs value={client} onChange={setClient} className="self-start" options={[['claude', 'Claude'], ['chatgpt', 'ChatGPT'], ['code', 'Claude Code'], ['cursor', 'Cursor & others'], ['cli', 'CLI']]} />
        <p className="text-[13px] text-muted">{s.text}</p>
        {s.code ? <Snippet value={s.code} /> : null}
      </div>
    </Card>
  );
}

function Safety() {
  const rows: [string, string, string][] = [
    ['Does it on its own', 'Reading your project, drafts, templates, contacts, segments and sends with test keys.', 'green'],
    ['Asks you in the chat first', 'Sending real email, creating API keys, importing contacts, changing settings and deleting things.', 'amber'],
    ['Needs your approval here', 'Launching broadcasts, deleting domains or projects, revoking keys, changing roles and anything about credits or payments.', 'red'],
    ['Never', 'Your password, signing in or out, and account deletion without your approval.', 'neutral'],
  ];
  return (
    <Card title="What agents can do" icon={ShieldCheck} description="Every request goes through the same permissions as your account. Risky steps stop and wait for you.">
      <div className="flex flex-col divide-y divide-line">
        {rows.map(([title, text, tone]) => (
          <div key={title} className="flex flex-col gap-1 py-3 first:pt-0 last:pb-0 sm:flex-row sm:items-start sm:gap-4">
            <span className="w-52 shrink-0"><Badge tone={tone as 'green'}>{title}</Badge></span>
            <p className="text-[13px] text-muted">{text}</p>
          </div>
        ))}
      </div>
    </Card>
  );
}

export default function ConnectorsPage() {
  const [state, reload] = useLoad(() => api<Agents>('GET', '/api/agents'), []);
  const confirm = useConfirm();
  const toast = useToast();
  if (!state.data) return <Loading />;
  const { connections, approvals, mcp_url: url } = state.data;

  const disconnect = async (c: Connection) => {
    if (!(await confirm('Disconnect app', `${c.app} will lose access to ${c.project} straight away.`, 'Disconnect', true))) return;
    try {
      await api('DELETE', `/api/agents/connections/${c.id}`);
      reload();
    } catch (err) {
      toast(describeError(err), 'error');
    }
  };

  return (
    <>
      <PageHeader title="Connectors" description="Connect Claude, ChatGPT, Cursor and other AI apps to Forke Mail over MCP, with you in control of anything risky." actions={<a className="text-sm font-medium text-accent hover:underline" href="https://www.forke.space/docs/forke-mail-connect" target="_blank" rel="noreferrer">Read the docs</a>} />
      <div className="flex flex-col gap-5">
        <Card title="MCP server URL" icon={Link2} description="One address for every app. Sign-in happens in your browser; no keys to copy.">
          <Snippet value={url} />
        </Card>
        <Setup url={url} />
        <Safety />
        <Card title="Connected apps" flush icon={Unplug}>
          {connections.length ? (
            <div className="divide-y divide-line">
              {connections.map((c) => (
                <div key={c.id} className="flex items-center gap-3 px-4 py-3">
                  <div className="min-w-0 flex-1">
                    <p className="truncate text-sm font-medium text-ink">{c.app}</p>
                    <p className="text-xs text-muted">{c.project} · connected {timeAgo(c.created_at)} · last used {c.last_used_at ? timeAgo(c.last_used_at) : 'never'}</p>
                  </div>
                  <Button size="sm" variant="danger" onClick={() => disconnect(c)}>Disconnect</Button>
                </div>
              ))}
            </div>
          ) : <Empty title="No apps connected" hint="Add the MCP URL to an AI app to connect it." icon={Bot} />}
        </Card>
        <Card title="Recent approval requests" flush>
          {approvals.length ? (
            <div className="divide-y divide-line">
              {approvals.map((a) => (
                <div key={a.id} className="flex items-center gap-3 px-4 py-3">
                  <div className="min-w-0 flex-1">
                    <p className="text-sm text-ink">{a.summary}</p>
                    <p className="text-xs text-muted">{a.requested_by} · {a.tool} · {timeAgo(a.created_at)}</p>
                  </div>
                  {a.status === 'pending'
                    ? <a className="text-sm font-medium text-accent hover:underline" href={`/approve/${a.id}`}>Review</a>
                    : <Badge value={a.status === 'used' ? 'done' : a.status === 'approved' ? 'active' : a.status === 'denied' ? 'rejected' : 'skipped'}>{a.status}</Badge>}
                </div>
              ))}
            </div>
          ) : <Empty title="Nothing to approve yet" hint="When an agent asks to do something risky, it shows up here." icon={ShieldCheck} />}
        </Card>
      </div>
    </>
  );
}
