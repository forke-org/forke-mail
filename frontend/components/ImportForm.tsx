import { FormEvent, useState } from 'react';
import { api, describeError } from '../lib/api';
import { plural } from '../lib/format';
import { useToast } from '../lib/toast';
import FileDrop from './FileDrop';
import { FormActions } from './Modal';
import ResultGrid from './ResultGrid';
import { Button, Callout, Checkbox, Field, Input, Tabs, Textarea } from './ui';

interface ImportResult { added: number; updated: number; skipped: number; invalid: number; risky_skipped: number }
interface Check { rows: number; invalid: number; duplicates: number; role: number; disposable: number; typos: number; samples: { email: string; reason: string; suggestion?: string }[] }

const reasons: Record<string, string> = { role: 'shared inbox', disposable: 'throwaway', typo: 'likely typo' };

export default function ImportForm({ emailId, onDone, onCancel }: { emailId?: string; onDone?: (r: ImportResult) => void; onCancel?: () => void }) {
  const toast = useToast();
  const [mode, setMode] = useState<'file' | 'text'>('file');
  const [file, setFile] = useState<File | null>(null);
  const [text, setText] = useState('');
  const [tags, setTags] = useState('');
  const [busy, setBusy] = useState(false);
  const [result, setResult] = useState<ImportResult | null>(null);
  const [check, setCheck] = useState<Check | null>(null);
  const [keepRisky, setKeepRisky] = useState(false);
  const flagged = check ? check.role + check.disposable + check.typos : 0;
  const reset = () => { setCheck(null); setResult(null); };

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
    if (keepRisky) data.append('keep_risky', 'true');
    setBusy(true);
    try {
      if (!check) {
        const c = await api<Check>('POST', '/api/contacts/import/check', data);
        if (c.role + c.disposable + c.typos > 0) {
          setCheck(c);
          return;
        }
      }
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
        <FileDrop file={file} onFile={(f) => { setFile(f); reset(); }} hint="Columns: email, first_name, last_name. Up to 50,000 rows." />
      ) : (
        <Field label="Contacts" hint="One per line: email, first name, last name.">
          <Textarea code rows={7} value={text} onChange={(e) => { setText(e.target.value); reset(); }} placeholder={'ada@example.com,Ada,Lovelace\ngrace@example.com,Grace,Hopper'} />
        </Field>
      )}
      <Field label="Tags" hint="Comma separated. Added to every imported contact.">
        <Input value={tags} onChange={(e) => setTags(e.target.value)} placeholder="newsletter, beta-users" />
      </Field>
      {emailId ? <p className="text-xs text-muted">Imported contacts are added to your contacts and to this email&apos;s audience.</p> : null}
      {check && !result ? (
        <Callout tone="warn" title={`${plural(flagged, 'address', 'addresses')} may hurt your sending`}>
          <p>{check.typos ? `${plural(check.typos, 'likely typo')} (always skipped, they would bounce). ` : ''}{check.role ? `${plural(check.role, 'shared inbox')} like info@ or support@. ` : ''}{check.disposable ? `${plural(check.disposable, 'throwaway address', 'throwaway addresses')}. ` : ''}Shared inboxes and throwaway addresses rarely engage and are more likely to report spam.</p>
          <ul className="mt-2 max-h-36 overflow-y-auto font-mono text-xs">
            {check.samples.map((f) => <li key={f.email}>{f.email} · {reasons[f.reason]}{f.suggestion ? ` → did you mean ${f.suggestion}?` : ''}</li>)}
          </ul>
          {check.role + check.disposable > 0 ? <div className="mt-3"><Checkbox label="Keep shared inboxes and throwaway addresses anyway" checked={keepRisky} onChange={(e) => setKeepRisky(e.target.checked)} /></div> : null}
        </Callout>
      ) : null}
      {result ? <ResultGrid items={[['Added', result.added], ['Updated', result.updated], ['Skipped', result.skipped + result.risky_skipped], ['Invalid', result.invalid]]} /> : null}
      <FormActions>
        {onCancel ? <Button onClick={onCancel}>{result ? 'Done' : 'Cancel'}</Button> : null}
        <Button variant="primary" type="submit" loading={busy}>{check && !result ? `Import ${plural(check.rows - (keepRisky ? check.typos : flagged), 'contact')}` : 'Import'}</Button>
      </FormActions>
    </form>
  );
}
