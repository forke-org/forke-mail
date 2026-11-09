import { CheckCircle2, Info, TriangleAlert, XCircle } from 'lucide-react';
import { createContext, ReactNode, useCallback, useContext, useState } from 'react';
import clsx from 'clsx';

export type ToastKind = 'info' | 'success' | 'warn' | 'error';
type Push = (message: string, kind?: ToastKind, ms?: number) => void;

const ToastContext = createContext<Push>(() => {});

const icons = { info: Info, success: CheckCircle2, warn: TriangleAlert, error: XCircle };
const tones = { info: 'text-info', success: 'text-success', warn: 'text-warning', error: 'text-danger' };

export function ToastProvider({ children }: { children: ReactNode }) {
  const [toasts, setToasts] = useState<{ id: string; message: string; kind: ToastKind }[]>([]);
  const push = useCallback<Push>((message, kind = 'info', ms = 4000) => {
    const id = Math.random().toString(36).slice(2);
    setToasts((list) => [...list, { id, message, kind }]);
    setTimeout(() => setToasts((list) => list.filter((t) => t.id !== id)), ms);
  }, []);
  return (
    <ToastContext.Provider value={push}>
      {children}
      <div className="pointer-events-none fixed right-4 bottom-4 left-4 z-[70] flex flex-col items-end gap-2 sm:left-auto" aria-live="polite">
        {toasts.map((t) => {
          const Icon = icons[t.kind];
          return (
            <div key={t.id} role="status" className="pointer-events-auto flex w-full max-w-sm animate-pop items-start gap-2.5 rounded-lg bg-surface px-3.5 py-3 text-sm text-ink shadow-pop">
              <Icon className={clsx('mt-px size-4 shrink-0', tones[t.kind])} />
              <span className="leading-snug">{t.message}</span>
            </div>
          );
        })}
      </div>
    </ToastContext.Provider>
  );
}

export function useToast() {
  return useContext(ToastContext);
}
