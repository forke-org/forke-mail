import { number } from '../lib/format';

export default function TagSelect({ tags, selected, onChange }) {
  if (!tags.length) return <p className="small muted">No tags yet. Add tags when importing contacts.</p>;
  const toggle = (id) => onChange(selected.includes(id) ? selected.filter((x) => x !== id) : [...selected, id]);
  return (
    <div className="chips">
      {tags.map((t) => (
        <button key={t.id} type="button" className={`chip chip-toggle${selected.includes(t.id) ? ' on' : ''}`} aria-pressed={selected.includes(t.id)} onClick={() => toggle(t.id)}>
          {t.name}<span className="chip-count">{number(t.contacts)}</span>
        </button>
      ))}
    </div>
  );
}
