import { ArrowRight, Hash, Megaphone, ShieldCheck, Zap } from 'lucide-react';
import { FormEvent, useState } from 'react';
import Logo from '../components/Logo';
import ThemeToggle from '../components/ThemeToggle';
import { Button, Field, FormError, Input } from '../components/ui';
import { describeError } from '../lib/api';
import { useSession } from '../lib/session';

const features = [
  { icon: Megaphone, title: 'Broadcasts', text: 'Newsletters and launches to every subscriber, with topics and a preference centre.' },
  { icon: Zap, title: 'Transactional', text: 'Codes and receipts that jump the queue ahead of every broadcast.' },
  { icon: ShieldCheck, title: 'Protected by default', text: 'Bounces, complaints and unsubscribes are suppressed automatically.' },
  { icon: Hash, title: 'Resend compatible', text: 'Point existing code at Forke Mail by changing one base URL.' },
];

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
        <div className="flex items-center justify-between">
          <Logo />
          <ThemeToggle />
        </div>
        <div className="mx-auto flex w-full max-w-[360px] flex-1 flex-col justify-center py-12">
          <h1 className="text-[28px] font-semibold tracking-[-0.03em] text-ink">Log in to Forke Mail</h1>
          <p className="mt-2 text-sm text-muted">Use your Forke admin account.</p>
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
        <p className="text-xs text-faint">© {new Date().getFullYear()} Forke · mail.forke.space</p>
      </div>
      <div className="relative hidden overflow-hidden border-l border-line bg-[#050505] lg:block">
        <div className="dot-grid absolute inset-0 opacity-60 [--dot:rgba(255,255,255,0.08)]" />
        <div className="absolute -top-40 -right-40 size-[720px] rounded-full bg-[radial-gradient(circle,rgba(255,122,0,0.55)_0%,rgba(255,92,0,0.22)_35%,transparent_68%)] blur-2xl" />
        <div className="absolute -bottom-60 -left-20 size-[520px] rounded-full bg-[radial-gradient(circle,rgba(255,122,0,0.22)_0%,transparent_65%)] blur-2xl" />
        <div className="relative flex h-full flex-col justify-between p-12 xl:p-16">
          <div className="inline-flex w-fit items-center gap-2 rounded-full border border-white/10 bg-white/5 px-3 py-1 text-xs text-white/70 backdrop-blur">
            <span className="size-1.5 animate-pulse rounded-full bg-[#4ade80]" />Delivering through Amazon SES
          </div>
          <div>
            <img src="/forke-mark.png" alt="" className="mb-8 size-16 rounded-2xl shadow-[0_20px_60px_-10px_rgba(255,122,0,0.6)]" />
            <h2 className="max-w-md text-[40px] leading-[1.05] font-semibold tracking-[-0.04em] text-white">
              Email that ships with Forke<span className="text-[#FF7A00]">*</span>
            </h2>
            <p className="mt-4 max-w-md text-[15px] leading-relaxed text-white/60">
              One place for every newsletter, changelog and sign-in code Forke sends.
            </p>
            <div className="mt-10 grid max-w-xl grid-cols-2 gap-3">
              {features.map(({ icon: Icon, title, text }) => (
                <div key={title} className="rounded-xl border border-white/[0.08] bg-white/[0.03] p-4 backdrop-blur-sm">
                  <Icon className="size-4 text-[#FFA347]" />
                  <div className="mt-3 text-sm font-medium text-white">{title}</div>
                  <div className="mt-1 text-[13px] leading-relaxed text-white/50">{text}</div>
                </div>
              ))}
            </div>
          </div>
          <div className="text-xs text-white/40">Built by the Forke team in India.</div>
        </div>
      </div>
    </div>
  );
}
