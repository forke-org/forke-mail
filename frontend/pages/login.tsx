import { ArrowRight, Hash, Megaphone, ShieldCheck, Zap } from 'lucide-react';
import { FormEvent, useState } from 'react';
import Logo from '../components/Logo';
import { Button, Field, FormError, Input } from '../components/ui';
import { describeError } from '../lib/api';
import { useSession } from '../lib/session';

const features = [
  { icon: Megaphone, title: 'Broadcasts', text: 'Newsletters and launches with topics and a built-in preference centre.' },
  { icon: Zap, title: 'Transactional', text: 'Codes and receipts that always jump ahead of bulk sends.' },
  { icon: ShieldCheck, title: 'Safe by default', text: 'Bounces, complaints and unsubscribes are suppressed for you.' },
  { icon: Hash, title: 'Simple API', text: 'One key and one base URL to send from any app.' },
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
      <div className="relative hidden overflow-hidden bg-[#0b0604] lg:block">
        <span aria-hidden className="rail-y pointer-events-none absolute inset-y-0 left-0 w-px [--rail:rgba(240,227,222,0.16)]" />
        <div className="dot-grid absolute inset-0 [--dot:rgba(240,227,222,0.07)] [mask-image:linear-gradient(180deg,#000,transparent_75%)]" />
        <div className="absolute inset-x-[-20%] bottom-[-55%] h-[110%] rounded-[50%] bg-[radial-gradient(closest-side,rgba(255,177,92,0.9),rgba(255,122,0,0.65)_35%,rgba(107,38,4,0.45)_65%,transparent)] blur-2xl" />
        <div className="absolute inset-x-0 bottom-0 h-[45%] bg-[repeating-linear-gradient(0deg,rgba(11,6,4,0.35)_0_2px,transparent_2px_6px)]" />
        <div className="relative flex h-full flex-col justify-between p-12 xl:p-16">
          <div className="inline-flex w-fit items-center gap-2 rounded-[5px] border border-white/10 bg-white/5 px-2.5 py-1 font-mono text-[11px] tracking-[0.06em] text-white/70 uppercase backdrop-blur">
            <span className="size-1.5 animate-pulse rounded-[1.5px] bg-[#4ade80]" />All systems sending
          </div>
          <div>
            <h2 className="max-w-md text-[44px] leading-[1.04] font-medium tracking-[-0.04em] text-white">
              Email for products that ship<span className="text-[#FF7A00]">*</span>
            </h2>
            <p className="mt-4 max-w-md text-[15px] leading-relaxed text-white/60">
              Newsletters, changelogs and sign-in codes from one dashboard and one API.
            </p>
            <div className="mt-10 grid max-w-xl grid-cols-2 gap-3">
              {features.map(({ icon: Icon, title, text }) => (
                <div key={title} className="rounded-[8px] border border-white/[0.09] bg-black/30 p-4 backdrop-blur-md">
                  <Icon className="size-4 text-[#FFA347]" />
                  <div className="mt-3 text-sm font-medium text-white">{title}</div>
                  <div className="mt-1 text-[13px] leading-relaxed text-white/55">{text}</div>
                </div>
              ))}
            </div>
          </div>
          <div className="font-mono text-[11px] tracking-[0.06em] text-white/50 uppercase">Broadcasts · Transactional · API</div>
        </div>
      </div>
    </div>
  );
}
