package server_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wrgl/wrgl/pkg/api/payload"
	"github.com/wrgl/wrgl/pkg/conf"
	"github.com/wrgl/wrgl/pkg/ref"
	"github.com/wrgl/wrgl/pkg/testutils"
	server_testutils "github.com/wrgl/wrgld/pkg/server/testutils"
	"github.com/wrgl/wrgld/pkg/webhook"
)

func (s *testSuite) TestTransaction(t *testing.T) {
	repo, cli, _, cleanup := s.s.NewClient(t, "", true)
	defer cleanup()

	ctr, err := cli.CreateTransaction(nil)
	require.NoError(t, err)
	tid, err := uuid.Parse(ctr.ID)
	require.NoError(t, err)

	cr1, err := cli.Commit("alpha", "initial commit", "file.csv", testutils.RawCSVBytesReader(testutils.BuildRawCSV(3, 4)), nil, nil)
	require.NoError(t, err)
	cr2, err := cli.Commit("alpha", "second commit", "file.csv", testutils.RawCSVBytesReader(testutils.BuildRawCSV(3, 4)), nil, &tid)
	require.NoError(t, err)
	cr3, err := cli.Commit("beta", "initial commit", "file.csv", testutils.RawCSVBytesReader(testutils.BuildRawCSV(3, 4)), nil, &tid)
	require.NoError(t, err)

	resp, err := cli.Request(http.MethodGet, fmt.Sprintf("/transactions/%s/", tid), nil, nil)
	require.NoError(t, err)
	b, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	assert.NotContains(t, string(b), `"end"`)
	gtr := &payload.GetTransactionResponse{}
	require.NoError(t, json.Unmarshal(b, gtr))
	assert.NotEmpty(t, gtr.Begin)
	assert.Empty(t, gtr.End)
	assert.Equal(t, []payload.TxBranch{
		{
			Name:       "alpha",
			CurrentSum: cr1.Sum.String(),
			NewSum:     cr2.Sum.String(),
		},
		{
			Name:   "beta",
			NewSum: cr3.Sum.String(),
		},
	}, gtr.Branches)

	resp, err = cli.DiscardTransaction(tid)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	_, err = cli.GetTransaction(tid)
	assert.Error(t, err)

	ctr, err = cli.CreateTransaction(nil)
	require.NoError(t, err)
	tid, err = uuid.Parse(ctr.ID)
	require.NoError(t, err)
	cr4, err := cli.Commit("alpha", "second commit", "file.csv", testutils.RawCSVBytesReader(testutils.BuildRawCSV(3, 4)), nil, &tid)
	require.NoError(t, err)
	cr5, err := cli.Commit("beta", "initial commit", "file.csv", testutils.RawCSVBytesReader(testutils.BuildRawCSV(3, 4)), nil, &tid)
	require.NoError(t, err)

	// setup webhook
	getWebhookPayload, cleanup := s.setupWebhook(t, repo, conf.CommitEventType)
	defer cleanup()

	resp, err = cli.CommitTransaction(tid)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	gtr, err = cli.GetTransaction(tid)
	require.NoError(t, err)
	assert.Equal(t, string(ref.TSCommitted), gtr.Status)
	assert.NotEmpty(t, gtr.End)

	// test webhook received commit event
	s.webhookWG.Wait()
	pl := getWebhookPayload()
	require.NotNil(t, pl)
	assert.Len(t, pl.Events, 1)
	ce := pl.Events[0].(*webhook.CommitEvent)
	sort.Slice(ce.Commits, func(i, j int) bool {
		return ce.Commits[i].Ref < ce.Commits[j].Ref
	})
	assert.Equal(t, &webhook.CommitEvent{
		Type:          conf.CommitEventType,
		TransactionID: tid.String(),
		Commits: []webhook.Commit{
			{
				Sum:     cr4.Sum.String(),
				Ref:     "heads/alpha",
				Message: fmt.Sprintf("commit [tx/%s]\nsecond commit", tid.String()),
			},
			{
				Sum:     cr5.Sum.String(),
				Ref:     "heads/beta",
				Message: fmt.Sprintf("commit [tx/%s]\ninitial commit", tid.String()),
			},
		},
		AuthorName:  server_testutils.Name,
		AuthorEmail: server_testutils.Email,
		Time:        ce.Time,
	}, ce)

	com1, err := cli.GetHead("alpha")
	require.NoError(t, err)
	assert.Equal(t, []*payload.Hex{
		cr1.Sum,
	}, com1.Parents)
	assert.Equal(t, cr4.Table, com1.Table.Sum)

	com2, err := cli.GetHead("beta")
	require.NoError(t, err)
	assert.Len(t, com2.Parents, 0)
	assert.Equal(t, cr5.Table, com2.Table.Sum)

	// test create transaction from payload
	req := &payload.CreateTransactionRequest{
		ID:     uuid.New().String(),
		Begin:  time.Now().Add(-time.Hour * 24),
		End:    time.Now(),
		Status: string(ref.TSCommitted),
	}
	ctr, err = cli.CreateTransaction(req)
	require.NoError(t, err)
	assert.Equal(t, req.ID, ctr.ID)
	id := uuid.Must(uuid.Parse(ctr.ID))
	tx, err := cli.GetTransaction(id)
	require.NoError(t, err)
	testutils.AssertTimeEqual(t, req.Begin, tx.Begin)
	testutils.AssertTimeEqual(t, req.End, *tx.End)
	assert.Equal(t, req.Status, tx.Status)
	assert.Len(t, tx.Branches, 0)

	_, err = cli.CreateTransaction(req)
	assert.Error(t, err)
}
