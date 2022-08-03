package server

import (
	"context"
	"net/http"
	"regexp"
	"strings"

	"github.com/wrgl/wrgl/pkg/auth"
	"github.com/wrgl/wrgl/pkg/conf"
)

type routeScope struct {
	Pat    *regexp.Regexp
	Method string
	Scope  string
}

var routeScopes []routeScope

var (
	patRefs          *regexp.Regexp
	patHead          *regexp.Regexp
	patRefsHead      *regexp.Regexp
	patUploadPack    *regexp.Regexp
	patReceivePack   *regexp.Regexp
	patCommits       *regexp.Regexp
	patCommit        *regexp.Regexp
	patSum           *regexp.Regexp
	patProfile       *regexp.Regexp
	patTables        *regexp.Regexp
	patTable         *regexp.Regexp
	patBlocks        *regexp.Regexp
	patTableBlocks   *regexp.Regexp
	patRows          *regexp.Regexp
	patTableRows     *regexp.Regexp
	patDiff          *regexp.Regexp
	patRootedBlocks  *regexp.Regexp
	patRootedRows    *regexp.Regexp
	patCommitProfile *regexp.Regexp
	patTableProfile  *regexp.Regexp
	patObjects       *regexp.Regexp
	patTransactions  *regexp.Regexp
	patTransaction   *regexp.Regexp
	patUUID          *regexp.Regexp
	patGC            *regexp.Regexp
)

func init() {
	patRefs = regexp.MustCompile(`^/refs/`)
	patHead = regexp.MustCompile(`^heads/[-_0-9a-zA-Z]+/`)
	patRefsHead = regexp.MustCompile(`^/refs/heads/[-_0-9a-zA-Z]+/`)
	patUploadPack = regexp.MustCompile(`^/upload-pack/`)
	patReceivePack = regexp.MustCompile(`^/receive-pack/`)
	patCommits = regexp.MustCompile(`^/commits/`)
	patRootedBlocks = regexp.MustCompile(`^/blocks/`)
	patRootedRows = regexp.MustCompile(`^/rows/`)
	patSum = regexp.MustCompile(`^[0-9a-f]{32}/`)
	patCommit = regexp.MustCompile(`^/commits/[0-9a-f]{32}/`)
	patTables = regexp.MustCompile(`^/tables/`)
	patTable = regexp.MustCompile(`^/tables/[0-9a-f]{32}/`)
	patProfile = regexp.MustCompile(`^profile/`)
	patBlocks = regexp.MustCompile(`^blocks/`)
	patTableBlocks = regexp.MustCompile(`^/tables/[0-9a-f]{32}/blocks/`)
	patRows = regexp.MustCompile(`^rows/`)
	patTableRows = regexp.MustCompile(`^/tables/[0-9a-f]{32}/rows/`)
	patDiff = regexp.MustCompile(`^/diff/[0-9a-f]{32}/[0-9a-f]{32}/`)
	patCommitProfile = regexp.MustCompile(`^/commits/[0-9a-f]{32}/profile/`)
	patTableProfile = regexp.MustCompile(`^/tables/[0-9a-f]{32}/profile/`)
	patObjects = regexp.MustCompile(`^/objects/`)
	patTransactions = regexp.MustCompile(`^/transactions/`)
	patTransaction = regexp.MustCompile(`^/transactions/[0-9a-f-]+/`)
	patUUID = regexp.MustCompile(`^[0-9a-f-]+/`)
	patGC = regexp.MustCompile(`^/gc/`)
	routeScopes = []routeScope{
		{
			Pat:    patRefsHead,
			Method: http.MethodGet,
			Scope:  auth.ScopeRepoRead,
		},
		{
			Pat:    patRefs,
			Method: http.MethodGet,
			Scope:  auth.ScopeRepoRead,
		},
		{
			Pat:    patUploadPack,
			Method: http.MethodPost,
			Scope:  auth.ScopeRepoRead,
		},
		{
			Pat:    patReceivePack,
			Method: http.MethodPost,
			Scope:  auth.ScopeRepoWrite,
		},
		{
			Pat:    patRootedBlocks,
			Method: http.MethodGet,
			Scope:  auth.ScopeRepoRead,
		},
		{
			Pat:    patRootedRows,
			Method: http.MethodGet,
			Scope:  auth.ScopeRepoRead,
		},
		{
			Pat:    patObjects,
			Method: http.MethodGet,
			Scope:  auth.ScopeRepoRead,
		},
		{
			Pat:    patCommit,
			Method: http.MethodGet,
			Scope:  auth.ScopeRepoRead,
		},
		{
			Pat:    patCommitProfile,
			Method: http.MethodGet,
			Scope:  auth.ScopeRepoRead,
		},
		{
			Pat:    patCommits,
			Method: http.MethodPost,
			Scope:  auth.ScopeRepoWrite,
		},
		{
			Pat:    patCommits,
			Method: http.MethodGet,
			Scope:  auth.ScopeRepoRead,
		},
		{
			Pat:    patTableBlocks,
			Method: http.MethodGet,
			Scope:  auth.ScopeRepoRead,
		},
		{
			Pat:    patTableRows,
			Method: http.MethodGet,
			Scope:  auth.ScopeRepoRead,
		},
		{
			Pat:    patTable,
			Method: http.MethodGet,
			Scope:  auth.ScopeRepoRead,
		},
		{
			Pat:    patTableProfile,
			Method: http.MethodGet,
			Scope:  auth.ScopeRepoRead,
		},
		{
			Pat:    patDiff,
			Method: http.MethodGet,
			Scope:  auth.ScopeRepoRead,
		},
		{
			Pat:    patTransactions,
			Method: http.MethodPost,
			Scope:  auth.ScopeRepoWrite,
		},
		{
			Pat:    patTransaction,
			Method: http.MethodGet,
			Scope:  auth.ScopeRepoRead,
		},
		{
			Pat:    patTransaction,
			Method: http.MethodPost,
			Scope:  auth.ScopeRepoWrite,
		},
		{
			Pat:    patGC,
			Method: http.MethodPost,
			Scope:  auth.ScopeRepoWrite,
		},
	}
}

