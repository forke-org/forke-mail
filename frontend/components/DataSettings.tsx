/**
 * @fileoverview Forke Platform
 * @copyright (c) 2026 Forke Inc. (https://www.forke.space/)
 *
 * Source-Available License (Non-Commercial / Fair Source).
 * This source code is open for inspection, learning, and personal development.
 * Commercial use, hosting, or resale as a paid service without an explicit
 * commercial license from Forke Inc. is strictly prohibited.
 */

import { Database, Trash2, Zap } from 'lucide-react';
import { api, describeError, ApiError } from '../lib/api';
import { formatDate, number } from '../lib/format';
import { useSession } from '../lib/session';
import { useToast } from '../lib/toast';
import type { DataUsage } from '../lib/types';
import { useLoad } from '../lib/useLoad';
import { Button, Card, Loading } from './ui';

function List({ items, kind, empty, onDelete, canDelete }: { items: DataUsage[]; kind: 'field' | 'event'; empty: string; onDelete: (u: DataUsage) => void; canDelete: boolean }) {
  if (!items.length) return <p className="text-[13px] text-muted">{empty}</p>;
  return (
    <div className="divide-y divide-line">
      {items.map((u) => (
        <div key={u.name} className="flex items-center justify-between gap-3 py-3 first:pt-0 last:pb-0">
          <div className="min-w-0">
            <div className="truncate font-mono text-[13px] text-ink">{u.name}</div>
            <div className="text-xs text-muted">
              {kind === 'field' ? `${number(u.count)} contacts` : `${number(u.count)} times · last ${u.last_seen ? formatDate(u.last_seen) : '-'}`}
              {u.used_by.length ? ` · used by ${u.used_by.join(', ')}` : ''}
            </div>
          </div>
          {canDelete ? <Button size="sm" icon={Trash2} disabled={u.used_by.length > 0} onClick={() => onDelete(u)}>Delete</Button> : null}
        </div>
      ))}
    </div>
  );
}

export default function DataSettings() {
  const { canManage } = useSession();
  const toast = useToast();
  const [state, reload] = useLoad(() => api<{ fields: DataUsage[]; events: DataUsage[] }>('GET', '/api/data'), []);
  if (!state.data) return <Loading />;
  const remove = (kind: 'fields' | 'events') => async (u: DataUsage) => {
    try {
      await api('DELETE', `/api/data/${kind}/${encodeURIComponent(u.name)}`);
      toast(`${u.name} deleted.`, 'success');
      reload();
    } catch (err) {
      toast(err instanceof ApiError && err.message === 'in_use' ? `Still used by ${(err.body.used_by || []).join(', ')}.` : describeError(err), 'error');
    }
  };
  return (
    <div className="flex flex-col gap-5">
      <Card title="Custom contact fields" icon={Database} description="Set through POST /api/events or contact imports. A field used by a segment can't be deleted.">
        <List items={state.data.fields} kind="field" canDelete={canManage} empty="No custom fields yet. Send contact data with an event to create them." onDelete={remove('fields')} />
      </Card>
      <Card title="Custom events" icon={Zap} description="Things your contacts did, sent from your app. An event used by a segment or workflow can't be deleted.">
        <List items={state.data.events} kind="event" canDelete={canManage} empty="No events yet. Send one with POST /api/events." onDelete={remove('events')} />
      </Card>
    </div>
  );
}
