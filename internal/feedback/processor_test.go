package feedback

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"forkemail/internal/db/dbtest"
	"forkemail/internal/sns"
	"forkemail/internal/sns/snstest"

	"github.com/jackc/pgx/v5/pgxpool"
)

var lenient = Thresholds{MaxBounceRate: 1, MaxComplaintRate: 1, MinSample: 1000000}

type fixture struct {
	pool      *pgxpool.Pool
	p         *Processor
	ws, email string
}

func setup(t *testing.T) fixture {
	pool := dbtest.Pool(t)
	ws := dbtest.Workspace(t, pool, "paid", 0)
	return fixture{pool: pool, p: &Processor{Pool: pool, Reputation: lenient}, ws: ws, email: dbtest.DraftEmail(t, pool, ws)}
}

func (f fixture) recipient(t *testing.T, address string) (string, string) {
	messageID := fmt.Sprintf("msg-%d-%s", time.Now().UnixNano(), address)
	return dbtest.SentRecipient(t, f.pool, f.ws, f.email, address, messageID), messageID
}

func (f fixture) event(kind, messageID, recipientID string, extra map[string]any) []byte {
	ev := map[string]any{
		"eventType": kind,
		"mail": map[string]any{
			"timestamp": time.Now().UTC().Format(time.RFC3339), "messageId": messageID,
			"tags": map[string][]string{"workspace_id": {f.ws}, "email_id": {f.email}, "recipient_id": {recipientID}},
		},
	}
	for k, v := range extra {
		ev[k] = v
	}
	raw, _ := json.Marshal(ev)
	return raw
}

