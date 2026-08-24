package mail

import (
	"net/http"
	"strings"

	"github.com/octarq-org/octarq/plugin"
	"github.com/octarq-org/octarq/plugins/dns"
)

func reporterIP(r *http.Request) string {
	if ip := r.Header.Get("X-Forwarded-For"); ip != "" {
		if idx := strings.IndexByte(ip, ','); idx >= 0 {
			return strings.TrimSpace(ip[:idx])
		}
		return strings.TrimSpace(ip)
	}
	return r.RemoteAddr
}

func (p *Plugin) isReservedMailbox(orgID uint, addr string) bool {
	parts := strings.SplitN(addr, "@", 2)
	if len(parts) != 2 {
		return false
	}
	user := strings.ToLower(parts[0])
	reserved := []string{"admin", "administrator", "hostmaster", "postmaster", "webmaster"}
	for _, r := range reserved {
		if user == r {
			return true
		}
	}
	return false
}

func (p *Plugin) emitEmail(e plugin.EmailEvent) {
	p.emailMu.RLock()
	handlers := p.emailHandlers
	p.emailMu.RUnlock()
	for _, h := range handlers {
		if h != nil {
			go h(e)
		}
	}
}

func (p *Plugin) OnEmail(handler func(plugin.EmailEvent)) {
	if handler == nil {
		return
	}
	p.emailMu.Lock()
	defer p.emailMu.Unlock()
	p.emailHandlers = append(p.emailHandlers, handler)
}

// mailHostDisabled reports whether host is listed as a mail host on the owner's domain
// but every such listing is disabled (so mail to it should be dropped).
func (p *Plugin) mailHostDisabled(orgID uint, host string) bool {
	if orgID == 0 {
		return false
	}
	normHost := dns.NormalizeHost(host)
	var doms []dns.Domain
	p.db.Where("owner_id = ? AND for_mail = ?", orgID, true).Find(&doms)
	listed := false
	for _, d := range doms {
		for _, mh := range d.MailHosts {
			if dns.NormalizeHost(mh.Host) == normHost {
				listed = true
				if mh.Enabled {
					return false
				}
			}
		}
	}
	return listed
}

// mailAddressDomainNotAnotherTenants reports whether orgID may create a mailbox
// at addr — true unless addr's domain is a mail host belonging to a *different*
// workspace.
//
// Scope note, because the weaker half of this rule is deliberate. The defect
// being fixed is cross-tenant squatting: mailbox addresses were globally unique,
// so one workspace could take `billing@victim.com` and permanently block the
// tenant that actually owns victim.com from ever creating it. That is what this
// refuses.
//
// It does NOT require the domain to be one the workspace has already registered.
// Requiring that would be a stricter and arguably better rule — you cannot
// receive mail at a domain you have not set up — but it is a product behaviour
// change, not a security fix: creating a mailbox ahead of registering its domain
// works today and is exercised by existing tests. Delivery is unaffected either
// way; the inbound path is already gated on the org's own token and its own mail
// hosts (see resolveMailbox), so an unregistered mailbox simply never receives.
func (p *Plugin) mailAddressDomainNotAnotherTenants(orgID uint, addr string) bool {
	if orgID == 0 || p.db == nil {
		return false
	}
	at := strings.LastIndex(addr, "@")
	if at < 0 {
		return false
	}
	normHost := dns.NormalizeHost(addr[at+1:])
	if normHost == "" {
		return false
	}
	var doms []dns.Domain
	if err := p.db.Where("for_mail = ?", true).Find(&doms).Error; err != nil {
		return false
	}
	for _, d := range doms {
		if d.OrgID == orgID {
			continue // your own domain never blocks you
		}
		for _, mh := range d.MailHosts {
			if dns.NormalizeHost(mh.Host) == normHost {
				return false // this hostname belongs to another workspace
			}
		}
	}
	return true
}

// resolveMailbox finds an enabled mailbox for the address within the given org,
// optionally creating one when catch-all is on and the recipient's domain (also
// owned by that org) is managed for mail. Scoping by org keeps one tenant's
// inbound webhook from delivering into another tenant's mailboxes.
func (p *Plugin) resolveMailbox(orgID uint, addr string) (*Mailbox, bool) {
	if addr == "" {
		return nil, false
	}
	// Drop mail to a temporarily disabled mail host, even for existing mailboxes.
	if at := strings.LastIndex(addr, "@"); at >= 0 && p.mailHostDisabled(orgID, addr[at+1:]) {
		return nil, false
	}
	var mb Mailbox
	if err := p.db.Where("address = ? AND enabled = ? AND owner_id = ?", addr, true, orgID).First(&mb).Error; err == nil {
		return &mb, true
	}
	if p.getWorkspaceSetting(orgID, "mail.catch_all") != "true" {
		return nil, false
	}
	// Reserved local-parts are never auto-created by catch-all.
	if p.isReservedMailbox(orgID, addr) {
		return nil, false
	}
	// The recipient host must be one of THIS org's mail-enabled domain's mail
	// hosts (apex or a configured subdomain like mail.example.com).
	//
	// Strict on purpose, and deliberately NOT the looser rule manual creation
	// uses: this path creates a mailbox by itself, from an inbound message, with
	// no operator in the loop. "Not another tenant's" is not enough when nobody
	// is choosing — anything routed at this org's webhook would materialise a
	// mailbox.
	if !p.ownsMailHost(orgID, addr) {
		return nil, false
	}
	mb = Mailbox{OrgID: orgID, Address: addr, Enabled: true, Note: "auto (catch-all)"}
	if err := p.db.Create(&mb).Error; err != nil {
		return nil, false
	}
	return &mb, true
}

// ownsMailHost reports whether addr's domain is one of orgID's own enabled mail
// hosts (the apex Name when a domain lists none explicitly).
//
// This is the strict form, used where a mailbox is created without an operator
// deciding — see resolveMailbox's catch-all branch.
func (p *Plugin) ownsMailHost(orgID uint, addr string) bool {
	if orgID == 0 || p.db == nil {
		return false
	}
	at := strings.LastIndex(addr, "@")
	if at < 0 {
		return false
	}
	normHost := dns.NormalizeHost(addr[at+1:])
	if normHost == "" {
		return false
	}
	var doms []dns.Domain
	if err := p.db.Where("owner_id = ? AND for_mail = ?", orgID, true).Find(&doms).Error; err != nil {
		return false
	}
	for _, d := range doms {
		hosts := d.EffectiveMailHosts()
		if len(hosts) == 0 {
			if dns.NormalizeHost(d.Name) == normHost {
				return true
			}
			continue
		}
		for _, mh := range hosts {
			if dns.NormalizeHost(mh) == normHost {
				return true
			}
		}
	}
	return false
}
