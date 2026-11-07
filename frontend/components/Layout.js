import Link from 'next/link';
import { useRouter } from 'next/router';
import { useSession } from '../lib/session';
import Icon from './Icon';
import ThemeToggle from './ThemeToggle';

const navItems = [
  { href: '/', label: 'Overview', icon: 'home' },
  { href: '/campaigns/', label: 'Campaigns', icon: 'campaigns', match: ['/campaigns', '/emails'] },
  { href: '/contacts/', label: 'Contacts', icon: 'contacts' },
  { href: '/suppressions/', label: 'Suppressions', icon: 'suppressions' },
  { href: '/activity/', label: 'Activity', icon: 'activity' },
  { href: '/domains/', label: 'Domains', icon: 'domains' },
  { href: '/keys/', label: 'API Keys', icon: 'keys' },
  { href: '/settings/', label: 'Settings', icon: 'settings' },
];

function isActive(pathname, item) {
  if (item.href === '/') return pathname === '/';
  return (item.match || [item.href.replace(/\/$/, '')]).some((m) => pathname.startsWith(m));
}

const workspacePauseReasons = {
  bounce_rate_exceeded: 'too many of your emails hard bounced',
  complaint_rate_exceeded: 'too many recipients marked your emails as spam',
};

function Usage({ me, exhausted, showUpgrade }) {
  const { trial_used: used, trial_limit: limit } = me.quota;
  const pct = limit ? Math.min(100, (used / limit) * 100) : 100;
  const tone = pct >= 100 ? ' full' : pct >= 80 ? ' warn' : '';
  return (
    <div className="usage">
      <div className="usage-label"><span>Trial usage</span><span>{used} / {limit}</span></div>
      <div className="meter"><div className={`meter-fill${tone}`} style={{ width: `${pct}%` }} /></div>
      <button className={`btn btn-sm btn-block ${exhausted ? 'btn-primary' : ''}`} type="button" onClick={() => showUpgrade()}>Upgrade</button>
    </div>
  );
}

export default function Layout({ children }) {
  const { me, trial, exhausted, logout, showUpgrade } = useSession();
  const router = useRouter();
  const initials = me.user.name.split(/\s+/).map((p) => p[0]).join('').slice(0, 2).toUpperCase();
  return (
    <div className="shell">
      <aside className="sidebar">
        <div className="sidebar-top">
          <Link href="/" className="brand"><span className="brand-mark">F</span><span>Forke Mail</span></Link>
          <div className="workspace-pill">
            <span className="workspace-name">{me.workspace.name}</span>
            <span className={`badge ${trial ? 'badge-amber' : 'badge-accent'}`}>{trial ? 'Trial' : 'Paid'}</span>
          </div>
        </div>
        <nav className="nav">
          {navItems.map((item) => (
            <Link key={item.href} href={item.href} className={isActive(router.pathname, item) ? 'active' : undefined}>
              <Icon name={item.icon} />{item.label}
            </Link>
          ))}
        </nav>
        {trial ? <Usage me={me} exhausted={exhausted} showUpgrade={showUpgrade} /> : null}
        <div className="sidebar-foot">
          <div className="user">
            <span className="avatar">{initials}</span>
            <div className="user-text">
              <span className="user-name">{me.user.name}</span>
              <span className="user-email">{me.user.email}</span>
            </div>
          </div>
          <div className="sidebar-foot-actions">
            <ThemeToggle />
            <button className="btn btn-ghost btn-sm btn-icon" type="button" title="Log out" aria-label="Log out" onClick={async () => { await logout(); router.push('/login/'); }}>
              <Icon name="logout" />
            </button>
          </div>
        </div>
      </aside>
      <main className="main">
        {me.workspace.sending_paused ? (
          <div className="banner banner-danger">
            <span>
              Sending is paused for this workspace because {workspacePauseReasons[me.workspace.pause_reason] || 'of its sending reputation'} in the last 30 days.
              Clean your contact list, then contact support to resume.
            </span>
          </div>
        ) : null}
        {exhausted ? (
          <div className="banner">
            <span>Your trial send limit is used up. Emails can't be sent until you upgrade.</span>
            <button className="btn btn-sm" type="button" onClick={() => showUpgrade()}>Upgrade</button>
          </div>
        ) : null}
        <div className="page">{children}</div>
      </main>
    </div>
  );
}
