import { DependencyList, useCallback, useEffect, useState } from 'react';
import { describeError } from './api';

export interface LoadState<T> {
  data: T | null;
  error: string;
  loading: boolean;
}

export function useLoad<T>(loader: () => Promise<T>, deps: DependencyList): [LoadState<T>, () => void] {
  const [state, setState] = useState<LoadState<T>>({ data: null, error: '', loading: true });
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

export function useDebounced<T>(value: T, ms: number): T {
  const [v, setV] = useState(value);
  useEffect(() => {
    const t = setTimeout(() => setV(value), ms);
    return () => clearTimeout(t);
  }, [value, ms]);
  return v;
}

export const never = <T,>() => new Promise<T>(() => {});
