package e2e_wrgl_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	confhelpers "github.com/wrgl/wrgl/pkg/conf/helpers"
	"github.com/wrgl/wrgl/pkg/errors"
	"github.com/wrgl/wrgl/pkg/local"
	"github.com/wrgl/wrgl/pkg/testutils"
)

func createRepoDir(t *testing.T) (rd *local.RepoDir, cleanup func()) {
	t.Helper()
	rootDir, err := testutils.TempDir("", "test_wrgl_*")
	require.NoError(t, err)
	wrglDir := filepath.Join(rootDir, ".wrgl")
	rd, err = local.NewRepoDir(wrglDir, "")
	require.NoError(t, err)
	err = rd.Init()
	require.NoError(t, err)
	viper.Set("wrgl_dir", wrglDir)
	cmd := rootCmd()
	cmd.SetArgs([]string{"config", "set", "user.email", "john@domain.com"})
	require.NoError(t, cmd.Execute())
	cmd.SetArgs([]string{"config", "set", "user.name", "John Doe"})
	require.NoError(t, cmd.Execute())
	return rd, func() { os.RemoveAll(rootDir) }
}

func assertCmdOutput(t *testing.T, cmd *cobra.Command, output string) {
	t.Helper()
	buf := bytes.NewBufferString("")
	cmd.SetOut(buf)
	err := cmd.Execute()
	assert.Equal(t, output, buf.String())
	require.NoError(t, err)
}

func assertCmdFailed(t *testing.T, cmd *cobra.Command, output string, err error) {
	t.Helper()
	buf := bytes.NewBufferString("")
	cmd.SetOut(buf)
	exErr := cmd.Execute()
	assert.True(t, errors.Contains(exErr, err), "expecting error %v to contain error %v", exErr, err)
	assert.Equal(t, output, buf.String())
}

func TestCredAuthCmd(t *testing.T) {
	defer confhelpers.MockGlobalConf(t, true)()

	rd, cleanUp := createRepoDir(t)
	defer cleanUp()
	ts := newTestServer(t, rd, "testdata/go-vcr/testCredAuth", false)
	defer ts.Stop(t)

	cmd := rootCmd()
	cmd.SetArgs([]string{"remote", "add", "origin", ts.URL})
	require.NoError(t, cmd.Execute())

	ts.RunAuthenticate(t, "credentials", "print", "origin")
	rpt := ts.GetCurrentToken(t)

	cmd = rootCmd()
	cmd.SetArgs([]string{"credentials", "list"})
	assertCmdOutput(t, cmd, strings.Join([]string{
		ts.URL,
		"",
	}, "\n"))

	cmd = rootCmd()
	cmd.SetArgs([]string{"credentials", "remove", ts.URL})
	require.NoError(t, cmd.Execute())

	cmd = rootCmd()
	cmd.SetArgs([]string{"credentials", "list"})
	assertCmdOutput(t, cmd, "")

	tokFile := filepath.Join(t.TempDir(), "tok.txt")
	require.NoError(t, os.WriteFile(tokFile, []byte(rpt), 0644))
	cmd = rootCmd()
	cmd.SetArgs([]string{"credentials", "authenticate", ts.URL, tokFile})
	require.NoError(t, cmd.Execute())

	cmd = rootCmd()
	cmd.SetArgs([]string{"credentials", "list"})
	assertCmdOutput(t, cmd, strings.Join([]string{
		ts.URL,
		"",
	}, "\n"))
}
