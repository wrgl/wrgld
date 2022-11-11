package wrgld

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/go-logr/logr"
	authfs "github.com/wrgl/wrgl/pkg/auth/fs"
	"github.com/wrgl/wrgl/pkg/conf"
	conffs "github.com/wrgl/wrgl/pkg/conf/fs"
	"github.com/wrgl/wrgl/pkg/local"
	"github.com/wrgl/wrgl/pkg/objects"
	"github.com/wrgl/wrgl/pkg/ref"
	authlocal "github.com/wrgl/wrgld/pkg/auth/local"
	authoauth2 "github.com/wrgl/wrgld/pkg/auth/oauth2"
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
	srv        *http.Server
	cleanups   []func()
	upSessions *server.UploadPackSessionMap
	rpSessions *server.ReceivePackSessionMap
}

func NewServer(rd *local.RepoDir, readTimeout, writeTimeout time.Duration, client *http.Client, logger logr.Logger) (*Server, error) {
	objstore, err := rd.OpenObjectsStore()
	if err != nil {
		return nil, err
	}
	refstore := rd.OpenRefStore()
	cs := conffs.NewStore(rd.FullPath, conffs.AggregateSource, "")
	c, err := cs.Open()
	if err != nil {
		return nil, err
	}
	s := &Server{
		upSessions: server.NewUploadPackSessionMap(0, 0),
		rpSessions: server.NewReceivePackSessionMap(0, 0),
		srv: &http.Server{
			ReadTimeout:  readTimeout,
			WriteTimeout: writeTimeout,
		},
		cleanups: []func(){
			func() { rd.Close() },
			func() { objstore.Close() },
		},
	}
	var handler http.Handler = server.NewServer(
		nil,
		func(r *http.Request) objects.Store { return objstore },
		func(r *http.Request) ref.Store { return refstore },
		func(r *http.Request) conf.Store { return cs },
		func(r *http.Request) server.UploadPackSessionStore { return s.upSessions },
		func(r *http.Request) server.ReceivePackSessionStore { return s.rpSessions },
		logger,
	)
	if c.Auth == nil {
		return nil, fmt.Errorf("auth config not defined")
	}
	if c.Auth.Type == conf.ATLegacy {
		authnS, err := authfs.NewAuthnStore(rd, c.TokenDuration())
		if err != nil {
			return nil, err
		}
		authzS, err := authfs.NewAuthzStore(rd)
		if err != nil {
			return nil, err
		}
		handler = authlocal.NewHandler(handler, c, authnS, authzS)
		s.cleanups = append(s.cleanups,
			func() { authnS.Close() },
			func() { authzS.Close() },
		)
	} else {
		if c == nil || c.Auth == nil || c.Auth.OAuth2 == nil {
			return nil, fmt.Errorf("empty auth.oauth2 config")
		}
		if c.Auth.OAuth2.OIDCProvider == nil {
			return nil, fmt.Errorf("empty auth.oauth2.oidcProvider config")
		}
		provider, err := authoauth2.NewOIDCProvider(c.Auth.OAuth2.OIDCProvider, client)
		if err != nil {
			return nil, err
		}
		handler, err = authoauth2.NewHandler(handler, c, provider)
		if err != nil {
			return nil, err
		}
	}
	s.srv.Handler = wrgldutils.ApplyMiddlewares(
		handler,
		LoggingMiddleware,
		RecoveryMiddleware,
	)
	return s, nil
}

func (s *Server) Start(addr string) error {
	s.srv.Addr = addr
	log.Printf("server started at %s", addr)
	return s.srv.ListenAndServe()
}

func (s *Server) Close() error {
	s.upSessions.Stop()
	s.rpSessions.Stop()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := s.srv.Shutdown(ctx); err != nil {
		return err
	}
	for i := len(s.cleanups) - 1; i >= 0; i-- {
		s.cleanups[i]()
	}
	return nil
}
