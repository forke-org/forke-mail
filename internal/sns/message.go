package sns

import "strings"

const (
	TypeNotification             = "Notification"
	TypeSubscriptionConfirmation = "SubscriptionConfirmation"
	TypeUnsubscribeConfirmation  = "UnsubscribeConfirmation"
)

type Message struct {
	Type             string `json:"Type"`
	MessageID        string `json:"MessageId"`
	Token            string `json:"Token"`
	TopicArn         string `json:"TopicArn"`
	Subject          string `json:"Subject"`
	Message          string `json:"Message"`
	Timestamp        string `json:"Timestamp"`
	SignatureVersion string `json:"SignatureVersion"`
	Signature        string `json:"Signature"`
	SigningCertURL   string `json:"SigningCertURL"`
	SubscribeURL     string `json:"SubscribeURL"`
	UnsubscribeURL   string `json:"UnsubscribeURL"`
}

func StringToSign(m Message) (string, bool) {
	var fields [][2]string
	switch m.Type {
	case TypeNotification:
		fields = [][2]string{{"Message", m.Message}, {"MessageId", m.MessageID}}
		if m.Subject != "" {
			fields = append(fields, [2]string{"Subject", m.Subject})
		}
		fields = append(fields, [2]string{"Timestamp", m.Timestamp}, [2]string{"TopicArn", m.TopicArn}, [2]string{"Type", m.Type})
	case TypeSubscriptionConfirmation, TypeUnsubscribeConfirmation:
		fields = [][2]string{
			{"Message", m.Message}, {"MessageId", m.MessageID}, {"SubscribeURL", m.SubscribeURL},
			{"Timestamp", m.Timestamp}, {"Token", m.Token}, {"TopicArn", m.TopicArn}, {"Type", m.Type},
		}
	default:
		return "", false
	}
	var b strings.Builder
	for _, f := range fields {
		b.WriteString(f[0] + "\n" + f[1] + "\n")
	}
	return b.String(), true
}
