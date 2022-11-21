package webhook_test

import (
	"encoding/hex"
	"sync"
	"testing"

	"github.com/brianvoe/gofakeit/v6"
	"github.com/go-logr/logr/testr"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/wrgl/wrgl/pkg/conf"
	"github.com/wrgl/wrgl/pkg/testutils"
	"github.com/wrgl/wrgld/pkg/webhook"
	webhooktest "github.com/wrgl/wrgld/pkg/webhook/test"
)

func TestSender(t *testing.T) {
	logger := testr.New(t)
	wg := &sync.WaitGroup{}

	// test zero webhook registered
	s := webhook.NewSenderWithConfig(conf.Config{}, logger, webhook.WithWaitGroup(wg))
	s.Flush()
	wg.Wait()

	// test webhooks registered
	wh1, pl1, cleanup := webhooktest.CreateWebhookHandler(t, []conf.WebhookEventType{conf.CommitEventType}, false)
	defer cleanup()
	wh2, pl2, cleanup := webhooktest.CreateWebhookHandler(t, []conf.WebhookEventType{conf.RefUpdateEventType}, true)
	defer cleanup()
	wh3, pl3, cleanup := webhooktest.CreateWebhookHandler(t, []conf.WebhookEventType{conf.CommitEventType, conf.RefUpdateEventType}, true)
	defer cleanup()
	s = webhook.NewSenderWithConfig(conf.Config{
		Webhooks: []conf.Webhook{wh1, wh2, wh3},
	}, logger, webhook.WithWaitGroup(wg))
	s.Flush()
	wg.Wait()
	assert.Nil(t, pl1())
	assert.Nil(t, pl2())
	assert.Nil(t, pl3())

	// test enqueue commit event
	s = webhook.NewSenderWithConfig(conf.Config{
		Webhooks: []conf.Webhook{wh1, wh2, wh3},
	}, logger, webhook.WithWaitGroup(wg))
	ce := &webhook.CommitEvent{
		TransactionID: uuid.New().String(),
		Commits: []webhook.Commit{
			{
				Sum:     hex.EncodeToString(testutils.SecureRandomBytes(16)),
				Ref:     "heads/my-branch",
				Message: gofakeit.LoremIpsumSentence(5),
			},
		},
		AuthorName:  gofakeit.Name(),
		AuthorEmail: gofakeit.Email(),
	}
	s.EnqueueEvent(ce)
	assert.NotEmpty(t, ce.Type)
	assert.NotEmpty(t, ce.Time)
	s.Flush()
	wg.Wait()
	assert.Equal(t, pl1(), &webhook.Payload{
		Events: []webhook.Event{
			ce,
		},
	})
	assert.Nil(t, pl2())
	assert.Equal(t, pl3(), &webhook.Payload{
		Events: []webhook.Event{
			ce,
		},
	})

	// test enqueue ref update event
	s = webhook.NewSenderWithConfig(conf.Config{
		Webhooks: []conf.Webhook{wh1, wh2, wh3},
	}, logger, webhook.WithWaitGroup(wg))
	ue := &webhook.RefUpdateEvent{
		OldSum:  hex.EncodeToString(testutils.SecureRandomBytes(16)),
		Sum:     hex.EncodeToString(testutils.SecureRandomBytes(16)),
		Ref:     "heads/my-branch",
		Action:  gofakeit.LoremIpsumWord(),
		Message: gofakeit.LoremIpsumSentence(5),
	}
	s.EnqueueEvent(ue)
	assert.NotEmpty(t, ue.Type)
	assert.NotEmpty(t, ue.Time)
	s.Flush()
	wg.Wait()
	assert.Nil(t, pl1())
	assert.Equal(t, pl2(), &webhook.Payload{
		Events: []webhook.Event{
			ue,
		},
	})
	assert.Equal(t, pl3(), &webhook.Payload{
		Events: []webhook.Event{
			ue,
		},
	})

	// test enqueue multiple events
	s = webhook.NewSenderWithConfig(conf.Config{
		Webhooks: []conf.Webhook{wh1, wh2, wh3},
	}, logger, webhook.WithWaitGroup(wg))
	s.EnqueueEvent(ce)
	s.EnqueueEvent(ue)
	s.Flush()
	wg.Wait()
	assert.Equal(t, pl1(), &webhook.Payload{
		Events: []webhook.Event{
			ce,
		},
	})
	assert.Equal(t, pl2(), &webhook.Payload{
		Events: []webhook.Event{
			ue,
		},
	})
	assert.Equal(t, pl3(), &webhook.Payload{
		Events: []webhook.Event{
			ce, ue,
		},
	})
}
