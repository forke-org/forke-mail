import { ArrowRight } from 'lucide-react';
import { useRouter } from 'next/router';
import { FormEvent, useEffect, useState } from 'react';
import AuthFrame from '../components/AuthFrame';
import { Button, ButtonLink, Field, FormError, Input } from '../components/ui';
import { api, describeError } from '../lib/api';
import { useSession } from '../lib/session';

export default function Welcome() {
  const router = useRouter();
  const { refreshMe } = useSession();
  const token = typeof router.query.token === 'string' ? router.query.token : '';
  const [email, setEmail] = useState<string | null>(null);
  const [invalid, setInvalid] = useState(false);
  const [form, setForm] = useState({ name: '', username: '', workspace: '', password: '' });
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  const set = (key: keyof typeof form) => (e: React.ChangeEvent<HTMLInputElement>) => setForm((f) => ({ ...f, [key]: e.target.value }));

  useEffect(() => {
    if (!router.isReady) return;
    if (!token) { setInvalid(true); return; }
    api<{ email: string }>('GET', `/api/access/invite?token=${encodeURIComponent(token)}`).then((d) => setEmail(d.email), () => setInvalid(true));
  }, [router.isReady, token]);

  const submit = async (e: FormEvent) => {
    e.preventDefault();
    setBusy(true);
    setError('');
    try {
      await api('POST', '/api/access/invite', { token, ...form });
      await refreshMe();
      router.replace('/?welcome=1');
    } catch (err) {
      setError(describeError(err));
      setBusy(false);
    }
  };

  if (invalid) {
    return (
      <AuthFrame eyebrow="Invite" title="This link has expired" description="Invite links work once and for 7 days. Ask for a new one, or log in if you've already set up your account.">
        <ButtonLink className="mt-8 h-10 w-full" href="/login/">Go to log in</ButtonLink>
      </AuthFrame>
    );
  }

  return (
    <AuthFrame eyebrow="You're in" title="Set up your account" description={email ? <>Signing up as <span className="text-ink">{email}</span>. You&apos;ll get an empty workspace to add your own domain.</> : 'Checking your invite…'}>
      <form className="mt-8 flex flex-col gap-4" onSubmit={submit}>
        <Field label="Your name"><Input autoComplete="name" required value={form.name} onChange={set('name')} className="h-10" /></Field>
        <Field label="Username" hint="3 to 32 characters: lowercase letters, numbers, dots, dashes or underscores. You can log in with it.">
          <Input autoComplete="username" autoCapitalize="none" spellCheck={false} required value={form.username} onChange={set('username')} className="h-10" />
        </Field>
        <Field label="Workspace name" hint="Usually your company or project."><Input required value={form.workspace} onChange={set('workspace')} className="h-10" /></Field>
        <Field label="Password" hint="At least 10 characters."><Input type="password" autoComplete="new-password" required minLength={10} value={form.password} onChange={set('password')} className="h-10" /></Field>
        <FormError>{error}</FormError>
        <Button variant="primary" type="submit" loading={busy} disabled={!email} className="mt-1 h-10 w-full">
          {busy ? 'Creating…' : <>Create my workspace<ArrowRight /></>}
        </Button>
      </form>
    </AuthFrame>
  );
}
