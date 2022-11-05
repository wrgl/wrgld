package e2e_wrgl_test

import (
	"encoding/hex"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wrgl/wrgl/pkg/conf"
	confhelpers "github.com/wrgl/wrgl/pkg/conf/helpers"
	"github.com/wrgl/wrgl/pkg/factory"
	"github.com/wrgl/wrgl/pkg/ref"
	server_testutils "github.com/wrgl/wrgld/pkg/server/testutils"
)

func TestPullCmd(t *testing.T) {
	defer confhelpers.MockGlobalConf(t, true)()
	ts := server_testutils.NewServer(t, nil)
	defer ts.Close()
	repo, url, _, cleanup := ts.NewRemote(t, "")
	defer cleanup()
	dbs := ts.GetDB(repo)
	rss := ts.GetRS(repo)
	sum1, _ := factory.CommitRandom(t, dbs, nil)
	sum2, c2 := factory.CommitRandom(t, dbs, [][]byte{sum1})
	require.NoError(t, ref.CommitHead(rss, "main", sum2, c2, nil))
	sum4, c4 := factory.CommitRandom(t, dbs, nil)
	sum5, c5 := factory.CommitRandom(t, dbs, [][]byte{sum4})
	require.NoError(t, ref.CommitHead(rss, "beta", sum5, c5, nil))
	sum6, c6 := factory.CommitRandom(t, dbs, nil)
	require.NoError(t, ref.CommitHead(rss, "gamma", sum6, c6, nil))

	rd, cleanUp := createRepoDir(t)
	defer cleanUp()
	db, err := rd.OpenObjectsStore()
	require.NoError(t, err)
	rs := rd.OpenRefStore()
	factory.CopyCommitsToNewStore(t, dbs, db, [][]byte{sum1, sum4})
	require.NoError(t, ref.CommitHead(rs, "beta", sum4, c4, nil))
	require.NoError(t, db.Close())

	cmd := rootCmd()
	cmd.SetArgs([]string{"remote", "add", "my-repo", url})
	require.NoError(t, cmd.Execute())

	cmd = rootCmd()
	cmd.SetArgs([]string{"pull", "main", "my-repo", "refs/heads/main:refs/remotes/my-repo/main", "--set-upstream"})
	assertCmdUnauthorized(t, cmd, url)

	// pull set upstream
	authenticate(t, ts, url)
	cmd = rootCmd()
	cmd.SetArgs([]string{"pull", "main", "my-repo", "refs/heads/main:refs/remotes/my-repo/main", "--set-upstream"})
	require.NoError(t, cmd.Execute())

	db, err = rd.OpenObjectsStore()
	require.NoError(t, err)
	sum, err := ref.GetHead(rs, "main")
	require.NoError(t, err)
	assert.Equal(t, sum2, sum)
	require.NoError(t, db.Close())

	sum3, c3 := factory.CommitRandom(t, dbs, [][]byte{sum2})
	require.NoError(t, ref.CommitHead(rss, "main", sum3, c3, nil))

	// pull with upstream already set
	cmd = rootCmd()
	cmd.SetArgs([]string{"pull", "main"})
	require.NoError(t, cmd.Execute())

	db, err = rd.OpenObjectsStore()
	require.NoError(t, err)
	sum, err = ref.GetHead(rs, "main")
	require.NoError(t, err)
	assert.Equal(t, sum3, sum)
	require.NoError(t, db.Close())

	// pull merge first fetch refspec
	cmd = rootCmd()
	cmd.SetArgs([]string{"pull", "beta", "my-repo", "--no-progress"})
	assertCmdOutput(t, cmd, "Already up to date.\n")

	db, err = rd.OpenObjectsStore()
	require.NoError(t, err)
	sum, err = ref.GetHead(rs, "beta")
	require.NoError(t, err)
	assert.Equal(t, sum4, sum)
	require.NoError(t, db.Close())

	cmd = rootCmd()
	cmd.SetArgs([]string{"pull", "main", "--no-progress"})
	assertCmdOutput(t, cmd, "Already up to date.\n")

	// configure gamma upstream
	cmd = rootCmd()
	cmd.SetArgs([]string{"config", "set", "branch.gamma.remote", "my-repo"})
	require.NoError(t, cmd.Execute())
	cmd = rootCmd()
	cmd.SetArgs([]string{"config", "set", "branch.gamma.merge", "refs/heads/gamma"})
	require.NoError(t, cmd.Execute())

	// pull all branches with upstream configured
	cmd = rootCmd()
	cmd.SetArgs([]string{"pull", "--all", "--no-progress"})
	assertCmdOutput(t, cmd, strings.Join([]string{
		"1 out of 2 branches updated",
		fmt.Sprintf("[gamma %s] %s", hex.EncodeToString(sum6)[:7], c6.Message),
		"",
	}, "\n"))
	sum, err = ref.GetHead(rs, "gamma")
	require.NoError(t, err)
	assert.Equal(t, sum6, sum)

	// pull from public repo as an anynomous user
	sum7, c7 := factory.CommitRandom(t, dbs, [][]byte{sum3})
	require.NoError(t, ref.CommitHead(rss, "main", sum7, c7, nil))
	unauthenticate(t, url)
	cmd = rootCmd()
	cmd.SetArgs([]string{"pull", "main"})
	assertCmdUnauthorized(t, cmd, url)

	cs := ts.GetConfS(repo)
	c, err := cs.Open()
	require.NoError(t, err)
	c.Auth = &conf.Auth{
		AnonymousRead: true,
	}
	require.NoError(t, cs.Save(c))

	cmd = rootCmd()
	cmd.SetArgs([]string{"pull", "main", "--no-progress"})
	assertCmdOutput(t, cmd, strings.Join([]string{
		fmt.Sprintf("From %s", url),
		fmt.Sprintf("   %s..%s  main        -> my-repo/main", hex.EncodeToString(sum3)[:7], hex.EncodeToString(sum7)[:7]),
		fmt.Sprintf("Fast forward to %s", hex.EncodeToString(sum7)[:7]),
		"",
	}, "\n"))
	sum, err = ref.GetHead(rs, "main")
	require.NoError(t, err)
	assert.Equal(t, sum7, sum)
}
