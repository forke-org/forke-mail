type QuotaBody = { error?: string; required?: number; remaining?: number };

const handlers = {
  unauthorized: () => {},
  rateLimited: (_seconds: string | number) => {},
  quota: (_body: QuotaBody | null) => {},
};

export function setApiHandlers(next: Partial<typeof handlers>) {
  Object.assign(handlers, next);
}

export class ApiError extends Error {
  status: number;
  body: Record<string, any>;
  constructor(status: number, body: Record<string, any> | null) {
    super((body && body.error) || `HTTP ${status}`);
    this.status = status;
    this.body = body || {};
  }
}

export async function api<T = any>(method: string, path: string, body?: unknown): Promise<T> {
  const init: RequestInit = { method, headers: {}, credentials: 'same-origin' };
  if (body instanceof FormData) {
    init.body = body;
  } else if (body !== undefined) {
    init.headers = { 'Content-Type': 'application/json' };
    init.body = JSON.stringify(body);
  }
  const res = await fetch(path, init);
  const data = res.status === 204 ? null : await res.json().catch(() => null);
  if (res.status === 401 && !path.startsWith('/api/auth/')) handlers.unauthorized();
  if (res.status === 429) handlers.rateLimited(res.headers.get('Retry-After') || (data && data.retry_after) || 1);
  if (res.status === 402) handlers.quota(data);
  if (!res.ok) throw new ApiError(res.status, data);
  return data as T;
}

const messages: Record<string, string> = {
  invalid_credentials: 'Incorrect email, username or password, or this account is not active.',
  email_taken: 'An account with this email already exists.',
  invalid_password: 'Password must be 8 to 72 characters.',
  invalid_email: 'Enter a valid email address.',
  invalid_name: 'Enter a name.',
  incorrect_password: 'That password is incorrect.',
  insufficient_scope: 'You do not have permission for that action.',
  forbidden: 'You do not have permission for that action.',
  from_domain_not_verified: 'The from address must use one of your verified domains or email addresses.',
  workspace_sending_paused: 'Sending is paused for this workspace because of its bounce or complaint rate.',
  invalid_identity: 'Enter a valid domain such as mail.example.com, or a valid email address.',
  identity_taken: 'That domain or address is already registered.',
  ses_request_failed: 'The email provider rejected the request. Try again in a moment.',
  invalid_from_address: 'Enter a valid from address.',
  invalid_subject: 'Enter a subject.',
  missing_body: 'Write some content for your email.',
  trial_cannot_create_full_access_key: 'Trial workspaces can only create sending access keys.',
  last_full_access_key: 'You cannot revoke the last full access key.',
  no_eligible_recipients: 'This audience has no active contacts to send to.',
  email_not_draft: 'Only drafts can be changed.',
  campaign_exists: 'A campaign with this name already exists.',
  tag_exists: 'That tag already exists.',
  rate_limited: 'Too many requests. Slow down a little.',
  too_many_rows: 'That file has more than 50,000 rows.',
  body_too_large: 'That file is larger than 5 MB.',
  not_found: 'Not found.',
  invalid_to: 'Enter a valid recipient address.',
  invalid_reply_to: 'Enter a valid reply-to address.',
  invalid_tags: 'Tags must be letters, numbers, dashes or underscores (up to 10).',
  tracking_requires_html: 'Open and click tracking need an HTML body.',
  recipient_suppressed: 'That address hard bounced, complained or was suppressed manually, so it cannot be emailed.',
  idempotency_key_reused: 'That idempotency key was already used with a different message.',
  invalid_idempotency_key: 'The idempotency key must be 1 to 256 printable characters.',
  daily_limit_reached: 'The daily send limit for this workspace has been reached.',
  monthly_limit_reached: 'The monthly sending cap has been reached. Sending resumes on the 1st.',
  workspace_inactive: 'This workspace is not active.',
  invalid_topic: 'Topic keys are lowercase letters, numbers, dashes or underscores (up to 40).',
  invalid_label: 'Enter a label of up to 60 characters.',
  html_needs_a_link: 'The email needs at least one https link so a click can count as activity.',
  invalid_inactive_days: 'Inactive days must be between 30 and 730.',
  invalid_grace_days: 'Grace days must be between 7 and 90.',
  invalid_min_broadcasts: 'Minimum broadcasts must be between 1 and 50.',
  webhooks_not_configured: 'Webhooks are not configured on this server (WEBHOOK_ENCRYPTION_KEY is missing).',
  invalid_webhook_url: 'Enter a public https URL on port 443.',
  unsafe_webhook_destination: 'That URL points to a private or blocked network address.',
  webhook_dns_unavailable: 'That hostname could not be resolved.',
  invalid_webhook_events: 'Choose at least one event.',
  webhook_endpoint_limit: 'This workspace already has the maximum of ten webhooks.',
  csv_invalid: 'That file is not a valid CSV.',
  invalid_csv: 'That file is not a valid CSV.',
  missing_csv: 'Choose a CSV file first.',
};

export function describeError(err: unknown): string {
  if (!(err instanceof ApiError)) return 'Network error. Check your connection.';
  if (err.message === 'trial_quota_exceeded') {
    return `This email needs ${err.body.required} sends but your trial has ${err.body.remaining} left.`;
  }
  if (err.message === 'monthly_limit_insufficient') {
    return `This email needs ${Number(err.body.required).toLocaleString()} sends but only ${Number(err.body.remaining).toLocaleString()} are left this month.`;
  }
  return messages[err.message] || err.message.replace(/_/g, ' ');
}
