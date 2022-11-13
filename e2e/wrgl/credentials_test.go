package e2e_wrgl_test

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	confhelpers "github.com/wrgl/wrgl/pkg/conf/helpers"
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
	// var httpErr *apiclient.HTTPError
	// if errors.As(exErr, &httpErr) {
	// 	t.Logf("exErr.Code: %s", spew.Sdump(httpErr.Code))
	// 	t.Logf("exErr.Body: %s", spew.Sdump(httpErr.Body))
	// 	t.Logf("exErr.RawBody: %s", spew.Sdump(httpErr.RawBody))
	// } else {
	// 	t.Logf("%v does not wrap http err", exErr)
	// }
	// if errors.As(err, &httpErr) {
	// 	t.Logf("err.Code: %s", spew.Sdump(httpErr.Code))
	// 	t.Logf("err.Body: %s", spew.Sdump(httpErr.Body))
	// 	t.Logf("err.RawBody: %s", spew.Sdump(httpErr.RawBody))
	// }
	assert.True(t, errors.Is(exErr, err) || exErr.Error() == err.Error(), "expecting error %q to contain error %q", exErr.Error(), err.Error())
	assert.Equal(t, output, buf.String())
}

func TestCredAuthCmd(t *testing.T) {
	defer confhelpers.MockGlobalConf(t, true)()

	rd, cleanUp := createRepoDir(t)
	defer cleanUp()
	ts := newTestServer(t, rd, "testCredAuth", false)
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
