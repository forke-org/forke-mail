import { ShieldAlert, ShieldCheck } from 'lucide-react';
import { FormActions, Modal } from './Modal';
import { Button } from './ui';

export interface SpamResult {
  available: boolean;
  score: number;
  threshold: number;
  spam: boolean;
  rules: { name: string; score: number; description: string }[];
  warnings: string[];
}

export default function SpamReport({ result, onClose }: { result: SpamResult; onClose: () => void }) {
  const bad = result.rules.filter((r) => r.score > 0).sort((a, b) => b.score - a.score);
  const Icon = result.spam ? ShieldAlert : ShieldCheck;
  return (
    <Modal title="Spam check" onClose={onClose}>
      <div className="flex flex-col gap-4">
        {result.available ? (
          <div className="flex items-center gap-3">
            <Icon className={result.spam ? 'size-6 text-danger' : 'size-6 text-success'} />
            <div>
              <div className="text-[15px] font-medium text-ink tabular-nums">{result.score.toFixed(1)} <span className="text-muted">/ {result.threshold.toFixed(1)} points</span></div>
              <div className="text-xs text-muted">{result.spam ? 'Too spammy to send. Fix the items below and check again.' : 'Looks fine. Lower is better.'}</div>
            </div>
          </div>
        ) : <p className="text-[13px] text-muted">The full content scan isn&apos;t available right now, so only the quick checks below ran.</p>}
        {bad.length ? (
          <div className="flex flex-col divide-y divide-line rounded-[6px] bg-surface-2 shadow-[inset_0_0_0_1px_var(--line)]">
            {bad.map((r) => (
              <div key={r.name} className="flex gap-3 px-3 py-2 text-[13px]">
                <span className="w-10 shrink-0 text-right font-mono text-danger tabular-nums">+{r.score.toFixed(1)}</span>
                <span className="text-ink-2">{r.description || r.name}</span>
              </div>
            ))}
          </div>
        ) : null}
        {result.warnings.length ? (
          <ul className="flex list-disc flex-col gap-1 pl-5 text-[13px] text-ink-2">
            {result.warnings.map((w) => <li key={w}>{w}</li>)}
          </ul>
        ) : null}
        {result.available && !bad.length && !result.warnings.length ? <p className="text-[13px] text-muted">No problems found.</p> : null}
        <FormActions><Button onClick={onClose}>Close</Button></FormActions>
      </div>
    </Modal>
  );
}
