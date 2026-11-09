package resendapi

import (
	"encoding/json"
	"net/http"
	"time"

	"forkemail/internal/httpx"
)

type tag struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type sendEmailRequest struct {
	From        string            `json:"from"`
	To          addressList       `json:"to"`
	Cc          addressList       `json:"cc"`
	Bcc         addressList       `json:"bcc"`
	ReplyTo     addressList       `json:"reply_to"`
	Subject     string            `json:"subject"`
	HTML        string            `json:"html"`
	Text        string            `json:"text"`
	Headers     map[string]string `json:"headers"`
	Tags        []tag             `json:"tags"`
	ScheduledAt string            `json:"scheduled_at"`
	Attachments []json.RawMessage `json:"attachments"`
}

type transactionalRequest struct {
	To          string            `json:"to"`
	FromName    string            `json:"from_name"`
	FromAddress string            `json:"from_address"`
	ReplyTo     string            `json:"reply_to"`
	Subject     string            `json:"subject"`
	HTML        string            `json:"html"`
	Text        string            `json:"text"`
	Tags        map[string]string `json:"tags"`
}

func (h *Handler) SendEmail(w http.ResponseWriter, r *http.Request) {
	var req sendEmailRequest
	if !decode(w, r, &req) {
		return
	}
	out, message := req.translate()
	if message != "" {
		fail(w, http.StatusUnprocessableEntity, message)
		return
	}
	c := delegate(h.Transactional.Send, r, out, "")
	if !c.ok() {
		relay(w, c)
		return
	}
	var sent struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(c.body.Bytes(), &sent)
	if v := c.header.Get("Idempotent-Replayed"); v != "" {
		w.Header().Set("Idempotent-Replayed", v)
	}
	httpx.JSON(w, http.StatusOK, map[string]string{"id": sent.ID})
}

func (req sendEmailRequest) translate() (transactionalRequest, string) {
	name, from, ok := parseFrom(req.From)
	switch {
	case !ok:
		return transactionalRequest{}, "invalid_from"
	case len(req.Cc) > 0 || len(req.Bcc) > 0:
		return transactionalRequest{}, "cc_and_bcc_not_supported"
	case req.ScheduledAt != "":
		return transactionalRequest{}, "scheduled_at_not_supported"
	case len(req.Attachments) > 0:
		return transactionalRequest{}, "attachments_not_supported"
	case len(req.Headers) > 0:
		return transactionalRequest{}, "custom_headers_not_supported"
	case len(req.ReplyTo) > 1:
		return transactionalRequest{}, "only_one_reply_to_supported"
	}
	to, ok := req.To.single()
	if !ok {
		return transactionalRequest{}, "exactly_one_to_address_required"
	}
	out := transactionalRequest{To: to, FromName: name, FromAddress: from, Subject: req.Subject, HTML: req.HTML, Text: req.Text, Tags: map[string]string{}}
	if len(req.ReplyTo) == 1 {
		if out.ReplyTo, ok = req.ReplyTo.single(); !ok {
			return transactionalRequest{}, "invalid_reply_to"
		}
	}
	for _, t := range req.Tags {
		out.Tags[t.Name] = t.Value
	}
	return out, ""
}

type messageView struct {
	ID          string     `json:"id"`
	To          string     `json:"to"`
	FromName    string     `json:"from_name"`
	FromAddress string     `json:"from_address"`
	Subject     string     `json:"subject"`
	Status      string     `json:"status"`
	CreatedAt   time.Time  `json:"created_at"`
	DeliveredAt *time.Time `json:"delivered_at"`
	BouncedAt   *time.Time `json:"bounced_at"`
	ComplainAt  *time.Time `json:"complained_at"`
	OpenedAt    *time.Time `json:"opened_at"`
	ClickedAt   *time.Time `json:"clicked_at"`
	HTML        string     `json:"html"`
	Text        string     `json:"text"`
}

func (h *Handler) GetEmail(w http.ResponseWriter, r *http.Request) {
	c := delegate(h.Transactional.Get, r, nil, "")
	if !c.ok() {
		relay(w, c)
		return
	}
	var m messageView
	_ = json.Unmarshal(c.body.Bytes(), &m)
	from := m.FromAddress
	if m.FromName != "" {
		from = m.FromName + " <" + m.FromAddress + ">"
	}
	httpx.JSON(w, http.StatusOK, map[string]any{
		"object": "email", "id": m.ID, "to": []string{m.To}, "from": from, "subject": m.Subject,
		"html": m.HTML, "text": m.Text, "created_at": m.CreatedAt, "last_event": lastEvent(m),
	})
}

func lastEvent(m messageView) string {
	switch {
	case m.ComplainAt != nil:
		return "complained"
	case m.BouncedAt != nil:
		return "bounced"
	case m.ClickedAt != nil:
		return "clicked"
	case m.OpenedAt != nil:
		return "opened"
	case m.DeliveredAt != nil:
		return "delivered"
	case m.Status == "sent":
		return "sent"
	case m.Status == "failed":
		return "failed"
	}
	return "queued"
}
