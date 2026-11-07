import { useState } from 'react';
import { api, describeError } from '../lib/api';
import { number, plural } from '../lib/format';
import { useToast } from '../lib/toast';
import { Field, Tabs } from './ui';

export default function ImportForm({ emailId, onDone, onCancel }) {
  const toast = useToast();
  const [mode, setMode] = useState('file');
  const [file, setFile] = useState(null);
  const [text, setText] = useState('');
  const [tags, setTags] = useState('');
  const [busy, setBusy] = useState(false);
  const [result, setResult] = useState(null);

  const submit = async (e) => {
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
      const res = await api('POST', '/api/contacts/import', data);
      setResult(res);
      toast(`Imported ${plural(res.added + res.updated, 'contact')}.`, 'success');
      if (onDone) onDone(res);
    } catch (err) {
      toast(describeError(err), 'error');
    } finally {
      setBusy(false);
    }
  };

  return (
    <form className="stack-sm" onSubmit={submit}>
      <Tabs value={mode} onChange={setMode} options={[['file', 'Upload CSV'], ['text', 'Paste']]} />
      {mode === 'file' ? (
        <label className="dropzone">
          <input type="file" accept=".csv,text/csv,text/plain" onChange={(e) => setFile(e.target.files[0] || null)} />
          <strong>{file ? file.name : 'Choose a CSV file'}</strong>
          <span className="small muted">Columns: email, first_name, last_name. Up to 50,000 rows.</span>
        </label>
      ) : (
        <Field label="Contacts" hint="One per line: email, first name, last name.">
          <textarea className="code" rows={7} value={text} onChange={(e) => setText(e.target.value)} placeholder={'ada@example.com,Ada,Lovelace\ngrace@example.com,Grace,Hopper'} />
        </Field>
      )}
      <Field label="Tags" hint="Comma separated. Added to every imported contact.">
        <input type="text" value={tags} onChange={(e) => setTags(e.target.value)} placeholder="newsletter, beta-users" />
      </Field>
      {emailId ? <p className="small muted">Imported contacts are added to your contacts and to this email&apos;s audience.</p> : null}
      {result ? (
        <div className="result-grid">
          {[['Added', result.added], ['Updated', result.updated], ['Skipped', result.skipped], ['Invalid', result.invalid]].map(([label, n]) => (
            <div key={label} className="result"><strong>{number(n)}</strong><span className="small muted">{label}</span></div>
          ))}
        </div>
      ) : null}
      <div className="actions actions-end">
        {onCancel ? <button className="btn" type="button" onClick={onCancel}>{result ? 'Done' : 'Cancel'}</button> : null}
        <button className="btn btn-primary" type="submit" disabled={busy}>{busy ? 'Importing…' : 'Import'}</button>
      </div>
    </form>
  );
}
