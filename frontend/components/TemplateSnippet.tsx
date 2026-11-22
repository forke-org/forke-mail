/**
 * @fileoverview Forke Platform
 * @copyright (c) 2026 Forke Inc. (https://www.forke.space/)
 *
 * Source-Available License (Non-Commercial / Fair Source).
 * This source code is open for inspection, learning, and personal development.
 * Commercial use, hosting, or resale as a paid service without an explicit
 * commercial license from Forke Inc. is strictly prohibited.
 */

import { useEffect, useState } from 'react';
import { useMe } from '../lib/session';
import { CopyButton } from './ui';

export function SendSnippetTemplate({ id, vars, sample }: { id: string; vars: string[]; sample: Record<string, string> }) {
  const me = useMe();
  const [origin, setOrigin] = useState('https://mail.forke.space');
  useEffect(() => setOrigin(window.location.origin), []);
  const domain = me.domains.find((d) => !d.includes('@'));
  const data = Object.fromEntries(vars.map((v) => [v, sample[v] || `your ${v}`]));
  const body = { from_address: domain ? `hello@${domain}` : 'hello@yourdomain.com', to: 'user@example.com', template_id: id, data };
  const code = `curl -X POST ${origin}/api/transactional \\
  -H "Authorization: Bearer $FORKE_MAIL_API_KEY" \\
  -H "Content-Type: application/json" \\
  -d '${JSON.stringify(body, null, 2)}'`;
  return (
    <div className="overflow-hidden rounded-[6px] bg-surface-2 shadow-[inset_0_0_0_1px_var(--line)]">
      <div className="flex items-center justify-between gap-3 border-b border-line px-3 py-2">
        <span className="font-mono text-xs text-muted">POST /api/transactional</span>
        <CopyButton value={code} label="Copy" />
      </div>
      <pre className="overflow-x-auto p-4 font-mono text-[12px] leading-relaxed text-ink-2">{code}</pre>
    </div>
  );
}
