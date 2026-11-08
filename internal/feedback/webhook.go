package feedback

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"

	"forkemail/internal/httpx"
	"forkemail/internal/sns"
)

const maxWebhookBytes = 256 << 10

type Webhook struct {
	Verifier  *sns.Verifier
	Processor *Processor
}

func (h *Webhook) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxWebhookBytes))
	if err != nil {
		httpx.Error(w, http.StatusRequestEntityTooLarge, "body_too_large")
		return
	}
	var m sns.Message
	if err := json.Unmarshal(body, &m); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid_json")
		return
	}
	if err := h.Verifier.Verify(r.Context(), m); err != nil {
		slog.Warn("rejected sns message", "type", m.Type, "err", err)
		httpx.Error(w, http.StatusForbidden, "forbidden")
		return
	}
	switch m.Type {
	case sns.TypeSubscriptionConfirmation:
		if err := h.Verifier.Confirm(r.Context(), m); err != nil {
			slog.Error("sns subscription confirmation failed", "err", err)
			httpx.Error(w, http.StatusInternalServerError, "confirmation_failed")
			return
		}
		slog.Info("sns subscription confirmed", "topic_arn", m.TopicArn)
	case sns.TypeUnsubscribeConfirmation:
		slog.Info("sns unsubscribe confirmation received", "topic_arn", m.TopicArn)
	case sns.TypeNotification:
		result, err := h.Processor.Handle(r.Context(), m.MessageID, []byte(m.Message))
		if err != nil {
			slog.Error("ses event processing failed", "sns_message_id", m.MessageID, "err", err)
			httpx.Error(w, http.StatusInternalServerError, "processing_failed")
			return
		}
		slog.Info("ses event", "sns_message_id", m.MessageID, "result", result)
	}
	w.WriteHeader(http.StatusOK)
}
