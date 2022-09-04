package wrgldoapiserver

import (
	"github.com/pckhoi/uma"
)

var umaResourceTypes = map[string]uma.ResourceType{
	"https://www.wrgl.co/rsrcs/repository": {
		Type:           "https://www.wrgl.co/rsrcs/repository",
		Description:    "A Wrgl repository",
		IconUri:        "https://www.wrgl.co/rsrcs/repository/icon.png",
		ResourceScopes: []string{"read", "write"},
	},
}

var umaSecuritySchemes = []string{
	"oidc",
}

var umaDefaultResource *uma.ResourceTemplate = uma.NewResourceTemplate("https://www.wrgl.co/rsrcs/repository", "")

var umaDefaultSecurity uma.Security = []map[string][]string{
	{
		"oidc": {"read"},
	},
}

var umaPaths uma.Paths = []uma.Path{
	uma.NewPath("/transactions", nil, map[string]uma.Operation{
		"POST": {
			Security: []map[string][]string{
				{
					"oidc": {"write"},
				},
			},
		},
	}),
	uma.NewPath("/refs", nil, map[string]uma.Operation{
		"GET": {},
	}),
	uma.NewPath("/tables/{hash}", nil, map[string]uma.Operation{
		"GET": {},
	}),
	uma.NewPath("/tables/{hash}/profile", nil, map[string]uma.Operation{
		"GET": {},
	}),
	uma.NewPath("/blocks", nil, map[string]uma.Operation{
		"GET": {},
	}),
	uma.NewPath("/receive-pack", nil, map[string]uma.Operation{
		"POST": {
			Security: []map[string][]string{
				{
					"oidc": {"write"},
				},
			},
		},
	}),
	uma.NewPath("/refs/heads/{branch}", nil, map[string]uma.Operation{
		"GET": {},
	}),
	uma.NewPath("/commits", nil, map[string]uma.Operation{
		"GET": {},
		"POST": {
			Security: []map[string][]string{
				{
					"oidc": {"write"},
				},
			},
		},
	}),
	uma.NewPath("/tables/{hash}/blocks", nil, map[string]uma.Operation{
		"GET": {},
	}),
	uma.NewPath("/tables/{hash}/rows", nil, map[string]uma.Operation{
		"GET": {},
	}),
	uma.NewPath("/rows", nil, map[string]uma.Operation{
		"GET": {},
	}),
	uma.NewPath("/transactions/{id}", nil, map[string]uma.Operation{
		"GET": {},
		"POST": {
			Security: []map[string][]string{
				{
					"oidc": {"write"},
				},
			},
		},
	}),
	uma.NewPath("/commits/{hash}", nil, map[string]uma.Operation{
		"GET": {},
	}),
	uma.NewPath("/commits/{hash}/profile", nil, map[string]uma.Operation{
		"GET": {},
	}),
	uma.NewPath("/diff/{newCommitHash}/{oldCommitHash}", nil, map[string]uma.Operation{
		"GET": {},
	}),
	uma.NewPath("/gc", nil, map[string]uma.Operation{
		"POST": {
			Security: []map[string][]string{
				{
					"oidc": {"write"},
				},
			},
		},
	}),
	uma.NewPath("/objects", nil, map[string]uma.Operation{
		"GET": {},
	}),
	uma.NewPath("/upload-pack", nil, map[string]uma.Operation{
		"POST": {},
	}),
}

// UMAManager returns an uma.Manager instance configured according to OpenAPI schema
func UMAManager(opts uma.ManagerOptions) *uma.Manager {
	return uma.New(
		opts,
		umaResourceTypes,
		umaSecuritySchemes,
		umaDefaultResource,
		umaDefaultSecurity,
		umaPaths,
	)
}
