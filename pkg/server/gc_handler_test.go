package server_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wrgl/wrgl/pkg/conf"
	"github.com/wrgl/wrgl/pkg/factory"
	"github.com/wrgl/wrgl/pkg/objects"
)

func (s *testSuite) TestGCHandler(t *testing.T) {
	repo, cli, _, cleanup := s.s.NewClient(t, "", true)
	defer cleanup()
	db := s.s.GetDB(repo)
	rs := s.s.GetRS(repo)

	sum, _ := factory.CommitRandom(t, db, nil)
	ctr, err := cli.CreateTransaction(nil)
	require.NoError(t, err)
	tid, err := uuid.Parse(ctr.ID)
	require.NoError(t, err)

	cs := s.s.GetConfS(repo)
	c, err := cs.Open()
	require.NoError(t, err)
	c.TransactionTTL = conf.Duration(time.Second)
	require.NoError(t, cs.Save(c))

	time.Sleep(time.Second)

	_, err = cli.GarbageCollect()
	require.NoError(t, err)

	assert.False(t, objects.CommitExist(db, sum))
	_, err = rs.GetTransaction(tid)
	assert.Error(t, err)
}
