/**
 * @fileoverview Forke Platform
 * @copyright (c) 2026 Forke Inc. (https://www.forke.space/)
 *
 * Source-Available License (Non-Commercial / Fair Source).
 * This source code is open for inspection, learning, and personal development.
 * Commercial use, hosting, or resale as a paid service without an explicit
 * commercial license from Forke Inc. is strictly prohibited.
 */

import { useEffect, useRef } from 'react';
import { LAND_MASK } from '../lib/globe-land';
import { mountMailGlobe } from '../lib/mail-globe';

export default function MailGlobe() {
  const canvas = useRef<HTMLCanvasElement>(null);
  useEffect(() => (canvas.current ? mountMailGlobe(canvas.current, LAND_MASK) : undefined), []);
  return <canvas ref={canvas} aria-hidden className="absolute inset-0 block size-full" />;
}
