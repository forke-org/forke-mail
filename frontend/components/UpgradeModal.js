import Link from 'next/link';
import { useSession } from '../lib/session';
import { Modal } from './Modal';

export default function UpgradeModal() {
  const { upgrade, hideUpgrade, me } = useSession();
  if (!upgrade || !me) return null;
  const { trial_used: used, trial_limit: limit } = me.quota;
  const needed = upgrade.required ? `This email needs ${upgrade.required} sends and you have ${upgrade.remaining} left.` : null;
  return (
    <Modal
      title="You've reached your trial limit"
      onClose={hideUpgrade}
      footer={(
        <>
          <button className="btn" type="button" onClick={hideUpgrade}>Not now</button>
          <Link className="btn btn-primary" href="/settings/" onClick={hideUpgrade}>View plans</Link>
        </>
      )}
    >
      <p className="muted">
        Trial workspaces can send {limit} emails in total. You have used {used}. {needed}
      </p>
      <div className="meter meter-lg"><div className="meter-fill full" style={{ width: `${limit ? Math.min(100, (used / limit) * 100) : 100}%` }} /></div>
      <p className="muted small">Upgrade to a paid plan to keep sending with daily limits instead of a fixed trial quota.</p>
    </Modal>
  );
}
