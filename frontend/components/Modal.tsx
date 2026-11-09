import clsx from 'clsx';
import { X } from 'lucide-react';
import { createContext, ReactNode, useCallback, useContext, useEffect, useState } from 'react';
import { Button } from './ui';

export function Modal({ title, description, onClose, footer, wide, children }: {
  title: ReactNode; description?: ReactNode; onClose: () => void; footer?: ReactNode; wide?: boolean; children: ReactNode;
}) {
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => { if (e.key === 'Escape') onClose(); };
    document.addEventListener('keydown', onKey);
    const prev = document.body.style.overflow;
    document.body.style.overflow = 'hidden';
    return () => {
      document.removeEventListener('keydown', onKey);
      document.body.style.overflow = prev;
    };
  }, [onClose]);
  return (
    <div
      className="fixed inset-0 z-[60] flex items-end justify-center bg-black/50 p-0 backdrop-blur-[2px] sm:items-start sm:p-4 sm:pt-[10vh]"
      onMouseDown={(e) => { if (e.target === e.currentTarget) onClose(); }}
    >
      <div role="dialog" aria-modal="true" className={clsx('flex max-h-[90vh] w-full animate-pop flex-col overflow-hidden rounded-t-2xl bg-surface shadow-pop sm:rounded-2xl', wide ? 'sm:max-w-3xl' : 'sm:max-w-lg')}>
        <div className="flex items-start justify-between gap-4 border-b border-line px-5 py-4">
          <div>
            <h2 className="text-base font-semibold text-ink">{title}</h2>
            {description ? <p className="mt-1 text-[13px] leading-relaxed text-muted">{description}</p> : null}
          </div>
          <button type="button" onClick={onClose} aria-label="Close" className="-mt-1 -mr-1.5 grid size-8 shrink-0 cursor-pointer place-items-center rounded-md text-muted hover:bg-surface-3 hover:text-ink"><X className="size-4" /></button>
        </div>
        <div className="flex flex-col gap-4 overflow-y-auto px-5 py-5">{children}</div>
        {footer ? <div className="flex items-center justify-end gap-2 border-t border-line bg-surface-2 px-5 py-3">{footer}</div> : null}
      </div>
    </div>
  );
}

type Confirm = (title: string, message: ReactNode, label?: string, danger?: boolean) => Promise<boolean>;

const ConfirmContext = createContext<Confirm>(async () => false);

export function ConfirmProvider({ children }: { children: ReactNode }) {
  const [pending, setPending] = useState<{ title: string; message: ReactNode; label: string; danger: boolean; resolve: (v: boolean) => void } | null>(null);
  const confirm = useCallback<Confirm>((title, message, label = 'Confirm', danger = false) => new Promise((resolve) => {
    setPending({ title, message, label, danger, resolve });
  }), []);
  const finish = (result: boolean) => {
    pending?.resolve(result);
    setPending(null);
  };
  return (
    <ConfirmContext.Provider value={confirm}>
      {children}
      {pending ? (
        <Modal
          title={pending.title}
          onClose={() => finish(false)}
          footer={(
            <>
              <Button onClick={() => finish(false)}>Cancel</Button>
              <Button variant={pending.danger ? 'danger-solid' : 'primary'} autoFocus onClick={() => finish(true)}>{pending.label}</Button>
            </>
          )}
        >
          <p className="text-sm leading-relaxed text-ink-2">{pending.message}</p>
        </Modal>
      ) : null}
    </ConfirmContext.Provider>
  );
}

export function useConfirm() {
  return useContext(ConfirmContext);
}

export function FormActions({ children }: { children: ReactNode }) {
  return <div className="-mx-5 -mb-5 mt-1 flex items-center justify-end gap-2 border-t border-line bg-surface-2 px-5 py-3">{children}</div>;
}
