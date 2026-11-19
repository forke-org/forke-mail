import { KeyRound, Trash2, UserRound } from 'lucide-react';
import { FormEvent, useState } from 'react';
import { api, describeError } from '../lib/api';
import { useSession } from '../lib/session';
import { useToast } from '../lib/toast';
import { useLoad } from '../lib/useLoad';
import { FormActions, Modal } from './Modal';
import { Button, Callout, Card, Field, FormError, Input, Loading } from './ui';

interface Account { name: string; email: string; username: string | null; forke: boolean; projects: number }

function DeleteAccount({ projects, onClose }: { projects: number; onClose: () => void }) {
  const [password, setPassword] = useState('');
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  const submit = async (e: FormEvent) => {
    e.preventDefault();
    setBusy(true);
    setError('');
    try {
      await api('POST', '/api/account/delete', { password });
      window.location.href = '/login/';
    } catch (err) {
      setError(describeError(err));
      setBusy(false);
    }
  };
  return (
    <Modal title="Delete your account" onClose={onClose}>
      <form className="flex flex-col gap-4" onSubmit={submit}>
        <p className="text-[13px] leading-relaxed text-muted">
          You&apos;re in {projects} {projects === 1 ? 'project' : 'projects'}. Projects where you are the only member are deleted with all their data and domains.
          Shared projects stay with the rest of the team. If you&apos;re the only owner of a shared project, make someone else an owner first.
        </p>
        <Field label="Your password"><Input type="password" autoComplete="current-password" required autoFocus value={password} onChange={(e) => setPassword(e.target.value)} /></Field>
        <FormError>{error}</FormError>
        <FormActions>
          <Button onClick={onClose}>Cancel</Button>
          <Button variant="danger" type="submit" loading={busy}>Delete my account</Button>
        </FormActions>
      </form>
    </Modal>
  );
}

export default function AccountSettings() {
  const toast = useToast();
  const { refreshMe } = useSession();
  const [state, reload] = useLoad(() => api<Account>('GET', '/api/account'), []);
  const [profile, setProfile] = useState<{ name: string; username: string } | null>(null);
  const [pw, setPw] = useState({ current: '', next: '' });
  const [errors, setErrors] = useState({ profile: '', password: '' });
  const [busy, setBusy] = useState('');
  const [deleting, setDeleting] = useState(false);
  if (!state.data) return state.error ? <FormError>{state.error}</FormError> : <Loading />;
  const a = state.data;
  const form = profile || { name: a.name, username: a.username || '' };

  if (a.forke) {
    return <Callout tone="accent" title="Forke admin account">Your name, password and access come from your Forke admin account. Change them in the Forke admin panel.</Callout>;
  }

  const saveProfile = async (e: FormEvent) => {
    e.preventDefault();
    setBusy('profile');
    setErrors((x) => ({ ...x, profile: '' }));
    try { await api('PATCH', '/api/account', form); await refreshMe(); reload(); setProfile(null); toast('Profile saved.', 'success'); } catch (err) { setErrors((x) => ({ ...x, profile: describeError(err) })); } finally { setBusy(''); }
  };
  const savePassword = async (e: FormEvent) => {
    e.preventDefault();
    setBusy('password');
    setErrors((x) => ({ ...x, password: '' }));
    try { await api('POST', '/api/account/password', { current: pw.current, new: pw.next }); setPw({ current: '', next: '' }); toast('Password changed. Other devices were signed out.', 'success'); } catch (err) { setErrors((x) => ({ ...x, password: describeError(err) })); } finally { setBusy(''); }
  };

  return (
    <div className="flex flex-col gap-5">
      <Card title="Profile" icon={UserRound} description={a.email}>
        <form className="flex flex-col gap-4" onSubmit={saveProfile}>
          <div className="grid gap-4 sm:grid-cols-2">
            <Field label="Name"><Input required maxLength={100} value={form.name} onChange={(e) => setProfile({ ...form, name: e.target.value })} /></Field>
            <Field label="Username" hint="You can log in with it."><Input autoCapitalize="none" spellCheck={false} value={form.username} onChange={(e) => setProfile({ ...form, username: e.target.value })} /></Field>
          </div>
          <FormError>{errors.profile}</FormError>
          <div className="flex justify-end"><Button type="submit" loading={busy === 'profile'} disabled={!profile}>Save profile</Button></div>
        </form>
      </Card>
      <Card title="Password" icon={KeyRound} description="Changing it signs you out everywhere else.">
        <form className="flex flex-col gap-4" onSubmit={savePassword}>
          <div className="grid gap-4 sm:grid-cols-2">
            <Field label="Current password"><Input type="password" autoComplete="current-password" required value={pw.current} onChange={(e) => setPw({ ...pw, current: e.target.value })} /></Field>
            <Field label="New password" hint="At least 10 characters."><Input type="password" autoComplete="new-password" required minLength={10} value={pw.next} onChange={(e) => setPw({ ...pw, next: e.target.value })} /></Field>
          </div>
          <FormError>{errors.password}</FormError>
          <div className="flex justify-end"><Button type="submit" loading={busy === 'password'}>Change password</Button></div>
        </form>
      </Card>
      <Card title="Delete account" icon={Trash2} description="Permanently delete your login and every project you're the only member of." className="shadow-[0_0_0_1px_color-mix(in_srgb,var(--danger)_35%,transparent)]">
        <div className="flex justify-end"><Button variant="danger" onClick={() => setDeleting(true)}>Delete my account</Button></div>
      </Card>
      {deleting ? <DeleteAccount projects={a.projects} onClose={() => setDeleting(false)} /> : null}
    </div>
  );
}
