package dns

import (
	"testing"

	"github.com/octarq-org/octarq/internal/models"
)

func TestEffectiveHosts(t *testing.T) {
	sub := Domain{
		Name:      "example.com",
		LinkHosts: models.HostList{{Host: "go.example.com", Enabled: true}, {Host: "off.example.com", Enabled: false}},
		MailHosts: models.HostList{{Host: "mail.example.com", Enabled: true}},
	}
	if got := sub.EffectiveLinkHosts(); len(got) != 1 || got[0] != "go.example.com" {
		t.Errorf("enabled link hosts only: %v", got)
	}
	if got := sub.EffectiveMailHosts(); len(got) != 1 || got[0] != "mail.example.com" {
		t.Errorf("mail hosts: %v", got)
	}
	// Blocks: a disabled-only host blocks; an enabled or unlisted host does not.
	if !sub.LinkHosts.Blocks("off.example.com") {
		t.Error("disabled host should block")
	}
	if sub.LinkHosts.Blocks("go.example.com") {
		t.Error("enabled host should not block")
	}
	if sub.LinkHosts.Blocks("unknown.example.com") {
		t.Error("unlisted host should not block")
	}
}
