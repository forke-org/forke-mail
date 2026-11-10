import clsx from 'clsx';
import { useEffect, useRef, useState } from 'react';
import { LOADER_SHAPES } from './forky-loader-data';

type Shape = (typeof LOADER_SHAPES)['mascot'] | (typeof LOADER_SHAPES)['word'];
type Line = { el: SVGPathElement; len: number; start: number; span: number; k: number };
type Built = { svg: SVGSVGElement; frame: SVGRectElement; lines: Line[]; ticks: SVGPathElement[]; lead: Line };

const OVERLAP = 0.55;
const STEPS = [
  { shape: 'mascot', mode: 'draw', ms: 1900 },
  { shape: 'mascot', mode: 'hold', ms: 650 },
  { shape: 'mascot', mode: 'erase', ms: 1300 },
  { shape: 'word', mode: 'draw', ms: 1600 },
  { shape: 'word', mode: 'hold', ms: 650 },
  { shape: 'word', mode: 'erase', ms: 1100 },
] as const;
const CYCLE = STEPS.reduce((total, s) => total + s.ms, 0);
const EXIT_MS = 380;
const MIN_MS = 1200;
const RECENT_MS = 60000;
const SHOWN_KEY = 'forky-loader-shown-at';

export function useLoaderMinimum() {
  const [done, setDone] = useState(false);
  useEffect(() => {
    let wait = MIN_MS;
    try {
      const last = Number(sessionStorage.getItem(SHOWN_KEY));
      if (last && Date.now() - last < RECENT_MS) wait = 0;
      sessionStorage.setItem(SHOWN_KEY, String(Date.now()));
    } catch {
      wait = MIN_MS;
    }
    const timer = setTimeout(() => setDone(true), wait);
    return () => clearTimeout(timer);
  }, []);
  return done;
}

const ease = (t: number) => (t < 0.5 ? 2 * t * t : 1 - Math.pow(-2 * t + 2, 2) / 2);

function viewBox(shape: Shape) {
  const pad = Math.max(shape.w, shape.h) * 0.06;
  return `${-pad} ${shape.top - pad} ${shape.w + pad * 2} ${shape.h + pad * 2}`;
}

function measure(svg: SVGSVGElement): Built {
  const paths = Array.from(svg.querySelectorAll<SVGPathElement>('path[data-line]'));
  const lengths = paths.map((p) => p.getTotalLength());
  const total = lengths.reduce((a, b) => a + b, 0) || 1;
  let acc = 0;
  const lines = paths.map((el, i) => {
    const len = lengths[i];
    el.style.strokeDasharray = `${len} ${len}`;
    el.style.strokeDashoffset = String(len);
    const line = { el, len, start: acc / total, span: len / total, k: 0 };
    acc += len;
    return line;
  });
  return {
    svg,
    frame: svg.querySelector('rect') as SVGRectElement,
    lines,
    ticks: Array.from(svg.querySelectorAll<SVGPathElement>('path[data-tick]')),
    lead: lines.reduce((a, b) => (b.len > a.len ? b : a)),
  };
}

function paint(shape: Built, progress: number, reverse: boolean) {
  for (const l of shape.lines) {
    const begin = (reverse ? 1 - l.start - l.span : l.start) * OVERLAP;
    const local = Math.min(1, Math.max(0, (progress - begin) / (1 - OVERLAP + l.span * OVERLAP)));
    l.k = reverse ? 1 - local : local;
    l.el.style.strokeDashoffset = String(l.len * (1 - l.k));
  }
}

function ShapeSvg({ shape, svgRef }: { shape: Shape; svgRef: React.RefObject<SVGSVGElement | null> }) {
  const s = Math.max(shape.w, shape.h) * 0.012;
  return (
    <svg ref={svgRef} viewBox={viewBox(shape)} preserveAspectRatio="xMidYMid meet" className="absolute inset-0 size-full overflow-visible opacity-0">
      <rect x={0} y={shape.top} width={shape.w} height={shape.h} fill="none" stroke="rgba(240,227,222,0.14)" strokeDasharray="3 5" vectorEffect="non-scaling-stroke" opacity={0} />
      {shape.parts.map((part, i) => (
        <path
          key={i}
          data-line=""
          d={part.d}
          fill="none"
          stroke={part.accent ? '#ff7a00' : '#f0e3de'}
          strokeWidth={1.4}
          strokeLinecap="round"
          strokeLinejoin="round"
          vectorEffect="non-scaling-stroke"
        />
      ))}
      {shape.ticks.map(([x, y], i) => (
        <path key={`t${i}`} data-tick="" d={`M${x - s} ${y}H${x + s}M${x} ${y - s}V${y + s}`} stroke="#ff7a00" strokeWidth={1} vectorEffect="non-scaling-stroke" opacity={0} />
      ))}
    </svg>
  );
}

