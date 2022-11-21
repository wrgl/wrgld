package server

import (
	"fmt"
	"net/http"
	"regexp"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"github.com/google/uuid"
	apiutils "github.com/wrgl/wrgl/pkg/api/utils"
	"github.com/wrgl/wrgl/pkg/conf"
	"github.com/wrgl/wrgl/pkg/objects"
	"github.com/wrgl/wrgl/pkg/ref"
	"github.com/wrgl/wrgl/pkg/router"
	"github.com/wrgl/wrgl/pkg/sorter"
	"github.com/wrgl/wrgld/pkg/webhook"
)

var (
	patRefs         *regexp.Regexp
	patHead         *regexp.Regexp
	patUploadPack   *regexp.Regexp
	patReceivePack  *regexp.Regexp
	patCommits      *regexp.Regexp
	patSum          *regexp.Regexp
	patProfile      *regexp.Regexp
	patTables       *regexp.Regexp
	patBlocks       *regexp.Regexp
	patRows         *regexp.Regexp
	patDiff         *regexp.Regexp
	patRootedBlocks *regexp.Regexp
	patRootedRows   *regexp.Regexp
	patObjects      *regexp.Regexp
	patTransactions *regexp.Regexp
	patUUID         *regexp.Regexp
	patGC           *regexp.Regexp
)

func init() {
	patRefs = regexp.MustCompile(`^/refs/`)
	patHead = regexp.MustCompile(`^heads/[-_0-9a-zA-Z]+/`)
	patUploadPack = regexp.MustCompile(`^/upload-pack/`)
	patReceivePack = regexp.MustCompile(`^/receive-pack/`)
	patCommits = regexp.MustCompile(`^/commits/`)
	patRootedBlocks = regexp.MustCompile(`^/blocks/`)
	patRootedRows = regexp.MustCompile(`^/rows/`)
	patSum = regexp.MustCompile(`^[0-9a-f]{32}/`)
	patTables = regexp.MustCompile(`^/tables/`)
	patProfile = regexp.MustCompile(`^profile/`)
	patBlocks = regexp.MustCompile(`^blocks/`)
	patRows = regexp.MustCompile(`^rows/`)
	patDiff = regexp.MustCompile(`^/diff/[0-9a-f]{32}/[0-9a-f]{32}/`)
	patObjects = regexp.MustCompile(`^/objects/`)
	patTransactions = regexp.MustCompile(`^/transactions/`)
	patUUID = regexp.MustCompile(`^[0-9a-f-]+/`)
	patGC = regexp.MustCompile(`^/gc/`)
}

type ServerOption func(s *Server)

func WithPostCommitCallback(postCommit PostCommitHook) ServerOption {
	return func(s *Server) {
		s.postCommit = postCommit
	}
}

func WithReceiverOptions(opts ...apiutils.ObjectReceiveOption) ServerOption {
	return func(s *Server) {
		s.receiverOpts = opts
	}
}

func WithWebhookSenderOptions(opts ...webhook.SenderOption) ServerOption {
	return func(s *Server) {
		s.webhookSenderOpts = opts
	}
}

type PostCommitHook func(r *http.Request, commit *objects.Commit, sum []byte, branch string, tid *uuid.UUID)

type Server struct {
	getDB             func(r *http.Request) objects.Store
	getRS             func(r *http.Request) ref.Store
	getConfig         func(r *http.Request) conf.Config
	getUpSession      func(r *http.Request) UploadPackSessionStore
	getRPSession      func(r *http.Request) ReceivePackSessionStore
	postCommit        PostCommitHook
	router            *router.Router
	maxAge            time.Duration
	logger            logr.Logger
	sPool             *sync.Pool
	receiverOpts      []apiutils.ObjectReceiveOption
	webhookSenderOpts []webhook.SenderOption
}

