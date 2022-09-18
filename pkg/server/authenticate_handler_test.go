package server_test

import (
	"bytes"
	"encoding/csv"
	"encoding/hex"
	"net/http"
	"regexp"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apiclient "github.com/wrgl/wrgl/pkg/api/client"
	"github.com/wrgl/wrgl/pkg/api/payload"
	"github.com/wrgl/wrgl/pkg/factory"
	"github.com/wrgl/wrgl/pkg/ref"
	"github.com/wrgl/wrgl/pkg/testutils"
	server_testutils "github.com/wrgl/wrgld/pkg/server/testutils"
)

func authHeader(token string) http.Header {
	return map[string][]string{
		"Authorization": {"Bearer " + token},
	}
}

func (s *testSuite) TestAuthenticate(t *testing.T) {
	srv := server_testutils.NewServer(t, regexp.MustCompile(`^/my-repo/`))
	defer srv.Close()
	repo, cli, _, cleanup := srv.NewClient(t, "/my-repo/", false)
	defer cleanup()
	db := srv.GetDB(repo)
	rs := srv.GetRS(repo)
	sum1, _ := factory.CommitRandom(t, db, nil)
	sum2, com := factory.CommitRandom(t, db, [][]byte{sum1})
	require.NoError(t, ref.CommitHead(rs, "main", sum2, com, nil))
	email := "user@test.com"
	name := "John Doe"

	tok := s.s.Authorize(t, email, name)

	buf := bytes.NewBuffer(nil)
	w := csv.NewWriter(buf)
	require.NoError(t, w.WriteAll(testutils.BuildRawCSV(4, 4)))
	w.Flush()
	// nothing come through because user has no scope
	_, err := cli.Commit("alpha", "initial commit", "file.csv", bytes.NewReader(buf.Bytes()), nil, nil)
	assert.Error(t, err)
	_, err = cli.Commit("alpha", "initial commit", "file.csv", bytes.NewReader(buf.Bytes()), nil, nil, apiclient.WithRequestHeader(authHeader(tok)))
	assert.Error(t, err)
	_, err = cli.GetCommit(sum2)
	assert.Error(t, err)
	_, err = cli.GetCommit(sum2, apiclient.WithRequestHeader(authHeader(tok)))
	assert.Error(t, err)
	_, err = cli.GetCommitProfile(sum2)
	assert.Error(t, err)
	_, err = cli.GetCommitProfile(sum2, apiclient.WithRequestHeader(authHeader(tok)))
	assert.Error(t, err)
	_, err = cli.GetTable(com.Table)
	assert.Error(t, err)
	_, err = cli.GetTable(com.Table, apiclient.WithRequestHeader(authHeader(tok)))
	assert.Error(t, err)
	_, err = cli.GetTableProfile(com.Table)
	assert.Error(t, err)
	_, err = cli.GetTableProfile(com.Table, apiclient.WithRequestHeader(authHeader(tok)))
	assert.Error(t, err)
	_, err = cli.GetBlocks(hex.EncodeToString(sum2), 0, 0, payload.BlockFormatCSV, false)
	assert.Error(t, err)
	_, err = cli.GetBlocks(hex.EncodeToString(sum2), 0, 0, payload.BlockFormatCSV, false, apiclient.WithRequestHeader(authHeader(tok)))
	assert.Error(t, err)
	_, err = cli.GetTableBlocks(com.Table, 0, 0, payload.BlockFormatCSV, false)
	assert.Error(t, err)
	_, err = cli.GetTableBlocks(com.Table, 0, 0, payload.BlockFormatCSV, false, apiclient.WithRequestHeader(authHeader(tok)))
	assert.Error(t, err)
	_, err = cli.GetTableRows(com.Table, []int{0})
	assert.Error(t, err)
	_, err = cli.GetTableRows(com.Table, []int{0}, apiclient.WithRequestHeader(authHeader(tok)))
	assert.Error(t, err)
	_, err = cli.GetRows(hex.EncodeToString(sum2), []int{0})
	assert.Error(t, err)
	_, err = cli.GetRows(hex.EncodeToString(sum2), []int{0}, apiclient.WithRequestHeader(authHeader(tok)))
	assert.Error(t, err)
	_, err = cli.Diff(sum1, sum2)
	assert.Error(t, err)
	_, err = cli.Diff(sum1, sum2, apiclient.WithRequestHeader(authHeader(tok)))
	assert.Error(t, err)
	_, err = cli.GetRefs(nil, nil)
	assert.Error(t, err)
	_, err = cli.GetRefs(nil, nil, apiclient.WithRequestHeader(authHeader(tok)))
	assert.Error(t, err)
	_, _, _, err = cli.PostUploadPack(&payload.UploadPackRequest{
		Wants: payload.BytesSliceToHexSlice([][]byte{sum2}),
		Done:  true,
	})
	assert.Error(t, err)
	_, _, _, err = cli.PostUploadPack(&payload.UploadPackRequest{
		Wants: payload.BytesSliceToHexSlice([][]byte{sum2}),
		Done:  true,
	}, apiclient.WithRequestHeader(authHeader(tok)))
	assert.Error(t, err)
	_, err = cli.PostReceivePack(map[string]*payload.Update{"main": {OldSum: payload.BytesToHex(sum2)}}, nil)
	assert.Error(t, err)
	_, err = cli.PostReceivePack(map[string]*payload.Update{"main": {OldSum: payload.BytesToHex(sum2)}}, nil, apiclient.WithRequestHeader(authHeader(tok)))
	assert.Error(t, err)
	_, err = cli.GarbageCollect()
	assert.Error(t, err)
	_, err = cli.GarbageCollect(apiclient.WithRequestHeader(authHeader(tok)))
	assert.Error(t, err)

	// only read actions come through
	readTok := s.s.Authorize(t, email, name, "read")
	_, err = cli.Commit("alpha", "initial commit", "file.csv", bytes.NewReader(buf.Bytes()), nil, nil, apiclient.WithRequestHeader(authHeader(readTok)))
	assert.Error(t, err)
	gcr, err := cli.GetCommit(sum2, apiclient.WithRequestHeader(authHeader(readTok)))
	require.NoError(t, err)
	assert.NotEmpty(t, gcr.Table)
	tProf, err := cli.GetCommitProfile(sum2, apiclient.WithRequestHeader(authHeader(readTok)))
	require.NoError(t, err)
	assert.NotEmpty(t, tProf.RowsCount)
	tr, err := cli.GetTable(com.Table, apiclient.WithRequestHeader(authHeader(readTok)))
	require.NoError(t, err)
	assert.NotEmpty(t, tr.Columns)
	tProf, err = cli.GetTableProfile(com.Table, apiclient.WithRequestHeader(authHeader(readTok)))
	require.NoError(t, err)
	assert.NotEmpty(t, tProf.RowsCount)
	resp, err := cli.GetBlocks(hex.EncodeToString(sum2), 0, 0, payload.BlockFormatCSV, false, apiclient.WithRequestHeader(authHeader(readTok)))
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	resp, err = cli.GetTableBlocks(com.Table, 0, 0, payload.BlockFormatCSV, false, apiclient.WithRequestHeader(authHeader(readTok)))
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	resp, err = cli.GetRows(hex.EncodeToString(sum2), []int{0}, apiclient.WithRequestHeader(authHeader(readTok)))
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	resp, err = cli.GetTableRows(com.Table, []int{0}, apiclient.WithRequestHeader(authHeader(readTok)))
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	dr, err := cli.Diff(sum1, sum2, apiclient.WithRequestHeader(authHeader(readTok)))
	require.NoError(t, err)
	assert.NotEmpty(t, dr.Columns)
	assert.NotEmpty(t, dr.OldColumns)
	assert.NotEmpty(t, dr.PK)
	assert.NotEmpty(t, dr.OldPK)
	refs, err := cli.GetRefs(nil, nil, apiclient.WithRequestHeader(authHeader(readTok)))
	require.NoError(t, err)
	assert.Greater(t, len(refs), 0)
	_, _, _, err = cli.PostUploadPack(&payload.UploadPackRequest{
		Wants: payload.BytesSliceToHexSlice([][]byte{sum2}),
		Done:  true,
	}, apiclient.WithRequestHeader(authHeader(readTok)))
	require.NoError(t, err)
	_, err = cli.PostReceivePack(map[string]*payload.Update{"main": {OldSum: payload.BytesToHex(sum2)}}, nil, apiclient.WithRequestHeader(authHeader(readTok)))
	assert.Error(t, err)
	_, err = cli.GarbageCollect(apiclient.WithRequestHeader(authHeader(readTok)))
	assert.Error(t, err)

	// now write actions come through as well
	writeTok := s.s.Authorize(t, email, name, "read", "write")
	cr, err := cli.Commit("alpha", "initial commit", "file.csv", bytes.NewReader(buf.Bytes()), nil, nil, apiclient.WithRequestHeader(authHeader(writeTok)))
	require.NoError(t, err)
	assert.NotEmpty(t, cr.Sum)
	resp, err = cli.PostReceivePack(map[string]*payload.Update{"main": {OldSum: payload.BytesToHex(sum2)}}, nil, apiclient.WithRequestHeader(authHeader(writeTok)))
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	_, err = cli.GarbageCollect(apiclient.WithRequestHeader(authHeader(writeTok)))
	require.NoError(t, err)

	// test scopes on transaction handlers
	_, err = cli.CreateTransaction(nil)
	assert.Error(t, err)
	_, err = cli.CreateTransaction(nil, apiclient.WithRequestHeader(authHeader(tok)))
	assert.Error(t, err)
	_, err = cli.CreateTransaction(nil, apiclient.WithRequestHeader(authHeader(readTok)))
	assert.Error(t, err)
	ctr, err := cli.CreateTransaction(nil, apiclient.WithRequestHeader(authHeader(writeTok)))
	require.NoError(t, err)
	tid, err := uuid.Parse(ctr.ID)
	require.NoError(t, err)
	_, err = cli.GetTransaction(tid)
	assert.Error(t, err)
	_, err = cli.GetTransaction(tid, apiclient.WithRequestHeader(authHeader(tok)))
	assert.Error(t, err)
	_, err = cli.GetTransaction(tid, apiclient.WithRequestHeader(authHeader(readTok)))
	require.NoError(t, err)
	_, err = cli.GetTransaction(tid, apiclient.WithRequestHeader(authHeader(writeTok)))
	require.NoError(t, err)
	_, err = cli.DiscardTransaction(tid)
	assert.Error(t, err)
	_, err = cli.DiscardTransaction(tid, apiclient.WithRequestHeader(authHeader(tok)))
	assert.Error(t, err)
	_, err = cli.DiscardTransaction(tid, apiclient.WithRequestHeader(authHeader(readTok)))
	assert.Error(t, err)
	_, err = cli.DiscardTransaction(tid, apiclient.WithRequestHeader(authHeader(writeTok)))
	require.NoError(t, err)
}
