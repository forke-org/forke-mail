import { FormEvent, useState } from 'react';
import { api, describeError } from '../lib/api';
import type { Campaign } from '../lib/types';
import { FormActions, Modal } from './Modal';
import { Button, Field, FormError, Input, Textarea } from './ui';

export default function CampaignForm({ campaign, onClose, onSaved }: { campaign?: Campaign; onClose: () => void; onSaved: (c: Campaign) => void }) {
  const [name, setName] = useState(campaign?.name || '');
  const [description, setDescription] = useState(campaign?.description || '');
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);

  const submit = async (e: FormEvent) => {
    e.preventDefault();
    setBusy(true);
    setError('');
    try {
      const body = { name: name.trim(), description: description.trim() };
      const saved = campaign ? await api<Campaign>('PATCH', `/api/campaigns/${campaign.id}`, body) : await api<Campaign>('POST', '/api/campaigns', body);
      onSaved(saved);
    } catch (err) {
      setError(describeError(err));
      setBusy(false);
    }
  };

  return (
    <Modal title={campaign ? 'Edit campaign' : 'New campaign'} description="A campaign groups related broadcasts, like the blog newsletter or the changelog." onClose={onClose}>
      <form className="flex flex-col gap-4" onSubmit={submit}>
        <Field label="Name"><Input required autoFocus maxLength={200} placeholder="Forke changelog" value={name} onChange={(e) => setName(e.target.value)} /></Field>
        <Field label="Description" hint="Optional."><Textarea rows={3} maxLength={500} value={description} onChange={(e) => setDescription(e.target.value)} placeholder="Monthly product updates for everyone on the list." /></Field>
        <FormError>{error}</FormError>
        <FormActions>
          <Button onClick={onClose}>Cancel</Button>
          <Button variant="primary" type="submit" loading={busy}>{campaign ? 'Save' : 'Create campaign'}</Button>
        </FormActions>
      </form>
    </Modal>
  );
}
