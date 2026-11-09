import clsx from 'clsx';

export default function Logo({ size = 'md', className }: { size?: 'md' | 'lg'; className?: string }) {
  return (
    <span className={clsx('inline-flex items-center gap-2', className)}>
      <img src="/forke-mark.png" alt="" className={clsx('rounded-[7px]', size === 'lg' ? 'size-8' : 'size-6')} />
      <span className={clsx('font-semibold tracking-[-0.04em] text-ink', size === 'lg' ? 'text-[24px]' : 'text-[19px]')}>
        forke<span className="text-accent">*</span>
      </span>
      <span className={clsx('rounded-md bg-accent-soft font-medium text-accent-ink', size === 'lg' ? 'px-2 py-0.5 text-sm' : 'px-1.5 py-px text-[11px]')}>mail</span>
    </span>
  );
}
