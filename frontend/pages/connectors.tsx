/**
 * @fileoverview Forke Platform
 * @copyright (c) 2026 Forke Inc. (https://www.forke.space/)
 *
 * Source-Available License (Non-Commercial / Fair Source).
 * This source code is open for inspection, learning, and personal development.
 * Commercial use, hosting, or resale as a paid service without an explicit
 * commercial license from Forke Inc. is strictly prohibited.
 */

import { Bot, Link2, ShieldCheck, Sparkles, Unplug } from 'lucide-react';
import { CodeLine, ConnectApps, PromptList, type PromptCard } from '../components/Agents';
import { useConfirm } from '../components/Modal';
import { Badge, Button, Card, Empty, Loading, PageHeader } from '../components/ui';
import { api, describeError } from '../lib/api';
import { timeAgo } from '../lib/format';
import { useToast } from '../lib/toast';
import { useLoad } from '../lib/useLoad';

type Connection = { id: string; app: string; project: string; created_at: string; last_used_at: string | null };
type Agents = { connections: Connection[]; prompts: PromptCard[]; mcp_url: string };

function Safety() {
  const rows: [string, string, string][] = [
    ['Does it on its own', 'Reading your project, drafts, templates, contacts, segments and sends with test keys.', 'green'],
    ['Asks you in the chat first', 'Sending real email, creating API keys, importing contacts, changing settings and deleting things.', 'amber'],
    ['Asks first, says it can\'t be undone', 'Launching broadcasts, deleting domains or projects, revoking keys, changing roles and anything about credits.', 'red'],
    ['Never', 'Your password, signing in or out, and Forke admin settings.', 'neutral'],
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
  const { connections, prompts, mcp_url: url } = state.data;

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
          <CodeLine value={url} />
        </Card>
        <Card title="Connect an AI app" icon={Bot} description="Give Claude, ChatGPT, Cursor or any MCP app access to this project. It can then add domains, write templates, send test emails and build email into your app for you."><ConnectApps url={url} /></Card>
        <Card title="Prompts to try" icon={Sparkles} description="Paste one into your AI app, or pick it from the app's prompt menu. Agents ask you before anything risky."><PromptList prompts={prompts} /></Card>
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
      </div>
    </>
  );
}
