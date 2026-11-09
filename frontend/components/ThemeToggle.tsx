import clsx from 'clsx';
import { Moon, Sun } from 'lucide-react';
import { useEffect, useState } from 'react';

export default function ThemeToggle({ className }: { className?: string }) {
  const [theme, setTheme] = useState('dark');
  useEffect(() => setTheme(document.documentElement.dataset.theme || 'dark'), []);
  const toggle = () => {
    const next = theme === 'dark' ? 'light' : 'dark';
    document.documentElement.dataset.theme = next;
    try { localStorage.setItem('forkemail.theme', next); } catch {}
    setTheme(next);
  };
  const label = theme === 'dark' ? 'Switch to light theme' : 'Switch to dark theme';
  return (
    <button type="button" onClick={toggle} aria-label={label} title={label} className={clsx('grid size-8 cursor-pointer place-items-center rounded-md text-muted transition-colors hover:bg-surface-3 hover:text-ink', className)}>
      {theme === 'dark' ? <Sun className="size-4" /> : <Moon className="size-4" />}
    </button>
  );
}
