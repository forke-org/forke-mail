import clsx from 'clsx';
import { ArrowLeft, Check, ChevronDown, Copy, Inbox, Info, Loader2, TriangleAlert, XCircle, type LucideIcon } from 'lucide-react';
import Link from 'next/link';
import { useRouter } from 'next/router';
import { ButtonHTMLAttributes, ChangeEvent, Children, forwardRef, Fragment, InputHTMLAttributes, isValidElement, KeyboardEvent, ReactNode, TextareaHTMLAttributes, useCallback, useEffect, useId, useRef, useState } from 'react';
import { createPortal } from 'react-dom';
import { AsciiSkeleton } from 'ascii-reveal';
import { humanize } from '../lib/format';
import { currentItem } from './nav';

type Variant = 'primary' | 'secondary' | 'ghost' | 'danger' | 'danger-solid';
type Size = 'xs' | 'sm' | 'md';

const variants: Record<Variant, string> = {
  primary: 'bg-accent text-[#1a0a00] glow-accent hover:bg-accent-hover disabled:shadow-none',
  secondary: 'bg-[color-mix(in_srgb,var(--ink)_4%,var(--surface))] text-ink shadow-[inset_0_0_0_1px_var(--line-strong)] hover:bg-surface-3',
  ghost: 'text-ink-2 hover:bg-surface-3 hover:text-ink',
  danger: 'text-danger hover:bg-danger-soft',
  'danger-solid': 'bg-danger text-white hover:opacity-90',
};

const sizes: Record<Size, string> = {
  xs: 'h-7 gap-1.5 rounded-[5px] px-2.5 text-xs',
  sm: 'h-8 gap-1.5 rounded-[6px] px-3 text-[13px]',
  md: 'h-9 gap-2 rounded-[6px] px-3.5 text-sm',
};

export function buttonClass(variant: Variant = 'secondary', size: Size = 'md', className?: string) {
  return clsx(
    'inline-flex shrink-0 cursor-pointer items-center justify-center font-medium whitespace-nowrap transition-[background-color,color,box-shadow,opacity] duration-150 select-none',
    'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent/50 focus-visible:ring-offset-1 focus-visible:ring-offset-bg',
    'disabled:pointer-events-none disabled:opacity-50 [&_svg]:size-4 [&_svg]:shrink-0',
    variants[variant], sizes[size], className,
  );
}

type ButtonProps = ButtonHTMLAttributes<HTMLButtonElement> & { variant?: Variant; size?: Size; icon?: LucideIcon; loading?: boolean };

export const Button = forwardRef<HTMLButtonElement, ButtonProps>(function Button(
  { variant = 'secondary', size = 'md', icon: Icon, loading, className, children, type = 'button', disabled, ...rest }, ref,
) {
  return (
    <button ref={ref} type={type} className={buttonClass(variant, size, className)} disabled={disabled || loading} {...rest}>
      {loading ? <Loader2 className="animate-spin" /> : Icon ? <Icon /> : null}
      {children}
    </button>
  );
});

export function ButtonLink({ href, variant = 'secondary', size = 'md', icon: Icon, className, children, onClick }: {
  href: string; variant?: Variant; size?: Size; icon?: LucideIcon; className?: string; children: ReactNode; onClick?: () => void;
}) {
  return (
    <Link href={href} className={buttonClass(variant, size, className)} onClick={onClick}>
      {Icon ? <Icon /> : null}
      {children}
    </Link>
  );
}

export const controlClass = clsx(
  'w-full rounded-[6px] border border-line-strong bg-[color-mix(in_srgb,var(--ink)_3%,var(--surface))] px-3 text-sm text-ink placeholder:text-faint',
  'transition-[border-color,box-shadow] duration-150 outline-none hover:border-[color-mix(in_srgb,var(--ink)_22%,transparent)]',
  'focus:border-accent focus:ring-3 focus:ring-accent/15 disabled:opacity-60',
);

export const Input = forwardRef<HTMLInputElement, InputHTMLAttributes<HTMLInputElement>>(function Input({ className, ...rest }, ref) {
  return <input ref={ref} className={clsx(controlClass, 'h-9', className)} {...rest} />;
});

