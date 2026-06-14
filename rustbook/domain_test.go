package rustbook

import (
	"testing"

	"github.com/tamnd/any-cli/kit"
)

// These tests are offline: they exercise the kit wiring and domain metadata,
// which need no network. The client's HTTP behaviour is covered in rustbook_test.go.

func TestDomainInfo(t *testing.T) {
	info := Domain{}.Info()
	if info.Scheme != "rustbook" {
		t.Errorf("Scheme = %q, want rustbook", info.Scheme)
	}
	if len(info.Hosts) == 0 || info.Hosts[0] != Host {
		t.Errorf("Hosts = %v, want [%s]", info.Hosts, Host)
	}
	if info.Identity.Binary != "rustbook" {
		t.Errorf("Identity.Binary = %q, want rustbook", info.Identity.Binary)
	}
}

func TestRegisterInstallsChapters(t *testing.T) {
	app := kit.New(Domain{}.Info().Identity)
	Domain{}.Register(app)
	// Verify the app was assembled without panic; kit.New + Register is the
	// same path NewApp takes in the binary.
	_ = app
}
