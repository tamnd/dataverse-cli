package dataverse

import (
	"context"
	"strings"

	"github.com/tamnd/any-cli/kit"
	"github.com/tamnd/any-cli/kit/errs"
)

// domain.go exposes the Harvard Dataverse as a kit Domain: a driver that a
// multi-domain host (ant) enables with a single blank import,
//
//	import _ "github.com/tamnd/dataverse-cli/dataverse"
//
// exactly as a database/sql program enables a driver with `import _
// "github.com/lib/pq"`. The init below registers it; the host then dereferences
// dataverse:// URIs by routing to the operations Register installs. The same
// Domain also builds the standalone dataverse binary (see cli.NewApp), so the
// binary and a host share one source of truth.
func init() { kit.Register(Domain{}) }

// Domain is the dataverse driver. It carries no state; the per-run client is
// built by the factory Register hands kit.
type Domain struct{}

// Info describes the scheme, the hostnames a pasted link is matched against, and
// the identity reused for the binary's help and version.
func (Domain) Info() kit.DomainInfo {
	return kit.DomainInfo{
		Scheme: "dataverse",
		Hosts:  []string{Host},
		Identity: kit.Identity{
			Binary: "dataverse",
			Short:  "A command line for Harvard Dataverse.",
			Long: `A command line for Harvard Dataverse.

dataverse reads public Harvard Dataverse data over plain HTTPS, shapes it into
clean records, and prints output that pipes into the rest of your tools. No API
key, nothing to run alongside it. Harvard Dataverse hosts 300k+ datasets across
every research domain.`,
			Site: Host,
			Repo: "https://github.com/tamnd/dataverse-cli",
		},
	}
}

// Register installs the client factory and every operation onto app.
func (Domain) Register(app *kit.App) {
	app.SetClient(newClient)

	kit.Handle(app, kit.OpMeta{Name: "search", Group: "read", List: true,
		Summary: "Search Harvard Dataverse datasets",
		Args:    []kit.Arg{{Name: "query", Help: "search query"}}}, searchDatasets)

	kit.Handle(app, kit.OpMeta{Name: "recent", Group: "read", List: true,
		Summary: "List recently published datasets"}, recentDatasets)
}

// newClient builds the client from the host-resolved config, so a host and the
// standalone binary pace and identify themselves the same way.
func newClient(_ context.Context, cfg kit.Config) (any, error) {
	c := NewClient()
	if cfg.UserAgent != "" {
		c.cfg.UserAgent = cfg.UserAgent
	}
	if cfg.Rate > 0 {
		c.cfg.Rate = cfg.Rate
	}
	if cfg.Retries > 0 {
		c.cfg.Retries = cfg.Retries
	}
	if cfg.Timeout > 0 {
		c.http.Timeout = cfg.Timeout
	}
	return c, nil
}

// --- inputs ---

type searchInput struct {
	Query  string  `kit:"arg"          help:"search query"`
	Limit  int     `kit:"flag,inherit" help:"max results"`
	Offset int     `kit:"flag"         help:"result offset"`
	Client *Client `kit:"inject"`
}

type recentInput struct {
	Limit  int     `kit:"flag,inherit" help:"max results"`
	Client *Client `kit:"inject"`
}

// --- handlers ---

func searchDatasets(ctx context.Context, in searchInput, emit func(*Dataset) error) error {
	limit := in.Limit
	if limit <= 0 {
		limit = 20
	}
	datasets, _, err := in.Client.SearchDatasets(ctx, in.Query, limit, in.Offset)
	if err != nil {
		return mapErr(err)
	}
	for _, d := range datasets {
		if err := emit(d); err != nil {
			return err
		}
	}
	return nil
}

func recentDatasets(ctx context.Context, in recentInput, emit func(*Dataset) error) error {
	limit := in.Limit
	if limit <= 0 {
		limit = 20
	}
	datasets, _, err := in.Client.RecentDatasets(ctx, limit)
	if err != nil {
		return mapErr(err)
	}
	for _, d := range datasets {
		if err := emit(d); err != nil {
			return err
		}
	}
	return nil
}

// --- Resolver: the URI-native string functions, pure and network-free ---

// Classify turns any accepted input — a bare DOI or a full Dataverse URL —
// into the canonical (type, id), so `ant resolve` and `ant url` touch no network.
func (Domain) Classify(input string) (uriType, id string, err error) {
	s := strings.TrimSpace(input)
	if s == "" {
		return "", "", errs.Usage("dataverse reference required")
	}
	return "dataset", trimDOI(s), nil
}

// Locate is the inverse: the live https URL for a (type, id).
func (Domain) Locate(uriType, id string) (string, error) {
	switch uriType {
	case "dataset":
		if strings.HasPrefix(id, "doi:") {
			return "https://doi.org/" + strings.TrimPrefix(id, "doi:"), nil
		}
		return "https://dataverse.harvard.edu/dataset.xhtml?persistentId=" + id, nil
	default:
		return "", errs.Usage("dataverse has no resource type %q", uriType)
	}
}

// mapErr converts a library error into the kit error kind that carries the right
// exit code.
func mapErr(err error) error {
	return err
}
