import { useCallback, useEffect, useRef, useState, type ReactNode } from 'react';

type Thumb = { size: number; offset: number; scrollable: boolean };
type Source = {
  read: () => { top: number; height: number; client: number };
  scrollTo: (top: number) => void;
  listen: (onChange: () => void) => () => void;
};

const PAD = 4;
const MIN_THUMB = 32;
const IDLE_MS = 900;

function useScrollbar(source: Source | null) {
  const [thumb, setThumb] = useState<Thumb>({ size: 0, offset: 0, scrollable: false });
  const [active, setActive] = useState(false);
  const idle = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
  const drag = useRef<{ startY: number; startTop: number; ratio: number } | null>(null);

  const measure = useCallback(() => {
    if (!source) return;
    const { top, height, client } = source.read();
    const range = height - client;
    if (range <= 1) {
      setThumb({ size: 0, offset: 0, scrollable: false });
      return;
    }
    const track = client - PAD * 2;
    const size = Math.max(MIN_THUMB, (client / height) * track);
    setThumb({ size, offset: PAD + (top / range) * (track - size), scrollable: true });
  }, [source]);

  const wake = useCallback(() => {
    setActive(true);
    clearTimeout(idle.current);
    idle.current = setTimeout(() => setActive(false), IDLE_MS);
  }, []);

  useEffect(() => {
    if (!source) return undefined;
    measure();
    const stop = source.listen(() => {
      measure();
      wake();
    });
    return () => {
      stop();
      clearTimeout(idle.current);
    };
  }, [source, measure, wake]);

  const onPointerDown = (e: React.PointerEvent<HTMLDivElement>) => {
    if (!source) return;
    e.preventDefault();
    e.currentTarget.setPointerCapture(e.pointerId);
    const { top, height, client } = source.read();
    const track = client - PAD * 2;
    drag.current = { startY: e.clientY, startTop: top, ratio: (height - client) / Math.max(1, track - thumb.size) };
  };
  const onPointerMove = (e: React.PointerEvent<HTMLDivElement>) => {
    if (!source || !drag.current) return;
    source.scrollTo(drag.current.startTop + (e.clientY - drag.current.startY) * drag.current.ratio);
  };
  const onPointerUp = () => {
    drag.current = null;
  };

  return { thumb, active, onPointerDown, onPointerMove, onPointerUp, wake };
}

function Bar({ bar }: { bar: ReturnType<typeof useScrollbar> }) {
  if (!bar.thumb.scrollable) return null;
  return (
    <div
      aria-hidden
      onPointerEnter={bar.wake}
      className="group/bar absolute top-0 right-0 bottom-0 z-40 w-3"
    >
      <div
        onPointerDown={bar.onPointerDown}
        onPointerMove={bar.onPointerMove}
        onPointerUp={bar.onPointerUp}
        onPointerCancel={bar.onPointerUp}
        style={{ height: bar.thumb.size, transform: `translateY(${bar.thumb.offset}px)` }}
        className={`absolute top-0 right-[3px] w-1 cursor-grab touch-none rounded-full bg-accent/80 transition-[opacity,width] duration-200 group-hover/bar:w-1.5 hover:bg-accent active:cursor-grabbing ${
          bar.active ? 'opacity-100' : 'opacity-0 group-hover/bar:opacity-100'
        }`}
      />
    </div>
  );
}

export default function ScrollArea({ children, className = '', contentClassName = '', id }: {
  children: ReactNode;
  className?: string;
  contentClassName?: string;
  id?: string;
}) {
  const viewport = useRef<HTMLDivElement>(null);
  const content = useRef<HTMLDivElement>(null);
  const [source, setSource] = useState<Source | null>(null);

  useEffect(() => {
    const el = viewport.current;
    const inner = content.current;
    if (!el || !inner) return;
    setSource({
      read: () => ({ top: el.scrollTop, height: el.scrollHeight, client: el.clientHeight }),
      scrollTo: (top) => { el.scrollTop = top; },
      listen: (onChange) => {
        const resize = new ResizeObserver(onChange);
        resize.observe(el);
        resize.observe(inner);
        el.addEventListener('scroll', onChange, { passive: true });
        return () => {
          resize.disconnect();
          el.removeEventListener('scroll', onChange);
        };
      },
    });
  }, []);

  const bar = useScrollbar(source);

  return (
    <div className={`relative min-h-0 ${className}`}>
      <div ref={viewport} id={id} className="no-scrollbar h-full overflow-y-auto overscroll-contain">
        <div ref={content} className={contentClassName}>{children}</div>
      </div>
      <Bar bar={bar} />
    </div>
  );
}
