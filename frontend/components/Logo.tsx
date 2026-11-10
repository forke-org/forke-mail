import clsx from 'clsx';

export default function Logo({ size = 'md', className }: { size?: 'md' | 'lg'; className?: string }) {
  return (
    <span className={clsx('inline-flex items-center gap-2', className)}>
      <img src="/forke-mark.png" alt="" className={clsx('rounded-[6px]', size === 'lg' ? 'size-8' : 'size-6')} />
      <span className={clsx('flex items-baseline gap-1.5 tracking-[-0.04em]', size === 'lg' ? 'text-[24px]' : 'text-[18px]')}>
        <span className="font-semibold text-ink">forke<span className="text-accent">*</span></span>
        <span className="font-normal text-muted">mail</span>
      </span>
    </span>
  );
}
