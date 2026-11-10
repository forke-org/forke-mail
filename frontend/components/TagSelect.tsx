import clsx from 'clsx';
import { Check } from 'lucide-react';
import { number } from '../lib/format';
import type { Tag } from '../lib/types';

export default function TagSelect({ tags, selected, onChange }: { tags: Tag[]; selected: string[]; onChange: (ids: string[]) => void }) {
  if (!tags.length) return <p className="text-xs text-muted">No tags yet. Add tags when importing contacts.</p>;
  const toggle = (id: string) => onChange(selected.includes(id) ? selected.filter((x) => x !== id) : [...selected, id]);
  return (
    <div className="flex flex-wrap gap-1.5">
      {tags.map((t) => {
        const on = selected.includes(t.id);
        return (
          <button
            key={t.id}
            type="button"
            aria-pressed={on}
            onClick={() => toggle(t.id)}
            className={clsx(
              'inline-flex h-7 cursor-pointer items-center gap-1.5 rounded-[5px] px-3 text-[13px] transition-colors',
              on ? 'bg-accent-soft text-accent-ink shadow-[inset_0_0_0_1px_var(--accent-line)]' : 'bg-surface-2 text-ink-2 shadow-[inset_0_0_0_1px_var(--line-strong)] hover:text-ink',
            )}
          >
            {on ? <Check className="size-3.5" /> : null}
            {t.name}
            <span className="text-xs text-muted tabular-nums">{number(t.contacts)}</span>
          </button>
        );
      })}
    </div>
  );
}
