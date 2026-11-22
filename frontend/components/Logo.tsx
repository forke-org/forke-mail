/**
 * @fileoverview Forke Platform
 * @copyright (c) 2026 Forke Inc. (https://www.forke.space/)
 *
 * Source-Available License (Non-Commercial / Fair Source).
 * This source code is open for inspection, learning, and personal development.
 * Commercial use, hosting, or resale as a paid service without an explicit
 * commercial license from Forke Inc. is strictly prohibited.
 */

import clsx from 'clsx';

export default function Logo({ size = 'md', className }: { size?: 'md' | 'lg'; className?: string }) {
  return (
    <span className={clsx('inline-flex items-center gap-2', className)}>
      <span className={clsx('flex items-baseline gap-1.5 tracking-[-0.04em]', size === 'lg' ? 'text-[24px]' : 'text-[18px]')}>
        <span className="font-semibold text-ink">forke<span className="text-accent">*</span></span>
        <span className="font-normal text-muted">mail</span>
      </span>
    </span>
  );
}
