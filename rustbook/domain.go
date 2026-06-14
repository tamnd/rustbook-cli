package rustbook

import (
	"context"

	"github.com/tamnd/any-cli/kit"
)

// domain.go exposes rustbook as a kit Domain: a driver that a multi-domain
// host (ant) enables with a single blank import,
//
//	import _ "github.com/tamnd/rustbook-cli/rustbook"
//
// exactly as a database/sql program enables a driver with `import _
// "github.com/lib/pq"`. The init below registers it; the host then dereferences
// rustbook:// URIs by routing to the operations Register installs. The same
// Domain also builds the standalone rustbook binary (see cli.NewApp), so the
// binary and a host share one source of truth.
func init() { kit.Register(Domain{}) }

// Domain is the rustbook driver. It carries no state; the per-run client is
// built by the factory Register hands kit.
type Domain struct{}

// Info describes the scheme, the hostnames a pasted link is matched against, and
// the identity reused for the binary's help and version.
func (Domain) Info() kit.DomainInfo {
	return kit.DomainInfo{
		Scheme: "rustbook",
		Hosts:  []string{Host},
		Identity: kit.Identity{
			Binary: "rustbook",
			Short:  "Browse The Rust Programming Language book from the command line",
			Long: `Browse The Rust Programming Language book from the command line.

rustbook reads the official Rust book at doc.rust-lang.org over plain HTTPS,
shapes it into clean records, and prints output that pipes into the rest of
your tools. No API key, nothing to run alongside it.`,
			Site: "doc.rust-lang.org/book",
			Repo: "https://github.com/tamnd/rustbook-cli",
		},
	}
}

// Register installs the client factory and every operation onto app.
func (Domain) Register(app *kit.App) {
	app.SetClient(newClient)

	kit.Handle(app, kit.OpMeta{Name: "chapters", Group: "read", List: true,
		Summary: "List all chapters of The Rust Programming Language book"},
		listChapters)
}

// newClient builds the client from the host-resolved config.
func newClient(_ context.Context, cfg kit.Config) (any, error) {
	c := NewClient()
	if cfg.UserAgent != "" {
		c.UserAgent = cfg.UserAgent
	}
	if cfg.Rate > 0 {
		c.Rate = cfg.Rate
	}
	if cfg.Retries > 0 {
		c.Retries = cfg.Retries
	}
	if cfg.Timeout > 0 {
		c.HTTP.Timeout = cfg.Timeout
	}
	return c, nil
}

// --- inputs ---

type chaptersIn struct {
	Limit  int     `kit:"flag,inherit" help:"max results"`
	Client *Client `kit:"inject"`
}

// --- handlers ---

func listChapters(ctx context.Context, in chaptersIn, emit func(*Chapter) error) error {
	chapters, err := in.Client.Chapters(ctx)
	if err != nil {
		return err
	}
	for i, ch := range chapters {
		if in.Limit > 0 && i >= in.Limit {
			break
		}
		if err := emit(ch); err != nil {
			return err
		}
	}
	return nil
}
