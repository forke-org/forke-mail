import { ArrowRightLeft } from 'lucide-react';
import { FormEvent, useEffect, useState } from 'react';
import { api, describeError } from '../lib/api';
import { number } from '../lib/format';
import { useMe, useSession } from '../lib/session';
import { useToast } from '../lib/toast';
import { Button, Card, Field, FormError, Input, Select } from './ui';

type Project = { id: string; name: string; role: string; plan: string; current: boolean };

export default function MoveCredits() {
  const me = useMe();
  const { superAdmin, trial, refreshMe } = useSession();
  const toast = useToast();
  const [targets, setTargets] = useState<Project[]>([]);
  const [to, setTo] = useState('');
  const [amount, setAmount] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const left = Math.max(0, me.quota.trial_limit - me.quota.trial_used);

  useEffect(() => {
    if (!superAdmin || !trial) return;
    api<{ projects: Project[] }>('GET', '/api/projects').then((d) => {
      const list = d.projects.filter((p) => !p.current && p.role === 'super_admin' && p.plan === 'trial');
      setTargets(list);
      setTo(list[0]?.id || '');
    }, () => setTargets([]));
  }, [superAdmin, trial, me.workspace.id]);

  if (!superAdmin || !trial || me.workspace.platform || !targets.length) return null;

  const submit = async (e: FormEvent) => {
    e.preventDefault();
    setBusy(true);
    setError('');
    try {
      await api('POST', '/api/projects/credits', { to, amount: Number(amount) });
      await refreshMe();
      setAmount('');
      toast(`Moved ${number(Number(amount))} credits.`, 'success');
    } catch (err) { setError(describeError(err)); } finally { setBusy(false); }
  };

  return (
    <Card title="Move credits" icon={ArrowRightLeft} description={`Your free credits are shared across the projects you own. This project has ${number(left)} left.`}>
      <form className="grid gap-3 sm:grid-cols-[minmax(0,1fr)_140px_auto] sm:items-end" onSubmit={submit}>
        <Field label="To project">
          <Select value={to} onChange={(e) => setTo(e.target.value)} aria-label="To project">
            {targets.map((p) => <option key={p.id} value={p.id}>{p.name}</option>)}
          </Select>
        </Field>
        <Field label="Credits"><Input type="number" min={1} max={left} required value={amount} onChange={(e) => setAmount(e.target.value)} /></Field>
        <Button type="submit" loading={busy} disabled={!amount || Number(amount) > left}>Move</Button>
      </form>
      <FormError>{error}</FormError>
    </Card>
  );
}
