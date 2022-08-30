package server_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apiclient "github.com/wrgl/wrgl/pkg/api/client"
	"github.com/wrgl/wrgl/pkg/api/payload"
	"github.com/wrgl/wrgl/pkg/factory"
	"github.com/wrgl/wrgl/pkg/objects"
	"github.com/wrgl/wrgl/pkg/ref"
)

func (s *testSuite) TestGetHead(t *testing.T) {
	repo, cli, m, cleanup := s.s.NewClient(t, "", true)
	defer cleanup()
	db := s.s.GetDB(repo)
	rs := s.s.GetRS(repo)
	parent, _ := factory.CommitRandom(t, db, nil)
	sum, com := factory.CommitRandom(t, db, [][]byte{parent})
	require.NoError(t, ref.CommitHead(rs, "main", sum, com, nil))
	tbl, err := objects.GetTable(db, com.Table)
	require.NoError(t, err)

	_, err = cli.GetHead("beta")
	assert.Error(t, err)

	cr, err := cli.GetHead("main")
	require.NoError(t, err)
	assert.Equal(t, &payload.Table{
		Sum:       payload.BytesToHex(com.Table),
		Columns:   tbl.Columns,
		RowsCount: tbl.RowsCount,
		PK:        tbl.PK,
		Exist:     true,
	}, cr.Table)
	assert.Equal(t, sum, cr.Sum[:])
	assert.Equal(t, com.AuthorName, cr.AuthorName)
	assert.Equal(t, com.AuthorEmail, cr.AuthorEmail)
	assert.Equal(t, com.Message, cr.Message)
	assert.Equal(t, com.Time.Format(time.RFC3339), cr.Time.Format(time.RFC3339))
	assert.Len(t, cr.Parents, 1)
	assert.Equal(t, com.Parents[0], (*cr.Parents[0])[:])

	// pass custom header
	req := m.Capture(t, func(header http.Header) {
		header.Set("Custom-Header", "123")
		cr, err = cli.GetHead("main", apiclient.WithRequestHeader(header))
		require.NoError(t, err)
		assert.NotEmpty(t, cr)
	})
	assert.Equal(t, "123", req.Header.Get("Custom-Header"))
}
