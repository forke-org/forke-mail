import Link from 'next/link';
import { useState } from 'react';
import AuthCard from '../components/AuthCard';
import { Field } from '../components/ui';
import { describeError } from '../lib/api';
import { useSession } from '../lib/session';

export default function Login() {
  const { login } = useSession();
  const [form, setForm] = useState({ email: '', password: '' });
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  const set = (key) => (e) => setForm((f) => ({ ...f, [key]: e.target.value }));

  const submit = async (e) => {
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
    <AuthCard title="Log in to Forke Mail" footer={<>Don&apos;t have an account? <Link href="/signup/">Sign up</Link></>}>
      <form className="stack-sm" onSubmit={submit}>
        <Field label="Email"><input type="email" autoComplete="email" required autoFocus value={form.email} onChange={set('email')} /></Field>
        <Field label="Password"><input type="password" autoComplete="current-password" required value={form.password} onChange={set('password')} /></Field>
        {error ? <p className="form-error">{error}</p> : null}
        <button className="btn btn-primary btn-block" type="submit" disabled={busy}>{busy ? 'Logging in…' : 'Log in'}</button>
      </form>
    </AuthCard>
  );
}