export const Textarea = forwardRef<HTMLTextAreaElement, TextareaHTMLAttributes<HTMLTextAreaElement> & { code?: boolean }>(function Textarea({ className, code, ...rest }, ref) {
  return <textarea ref={ref} className={clsx(controlClass, 'py-2 leading-relaxed', code && 'font-mono text-[12.5px]', className)} {...rest} />;
});

type Option = { value: string; label: ReactNode; text: string; disabled?: boolean };

function readOptions(children: ReactNode): Option[] {
  const list: Option[] = [];
  Children.forEach(children, (child) => {
    if (!isValidElement(child)) return;
    const props = child.props as { value?: string | number; children?: ReactNode; disabled?: boolean };
    if (child.type === Fragment) { list.push(...readOptions(props.children)); return; }
    const text = Children.toArray(props.children).join('');
    list.push({ value: String(props.value ?? text), label: props.children, text, disabled: props.disabled });
  });
  return list;
}

type SelectProps = {
  value?: string | number;
  onChange?: (e: ChangeEvent<HTMLSelectElement>) => void;
  children?: ReactNode;
  className?: string;
  disabled?: boolean;
  'aria-label'?: string;
};

export function Select({ value, onChange, children, className, disabled, 'aria-label': ariaLabel }: SelectProps) {
  const options = readOptions(children);
  const current = options.find((o) => o.value === String(value ?? '')) || options[0];
  const [open, setOpen] = useState(false);
  const [active, setActive] = useState(0);
  const [place, setPlace] = useState<{ left: number; width: number; top?: number; bottom?: number; maxHeight: number } | null>(null);
  const trigger = useRef<HTMLButtonElement>(null);
  const list = useRef<HTMLUListElement>(null);
  const id = useId();

  const measure = useCallback(() => {
    const r = trigger.current?.getBoundingClientRect();
    if (!r) return;
    const below = window.innerHeight - r.bottom - 12;
    const above = r.top - 12;
    const up = below < 220 && above > below;
    setPlace({
      left: Math.min(r.left, window.innerWidth - Math.max(r.width, 200) - 8),
      width: Math.max(r.width, 200),
      ...(up ? { bottom: window.innerHeight - r.top + 4 } : { top: r.bottom + 4 }),
      maxHeight: Math.min(288, up ? above : below),
    });
  }, []);

  const show = () => {
    if (disabled) return;
    measure();
    setActive(Math.max(0, options.indexOf(current)));
    setOpen(true);
  };
  const choose = (o: Option) => {
    if (o.disabled) return;
    setOpen(false);
    trigger.current?.focus();
    if (o.value !== String(value ?? '')) onChange?.({ target: { value: o.value }, currentTarget: { value: o.value } } as ChangeEvent<HTMLSelectElement>);
  };

  useEffect(() => {
    if (!open) return undefined;
    const away = (e: MouseEvent) => {
      if (!trigger.current?.contains(e.target as Node) && !list.current?.contains(e.target as Node)) setOpen(false);
    };
    const close = () => setOpen(false);
    document.addEventListener('mousedown', away);
    window.addEventListener('resize', close);
    window.addEventListener('scroll', measure, true);
    return () => {
      document.removeEventListener('mousedown', away);
      window.removeEventListener('resize', close);
      window.removeEventListener('scroll', measure, true);
    };
  }, [open, measure]);

  useEffect(() => {
    if (open) list.current?.querySelector<HTMLElement>(`[data-index="${active}"]`)?.scrollIntoView({ block: 'nearest' });
  }, [open, active]);

  const onKey = (e: KeyboardEvent<HTMLButtonElement>) => {
    const step = (dir: number) => {
      let i = active;
      for (let n = 0; n < options.length; n += 1) {
        i = (i + dir + options.length) % options.length;
        if (!options[i].disabled) break;
      }
      setActive(i);
    };
    if (!open && ['ArrowDown', 'ArrowUp', 'Enter', ' '].includes(e.key)) { e.preventDefault(); show(); return; }
    if (!open) return;
    if (e.key === 'ArrowDown') { e.preventDefault(); step(1); }
    else if (e.key === 'ArrowUp') { e.preventDefault(); step(-1); }
    else if (e.key === 'Home') { e.preventDefault(); setActive(0); }
    else if (e.key === 'End') { e.preventDefault(); setActive(options.length - 1); }
    else if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); if (options[active]) choose(options[active]); }
    else if (e.key === 'Escape' || e.key === 'Tab') { setOpen(false); }
    else if (e.key.length === 1) {
      const i = options.findIndex((o) => o.text.toLowerCase().startsWith(e.key.toLowerCase()));
      if (i >= 0) setActive(i);
    }
  };

  return (
    <div className={clsx('relative', className)}>
      <button
        ref={trigger}
        type="button"
        role="combobox"
        aria-label={ariaLabel}
        aria-haspopup="listbox"
        aria-expanded={open}
        aria-controls={open ? id : undefined}
        aria-activedescendant={open ? `${id}-${active}` : undefined}
        disabled={disabled}
        onClick={() => (open ? setOpen(false) : show())}
        onKeyDown={onKey}
        className={clsx(controlClass, 'flex h-9 cursor-pointer items-center gap-2 pr-2.5 text-left', open && 'border-accent ring-3 ring-accent/15')}
      >
        <span className="min-w-0 flex-1 truncate">{current?.label}</span>
        <ChevronDown className={clsx('size-4 shrink-0 text-muted transition-transform duration-150', open && 'rotate-180')} />
      </button>
      {open && place ? createPortal(
        <ul
          ref={list}
          id={id}
          role="listbox"
          aria-label={ariaLabel}
          style={{ left: place.left, width: place.width, top: place.top, bottom: place.bottom, maxHeight: place.maxHeight }}
          className="no-scrollbar fixed z-[80] animate-pop overflow-y-auto rounded-[8px] bg-surface p-1 shadow-pop"
        >
          {options.map((o, i) => {
            const selected = o.value === current?.value;
            return (
              <li
                key={o.value}
                id={`${id}-${i}`}
                data-index={i}
                role="option"
                aria-selected={selected}
                aria-disabled={o.disabled}
                onMouseEnter={() => setActive(i)}
                onMouseDown={(e) => e.preventDefault()}
                onClick={() => choose(o)}
                className={clsx(
                  'flex h-8 cursor-pointer items-center gap-2 rounded-[5px] px-2.5 text-[13px] select-none',
                  i === active ? 'bg-surface-3 text-ink' : 'text-ink-2',
                  o.disabled && 'cursor-not-allowed opacity-50',
                )}
              >
                <span className="min-w-0 flex-1 truncate">{o.label}</span>
                {selected ? <Check className="size-3.5 shrink-0 text-accent" /> : null}
              </li>
            );
          })}
        </ul>,
        document.body,
      ) : null}
    </div>
  );
}

