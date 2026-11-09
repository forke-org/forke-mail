export type Role = 'super_admin' | 'admin';

export interface Me {
  user: { name: string; email: string; role: Role } | null;
  key: { scope: string; prefix: string } | null;
  workspace: {
    id: string;
    name: string;
    plan: 'trial' | 'paid';
    status: string;
    sending_paused: boolean;
    pause_reason: string | null;
  };
  domains: string[];
  quota: {
    trial_used: number;
    trial_limit: number;
    daily_limit: number;
    sent_today: number;
    sent_this_month: number;
    monthly_limit: number;
  };
  rate_limits: { api_per_second: number; api_burst: number; send_per_second: number };
}

export interface Audience {
  all: boolean;
  tag_ids: string[];
  contact_ids: string[];
  size?: number;
}

export interface Email {
  id: string;
  campaign_id: string;
  campaign_name: string;
  subject: string;
  from_name: string;
  from_address: string;
  reply_to: string;
  topic: string | null;
  html: string;
  text: string;
  status: string;
  pause_reason: string | null;
  started_at: string | null;
  finished_at: string | null;
  created_at: string;
  recipients: number;
  processed: number;
  audience: Audience;
}

export interface Campaign {
  id: string;
  name: string;
  description: string;
  emails: number;
  sent: number;
  last_email_at: string | null;
  created_at: string;
}

export interface Contact {
  id: string;
  email: string;
  first_name: string;
  last_name: string;
  status: string;
  tags: string[];
  created_at: string;
}

export interface Tag {
  id: string;
  name: string;
  contacts: number;
}

export interface Topic {
  key: string;
  label: string;
}

export interface Report {
  id: string;
  status: string;
  pause_reason: string | null;
  total: number;
  progress: number;
  counts: Record<'pending' | 'sent' | 'delivered' | 'bounced' | 'complained' | 'unsubscribed' | 'failed' | 'skipped' | 'opened' | 'clicked', number>;
}

export interface Recipient {
  id: string;
  email: string;
  status: string;
  attempts: number;
  sent_at: string | null;
  delivered_at: string | null;
  bounced_at: string | null;
  complained_at: string | null;
  unsubscribed_at: string | null;
  opened_at?: string | null;
  clicked_at?: string | null;
  last_error: string | null;
}

export interface Suppression {
  id: string;
  email: string;
  reason: string;
  source: string;
  created_at: string;
}

export interface EventRow {
  id: number;
  email_id: string | null;
  message_id: string | null;
  subject: string | null;
  campaign_name: string | null;
  recipient: string | null;
  type: string;
  detail: Record<string, unknown> | null;
  created_at: string;
}

export interface DnsRecord {
  type: string;
  name: string;
  value: string;
}

export interface Domain {
  id: string;
  name: string;
  kind: 'domain' | 'email';
  status: string;
  ses_status: string | null;
  last_checked_at: string | null;
  created_at: string;
  records: DnsRecord[];
}

export interface ApiKey {
  id: string;
  name: string;
  prefix: string;
  scope: string;
  last_used_at: string | null;
  revoked_at: string | null;
  created_at: string;
}

export interface TransactionalMessage {
  id: string;
  to: string;
  subject: string;
  from_name: string;
  from_address: string;
  status: string;
  attempts: number;
  dispatch_ms: number | null;
  last_error: string | null;
  track_opens: boolean;
  track_clicks: boolean;
  open_count: number;
  click_count: number;
  delivered_at: string | null;
  bounced_at: string | null;
  complained_at: string | null;
  opened_at: string | null;
  clicked_at: string | null;
  created_at: string;
  html?: string;
  text?: string;
  tags?: Record<string, string>;
  replayed?: boolean;
  events?: { type: string; detail: Record<string, unknown> | null; created_at: string }[];
}

export interface TransactionalStats {
  sent: number;
  failed: number;
  queued: number;
  sending: number;
  delivered: number;
  bounced: number;
  complained: number;
  opened: number;
  clicked: number;
  tracked_opens: number;
  tracked_clicks: number;
  dispatch: { p50_ms: number | null; p95_ms: number | null; p99_ms: number | null };
}

export interface Dashboard {
  sending_mode: 'log' | 'sandbox' | 'production';
  contacts: number;
  active_contacts: number;
  campaigns: number;
  emails_sent: number;
  sent_today: number;
  sent_last_7_days: number;
  events_last_7_days: Record<string, number>;
  recent_emails: { id: string; subject: string; campaign_name: string; status: string; recipients: number; created_at: string }[];
}

export interface Reengagement {
  enabled: boolean;
  from_name: string;
  from_address: string;
  reply_to: string;
  subject: string;
  html: string;
  inactive_days: number;
  grace_days: number;
  min_broadcasts: number;
}

export interface Webhook {
  id: string;
  url: string;
  events: string[];
  enabled: boolean;
  secret_version: number;
  created_at: string;
}

export interface WebhookDelivery {
  id: string;
  event_id: string;
  status: string;
  attempts: number;
  response_code: number | null;
  last_error: string | null;
  next_attempt_at: string;
  created_at: string;
  finished_at: string | null;
}