type emailKey struct{}

func SetEmail(r *http.Request, email string) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), emailKey{}, email))
}

func GetEmail(r *http.Request) string {
	if i := r.Context().Value(emailKey{}); i != nil {
		return i.(string)
	}
	return ""
}

type nameKey struct{}

func SetName(r *http.Request, name string) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), nameKey{}, name))
}

func GetName(r *http.Request) string {
	if i := r.Context().Value(nameKey{}); i != nil {
		return i.(string)
	}
	return ""
}

type AuthzMiddlewareOptions struct {
	RootPath             *regexp.Regexp
	MaskUnauthorizedPath bool
	Enforce              func(r *http.Request, scope string) bool
	GetConfig            func(r *http.Request) *conf.Config
}

func AuthorizeMiddleware(options AuthzMiddlewareOptions) func(handler http.Handler) http.Handler {
	return func(handler http.Handler) http.Handler {
		return http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
			var route *routeScope
			p := r.URL.Path
			if options.RootPath != nil {
				p = strings.TrimPrefix(p, options.RootPath.FindString(p))
				if !strings.HasPrefix(p, "/") {
					p = "/" + p
				}
			}
			for _, o := range routeScopes {
				if o.Pat.MatchString(p) && o.Method == r.Method {
					route = &o
					break
				}
			}
			if route == nil {
				SendHTTPError(rw, r, http.StatusNotFound)
				return
			}
			if route.Scope != "" {
				c := options.GetConfig(r)
				if (route.Scope == auth.ScopeRepoRead && c.Auth != nil && c.Auth.AnonymousRead) || options.Enforce(r, route.Scope) {
					handler.ServeHTTP(rw, r)
					return
				}
				if options.MaskUnauthorizedPath {
					SendHTTPError(rw, r, http.StatusNotFound)
				} else {
					SendHTTPError(rw, r, http.StatusForbidden)
				}
			}
		})
	}
}