export function Checkbox({ label, className, ...rest }: InputHTMLAttributes<HTMLInputElement> & { label?: ReactNode }) {
  const box = <input type="checkbox" className={clsx('size-4 cursor-pointer rounded accent-accent', className)} {...rest} />;
  if (!label) return box;
  return <label className="inline-flex cursor-pointer items-center gap-2 text-sm text-ink-2">{box}{label}</label>;
}

export function Field({ label, hint, error, children, className }: { label: ReactNode; hint?: ReactNode; error?: string; children: ReactNode; className?: string }) {
  return (
    <label className={clsx('flex flex-col gap-1.5', className)}>
      <span className="text-[13px] font-medium text-ink-2">{label}</span>
      {children}
      {hint ? <span className="text-xs leading-relaxed text-muted">{hint}</span> : null}
      {error ? <span className="text-xs text-danger">{error}</span> : null}
    </label>
  );
}

export function FormError({ children }: { children?: ReactNode }) {
  if (!children) return null;
  return (
    <div className="flex items-start gap-2 rounded-lg bg-danger-soft px-3 py-2.5 text-[13px] text-danger">
      <XCircle className="mt-px size-4 shrink-0" />
      <span>{children}</span>
    </div>
  );
}

type Tone = 'green' | 'blue' | 'amber' | 'red' | 'accent' | 'neutral';