func NewServer(
	rootPath *regexp.Regexp,
	getDB func(r *http.Request) objects.Store,
	getRS func(r *http.Request) ref.Store,
	getConfS func(r *http.Request) conf.Config,
	getUpSession func(r *http.Request) UploadPackSessionStore,
	getRPSession func(r *http.Request) ReceivePackSessionStore,
	logger logr.Logger,
	opts ...ServerOption,
) *Server {
	s := &Server{
		getDB:        getDB,
		getRS:        getRS,
		getConfig:    getConfS,
		getUpSession: getUpSession,
		getRPSession: getRPSession,
		maxAge:       90 * 24 * time.Hour,
		logger:       logger,
		sPool: &sync.Pool{
			New: func() interface{} {
				s, err := sorter.NewSorter(sorter.WithRunSize(8 * 1024 * 1024))
				if err != nil {
					panic(err)
				}
				return s
			},
		},
	}
	s.router = router.NewRouter(rootPath, &router.Routes{
		Subs: []*router.Routes{
			{
				Pat: patTransactions,
				Subs: []*router.Routes{
					{
						Method:      http.MethodPost,
						HandlerFunc: s.handleCreateTransaction,
					},
					{
						Method:      http.MethodGet,
						Pat:         patUUID,
						HandlerFunc: s.handleGetTransaction,
					},
					{
						Method:      http.MethodPost,
						Pat:         patUUID,
						HandlerFunc: s.handleUpdateTransaction,
					},
				},
			},
			{
				Pat:         patGC,
				Method:      http.MethodPost,
				HandlerFunc: s.handleGC,
			},
			{
				Pat: patRefs,
				Subs: []*router.Routes{
					{
						Method:      http.MethodGet,
						HandlerFunc: s.handleGetRefs,
					},
					{
						Method:      http.MethodGet,
						Pat:         patHead,
						HandlerFunc: s.handleGetHead,
					},
				},
			},
			{
				Method:      http.MethodPost,
				Pat:         patUploadPack,
				HandlerFunc: s.handleUploadPack,
			},
			{
				Method:      http.MethodPost,
				Pat:         patReceivePack,
				HandlerFunc: s.handleReceivePack,
			},
			{
				Method:      http.MethodGet,
				Pat:         patRootedBlocks,
				HandlerFunc: s.handleGetBlocks,
			},
			{
				Method:      http.MethodGet,
				Pat:         patRootedRows,
				HandlerFunc: s.handleGetRows,
			},
			{
				Method:      http.MethodGet,
				Pat:         patObjects,
				HandlerFunc: s.handleGetObjects,
			},
			{
				Pat: patCommits,
				Subs: []*router.Routes{
					{
						Method:      http.MethodPost,
						HandlerFunc: s.handleCommit,
					},
					{
						Method:      http.MethodGet,
						HandlerFunc: s.handleGetCommits,
					},
					{
						Method:      http.MethodGet,
						Pat:         patSum,
						HandlerFunc: s.handleGetCommit,
						Subs: []*router.Routes{
							{
								Method:      http.MethodGet,
								Pat:         patProfile,
								HandlerFunc: s.handleGetCommitProfile,
							},
						},
					},
				}},
			{
				Pat: patTables,
				Subs: []*router.Routes{
					{
						Method:      http.MethodGet,
						Pat:         patSum,
						HandlerFunc: s.handleGetTable,
						Subs: []*router.Routes{
							{
								Method:      http.MethodGet,
								Pat:         patProfile,
								HandlerFunc: s.handleGetTableProfile,
							},
							{
								Method:      http.MethodGet,
								Pat:         patBlocks,
								HandlerFunc: s.handleGetTableBlocks,
							},
							{
								Method:      http.MethodGet,
								Pat:         patRows,
								HandlerFunc: s.handleGetTableRows,
							},
						}},
				}},
			{
				Method:      http.MethodGet,
				Pat:         patDiff,
				HandlerFunc: s.handleDiff,
			},
		},
	})
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func (s *Server) Ready() bool {
	return true
}

func (s *Server) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	s.router.ServeHTTP(rw, r)
}

func (s *Server) cacheControlImmutable(rw http.ResponseWriter) {
	rw.Header().Set(
		"Cache-Control",
		fmt.Sprintf("public, immutable, max-age=%d", int(s.maxAge.Seconds())),
	)
}