func (f fixture) handle(t *testing.T, snsID string, raw []byte) string {
	t.Helper()
	res, err := f.p.Handle(context.Background(), snsID, raw)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func (f fixture) count(t *testing.T, sql string, args ...any) string {
	t.Helper()
	return dbtest.QueryString(t, f.pool, sql, args...)
}

func bounce(kind, subType string) map[string]any {
	return map[string]any{"bounce": map[string]any{
		"bounceType": kind, "bounceSubType": subType, "timestamp": time.Now().UTC().Format(time.RFC3339),
		"bouncedRecipients": []map[string]string{{"emailAddress": "x", "diagnosticCode": "smtp; 550"}},
	}}
}

func TestPermanentBounceSuppresses(t *testing.T) {
	f := setup(t)
	for _, sub := range []string{"General", "OnAccountSuppressionList"} {
		address := strings.ToLower(sub) + "@x.io"
		rid, mid := f.recipient(t, address)
		if res := f.handle(t, "", f.event("Bounce", mid, rid, bounce("Permanent", sub))); res != Processed {
			t.Fatalf("%s: result %s", sub, res)
		}
		if got := f.count(t, `SELECT source FROM suppressions WHERE workspace_id = $1 AND email = $2`, f.ws, address); got != "ses_bounce" {
			t.Errorf("%s: suppression source %s", sub, got)
		}
		if got := f.count(t, `SELECT status FROM contacts WHERE workspace_id = $1 AND email = $2`, f.ws, address); got != "bounced" {
			t.Errorf("%s: contact status %s", sub, got)
		}
		if got := f.count(t, `SELECT bounce_type FROM email_recipients WHERE id = $1 AND bounced_at IS NOT NULL`, rid); got != "Permanent/"+sub {
			t.Errorf("%s: bounce_type %s", sub, got)
		}
	}
}

func TestTransientBounceDoesNotSuppress(t *testing.T) {
	f := setup(t)
	for _, kind := range []string{"Transient", "Undetermined"} {
		address := strings.ToLower(kind) + "@x.io"
		rid, mid := f.recipient(t, address)
		f.handle(t, "", f.event("Bounce", mid, rid, bounce(kind, "General")))
		if got := f.count(t, `SELECT COUNT(*)::text FROM suppressions WHERE workspace_id = $1`, f.ws); got != "0" {
			t.Fatalf("%s bounce created a suppression", kind)
		}
		if got := f.count(t, `SELECT (bounced_at IS NULL)::text || ':' || status FROM email_recipients WHERE id = $1`, rid); got != "true:sent" {
			t.Errorf("%s: recipient %s", kind, got)
		}
		if got := f.count(t, `SELECT type FROM email_events WHERE recipient_id = $1`, rid); got != "soft_bounced" {
			t.Errorf("%s: event %s", kind, got)
		}
	}
}

func TestComplaintSuppresses(t *testing.T) {
	f := setup(t)
	rid, mid := f.recipient(t, "angry@x.io")
	f.handle(t, "", f.event("Complaint", mid, rid, map[string]any{"complaint": map[string]any{
		"complainedRecipients": []map[string]string{{"emailAddress": "angry@x.io"}}, "complaintFeedbackType": "abuse",
	}}))
	if got := f.count(t, `SELECT source || ':' || reason FROM suppressions WHERE workspace_id = $1`, f.ws); got != "ses_complaint:complained" {
		t.Errorf("suppression %s", got)
	}
	if got := f.count(t, `SELECT status FROM contacts WHERE workspace_id = $1 AND email = 'angry@x.io'`, f.ws); got != "complained" {
		t.Errorf("contact status %s", got)
	}
	if got := f.count(t, `SELECT (complained_at IS NOT NULL)::text FROM email_recipients WHERE id = $1`, rid); got != "true" {
		t.Error("complained_at not set")
	}
}

func TestDeliveryRejectAndDelay(t *testing.T) {
	f := setup(t)
	rid, mid := f.recipient(t, "ok@x.io")
	f.handle(t, "", f.event("Delivery", mid, rid, nil))
	f.handle(t, "", f.event("DeliveryDelay", mid, rid, map[string]any{"deliveryDelay": map[string]any{"delayType": "MailboxFull"}}))
	if got := f.count(t, `SELECT (delivered_at IS NOT NULL)::text FROM email_recipients WHERE id = $1`, rid); got != "true" {
		t.Error("delivered_at not set")
	}
	rejected, rmid := f.recipient(t, "virus@x.io")
	f.handle(t, "", f.event("Reject", rmid, rejected, map[string]any{"reject": map[string]string{"reason": "Bad content"}}))
	if got := f.count(t, `SELECT status || ':' || last_error FROM email_recipients WHERE id = $1`, rejected); got != "failed:rejected: Bad content" {
		t.Errorf("rejected recipient %s", got)
	}
	if got := f.count(t, `SELECT string_agg(type, ',' ORDER BY id) FROM email_events WHERE workspace_id = $1`, f.ws); got != "delivered,delayed,rejected" {
		t.Errorf("events %s", got)
	}
	if got := f.count(t, `SELECT COUNT(*)::text FROM suppressions WHERE workspace_id = $1`, f.ws); got != "0" {
		t.Error("delivery, delay or reject created a suppression")
	}
}

func TestDuplicateSNSMessageProcessedOnce(t *testing.T) {
	f := setup(t)
	rid, mid := f.recipient(t, "dup@x.io")
	snsID := "sns-" + mid
	raw := f.event("Delivery", mid, rid, nil)
	if res := f.handle(t, snsID, raw); res != Processed {
		t.Fatalf("first: %s", res)
	}
	if res := f.handle(t, snsID, raw); res != Duplicate {
		t.Fatalf("second: %s", res)
	}
	if got := f.count(t, `SELECT COUNT(*)::text FROM email_events WHERE recipient_id = $1`, rid); got != "1" {
		t.Fatalf("events = %s", got)
	}
}

func TestUnknownMessageAndTagMismatchIgnored(t *testing.T) {
	f := setup(t)
	if res := f.handle(t, "", f.event("Bounce", "never-sent", "", bounce("Permanent", "General"))); res != UnknownMessage {
		t.Fatalf("unknown: %s", res)
	}
	other := setup(t)
	rid, mid := f.recipient(t, "target@x.io")
	forged := other.event("Bounce", mid, rid, bounce("Permanent", "General"))
	if res := f.handle(t, "", forged); res != TagMismatch {
		t.Fatalf("mismatch: %s", res)
	}
	if got := f.count(t, `SELECT COUNT(*)::text FROM suppressions WHERE workspace_id IN ($1, $2)`, f.ws, other.ws); got != "0" {
		t.Fatal("mismatched event created a suppression")
	}
	if got := f.count(t, `SELECT (bounced_at IS NULL)::text FROM email_recipients WHERE id = $1`, rid); got != "true" {
		t.Fatal("mismatched event changed the recipient")
	}
}

func TestWebhookEndToEnd(t *testing.T) {
	f := setup(t)
	signer := snstest.NewSigner(t)
	h := &Webhook{Verifier: signer.Verifier(), Processor: f.p}
	post := func(m sns.Message) int {
		body, _ := json.Marshal(m)
		req := httptest.NewRequest(http.MethodPost, "/webhooks/ses", strings.NewReader(string(body)))
		req.Header.Set("Content-Type", "text/plain; charset=UTF-8")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code
	}
	rid, mid := f.recipient(t, "hook@x.io")
	n := sns.Message{Type: sns.TypeNotification, MessageID: "sns-hook-" + mid, Message: string(f.event("Bounce", mid, rid, bounce("Permanent", "General")))}
	if code := post(signer.Sign(t, n)); code != http.StatusOK {
		t.Fatalf("signed bounce returned %d", code)
	}
	if got := f.count(t, `SELECT source FROM suppressions WHERE workspace_id = $1 AND email = 'hook@x.io'`, f.ws); got != "ses_bounce" {
		t.Fatalf("suppression %s", got)
	}
	unknown := sns.Message{Type: sns.TypeNotification, MessageID: "sns-unknown-" + mid, Message: string(f.event("Delivery", "nope", "", nil))}
	if code := post(signer.Sign(t, unknown)); code != http.StatusOK {
		t.Fatalf("unknown message id returned %d, want 200", code)
	}
	forged := signer.Sign(t, n)
	forged.Message = strings.Replace(forged.Message, "Permanent", "Transient", 1)
	if code := post(forged); code != http.StatusForbidden {
		t.Fatalf("tampered returned %d, want 403", code)
	}
	confirm := sns.Message{Type: sns.TypeSubscriptionConfirmation, MessageID: "c", Token: "t", Message: "m",
		SubscribeURL: "https://sns.us-east-1.amazonaws.com/?Action=ConfirmSubscription&Token=t"}
	if code := post(signer.Sign(t, confirm)); code != http.StatusOK {
		t.Fatalf("confirmation returned %d", code)
	}
	evil := confirm
	evil.SubscribeURL = "https://attacker.example/confirm"
	if code := post(signer.Sign(t, evil)); code == http.StatusOK {
		t.Fatal("followed untrusted subscribe url")
	}
	for _, u := range signer.Requests {
		if strings.Contains(u, "attacker") {
			t.Fatal("requested attacker url")
		}
	}
}
