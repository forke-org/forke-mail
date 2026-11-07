import ThemeToggle from './ThemeToggle';

export default function AuthCard({ title, subtitle, footer, children }) {
  return (
    <div className="auth-screen">
      <ThemeToggle className="auth-theme" />
      <div className="auth-card">
        <div className="brand brand-lg"><span className="brand-mark">F</span><span>Forke Mail</span></div>
        <h1>{title}</h1>
        {subtitle ? <p className="muted">{subtitle}</p> : null}
        {children}
      </div>
      {footer ? <p className="auth-footer muted">{footer}</p> : null}
    </div>
  );
}
