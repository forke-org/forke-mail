-- Copyright (c) 2026 Forke Inc. (https://www.forke.space/)
-- Source-Available License (Non-Commercial / Fair Source).
-- Open for inspection, learning, and non-commercial development.
-- Commercial use, hosting, or resale without authorization is strictly prohibited.

-- +goose Up
ALTER TABLE emails DROP CONSTRAINT emails_pause_reason_check;
ALTER TABLE emails ADD CONSTRAINT emails_pause_reason_check CHECK (pause_reason IN (
    'manual', 'trial_quota_exhausted', 'daily_limit_reached', 'ses_sending_paused', 'identity_not_verified', 'reputation', 'monthly_limit_reached'));

-- +goose Down
UPDATE emails SET pause_reason = 'manual' WHERE pause_reason = 'monthly_limit_reached';
ALTER TABLE emails DROP CONSTRAINT emails_pause_reason_check;
ALTER TABLE emails ADD CONSTRAINT emails_pause_reason_check CHECK (pause_reason IN (
    'manual', 'trial_quota_exhausted', 'daily_limit_reached', 'ses_sending_paused', 'identity_not_verified', 'reputation'));
