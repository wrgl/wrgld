package webhooktest

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wrgl/wrgl/pkg/conf"
	"github.com/wrgl/wrgl/pkg/testutils"
	"github.com/wrgl/wrgld/pkg/webhook"
)

type webhookHandler struct {
	body   *webhook.Payload
	t      *testing.T
	secret string
}

func (h *webhookHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	assert.Equal(h.t, r.Header.Get("Content-Type"), "application/json")
	b, err := io.ReadAll(r.Body)
	require.NoError(h.t, err)
	h.body = &webhook.Payload{}
	require.NoError(h.t, json.Unmarshal(b, h.body))
	if h.secret != "" {
		sig, err := hex.DecodeString(r.Header.Get(webhook.SignatureHeader))
		require.NoError(h.t, err)
		hash := hmac.New(sha256.New, []byte(h.secret))
		_, err = hash.Write(b)
		require.NoError(h.t, err)
		assert.True(h.t, hmac.Equal(sig, hash.Sum(nil)))
	}
	rw.WriteHeader(http.StatusOK)
}

func (h *webhookHandler) getPayload() (body *webhook.Payload) {
	pl := h.body
	h.body = nil
	return pl
}

func CreateWebhookHandler(t *testing.T, eventTypes []conf.WebhookEventType, withSecretToken bool) (
	wh conf.Webhook, getPayload func() (body *webhook.Payload), cleanup func(),
) {
	h := &webhookHandler{t: t}
	srv := httptest.NewServer(h)
	obj := &conf.Webhook{
		URL:        srv.URL,
		EventTypes: eventTypes,
	}
	if withSecretToken {
		obj.SecretToken = testutils.BrokenRandomAlphaNumericString(10)
		h.secret = obj.SecretToken
	}
	return *obj, h.getPayload, func() {
		srv.Close()
	}
}
