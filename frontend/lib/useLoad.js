import { useCallback, useEffect, useState } from 'react';
import { describeError } from './api';

export function useLoad(loader, deps) {
  const [state, setState] = useState({ data: null, error: '', loading: true });
  const load = useCallback(() => {
    let cancelled = false;
    setState((s) => ({ ...s, error: '', loading: true }));
    loader().then(
      (data) => { if (!cancelled) setState({ data, error: '', loading: false }); },
      (err) => { if (!cancelled) setState({ data: null, error: describeError(err), loading: false }); },
    );
    return () => { cancelled = true; };
  }, deps);
  useEffect(() => load(), [load]);
  return [state, load];
}
