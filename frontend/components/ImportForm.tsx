import { FormEvent, useState } from 'react';
import { api, describeError } from '../lib/api';
import { plural } from '../lib/format';
import { useToast } from '../lib/toast';
import FileDrop from './FileDrop';
import { FormActions } from './Modal';
import ResultGrid from './ResultGrid';
import { Button, Field, Input, Tabs, Textarea } from './ui';

interface ImportResult { added: number; updated: number; skipped: number; invalid: number }

export default function ImportForm({ emailId, onDone, onCancel }: { emailId?: string; onDone?: (r: ImportResult) => void; onCancel?: () => void }) {
  const toast = useToast();
  const [mode, setMode] = useState<'file' | 'text'>('file');
  const [file, setFile] = useState<File | null>(null);
  const [text, setText] = useState('');
  const [tags, setTags] = useState('');
  const [busy, setBusy] = useState(false);
  const [result, setResult] = useState<ImportResult | null>(null);

  const submit = async (e: FormEvent) => {
    e.preventDefault();
    const data = new FormData();
    if (mode === 'file') {
      if (!file) { toast('Choose a CSV file first.', 'warn'); return; }
      data.append('file', file);
    } else {
      if (!text.trim()) { toast('Paste at least one contact.', 'warn'); return; }
      data.append('text', text);
    }
    if (tags.trim()) data.append('tags', tags);
    if (emailId) data.append('email_id', emailId);
    setBusy(true);
    try {
      const res = await api<ImportResult>('POST', '/api/contacts/import', data);
      setResult(res);
      toast(`Imported ${plural(res.added + res.updated, 'contact')}.`, 'success');
      onDone?.(res);
    } catch (err) {
      toast(describeError(err), 'error');
    } finally {
      setBusy(false);
    }
  };

  return (
    <form className="flex flex-col gap-4" onSubmit={submit}>
      <Tabs value={mode} onChange={setMode} options={[['file', 'Upload CSV'], ['text', 'Paste']]} />
      {mode === 'file' ? (
        <FileDrop file={file} onFile={setFile} hint="Columns: email, first_name, last_name. Up to 50,000 rows." />
      ) : (
        <Field label="Contacts" hint="One per line: email, first name, last name.">
          <Textarea code rows={7} value={text} onChange={(e) => setText(e.target.value)} placeholder={'ada@example.com,Ada,Lovelace\ngrace@example.com,Grace,Hopper'} />
        </Field>
      )}
      <Field label="Tags" hint="Comma separated. Added to every imported contact.">
        <Input value={tags} onChange={(e) => setTags(e.target.value)} placeholder="newsletter, beta-users" />
      </Field>
      {emailId ? <p className="text-xs text-muted">Imported contacts are added to your contacts and to this email&apos;s audience.</p> : null}
      {result ? <ResultGrid items={[['Added', result.added], ['Updated', result.updated], ['Skipped', result.skipped], ['Invalid', result.invalid]]} /> : null}
      <FormActions>
        {onCancel ? <Button onClick={onCancel}>{result ? 'Done' : 'Cancel'}</Button> : null}
        <Button variant="primary" type="submit" loading={busy}>Import</Button>
      </FormActions>
    </form>
  );
}
