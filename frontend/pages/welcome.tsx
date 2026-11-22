/**
 * @fileoverview Forke Platform
 * @copyright (c) 2026 Forke Inc. (https://www.forke.space/)
 *
 * Source-Available License (Non-Commercial / Fair Source).
 * This source code is open for inspection, learning, and personal development.
 * Commercial use, hosting, or resale as a paid service without an explicit
 * commercial license from Forke Inc. is strictly prohibited.
 */

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
  const teamToken = typeof router.query.team === 'string' ? router.query.team : '';
  const token = teamToken || (typeof router.query.token === 'string' ? router.query.token : '');
  const joining = Boolean(teamToken);
  const [email, setEmail] = useState<string | null>(null);
  const [teamName, setTeamName] = useState('');
  const [invalid, setInvalid] = useState(false);
  const [form, setForm] = useState({ name: '', username: '', workspace: '', password: '' });
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  const set = (key: keyof typeof form) => (e: React.ChangeEvent<HTMLInputElement>) => setForm((f) => ({ ...f, [key]: e.target.value }));

  useEffect(() => {
    if (!router.isReady) return;
    if (!token) { setInvalid(true); return; }
    const path = joining ? '/api/team/join' : '/api/access/invite';
    api<{ email: string; workspace?: string }>('GET', `${path}?token=${encodeURIComponent(token)}`).then((d) => { setEmail(d.email); setTeamName(d.workspace || ''); }, () => setInvalid(true));
  }, [router.isReady, token, joining]);

  const submit = async (e: FormEvent) => {
    e.preventDefault();
    setBusy(true);
    setError('');
    try {
      if (joining) await api('POST', '/api/team/join', { token, name: form.name, username: form.username, password: form.password });
      else await api('POST', '/api/access/invite', { token, ...form });
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
    <AuthFrame eyebrow="You're in" title={joining && teamName ? `Join ${teamName}` : 'Set up your account'} description={!email ? 'Checking your invite…' : joining ? <>Signing up as <span className="text-ink">{email}</span>. You&apos;ll join the {teamName} team.</> : <>Signing up as <span className="text-ink">{email}</span>. You&apos;ll get an empty workspace to add your own domain.</>}>
      <form className="mt-8 flex flex-col gap-4" onSubmit={submit}>
        <Field label="Your name"><Input autoComplete="name" required value={form.name} onChange={set('name')} className="h-10" /></Field>
        <Field label="Username" hint="3 to 32 characters: lowercase letters, numbers, dots, dashes or underscores. You can log in with it.">
          <Input autoComplete="username" autoCapitalize="none" spellCheck={false} required value={form.username} onChange={set('username')} className="h-10" />
        </Field>
        {joining ? null : <Field label="Workspace name" hint="Usually your company or project."><Input required value={form.workspace} onChange={set('workspace')} className="h-10" /></Field>}
        <Field label="Password" hint="At least 10 characters."><Input type="password" autoComplete="new-password" required minLength={10} value={form.password} onChange={set('password')} className="h-10" /></Field>
        <FormError>{error}</FormError>
        <Button variant="primary" type="submit" loading={busy} disabled={!email} className="mt-1 h-10 w-full">
          {busy ? 'Creating…' : <>{joining ? 'Join the team' : 'Create my workspace'}<ArrowRight /></>}
        </Button>
      </form>
    </AuthFrame>
  );
}
