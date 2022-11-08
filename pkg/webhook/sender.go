package webhook

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"sync"

	"github.com/go-logr/logr"
	"github.com/wrgl/wrgl/pkg/conf"
)

const (
	SignatureHeader = "X-Wrgl-Signature-256"
)

type Sender struct {
	webhooks []*conf.Webhook
	events   map[conf.WebhookEventType][]Event
	logger   logr.Logger
	wg       *sync.WaitGroup
	mutex    sync.Mutex
}

type SenderOption func(s *Sender)

func WithWaitGroup(wg *sync.WaitGroup) SenderOption {
	return func(s *Sender) {
		s.wg = wg
	}
}

func NewSender(cs conf.Store, logger logr.Logger, opts ...SenderOption) (*Sender, error) {
	c, err := cs.Open()
	if err != nil {
		return nil, err
	}
	s := NewSenderWithConfig(c, logger, opts...)
	return s, nil
}

func NewSenderWithConfig(c *conf.Config, logger logr.Logger, opts ...SenderOption) *Sender {
	s := &Sender{
		webhooks: make([]*conf.Webhook, len(c.Webhooks)),
		events:   map[conf.WebhookEventType][]Event{},
		logger:   logger,
	}
	for i, wh := range c.Webhooks {
		s.webhooks[i] = &conf.Webhook{}
		*s.webhooks[i] = wh
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func (s *Sender) EnqueueEvent(evt Event) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	evt.SetType()
	s.events[evt.GetType()] = append(s.events[evt.GetType()], evt)
}

func (s *Sender) Flush() {
	if s.wg != nil {
		s.wg.Add(1)
	}
	go func() {
		s.mutex.Lock()
		defer s.mutex.Unlock()
		if s.wg != nil {
			defer s.wg.Done()
		}
		for _, wh := range s.webhooks {
			pl := &Payload{}
			for _, et := range wh.EventTypes {
				pl.Events = append(pl.Events, s.events[et]...)
			}
			if len(pl.Events) == 0 {
				continue
			}
			b, err := json.Marshal(pl)
			if err != nil {
				s.logger.Error(err, "error marshaling json")
			}
			req, err := http.NewRequest(http.MethodPost, wh.URL, bytes.NewReader(b))
			if err != nil {
				s.logger.Error(err, "error creating new request", "url", wh.URL)
			}
			req.Header.Set("Content-Type", "application/json")
			if wh.SecretToken != "" {
				h := hmac.New(sha256.New, []byte(wh.SecretToken))
				_, err := h.Write(b)
				if err != nil {
					s.logger.Error(err, "error digesting payload")
				}
				req.Header.Set(SignatureHeader, hex.EncodeToString(h.Sum(nil)))
			}
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				s.logger.Error(err, "error sending payload")
			} else {
				s.logger.Info("sent payload to webhook",
					"url", wh.URL,
					"status", resp.StatusCode,
					"events_count", len(pl.Events),
				)
			}
		}
		s.events = map[conf.WebhookEventType][]Event{}
	}()
}
