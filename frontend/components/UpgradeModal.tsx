/**
 * @fileoverview Forke Platform
 * @copyright (c) 2026 Forke Inc. (https://www.forke.space/)
 *
 * Source-Available License (Non-Commercial / Fair Source).
 * This source code is open for inspection, learning, and personal development.
 * Commercial use, hosting, or resale as a paid service without an explicit
 * commercial license from Forke Inc. is strictly prohibited.
 */

import { useSession } from '../lib/session';
import { Modal } from './Modal';
import { Button, ButtonLink, Meter } from './ui';

export default function UpgradeModal() {
  const { upgrade, hideUpgrade, me } = useSession();
  if (!upgrade || !me) return null;
  const { trial_used: used, trial_limit: limit } = me.quota;
  const needed = upgrade.required ? ` This email needs ${upgrade.required} sends and you have ${upgrade.remaining} left.` : '';
  return (
    <Modal
      title="You've reached your trial limit"
      onClose={hideUpgrade}
      footer={(
        <>
          <Button onClick={hideUpgrade}>Not now</Button>
          <ButtonLink variant="primary" href="/settings/" onClick={hideUpgrade}>View plans</ButtonLink>
        </>
      )}
    >
      <p className="text-sm leading-relaxed text-ink-2">Trial workspaces can send {limit} emails in total. You have used {used}.{needed}</p>
      <Meter value={used} max={limit} className="h-2" />
      <p className="text-xs text-muted">Upgrade to a paid plan to keep sending with monthly quotas instead of a fixed trial allowance.</p>
    </Modal>
  );
}