const toneClass: Record<Tone, string> = {
  green: 'bg-success-soft text-success',
  blue: 'bg-info-soft text-info',
  amber: 'bg-warning-soft text-warning',
  red: 'bg-danger-soft text-danger',
  accent: 'bg-accent-soft text-accent-ink',
  neutral: 'bg-surface-3 text-muted',
};

const badgeTones: Record<string, Tone> = {
  active: 'green', sent: 'green', delivered: 'green', done: 'green', verified: 'green', paid: 'accent', production: 'green',
  sending: 'blue', queued: 'blue', scheduled: 'blue', sending_access: 'blue', draft: 'neutral',
  trial: 'amber', pending: 'amber', delayed: 'amber', soft_bounced: 'amber', paused: 'amber', skipped: 'amber', unsubscribed: 'amber', sandbox: 'amber',
  bounced: 'red', complained: 'red', failed: 'red', invalid: 'red', revoked: 'red', rejected: 'red', sending_paused: 'red',
  full_access: 'accent', opened: 'accent', clicked: 'accent', super_admin: 'accent', admin: 'blue',
};

const badgeLabels: Record<string, string> = {
  done: 'sent', full_access: 'full access', sending_access: 'sending access', soft_bounced: 'soft bounce', super_admin: 'owner',
};

export function Badge({ value, tone, dot = true, children }: { value?: string; tone?: Tone; dot?: boolean; children?: ReactNode }) {
  const t = tone || (value ? badgeTones[value] : undefined) || 'neutral';
  return (
    <span className={clsx('inline-flex h-[22px] items-center gap-1.5 rounded-[4px] px-1.5 text-xs font-medium whitespace-nowrap capitalize', toneClass[t])}>
      {dot ? <span className="size-1.5 rounded-[1.5px] bg-current" /> : null}
      {children ?? (value ? badgeLabels[value] || humanize(value) : null)}
    </span>
  );
}

export function Chip({ children }: { children: ReactNode }) {
  return <span className="inline-flex h-[22px] items-center rounded-[4px] bg-surface-3 px-1.5 font-mono text-[11.5px] text-ink-2">{children}</span>;
}

export function Card({ title, description, actions, children, flush, className, icon: Icon }: {
  title?: ReactNode; description?: ReactNode; actions?: ReactNode; children?: ReactNode; flush?: boolean; className?: string; icon?: LucideIcon;
}) {
  return (
    <section className={clsx('overflow-hidden rounded-[8px] bg-surface shadow-card', className)}>
      {title ? (
        <div className="flex flex-wrap items-center justify-between gap-3 border-b border-line bg-surface-2 px-5 py-3">
          <div className="flex min-w-0 items-center gap-2.5">
            {Icon ? <span className="grid size-7 place-items-center rounded-[5px] bg-surface-3 text-ink-2"><Icon className="size-4" /></span> : null}
            <div className="min-w-0">
              <h2 className="text-sm font-semibold text-ink">{title}</h2>
              {description ? <p className="mt-0.5 text-xs text-muted">{description}</p> : null}
            </div>
          </div>
          {actions ? <div className="flex items-center gap-2">{actions}</div> : null}
        </div>
      ) : null}
      {flush ? children : <div className="p-5">{children}</div>}
    </section>
  );
}

export function PageHeader({ title, description, back, actions, meta }: {
  title: ReactNode; description?: ReactNode; back?: { href: string; label: string }; actions?: ReactNode; meta?: ReactNode;
}) {
  const router = useRouter();
  const section = currentItem(router.pathname);
  return (
    <header className="mb-7 flex animate-fade-up flex-col gap-4 md:flex-row md:items-end md:justify-between">
      <div className="min-w-0">
        {back ? (
          <Link href={back.href} className="mb-3 inline-flex max-w-full items-center gap-1.5 text-[13px] text-muted transition-colors hover:text-ink">
            <ArrowLeft className="size-3.5 shrink-0" /><span className="truncate">{back.label}</span>
          </Link>
        ) : section?.group ? (
          <div className="mb-2.5 flex items-center gap-2 font-mono text-[11px] tracking-[0.08em] text-muted uppercase">
            <span className="size-1.5 rounded-[1.5px] bg-accent" />{section.group}
          </div>
        ) : null}
        <div className="flex flex-wrap items-center gap-x-3 gap-y-2">
          <h1 className="min-w-0 text-[24px] leading-tight font-medium tracking-[-0.03em] break-words text-ink sm:text-[28px]">{title}</h1>
          {meta}
        </div>
        {description ? <p className="mt-2 max-w-2xl text-sm leading-relaxed text-muted">{description}</p> : null}
      </div>
      {actions ? <div className="flex flex-wrap items-center gap-2 [&>*]:max-sm:flex-1">{actions}</div> : null}
    </header>
  );
}

