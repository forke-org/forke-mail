import Link from 'next/link';
import { useState } from 'react';
import AuthCard from '../components/AuthCard';
import { Field } from '../components/ui';
import { describeError } from '../lib/api';
import { useSession } from '../lib/session';

export default function Signup() {
  const { signup } = useSession();
  const [form, setForm] = useState({ name: '', email: '', password: '', workspace: '' });
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  const set = (key) => (e) => setForm((f) => ({ ...f, [key]: e.target.value }));

  const submit = async (e) => {
    e.preventDefault();
    setBusy(true);
    setError('');
    try {
      await signup({ name: form.name.trim(), email: form.email.trim(), password: form.password, workspace: form.workspace.trim() });
    } catch (err) {
      setError(describeError(err));
      setBusy(false);
    }
  };

  return (
    <AuthCard
      title="Create your account"
      subtitle="Start with a free trial: 100 emails, no card required."
      footer={<>Already have an account? <Link href="/login/">Log in</Link></>}
    >
      <form className="stack-sm" onSubmit={submit}>
        <Field label="Name"><input type="text" autoComplete="name" required autoFocus maxLength={100} value={form.name} onChange={set('name')} /></Field>
        <Field label="Email"><input type="email" autoComplete="email" required value={form.email} onChange={set('email')} /></Field>
        <Field label="Password" hint="At least 8 characters."><input type="password" autoComplete="new-password" required minLength={8} maxLength={72} value={form.password} onChange={set('password')} /></Field>
        <Field label="Company or workspace name" hint="Optional."><input type="text" maxLength={200} value={form.workspace} onChange={set('workspace')} /></Field>
        {error ? <p className="form-error">{error}</p> : null}
        <button className="btn btn-primary btn-block" type="submit" disabled={busy}>{busy ? 'Creating account…' : 'Create account'}</button>
      </form>
    </AuthCard>
  );
}
