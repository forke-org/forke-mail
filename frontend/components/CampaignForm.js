import { useState } from 'react';
import { api, describeError } from '../lib/api';
import { Modal } from './Modal';
import { Field } from './ui';

export default function CampaignForm({ campaign, onClose, onSaved }) {
  const [name, setName] = useState(campaign?.name || '');
  const [description, setDescription] = useState(campaign?.description || '');
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);

  const submit = async (e) => {
    e.preventDefault();
    setBusy(true);
    setError('');
    try {
      const body = { name: name.trim(), description: description.trim() };
      const saved = campaign ? await api('PATCH', `/api/campaigns/${campaign.id}`, body) : await api('POST', '/api/campaigns', body);
      onSaved(saved);
    } catch (err) {
      setError(describeError(err));
      setBusy(false);
    }
  };

  return (
    <Modal title={campaign ? 'Edit campaign' : 'New campaign'} description="A campaign groups related emails, like announcements or onboarding." onClose={onClose}>
      <form className="stack-sm" onSubmit={submit}>
        <Field label="Name"><input type="text" required autoFocus maxLength={200} placeholder="Forke product updates" value={name} onChange={(e) => setName(e.target.value)} /></Field>
        <Field label="Description" hint="Optional."><textarea rows={3} maxLength={500} value={description} onChange={(e) => setDescription(e.target.value)} placeholder="Monthly feature announcements for all customers." /></Field>
        {error ? <p className="form-error">{error}</p> : null}
        <div className="actions actions-end">
          <button className="btn" type="button" onClick={onClose}>Cancel</button>
          <button className="btn btn-primary" type="submit" disabled={busy}>{campaign ? 'Save' : 'Create campaign'}</button>
        </div>
      </form>
    </Modal>
  );
}
