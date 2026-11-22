/**
 * @fileoverview Forke Platform
 * @copyright (c) 2026 Forke Inc. (https://www.forke.space/)
 *
 * Source-Available License (Non-Commercial / Fair Source).
 * This source code is open for inspection, learning, and personal development.
 * Commercial use, hosting, or resale as a paid service without an explicit
 * commercial license from Forke Inc. is strictly prohibited.
 */

import type { ReactNode } from 'react';
import Logo from './Logo';
import MailGlobe from './MailGlobe';

export default function AuthFrame({ eyebrow, title, description, children }: { eyebrow: string; title: string; description: ReactNode; children: ReactNode }) {
  return (
    <div className="grid min-h-screen lg:grid-cols-[minmax(0,1fr)_minmax(0,1.1fr)]">
      <div className="relative flex flex-col px-6 py-6 sm:px-10">
        <Logo />
        <div className="mx-auto flex w-full max-w-[360px] flex-1 flex-col justify-center py-12">
          <div className="mb-3 flex items-center gap-2 font-mono text-[11px] tracking-[0.08em] text-muted uppercase">
            <span className="size-1.5 rounded-[1.5px] bg-accent" />{eyebrow}
          </div>
          <h1 className="text-[30px] leading-tight font-medium tracking-[-0.035em] text-ink">{title}</h1>
          <p className="mt-2 text-sm text-muted">{description}</p>
          {children}
        </div>
        <p className="text-xs text-faint">© 2026 forke &middot; sends through Amazon SES &middot; <a href="https://status.forke.space" target="_blank" rel="noreferrer" className="transition-colors hover:text-ink">Status</a></p>
      </div>
      <div className="relative hidden overflow-hidden bg-[radial-gradient(120%_90%_at_0%_50%,#1d1310_0%,#120c0a_45%,#0d0908_100%)] lg:block">
        <MailGlobe />
        <span aria-hidden className="rail-y pointer-events-none absolute inset-y-0 left-0 w-px [--rail:rgba(240,227,222,0.16)]" />
      </div>
    </div>
  );
}
