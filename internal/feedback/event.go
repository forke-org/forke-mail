package feedback

import "time"

type Event struct {
	EventType        string         `json:"eventType,omitempty"`
	NotificationType string         `json:"notificationType,omitempty"`
	Mail             Mail           `json:"mail"`
	Bounce           *Bounce        `json:"bounce,omitempty"`
	Complaint        *Complaint     `json:"complaint,omitempty"`
	Delivery         *Delivery      `json:"delivery,omitempty"`
	Reject           *Reject        `json:"reject,omitempty"`
	DeliveryDelay    *DeliveryDelay `json:"deliveryDelay,omitempty"`
}

type Mail struct {
	Timestamp   time.Time           `json:"timestamp"`
	MessageID   string              `json:"messageId"`
	Source      string              `json:"source,omitempty"`
	Destination []string            `json:"destination,omitempty"`
	Tags        map[string][]string `json:"tags,omitempty"`
}

type Recipient struct {
	EmailAddress   string `json:"emailAddress"`
	Action         string `json:"action,omitempty"`
	Status         string `json:"status,omitempty"`
	DiagnosticCode string `json:"diagnosticCode,omitempty"`
}

type Bounce struct {
	BounceType        string      `json:"bounceType"`
	BounceSubType     string      `json:"bounceSubType"`
	BouncedRecipients []Recipient `json:"bouncedRecipients"`
	Timestamp         time.Time   `json:"timestamp"`
	FeedbackID        string      `json:"feedbackId,omitempty"`
}

type Complaint struct {
	ComplainedRecipients  []Recipient `json:"complainedRecipients"`
	ComplaintFeedbackType string      `json:"complaintFeedbackType,omitempty"`
	Timestamp             time.Time   `json:"timestamp"`
	FeedbackID            string      `json:"feedbackId,omitempty"`
}

type Delivery struct {
	Timestamp            time.Time `json:"timestamp"`
	Recipients           []string  `json:"recipients"`
	ProcessingTimeMillis int       `json:"processingTimeMillis,omitempty"`
	SMTPResponse         string    `json:"smtpResponse,omitempty"`
}

type Reject struct {
	Reason string `json:"reason"`
}

type DeliveryDelay struct {
	DelayType         string      `json:"delayType"`
	ExpirationTime    string      `json:"expirationTime,omitempty"`
	DelayedRecipients []Recipient `json:"delayedRecipients"`
	Timestamp         time.Time   `json:"timestamp"`
}

func (e Event) Kind() string {
	if e.EventType != "" {
		return e.EventType
	}
	return e.NotificationType
}

func at(t time.Time) time.Time {
	if t.IsZero() {
		return time.Now()
	}
	return t
}
