/**
 * @fileoverview Forke Platform
 * @copyright (c) 2026 Forke Inc. (https://www.forke.space/)
 *
 * Source-Available License (Non-Commercial / Fair Source).
 * This source code is open for inspection, learning, and personal development.
 * Commercial use, hosting, or resale as a paid service without an explicit
 * commercial license from Forke Inc. is strictly prohibited.
 */

import { ArrowLeft, ArrowRight, MailCheck } from 'lucide-react';
import { FormEvent, useState } from 'react';
import AuthFrame from '../components/AuthFrame';
import { Button, Field, FormError, Input } from '../components/ui';
import { api, describeError } from '../lib/api';
import { useSession } from '../lib/session';

function EarlyAccess({ onBack }: { onBack: () => void }) {
  const [email, setEmail] = useState('');
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  const [done, setDone] = useState(false);

  const submit = async (e: FormEvent) => {
    e.preventDefault();
    setBusy(true);
    setError('');
    try {
      await api('POST', '/api/access/request', { email: email.trim() });
      setDone(true);
    } catch (err) {
      setError(describeError(err));
    } finally {
      setBusy(false);
    }
  };

  if (done) {
    return (
      <AuthFrame eyebrow="Early access" title="You're on the list" description={<>We&apos;ll email <span className="text-ink">{email.trim()}</span> a link to set up your account as soon as you&apos;re approved.</>}>
        <div className="mt-8 flex items-start gap-3 rounded-[8px] bg-surface-2 p-4 text-[13px] text-muted shadow-[inset_0_0_0_1px_var(--line)]">
          <MailCheck className="mt-0.5 size-4 shrink-0 text-accent" />
          <span>Nothing else to do for now. The link works for 7 days once it arrives.</span>
        </div>
        <Button className="mt-6 h-10 w-full" icon={ArrowLeft} onClick={onBack}>Back to log in</Button>
      </AuthFrame>
    );
  }

  return (
    <AuthFrame eyebrow="Early access" title="Request early access" description="Forke Mail is invite only for now. Leave your email and we'll send you a link to set up your account once you're approved.">
      <form className="mt-8 flex flex-col gap-4" onSubmit={submit}>
        <Field label="Email">
          <Input type="email" autoComplete="email" autoCapitalize="none" spellCheck={false} required autoFocus value={email} onChange={(e) => setEmail(e.target.value)} className="h-10" />
        </Field>
        <FormError>{error}</FormError>
        <Button variant="primary" type="submit" loading={busy} className="mt-1 h-10 w-full">
          {busy ? 'Sending…' : <>Request access<ArrowRight /></>}
        </Button>
        <Button className="h-10 w-full" icon={ArrowLeft} onClick={onBack}>Back to log in</Button>
      </form>
    </AuthFrame>
  );
}

export default function Login() {
  const { login } = useSession();
  const [form, setForm] = useState({ email: '', password: '' });
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  const [requesting, setRequesting] = useState(false);
  const set = (key: 'email' | 'password') => (e: React.ChangeEvent<HTMLInputElement>) => setForm((f) => ({ ...f, [key]: e.target.value }));

  const submit = async (e: FormEvent) => {
    e.preventDefault();
    setBusy(true);
    setError('');
    try {
      await login(form.email.trim(), form.password);
    } catch (err) {
      setError(describeError(err));
      setBusy(false);
    }
  };

  if (requesting) return <EarlyAccess onBack={() => setRequesting(false)} />;

  return (
    <AuthFrame eyebrow="Welcome back" title="Log in to your workspace" description="Enter the email or username and password for your account.">
      <form className="mt-8 flex flex-col gap-4" onSubmit={submit}>
        <Field label="Email or username">
          <Input autoComplete="username" autoCapitalize="none" spellCheck={false} required autoFocus value={form.email} onChange={set('email')} className="h-10" />
        </Field>
        <Field label="Password">
          <Input type="password" autoComplete="current-password" required value={form.password} onChange={set('password')} className="h-10" />
        </Field>
        <FormError>{error}</FormError>
        <Button variant="primary" type="submit" loading={busy} className="mt-1 h-10 w-full">
          {busy ? 'Logging in…' : <>Log in<ArrowRight /></>}
        </Button>
      </form>
      <div className="mt-6 flex items-center gap-3 text-xs text-faint"><span className="h-px flex-1 bg-line" />No account yet?<span className="h-px flex-1 bg-line" /></div>
      <Button className="mt-4 h-10 w-full" onClick={() => setRequesting(true)}>Need early access</Button>
    </AuthFrame>
  );
}
