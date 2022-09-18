package server_testutils

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/golang-jwt/jwt"
	"github.com/pckhoi/uma"
	"github.com/stretchr/testify/require"
	apiclient "github.com/wrgl/wrgl/pkg/api/client"
	"github.com/wrgl/wrgl/pkg/auth"
	authtest "github.com/wrgl/wrgl/pkg/auth/test"
	"github.com/wrgl/wrgl/pkg/conf"
	confmock "github.com/wrgl/wrgl/pkg/conf/mock"
	"github.com/wrgl/wrgl/pkg/objects"
	objmock "github.com/wrgl/wrgl/pkg/objects/mock"
	"github.com/wrgl/wrgl/pkg/ref"
	refmock "github.com/wrgl/wrgl/pkg/ref/mock"
	"github.com/wrgl/wrgl/pkg/testutils"
	wrgldoapiserver "github.com/wrgl/wrgld/pkg/oapi/server"
	"github.com/wrgl/wrgld/pkg/server"
)

const (
	Email    = "test@user.com"
	Password = "password"
	Name     = "Test User"
)

type Claims struct {
	jwt.StandardClaims
	Email  string   `json:"email,omitempty"`
	Name   string   `json:"name,omitempty"`
	Scopes []string `json:"scopes,omitempty"`
}

type repoKey struct{}

func setRepo(r *http.Request, repo string) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), repoKey{}, repo))
}

func getRepo(r *http.Request) string {
	if i := r.Context().Value(repoKey{}); i != nil {
		return i.(string)
	}
	return ""
}

type Server struct {
	dbMx       sync.Mutex
	rsMx       sync.Mutex
	azMx       sync.Mutex
	cMx        sync.Mutex
	upMx       sync.Mutex
	rpMx       sync.Mutex
	db         map[string]objects.Store
	rs         map[string]ref.Store
	authzS     map[string]auth.AuthzStore
	confS      map[string]conf.Store
	upSessions map[string]*server.UploadPackSessionMap
	rpSessions map[string]*server.ReceivePackSessionMap
	s          *server.Server
	T          *testing.T
	cleanups   []func()
}

func (s *Server) Close() {
	wg := sync.WaitGroup{}
	for _, f := range s.cleanups {
		wg.Add(1)
		f := f
		go func() {
			f()
			wg.Done()
		}()
	}
	wg.Wait()
}

func NewServer(t *testing.T, rootPath *regexp.Regexp, opts ...server.ServerOption) *Server {
	ts := &Server{
		db:         map[string]objects.Store{},
		rs:         map[string]ref.Store{},
		authzS:     map[string]auth.AuthzStore{},
		confS:      map[string]conf.Store{},
		upSessions: map[string]*server.UploadPackSessionMap{},
		rpSessions: map[string]*server.ReceivePackSessionMap{},
		T:          t,
	}
	ts.s = server.NewServer(
		rootPath,
		func(r *http.Request) objects.Store {
			return ts.GetDB(getRepo(r))
		},
		func(r *http.Request) ref.Store {
			return ts.GetRS(getRepo(r))
		},
		func(r *http.Request) conf.Store {
			return ts.GetConfS(getRepo(r))
		},
		func(r *http.Request) server.UploadPackSessionStore {
			return ts.GetUpSessions(getRepo(r))
		},
		func(r *http.Request) server.ReceivePackSessionStore {
			return ts.GetRpSessions(getRepo(r))
		},
		opts...,
	)
	return ts
}

func (s *Server) GetAuthzS(repo string) auth.AuthzStore {
	s.azMx.Lock()
	defer s.azMx.Unlock()
	if _, ok := s.authzS[repo]; !ok {
		s.authzS[repo] = authtest.NewAuthzStore()
	}
	return s.authzS[repo]
}

func (s *Server) GetDB(repo string) objects.Store {
	s.dbMx.Lock()
	defer s.dbMx.Unlock()
	if _, ok := s.db[repo]; !ok {
		db := objmock.NewStore()
		s.db[repo] = db
		s.cleanups = append(s.cleanups, func() {
			require.NoError(s.T, db.Close())
		})
	}
	return s.db[repo]
}

func (s *Server) GetRS(repo string) ref.Store {
	s.rsMx.Lock()
	defer s.rsMx.Unlock()
	if _, ok := s.rs[repo]; !ok {
		var cleanup func()
		s.rs[repo], cleanup = refmock.NewStore(s.T)
		s.cleanups = append(s.cleanups, cleanup)
	}
	return s.rs[repo]
}

func (s *Server) GetConfS(repo string) conf.Store {
	s.cMx.Lock()
	defer s.cMx.Unlock()
	if _, ok := s.confS[repo]; !ok {
		s.confS[repo] = &confmock.Store{}
	}
	return s.confS[repo]
}