export function Stat({ label, value, hint, icon: Icon, tone = 'neutral', children }: {
  label: ReactNode; value: ReactNode; hint?: ReactNode; icon?: LucideIcon; tone?: Tone; children?: ReactNode;
}) {
  return (
    <div className="flex flex-col gap-1 bg-surface p-5">
      <div className="flex items-center justify-between gap-2">
        <span className="text-[13px] text-muted">{label}</span>
        {Icon ? <span className={clsx('grid size-7 place-items-center rounded-[5px]', toneClass[tone])}><Icon className="size-3.5" /></span> : null}
      </div>
      <span className="mt-1 text-[28px] font-medium tracking-[-0.03em] text-ink tabular-nums">{value}</span>
      {hint ? <span className="text-xs text-muted">{hint}</span> : null}
      {children}
    </div>
  );
}

export function Loading({ label = 'Loading…', className }: { label?: string; className?: string }) {
  return (
    <div role="status" className={clsx('w-full px-5 py-8', className)}>
      <span className="sr-only">{label}</span>
      <AsciiSkeleton lines={4} className="block text-faint" />
    </div>
  );
}

export function Skeleton({ className, seed }: { className?: string; seed?: number }) {
  return <AsciiSkeleton lines={1} widths={['100%']} seed={seed} className={clsx('block overflow-hidden text-faint', className)} />;
}

export function Empty({ title, hint, action, icon: Icon = Inbox }: { title: ReactNode; hint?: ReactNode; action?: ReactNode; icon?: LucideIcon }) {
  return (
    <div className="flex flex-col items-center px-6 py-14 text-center">
      <div className="dot-grid mb-4 grid size-14 place-items-center rounded-[8px] border border-dashed border-line-strong">
        <span className="grid size-9 place-items-center rounded-[6px] bg-surface shadow-card"><Icon className="size-4.5 text-accent" /></span>
      </div>
      <div className="text-sm font-semibold text-ink">{title}</div>
      {hint ? <p className="mt-1 max-w-sm text-[13px] leading-relaxed text-muted">{hint}</p> : null}
      {action ? <div className="mt-4">{action}</div> : null}
    </div>
  );
}

export function Meter({ value, max, className, tone }: { value: number; max: number; className?: string; tone?: 'auto' | 'accent' }) {
  const pct = max > 0 ? Math.min(100, (value / max) * 100) : 0;
  const color = tone === 'accent' ? 'bg-accent' : pct >= 100 ? 'bg-danger' : pct >= 80 ? 'bg-warning' : 'bg-accent';
  return (
    <div className={clsx('h-1.5 overflow-hidden rounded-[2px] bg-surface-3', className)}>
      <div className={clsx('h-full rounded-[2px] transition-[width] duration-500', color)} style={{ width: `${pct}%` }} />
    </div>
  );
}

export function Progress({ percent }: { percent: number }) {
  const value = Math.max(0, Math.min(100, Number(percent) || 0));
  return (
    <div className="flex min-w-28 items-center gap-2.5">
      <Meter value={value} max={100} tone="accent" className="flex-1" />
      <span className="w-9 text-right text-xs text-muted tabular-nums">{Math.round(value)}%</span>
    </div>
  );
}

