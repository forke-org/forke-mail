import { Hash, Pencil, Plus, SlidersHorizontal } from 'lucide-react';
import { FormEvent, useMemo, useState } from 'react';
import DataTable, { Columns } from '../components/DataTable';
import { FormActions, Modal } from '../components/Modal';
import { Button, Card, Chip, Field, FormError, Input, PageHeader } from '../components/ui';
import { api, describeError } from '../lib/api';
import { useToast } from '../lib/toast';
import type { Topic } from '../lib/types';
import { useLoad } from '../lib/useLoad';

function slug(label: string) {
  return label.toLowerCase().trim().replace(/[^a-z0-9]+/g, '-').replace(/^-+|-+$/g, '').slice(0, 40);
}

function TopicForm({ topic, onClose, onSaved }: { topic?: Topic; onClose: () => void; onSaved: (t: Topic) => void }) {
  const [label, setLabel] = useState(topic?.label || '');
  const [key, setKey] = useState(topic?.key || '');
  const [keyEdited, setKeyEdited] = useState(Boolean(topic));
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  const submit = async (e: FormEvent) => {
    e.preventDefault();
    setBusy(true);
    setError('');
    try {
      onSaved(await api<Topic>('PUT', `/api/topics/${encodeURIComponent(key)}`, { label: label.trim() }));
    } catch (err) {
      setError(describeError(err));
      setBusy(false);
    }
  };
  return (
    <Modal title={topic ? 'Rename topic' : 'New topic'} description="Contacts choose which topics they want in the preference centre." onClose={onClose}>
      <form className="flex flex-col gap-4" onSubmit={submit}>
        <Field label="Label" hint="Shown to subscribers, e.g. “Product updates”.">
          <Input required autoFocus maxLength={60} value={label} onChange={(e) => { setLabel(e.target.value); if (!keyEdited) setKey(slug(e.target.value)); }} />
        </Field>
        <Field label="Key" hint={topic ? 'Keys cannot be changed.' : 'Used by the API as the email’s topic. Lowercase, numbers, - and _.'}>
          <Input required disabled={Boolean(topic)} className="font-mono text-[13px]" maxLength={40} value={key} onChange={(e) => { setKeyEdited(true); setKey(e.target.value); }} />
        </Field>
        <FormError>{error}</FormError>
        <FormActions>
          <Button onClick={onClose}>Cancel</Button>
          <Button variant="primary" type="submit" loading={busy}>{topic ? 'Save' : 'Create topic'}</Button>
        </FormActions>
      </form>
    </Modal>
  );
}

export default function Topics() {
  const toast = useToast();
  const [form, setForm] = useState<{ topic?: Topic } | null>(null);
  const [state, reload] = useLoad(() => api<{ topics: Topic[] }>('GET', '/api/topics'), []);

  const columns = useMemo<Columns<Topic>>(() => [
    {
      accessorKey: 'label', header: 'Topic',
      cell: ({ row }) => (
        <div className="flex items-center gap-3">
          <span className="grid size-8 place-items-center rounded-lg bg-accent-soft text-accent"><Hash className="size-4" /></span>
          <span className="font-medium text-ink">{row.original.label}</span>
        </div>
      ),
    },
    { accessorKey: 'key', header: 'Key', cell: ({ getValue }) => <Chip>{getValue()}</Chip> },
    {
      id: 'actions', header: '', meta: { className: 'w-24 text-right' },
      cell: ({ row }) => <Button size="xs" variant="ghost" icon={Pencil} onClick={(e) => { e.stopPropagation(); setForm({ topic: row.original }); }}>Rename</Button>,
    },
  ], []);

  const addButton = <Button variant="primary" icon={Plus} onClick={() => setForm({})}>New topic</Button>;
  return (
    <>
      <PageHeader title="Topics" description="Let subscribers pick what they hear about instead of unsubscribing from everything." actions={addButton} />
      <div className="grid items-start gap-5 lg:grid-cols-[minmax(0,1fr)_320px]">
        <Card flush>
          <DataTable
            columns={columns}
            data={state.data ? state.data.topics : null}
            error={state.error}
            getRowId={(t) => t.key}
            noun="topics"
            empty={{ title: 'No topics yet', hint: 'Create topics like Blog, Changelog and Product to give subscribers a choice.', action: addButton }}
          />
        </Card>
        <Card title="How topics work" icon={SlidersHorizontal}>
          <ol className="flex flex-col gap-3 text-[13px] leading-relaxed text-ink-2">
            <li><span className="font-medium text-ink">1. Tag the broadcast.</span> Pick a topic in the email editor, or send <code className="rounded bg-surface-3 px-1 font-mono text-xs">topic</code> through the API.</li>
            <li><span className="font-medium text-ink">2. Subscribers choose.</span> The unsubscribe link opens a preference centre where they can switch topics off or pause all email.</li>
            <li><span className="font-medium text-ink">3. Opt-outs are skipped.</span> Contacts who turned a topic off never receive broadcasts tagged with it.</li>
          </ol>
        </Card>
      </div>
      {form ? (
        <TopicForm
          topic={form.topic}
          onClose={() => setForm(null)}
          onSaved={(t) => { setForm(null); toast(`${t.label} saved.`, 'success'); reload(); }}
        />
      ) : null}
    </>
  );
}
