import clsx from 'clsx';
import { FileText, UploadCloud } from 'lucide-react';
import { useState } from 'react';

export default function FileDrop({ file, onFile, hint }: { file: File | null; onFile: (f: File | null) => void; hint: string }) {
  const [over, setOver] = useState(false);
  return (
    <label
      onDragOver={(e) => { e.preventDefault(); setOver(true); }}
      onDragLeave={() => setOver(false)}
      onDrop={(e) => { e.preventDefault(); setOver(false); onFile(e.dataTransfer.files[0] || null); }}
      className={clsx(
        'flex cursor-pointer flex-col items-center gap-2 rounded-xl border border-dashed px-4 py-8 text-center transition-colors',
        over ? 'border-accent bg-accent-soft' : 'border-line-strong hover:border-accent-line hover:bg-surface-2',
      )}
    >
      <input type="file" className="sr-only" accept=".csv,text/csv,text/plain" onChange={(e) => onFile(e.target.files?.[0] || null)} />
      <span className="grid size-10 place-items-center rounded-xl bg-accent-soft text-accent">{file ? <FileText className="size-5" /> : <UploadCloud className="size-5" />}</span>
      <span className="text-sm font-medium text-ink">{file ? file.name : 'Drop a CSV here or click to browse'}</span>
      <span className="text-xs text-muted">{hint}</span>
    </label>
  );
}