export default function ForkyLoader({ label = 'Loading', fullScreen = false, className, exiting = false, onExited }: {
  label?: string; fullScreen?: boolean; className?: string; exiting?: boolean; onExited?: () => void;
}) {
  const stage = useRef<HTMLDivElement>(null);
  const pen = useRef<HTMLSpanElement>(null);
  const mascotRef = useRef<SVGSVGElement>(null);
  const wordRef = useRef<SVGSVGElement>(null);
  const root = useRef<HTMLDivElement>(null);
  const exitRef = useRef(exiting);
  const exitedRef = useRef(onExited);
  const doneRef = useRef(false);
  exitRef.current = exiting;
  exitedRef.current = onExited;

  const finish = () => {
    if (doneRef.current) return;
    doneRef.current = true;
    if (root.current) root.current.style.opacity = '0';
    setTimeout(() => exitedRef.current?.(), 200);
  };
  const finishRef = useRef(finish);
  finishRef.current = finish;

  useEffect(() => {
    if (!exiting) return undefined;
    if (document.hidden) { finishRef.current(); return undefined; }
    const fallback = setTimeout(() => finishRef.current(), EXIT_MS + 400);
    return () => clearTimeout(fallback);
  }, [exiting]);

  useEffect(() => {
    if (!mascotRef.current || !wordRef.current) return undefined;
    const shapes = { mascot: measure(mascotRef.current), word: measure(wordRef.current) };
    const finish = () => finishRef.current();
    if (window.matchMedia('(prefers-reduced-motion: reduce)').matches) {
      paint(shapes.mascot, 1, false);
      shapes.mascot.svg.style.opacity = '1';
      shapes.mascot.frame.style.opacity = '1';
      const wait = setInterval(() => { if (exitRef.current) { clearInterval(wait); finish(); } }, 50);
      return () => clearInterval(wait);
    }
    let exitAt = 0;
    let frozen: number[] = [];
    const placePen = (shape: Built) => {
      const dot = pen.current;
      const box = stage.current?.getBoundingClientRect();
      const l = shape.lead;
      const m = l.el.getScreenCTM();
      if (!dot || !box || !m || l.k <= 0.001 || l.k >= 0.999) { if (dot) dot.style.opacity = '0'; return; }
      const pt = l.el.getPointAtLength(l.len * l.k);
      dot.style.opacity = '1';
      dot.style.transform = `translate(${pt.x * m.a + m.e - box.left}px, ${pt.y * m.d + m.f - box.top}px)`;
    };
    let raf = 0;
    const start = performance.now();
    const frame = (now: number) => {
      if (exitRef.current) {
        const visible = [shapes.mascot, shapes.word].find((s) => s.svg.style.opacity === '1') || shapes.mascot;
        if (!exitAt) { exitAt = now; frozen = visible.lines.map((l) => l.k); }
        const e = ease(Math.min(1, (now - exitAt) / EXIT_MS));
        visible.lines.forEach((l, i) => { l.k = frozen[i] * (1 - e); l.el.style.strokeDashoffset = String(l.len * (1 - l.k)); });
        visible.frame.style.opacity = String(1 - e);
        visible.ticks.forEach((t) => { t.style.opacity = String(Math.min(Number(t.style.opacity || 0), 1 - e)); });
        placePen(visible);
        if (e >= 1) { if (pen.current) pen.current.style.opacity = '0'; finish(); return; }
        raf = requestAnimationFrame(frame);
        return;
      }
      let c = (now - start) % CYCLE;
      let step: (typeof STEPS)[number] = STEPS[0];
      for (const s of STEPS) { if (c < s.ms) { step = s; break; } c -= s.ms; }
      const p = ease(c / step.ms);
      const shape = shapes[step.shape];
      shapes.mascot.svg.style.opacity = step.shape === 'mascot' ? '1' : '0';
      shapes.word.svg.style.opacity = step.shape === 'word' ? '1' : '0';
      const reverse = step.mode === 'erase';
      paint(shape, step.mode === 'hold' ? 1 : p, reverse);
      const drawn = step.mode === 'draw' ? p : step.mode === 'hold' ? 1 : 1 - p;
      shape.frame.style.opacity = String(Math.min(1, drawn * 1.4));
      shape.ticks.forEach((t, i) => {
        const o = step.mode === 'hold' ? 1 : step.mode === 'draw' ? (p - 0.75 - (i % 5) * 0.03) / 0.15 : 1 - p * 3;
        t.style.opacity = String(Math.min(1, Math.max(0, o)));
      });
      if (step.mode === 'hold') { if (pen.current) pen.current.style.opacity = '0'; } else placePen(shape);
      raf = requestAnimationFrame(frame);
    };
    raf = requestAnimationFrame(frame);
    return () => cancelAnimationFrame(raf);
  }, []);

  return (
    <div
      ref={root}
      role="status"
      aria-label={label}
      className={clsx(
        'grid place-items-center bg-bg transition-opacity duration-200',
        fullScreen ? 'fixed inset-0 z-[90]' : 'min-h-[320px] w-full',
        className,
      )}
    >
      <div className="grid w-[min(360px,64vw)] justify-items-center gap-6">
        <div ref={stage} className="relative aspect-[1.6] w-full">
          <div aria-hidden className="dot-grid pointer-events-none absolute -inset-[18%] [mask-image:radial-gradient(closest-side,#000_40%,transparent)]" />
          <ShapeSvg shape={LOADER_SHAPES.mascot} svgRef={mascotRef} />
          <ShapeSvg shape={LOADER_SHAPES.word} svgRef={wordRef} />
          <span
            ref={pen}
            aria-hidden
            className="pointer-events-none absolute top-0 left-0 -mt-[3px] -ml-[3px] size-1.5 rounded-full bg-accent opacity-0 shadow-[0_0_0_3px_rgba(255,122,0,0.18),0_0_12px_rgba(255,122,0,0.7)]"
          />
        </div>
        <div className="flex items-center gap-2 font-mono text-[10.5px] tracking-[0.1em] text-muted uppercase">
          <i className="size-[5px] animate-pulse rounded-[1.5px] bg-accent" />
          {label}
        </div>
      </div>
    </div>
  );
}
