package e2e_wrgl_test

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/go-logr/logr/testr"
	"github.com/pckhoi/uma"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
	"github.com/wrgl/wrgl/cmd/wrgl"
	"github.com/wrgl/wrgl/cmd/wrgl/utils"
	"github.com/wrgl/wrgl/pkg/conf"
	conffs "github.com/wrgl/wrgl/pkg/conf/fs"
	"github.com/wrgl/wrgl/pkg/credentials"
	"github.com/wrgl/wrgl/pkg/local"
	wrgldcmd "github.com/wrgl/wrgld/cmd"
	"gopkg.in/dnaeon/go-vcr.v3/recorder"
)

type wrappedHandler struct {
	h http.Handler
}

func (h *wrappedHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	h.h.ServeHTTP(rw, r)
}

type testServer struct {
	rec       *recorder.Recorder
	srv       *wrgldcmd.Server
	ts        *httptest.Server
	URL       string
	Client    *http.Client
	updateVCR bool
}

func newTestServer(t *testing.T, rd *local.RepoDir, cassetteName string, updateVCR bool) *testServer {
	mode := recorder.ModeRecordOnce
	if updateVCR {
		mode = recorder.ModeRecordOnly
	}
	rec, err := recorder.NewWithOptions(&recorder.Options{
		CassetteName:       "testdata/go-vcr/" + cassetteName,
		Mode:               mode,
		SkipRequestLatency: true,
	})
	require.NoError(t, err)
	rec.AddPassthrough(func(req *http.Request) bool {
		return req.Host != "localhost:8080"
	})

	cs := conffs.NewStore(rd.FullPath, conffs.LocalSource, "")
	c, err := cs.Open()
	require.NoError(t, err)
	c.Auth = &conf.Auth{
		Keycloak: &conf.AuthKeycloak{
			Issuer:       "http://localhost:8080/realms/test-realm",
			ClientID:     "wrgld",
			ClientSecret: "change-me",
		},
		RepositoryName: "my repo",
	}
	handler := &wrappedHandler{}
	ts := httptest.NewServer(handler)
	c.BaseURL = ts.URL
	logger := testr.NewWithOptions(t, testr.Options{
		Verbosity: 1,
	})
	srv, kp, resourceID, err := wrgldcmd.NewServer(rd, rec.GetDefaultClient(), c, logger, true)
	require.NoError(t, err)
	handler.h = srv

	_, err = kp.CreatePermissionForResource(resourceID, &uma.KcPermission{
		Name:        "reader-read-" + resourceID,
		Description: "reader can read",
		Scopes:      []string{"read"},
		Roles:       []string{"reader"},
	})
	require.NoError(t, err)
	_, err = kp.CreatePermissionForResource(resourceID, &uma.KcPermission{
		Name:        "writer-write-" + resourceID,
		Description: "writer can write",
		Scopes:      []string{"write"},
		Roles:       []string{"writer"},
	})
	require.NoError(t, err)

	return &testServer{
		rec:       rec,
		srv:       srv,
		URL:       ts.URL,
		ts:        ts,
		updateVCR: updateVCR,
		Client:    rec.GetDefaultClient(),
	}
}

func rootCmd() *cobra.Command {
	cmd := wrgl.RootCmd()
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	return cmd
}

func (s *testServer) RunAuthenticate(t *testing.T, args ...string) {
	t.Helper()
	cmd := rootCmd()
	out := bytes.NewBuffer(nil)
	cmd.SetOut(out)
	cmd.SetArgs(args)
	if s.updateVCR {
		instrChan := readLoginInstruction(t, out)
		go userVerify(t, instrChan)
	}
	logger := testr.NewWithOptions(t, testr.Options{
		Verbosity: 1,
	})
	require.NoError(t, cmd.ExecuteContext(utils.SetLogger(
		utils.SetClient(context.Background(), s.Client),
		&logger,
	)))
}

func (s *testServer) GetCurrentToken(t *testing.T) []byte {
	t.Helper()
	cs, err := credentials.NewStore()
	require.NoError(t, err)
	u, err := url.Parse(s.URL)
	require.NoError(t, err)
	tok := cs.GetRPT(*u)
	require.NotEmpty(t, tok)
	return []byte(tok)
}

func (s *testServer) Stop(t *testing.T) {
	s.ts.Close()
	require.NoError(t, s.srv.Close())
	require.NoError(t, s.rec.Stop())
}
