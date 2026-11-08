import { createContext, useCallback, useContext, useEffect, useState } from 'react';

const ConfirmContext = createContext(null);

export function Modal({ title, description, onClose, footer, wide, children }) {
  useEffect(() => {
    const onKey = (e) => { if (e.key === 'Escape') onClose(); };
    document.addEventListener('keydown', onKey);
    return () => document.removeEventListener('keydown', onKey);
  }, [onClose]);
  return (
    <div className="modal-overlay" onMouseDown={(e) => { if (e.target === e.currentTarget) onClose(); }}>
      <div className={`modal${wide ? ' modal-wide' : ''}`} role="dialog" aria-modal="true">
        <div className="modal-head">
          <div>
            <h2>{title}</h2>
            {description ? <p className="muted small">{description}</p> : null}
          </div>
          <button className="btn btn-ghost btn-icon" type="button" onClick={onClose} aria-label="Close">✕</button>
        </div>
        <div className="modal-body">{children}</div>
        {footer ? <div className="modal-foot">{footer}</div> : null}
      </div>
    </div>
  );
}

export function ConfirmProvider({ children }) {
  const [pending, setPending] = useState(null);
  const confirm = useCallback((title, message, label = 'Confirm', danger = false) => new Promise((resolve) => {
    setPending({ title, message, label, danger, resolve });
  }), []);
  const finish = (result) => {
    pending.resolve(result);
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
              <button className="btn" type="button" onClick={() => finish(false)}>Cancel</button>
              <button className={`btn ${pending.danger ? 'btn-danger-solid' : 'btn-primary'}`} type="button" autoFocus onClick={() => finish(true)}>{pending.label}</button>
            </>
          )}
        >
          <p className="muted">{pending.message}</p>
        </Modal>
      ) : null}
    </ConfirmContext.Provider>
  );
}

export function useConfirm() {
  return useContext(ConfirmContext);
}