export function Tabs<T extends string>({ value, options, onChange, className }: { value: T; options: [T, ReactNode][]; onChange: (v: T) => void; className?: string }) {
  return (
    <div role="tablist" className={clsx('inline-flex rounded-[6px] bg-surface-2 p-0.5 shadow-[inset_0_0_0_1px_var(--line)]', className)}>
      {options.map(([v, label]) => (
        <button
          key={v}
          type="button"
          role="tab"
          aria-selected={v === value}
          onClick={() => onChange(v)}
          className={clsx(
            'h-7 cursor-pointer rounded-[4px] px-3 text-[13px] font-medium transition-all',
            v === value ? 'bg-surface text-ink shadow-[0_1px_2px_rgba(0,0,0,0.08),0_0_0_1px_var(--line-strong)]' : 'text-muted hover:text-ink',
          )}
        >
          {label}
        </button>
      ))}
    </div>
  );
}

const calloutTones = {
  info: { cls: 'bg-info-soft text-info', icon: Info },
  warn: { cls: 'bg-warning-soft text-warning', icon: TriangleAlert },
  danger: { cls: 'bg-danger-soft text-danger', icon: XCircle },
  accent: { cls: 'bg-accent-soft text-accent-ink', icon: Info },
};

export function Callout({ tone = 'info', title, children, action }: { tone?: keyof typeof calloutTones; title?: ReactNode; children?: ReactNode; action?: ReactNode }) {
  const { cls, icon: Icon } = calloutTones[tone];
  return (
    <div className={clsx('flex flex-col gap-3 rounded-[6px] px-4 py-3 text-[13px] shadow-[inset_0_0_0_1px_color-mix(in_srgb,currentColor_18%,transparent)] sm:flex-row sm:items-center', cls)}>
      <div className="flex flex-1 items-start gap-2.5">
        <Icon className="mt-px size-4 shrink-0" />
        <div className="leading-relaxed">
          {title ? <div className="font-semibold">{title}</div> : null}
          {children ? <div className={title ? 'text-ink-2' : ''}>{children}</div> : null}
        </div>
      </div>
      {action}
    </div>
  );
}

export function CopyButton({ value, label }: { value: string; label?: string }) {
  const [copied, setCopied] = useState(false);
  const copy = async (e: React.MouseEvent) => {
    e.stopPropagation();
    try {
      await navigator.clipboard.writeText(value);
      setCopied(true);
      setTimeout(() => setCopied(false), 1500);
    } catch {
      setCopied(false);
    }
  };
  return (
    <button type="button" onClick={copy} aria-label={label || 'Copy'} title={copied ? 'Copied' : 'Copy'} className="inline-grid size-7 shrink-0 cursor-pointer place-items-center rounded-[5px] text-muted transition-colors hover:bg-surface-3 hover:text-ink">
      {copied ? <Check className="size-3.5 text-success" /> : <Copy className="size-3.5" />}
    </button>
  );
}

export function EmailFrame({ html, className }: { html: string; className?: string }) {
  return <iframe className={clsx('block h-[560px] w-full border-0 bg-white', className)} sandbox="" referrerPolicy="no-referrer" title="Email preview" srcDoc={html} />;
}

export function Kbd({ children }: { children: ReactNode }) {
  return <kbd className="inline-flex h-5 min-w-5 items-center justify-center rounded border border-line-strong bg-surface-2 px-1 font-sans text-[11px] font-medium text-muted">{children}</kbd>;
}

export function DefinitionRow({ label, children }: { label: ReactNode; children: ReactNode }) {
  return (
    <div className="flex flex-col gap-1 border-b border-line py-3 last:border-0 sm:flex-row sm:items-center sm:justify-between">
      <span className="text-[13px] text-muted">{label}</span>
      <span className="text-sm text-ink">{children}</span>
    </div>
  );
}

export function SearchInput({ value, onChange, placeholder, className }: { value: string; onChange: (v: string) => void; placeholder: string; className?: string }) {
  return (
    <div className={clsx('relative', className)}>
      <svg className="pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2 text-muted" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" aria-hidden="true"><circle cx="11" cy="11" r="7" /><path d="m20 20-3.5-3.5" /></svg>
      <input type="search" value={value} onChange={(e) => onChange(e.target.value)} placeholder={placeholder} className={clsx(controlClass, 'h-9 pl-9')} />
    </div>
  );
}

export function Toolbar({ children }: { children: ReactNode }) {
  return <div className="flex flex-col gap-2 border-b border-line px-4 py-3 sm:flex-row sm:items-center">{children}</div>;
}
