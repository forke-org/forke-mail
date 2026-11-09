import {
  Activity, Ban, Globe, Hash, KeyRound, Webhook, LayoutDashboard, Megaphone, Settings, Users, Zap, type LucideIcon,
} from 'lucide-react';

export interface NavItem {
  href: string;
  label: string;
  icon: LucideIcon;
  match?: string[];
  superAdmin?: boolean;
  keywords?: string;
}

export const navGroups: { label: string | null; items: NavItem[] }[] = [
  { label: null, items: [{ href: '/', label: 'Overview', icon: LayoutDashboard, keywords: 'home dashboard' }] },
  {
    label: 'Sending',
    items: [
      { href: '/campaigns/', label: 'Broadcasts', icon: Megaphone, match: ['/campaigns', '/emails'], keywords: 'campaigns emails newsletter' },
      { href: '/transactional/', label: 'Transactional', icon: Zap, keywords: 'api messages otp receipts' },
      { href: '/activity/', label: 'Activity', icon: Activity, keywords: 'events logs delivered bounced' },
    ],
  },
  {
    label: 'Audience',
    items: [
      { href: '/contacts/', label: 'Contacts', icon: Users, keywords: 'subscribers people import' },
      { href: '/topics/', label: 'Topics', icon: Hash, keywords: 'preferences preference centre subscriptions' },
      { href: '/suppressions/', label: 'Suppressions', icon: Ban, keywords: 'blocklist unsubscribed bounces' },
    ],
  },
  {
    label: 'Configuration',
    items: [
      { href: '/domains/', label: 'Domains', icon: Globe, keywords: 'dns dkim spf identities' },
      { href: '/webhooks/', label: 'Webhooks', icon: Webhook, keywords: 'events sync callbacks forke admin' },
      { href: '/keys/', label: 'API keys', icon: KeyRound, superAdmin: true, keywords: 'tokens api' },
      { href: '/settings/', label: 'Settings', icon: Settings, keywords: 'plan usage re-engagement account' },
    ],
  },
];

export function isActive(pathname: string, item: NavItem) {
  if (item.href === '/') return pathname === '/';
  return (item.match || [item.href.replace(/\/$/, '')]).some((m) => pathname.startsWith(m));
}

export function currentItem(pathname: string) {
  for (const group of navGroups) {
    for (const item of group.items) if (isActive(pathname, item)) return { group: group.label, item };
  }
  return null;
}
