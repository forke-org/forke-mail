package compatapi

import (
	"encoding/json"
	"net/http"
	"strings"
	"unicode/utf8"

	"forkemail/internal/auth"
	"forkemail/internal/httpx"
)

const defaultCampaign = "Broadcasts"

type broadcastRequest struct {
	AudienceID  string      `json:"audience_id"`
	SegmentID   string      `json:"segment_id"`
	From        string      `json:"from"`
	ReplyTo     addressList `json:"reply_to"`
	Subject     string      `json:"subject"`
	HTML        string      `json:"html"`
	Text        string      `json:"text"`
	Name        string      `json:"name"`
	Topic       string      `json:"topic"`
	ScheduledAt string      `json:"scheduled_at"`
}

type emailFields struct {
	Subject     string        `json:"subject"`
	FromName    string        `json:"from_name"`
	FromAddress string        `json:"from_address"`
	ReplyTo     string        `json:"reply_to"`
	Topic       string        `json:"topic,omitempty"`
	HTML        string        `json:"html"`
	Text        string        `json:"text"`
	Audience    audienceRules `json:"audience"`
}

type audienceRules struct {
	All    bool     `json:"all"`
	TagIDs []string `json:"tag_ids"`
}

func (h *Handler) CreateBroadcast(w http.ResponseWriter, r *http.Request) {
	var req broadcastRequest
	if !decode(w, r, &req) {
		return
	}
	fields, message := req.translate()
	if message != "" {
		fail(w, http.StatusUnprocessableEntity, message)
		return
	}
	campaignID, err := h.campaign(r, req.Name)
	if err != nil {
		httpx.Internal(w, r, err)
		return
	}
	c := delegate(h.Emails.Create, r, fields, campaignID)
	if !c.ok() {
		relay(w, c)
		return
	}
	var created struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(c.body.Bytes(), &created)
	httpx.JSON(w, http.StatusCreated, map[string]string{"id": created.ID})
}

func (req broadcastRequest) translate() (emailFields, string) {
	audienceID := req.AudienceID
	if audienceID == "" {
		audienceID = req.SegmentID
	}
	name, from, ok := parseFrom(req.From)
	switch {
	case !ok:
		return emailFields{}, "invalid_from"
	case !httpx.ValidID(audienceID):
		return emailFields{}, "audience_id_required"
	case req.ScheduledAt != "":
		return emailFields{}, "scheduled_at_not_supported"
	case len(req.ReplyTo) > 1:
		return emailFields{}, "only_one_reply_to_supported"
	}
	out := emailFields{Subject: req.Subject, FromName: name, FromAddress: from, Topic: req.Topic, HTML: req.HTML, Text: req.Text,
		Audience: audienceRules{TagIDs: []string{audienceID}}}
	if len(req.ReplyTo) == 1 {
		if out.ReplyTo, ok = req.ReplyTo.single(); !ok {
			return emailFields{}, "invalid_reply_to"
		}
	}
	return out, ""
}

func (h *Handler) campaign(r *http.Request, name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		name = defaultCampaign
	}
	if utf8.RuneCountInString(name) > 200 {
		name = string([]rune(name)[:200])
	}
	var id string
	err := h.Pool.QueryRow(r.Context(), `
		WITH created AS (
			INSERT INTO campaigns (workspace_id, name) VALUES ($1, $2)
			ON CONFLICT (workspace_id, lower(name)) DO NOTHING RETURNING id
		)
		SELECT id FROM created
		UNION ALL
		SELECT id FROM campaigns WHERE workspace_id = $1 AND lower(name) = lower($2)
		LIMIT 1`, auth.FromContext(r.Context()).WorkspaceID, name).Scan(&id)
	return id, err
}

func (h *Handler) SendBroadcast(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ScheduledAt string `json:"scheduled_at"`
	}
	if r.ContentLength != 0 && !decode(w, r, &body) {
		return
	}
	if body.ScheduledAt != "" {
		fail(w, http.StatusUnprocessableEntity, "scheduled_at_not_supported")
		return
	}
	c := delegate(h.Emails.Send, r, nil, "")
	if !c.ok() {
		relay(w, c)
		return
	}
	var sent struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(c.body.Bytes(), &sent)
	httpx.JSON(w, http.StatusOK, map[string]string{"id": sent.ID})
}
