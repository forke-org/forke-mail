import { ArrowRight } from 'lucide-react';
import { FormEvent, useState } from 'react';
import Logo from '../components/Logo';
import MailGlobe from '../components/MailGlobe';
import { Button, Field, FormError, Input } from '../components/ui';
import { describeError } from '../lib/api';
import { useSession } from '../lib/session';

export default function Login() {
  const { login } = useSession();
  const [form, setForm] = useState({ email: '', password: '' });
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
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

  return (
    <div className="grid min-h-screen lg:grid-cols-[minmax(0,1fr)_minmax(0,1.1fr)]">
      <div className="relative flex flex-col px-6 py-6 sm:px-10">
        <Logo />
        <div className="mx-auto flex w-full max-w-[360px] flex-1 flex-col justify-center py-12">
          <div className="mb-3 flex items-center gap-2 font-mono text-[11px] tracking-[0.08em] text-muted uppercase">
            <span className="size-1.5 rounded-[1.5px] bg-accent" />Welcome back
          </div>
          <h1 className="text-[30px] leading-tight font-medium tracking-[-0.035em] text-ink">Log in to your workspace</h1>
          <p className="mt-2 text-sm text-muted">Enter the email or username and password for your account.</p>
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
        </div>
        <p className="text-xs text-faint">© {new Date().getFullYear()} Forke Mail</p>
      </div>
      <div className="relative hidden overflow-hidden bg-[radial-gradient(120%_90%_at_0%_50%,#1d1310_0%,#120c0a_45%,#0d0908_100%)] lg:block">
        <MailGlobe />
        <span aria-hidden className="rail-y pointer-events-none absolute inset-y-0 left-0 w-px [--rail:rgba(240,227,222,0.16)]" />
      </div>
    </div>
  );
}
