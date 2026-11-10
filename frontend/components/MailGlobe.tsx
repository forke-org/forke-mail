import { useEffect, useRef } from 'react';
import { LAND_MASK } from '../lib/globe-land';
import { mountMailGlobe } from '../lib/mail-globe';

export default function MailGlobe() {
  const canvas = useRef<HTMLCanvasElement>(null);
  useEffect(() => (canvas.current ? mountMailGlobe(canvas.current, LAND_MASK) : undefined), []);
  return <canvas ref={canvas} aria-hidden className="absolute inset-0 block size-full" />;
}
