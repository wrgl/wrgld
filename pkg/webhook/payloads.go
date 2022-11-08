package webhook

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/wrgl/wrgl/pkg/conf"
)

type Event interface {
	GetType() conf.WebhookEventType
	SetType()
}

type Events []Event

type Payload struct {
	Events Events `json:"events"`
}

func (sl *Events) UnmarshalJSON(b []byte) error {
	data := []json.RawMessage{}
	if err := json.Unmarshal(b, &data); err != nil {
		return err
	}
	for _, v := range data {
		ce := &CommitEvent{}
		var e Event
		if err := json.Unmarshal(v, ce); err != nil {
			return err
		}
		switch ce.Type {
		case conf.CommitEventType:
			*sl = append(*sl, ce)
			continue
		case conf.RefUpdateEventType:
			e = &RefUpdateEvent{}
		default:
			return fmt.Errorf("unhandled event type %q", ce.Type)
		}
		if err := json.Unmarshal(v, e); err != nil {
			return err
		}
		*sl = append(*sl, e)
	}
	return nil
}

type Commit struct {
	Sum     string `json:"sum"`
	Ref     string `json:"ref"`
	Message string `json:"message"`
}

type CommitEvent struct {
	Type          conf.WebhookEventType `json:"type"`
	TransactionID string                `json:"transactionId,omitempty"`
	Commits       []Commit              `json:"commits"`
	AuthorName    string                `json:"authorName"`
	AuthorEmail   string                `json:"authorEmail"`
	Time          string                `json:"time"`
}

func (e *CommitEvent) GetType() conf.WebhookEventType {
	return conf.CommitEventType
}

func (e *CommitEvent) SetType() {
	e.Type = conf.CommitEventType
	e.Time = time.Now().Format(time.RFC3339)
}

type RefUpdateEvent struct {
	Type    conf.WebhookEventType `json:"type"`
	OldSum  string                `json:"oldSum"`
	Sum     string                `json:"sum"`
	Ref     string                `json:"ref"`
	Action  string                `json:"action,omitempty"`
	Message string                `json:"message,omitempty"`
	Time    string                `json:"time"`
}

func (e *RefUpdateEvent) GetType() conf.WebhookEventType {
	return conf.RefUpdateEventType
}

func (e *RefUpdateEvent) SetType() {
	e.Type = conf.RefUpdateEventType
	e.Time = time.Now().Format(time.RFC3339)
}
