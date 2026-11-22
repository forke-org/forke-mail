/**
 * @fileoverview Forke Platform
 * @copyright (c) 2026 Forke Inc. (https://www.forke.space/)
 *
 * Source-Available License (Non-Commercial / Fair Source).
 * This source code is open for inspection, learning, and personal development.
 * Commercial use, hosting, or resale as a paid service without an explicit
 * commercial license from Forke Inc. is strictly prohibited.
 */

import { number } from '../lib/format';

export default function ResultGrid({ items }: { items: [string, number][] }) {
  return (
    <div className="grid grid-cols-2 gap-2 sm:grid-cols-4">
      {items.map(([label, n]) => (
        <div key={label} className="rounded-lg bg-surface-2 px-3 py-2.5 shadow-[inset_0_0_0_1px_var(--line)]">
          <div className="text-lg font-semibold text-ink tabular-nums">{number(n)}</div>
          <div className="text-xs text-muted">{label}</div>
        </div>
      ))}
    </div>
  );
}