func (s *Server) GetUpSessions(repo string) server.UploadPackSessionStore {
	s.upMx.Lock()
	defer s.upMx.Unlock()
	if _, ok := s.upSessions[repo]; !ok {
		ses := server.NewUploadPackSessionMap(100*time.Millisecond, 0)
		s.upSessions[repo] = ses
		s.cleanups = append(s.cleanups, ses.Stop)
	}
	return s.upSessions[repo]
}

func (s *Server) GetRpSessions(repo string) server.ReceivePackSessionStore {
	s.rpMx.Lock()
	defer s.rpMx.Unlock()
	if _, ok := s.rpSessions[repo]; !ok {
		ses := server.NewReceivePackSessionMap(100*time.Millisecond, 0)
		s.rpSessions[repo] = ses
		s.cleanups = append(s.cleanups, ses.Stop)
	}
	return s.rpSessions[repo]
}

func (s *Server) Authorize(t *testing.T, email, name string, scopes ...string) (signedToken string) {
	t.Helper()
	tok := jwt.NewWithClaims(jwt.SigningMethodNone, &Claims{
		Email:  email,
		Name:   name,
		Scopes: scopes,
	})
	signedToken, err := tok.SignedString(jwt.UnsafeAllowNoneSignatureType)
	require.NoError(t, err)
	return
}

func (s *Server) AdminToken(t *testing.T) (signedToken string) {
	return s.Authorize(t, Email, Name, "read", "write")
}

func (s *Server) NewRemote(t *testing.T, pathPrefix string) (repo string, uri string, m *RequestCaptureMiddleware, cleanup func()) {
	t.Helper()
	repo = testutils.BrokenRandomLowerAlphaString(6)
	cs := s.GetConfS(repo)
	c, err := cs.Open()
	require.NoError(t, err)
	c.User = &conf.User{
		Email: Email,
		Name:  Name,
	}
	require.NoError(t, cs.Save(c))
	m = NewRequestCaptureMiddleware(&GZIPAwareHandler{
		T: t,
		HandlerFunc: func(rw http.ResponseWriter, r *http.Request) {
			r = setRepo(r, repo)
			s.s.ServeHTTP(rw, r)
		},
	})
	umaMan := wrgldoapiserver.UMAManager(uma.ManagerOptions{
		GetBaseURL: func(r *http.Request) url.URL {
			return url.URL{
				Scheme: "http",
				Host:   r.Host,
				Path:   pathPrefix,
			}
		},
		LocalEnforce: func(r *http.Request, resource uma.Resource, scopes []string) bool {
			var existingScopes []string
			if authHeader := r.Header.Get("Authorization"); authHeader != "" {
				claims := &Claims{}
				_, err := jwt.ParseWithClaims(
					strings.TrimPrefix(authHeader, "Bearer "), claims,
					func(t *jwt.Token) (interface{}, error) { return jwt.UnsafeAllowNoneSignatureType, nil },
				)
				require.NoError(t, err)
				existingScopes = claims.Scopes
			} else {
				cs := s.GetConfS(repo)
				c, _ := cs.Open()
				if c.Auth != nil && c.Auth.AnonymousRead {
					existingScopes = []string{"read"}
				} else {
					return false
				}
			}
		outer:
			for _, scope := range scopes {
				for _, s := range existingScopes {
					if scope == s {
						continue outer
					}
				}
				return false
			}
			return true
		},
		EditUnauthorizedResponse: func(rw http.ResponseWriter) {
			rw.Header().Add("Content-Type", "application/json")
			rw.WriteHeader(http.StatusUnauthorized)
			rw.Write([]byte(`{"message":"Unauthorized"}`))
		},
	})
	var handler http.Handler = ApplyMiddlewares(
		m,
		func(h http.Handler) http.Handler {
			return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
				if s := r.Header.Get("Authorization"); s != "" {
					claims := &Claims{}
					_, err := jwt.ParseWithClaims(
						strings.TrimPrefix(s, "Bearer "), claims,
						func(t *jwt.Token) (interface{}, error) { return jwt.UnsafeAllowNoneSignatureType, nil },
					)
					require.NoError(t, err)
					r = server.SetAuthor(r, &server.Author{
						Email: claims.Email,
						Name:  claims.Name,
					})
				}
				h.ServeHTTP(rw, r)
			})
		},
		umaMan.Middleware,
	)
	if pathPrefix != "" {
		mux := http.NewServeMux()
		mux.Handle(pathPrefix, handler)
		handler = mux
	}
	ts := httptest.NewServer(handler)
	return repo, strings.TrimSuffix(ts.URL+pathPrefix, "/"), m, ts.Close
}

func (s *Server) NewClient(t *testing.T, pathPrefix string, authorized bool) (string, *apiclient.Client, *RequestCaptureMiddleware, func()) {
	t.Helper()
	repo, url, m, cleanup := s.NewRemote(t, pathPrefix)
	var opts []apiclient.ClientOption
	if authorized {
		opts = append(opts, apiclient.WithRelyingPartyToken(s.AdminToken(t)))
	}
	cli, err := apiclient.NewClient(url, opts...)
	require.NoError(t, err)
	return repo, cli, m, cleanup
}
