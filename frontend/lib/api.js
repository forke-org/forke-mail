const handlers = { unauthorized: () => {}, rateLimited: () => {}, quota: () => {} };

export function setApiHandlers(next) {
  Object.assign(handlers, next);
}

export class ApiError extends Error {
  constructor(status, body) {
    super((body && body.error) || `HTTP ${status}`);
    this.status = status;
    this.body = body || {};
  }
}

export async function api(method, path, body) {
  const init = { method, headers: {}, credentials: 'same-origin' };
  if (body instanceof FormData) {
    init.body = body;
  } else if (body !== undefined) {
    init.headers['Content-Type'] = 'application/json';
    init.body = JSON.stringify(body);
  }
  const res = await fetch(path, init);
  const data = res.status === 204 ? null : await res.json().catch(() => null);
  if (res.status === 401 && !path.startsWith('/api/auth/')) handlers.unauthorized();
  if (res.status === 429) handlers.rateLimited(res.headers.get('Retry-After') || (data && data.retry_after) || 1);
  if (res.status === 402) handlers.quota(data);
  if (!res.ok) throw new ApiError(res.status, data);
  return data;
}

const messages = {
  invalid_credentials: 'Incorrect email, username or password, or this Forke account is not an active admin.',
  email_taken: 'An account with this email already exists.',
  invalid_password: 'Password must be 8 to 72 characters.',
  invalid_email: 'Enter a valid email address.',
  invalid_name: 'Enter a name.',
  incorrect_password: 'That password is incorrect.',
  insufficient_scope: 'You do not have permission for that action.',
  from_domain_not_verified: 'The from address must use one of your verified domains or email addresses.',
  workspace_sending_paused: 'Sending is paused for this workspace because of its bounce or complaint rate.',
  invalid_identity: 'Enter a valid domain such as mail.example.com, or a valid email address.',
  identity_taken: 'That domain or address is already registered.',
  ses_request_failed: 'Amazon SES rejected the request. Check the server logs for details.',
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
  workspace_inactive: 'This workspace is not active.',
};

export function describeError(err) {
  if (!(err instanceof ApiError)) return 'Network error. Check your connection.';
  if (err.message === 'trial_quota_exceeded') {
    return `This email needs ${err.body.required} sends but your trial has ${err.body.remaining} left.`;
  }
  return messages[err.message] || err.message.replace(/_/g, ' ');
}
