package wrgld

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/pckhoi/uma"
	"github.com/wrgl/wrgl/pkg/conf"
	conffs "github.com/wrgl/wrgl/pkg/conf/fs"
	"github.com/wrgl/wrgl/pkg/local"
	"github.com/wrgl/wrgl/pkg/objects"
	"github.com/wrgl/wrgl/pkg/ref"
	wrgldoapiserver "github.com/wrgl/wrgld/pkg/oapi/server"
	"github.com/wrgl/wrgld/pkg/server"
	wrgldutils "github.com/wrgl/wrgld/pkg/utils"
)

type ServerOptions struct {
	ObjectsStore objects.Store

	RefStore ref.Store

	ConfigStore conf.Store

	ReadTimeout  time.Duration
	WriteTimeout time.Duration
}

type Server struct {
	handler    http.Handler
	cleanups   []func()
	upSessions *server.UploadPackSessionMap
	rpSessions *server.ReceivePackSessionMap
}

func NewServer(rd *local.RepoDir, client *http.Client) (*Server, *uma.KeycloakProvider, string, error) {
	objstore, err := rd.OpenObjectsStore()
	if err != nil {
		return nil, nil, "", err
	}
	refstore := rd.OpenRefStore()
	cs := conffs.NewStore(rd.FullPath, conffs.AggregateSource, "")
	c, err := cs.Open()
	if err != nil {
		return nil, nil, "", err
	}
	s := &Server{
		upSessions: server.NewUploadPackSessionMap(0, 0),
		rpSessions: server.NewReceivePackSessionMap(0, 0),
		cleanups: []func(){
			func() { rd.Close() },
			func() { objstore.Close() },
		},
	}
	if c.Auth == nil || c.Auth.Keycloak == nil {
		return nil, nil, "", fmt.Errorf("auth config not defined")
	}
	if c.Auth.RepositoryName == "" {
		return nil, nil, "", fmt.Errorf("auth.repositoryName not defined")
	}
	rs := rd.OpenUMAStore()
	kc := c.Auth.Keycloak
	ctx := context.Background()
	opts := []uma.KeycloakOption{
		uma.WithKeycloakOwnerManagedAccess(),
	}
	if client != nil {
		ctx = oidc.ClientContext(ctx, client)
		opts = append(opts, uma.WithKeycloakClient(client))
	}
	kp, err := uma.NewKeycloakProvider(
		kc.Issuer,
		kc.ClientID,
		kc.ClientSecret,
		oidc.NewRemoteKeySet(ctx, kc.Issuer+"/protocol/openid-connect/certs"),
		opts...,
	)
	if err != nil {
		return nil, nil, "", err
	}
	baseURL, err := url.Parse(c.BaseURL)
	if err != nil {
		return nil, nil, "", err
	}
	manOpts := &uma.ManagerOptions{
		GetBaseURL: func(r *http.Request) url.URL {
			return *baseURL
		},
		GetProvider: func(r *http.Request) uma.Provider {
			return kp
		},
		GetResourceStore: func(r *http.Request) uma.ResourceStore {
			return rs
		},
		GetResourceName: func(rsc uma.Resource) string {
			return c.Auth.RepositoryName
		},
		EditUnauthorizedResponse: func(rw http.ResponseWriter) {
			rw.Header().Add("Content-Type", "application/json")
			rw.WriteHeader(http.StatusUnauthorized)
			rw.Write([]byte(`{"message":"Unauthorized"}`))
		},
	}
	if c.Auth.AnonymousRead {
		manOpts.AnonymousScopes = func(resource uma.Resource) (scopes []string) {
			return []string{"read"}
		}
	}
	umaMan := wrgldoapiserver.UMAManager(*manOpts)

	var resourceID string
	resourceID, err = rs.Get(c.Auth.RepositoryName)
	if err != nil {
		resp, err := umaMan.RegisterResourceAt(rs, kp, *baseURL, "/refs")
		if err != nil {
			return nil, nil, "", err
		}
		resourceID = resp.ID
	}
	var handler http.Handler = server.NewServer(
		nil,
		func(r *http.Request) objects.Store { return objstore },
		func(r *http.Request) ref.Store { return refstore },
		func(r *http.Request) conf.Store { return cs },
		func(r *http.Request) server.UploadPackSessionStore { return s.upSessions },
		func(r *http.Request) server.ReceivePackSessionStore { return s.rpSessions },
	)
	s.handler = wrgldutils.ApplyMiddlewares(
		handler,
		umaMan.Middleware,
		LoggingMiddleware,
		RecoveryMiddleware,
	)
	return s, kp, resourceID, nil
}

func (s *Server) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	s.handler.ServeHTTP(rw, r)
}

func (s *Server) Close() error {
	s.upSessions.Stop()
	s.rpSessions.Stop()
	for i := len(s.cleanups) - 1; i >= 0; i-- {
		s.cleanups[i]()
	}
	return nil
}
