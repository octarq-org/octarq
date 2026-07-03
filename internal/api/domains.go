package api

import (
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/Jungley8/led/internal/dnsprovider"
	"github.com/Jungley8/led/internal/models"
)

// providerErr logs an upstream DNS-provider failure and returns it as a 400 so
// the real message reaches the browser. (A 5xx would be replaced by the
// Cloudflare proxy's own error page, hiding the cause.)
func (h *Handler) providerErr(w http.ResponseWriter, action string, err error) {
	log.Printf("dns provider: %s failed: %v", action, err)
	writeErr(w, http.StatusBadRequest, action+": "+err.Error())
}

func (h *Handler) dnsProviders(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, dnsprovider.Names())
}

// normalizeHost cleans a user-supplied host into a bare lowercase hostname
// (no scheme, no path, no trailing dot).
func normalizeHost(host string) string {
	host = strings.ToLower(strings.TrimSpace(host))
	host = strings.TrimPrefix(host, "https://")
	host = strings.TrimPrefix(host, "http://")
	host = strings.TrimSuffix(host, "/")
	host = strings.TrimSuffix(host, ".")
	if i := strings.IndexAny(host, "/:"); i >= 0 {
		host = host[:i]
	}
	return host
}

// hostEntry is a host with its enable flag in create/update payloads.
type hostEntry struct {
	Host    string `json:"host"`
	Enabled *bool  `json:"enabled"`
}

// normalizeHosts cleans and de-duplicates a host list, preserving each host's
// enabled flag (defaulting to enabled). A service (links/mail) is considered
// configured when its host list is non-empty — there is no separate toggle.
func normalizeHosts(hosts []hostEntry) models.HostList {
	seen := map[string]bool{}
	var out models.HostList
	for _, h := range hosts {
		name := normalizeHost(h.Host)
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		enabled := true
		if h.Enabled != nil {
			enabled = *h.Enabled
		}
		out = append(out, models.Host{Host: name, Enabled: enabled})
	}
	return out
}

// syncDomains imports every zone the given credentials can access, creating a
// Domain for each new zone and refreshing the zone id / stored credentials on
// existing ones. User flags (forLink/forMail/note) on existing domains are kept.
func (h *Handler) syncDomains(w http.ResponseWriter, r *http.Request) {
	var d struct {
		ProviderAccountID uint `json:"providerAccountId"`
	}
	if err := readJSON(r, &d); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	if d.ProviderAccountID == 0 {
		writeErr(w, http.StatusBadRequest, "providerAccountId is required")
		return
	}
	var acc models.ProviderAccount
	if err := h.db.First(&acc, d.ProviderAccountID).Error; err != nil {
		writeErr(w, http.StatusNotFound, "provider account not found")
		return
	}

	creds, err := h.cipher.Decrypt(acc.Config)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "decrypt provider credentials")
		return
	}

	prov, err := dnsprovider.New(acc.Type, creds)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	zones, err := prov.ListZones(r.Context())
	if err != nil {
		h.providerErr(w, "list zones", err)
		return
	}
	var created, updated int
	for _, z := range zones {
		name := strings.ToLower(z.Name)
		var dom models.Domain
		if h.db.Where("name = ?", name).First(&dom).Error == nil {
			dom.ZoneID = z.ID
			dom.ProviderAccountID = acc.ID
			h.db.Save(&dom)
			updated++
		} else {
			h.db.Create(&models.Domain{
				OrgID: h.orgID(r),
				Name:  name, ProviderAccountID: acc.ID, ZoneID: z.ID,
			})
			created++
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true, "total": len(zones), "created": created, "updated": updated,
	})
}

// domainDTO is the create/update payload.
// LinkHosts/MailHosts are pointers so we can distinguish "not sent" (nil)
// from "explicitly set to empty" ([]) in PATCH-style updates.
type domainDTO struct {
	Name              string `json:"name"`
	ProviderAccountID uint   `json:"providerAccountId"`
	ZoneID            string `json:"zoneId"`
	Note              string `json:"note"`
	// ForLink/ForMail are pointer booleans so "not sent" (nil) is distinct
	// from an explicit true/false, enabling domain-level master toggles that
	// are independent of the individual host lists.
	ForMail   *bool        `json:"forMail"`
	ForLink   *bool        `json:"forLink"`
	LinkHosts *[]hostEntry `json:"linkHosts"`
	MailHosts *[]hostEntry `json:"mailHosts"`
}

func (h *Handler) listDomains(w http.ResponseWriter, r *http.Request) {
	var ds []models.Domain
	q := h.orgDB(r).Order("created_at DESC")
	if s := r.URL.Query().Get("q"); s != "" {
		like := "%" + s + "%"
		q = q.Where("name LIKE ? OR note LIKE ?", like, like)
	}
	limit := 50
	if l, _ := strconv.Atoi(r.URL.Query().Get("limit")); l > 0 && l <= 500 {
		limit = l
	}
	offset := 0
	if o, _ := strconv.Atoi(r.URL.Query().Get("offset")); o > 0 {
		offset = o
	}
	q = q.Limit(limit).Offset(offset)
	q.Find(&ds)
	writeJSON(w, http.StatusOK, ds)
}

func (h *Handler) createDomain(w http.ResponseWriter, r *http.Request) {
	var d domainDTO
	if err := readJSON(r, &d); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	d.Name = strings.TrimSpace(strings.ToLower(d.Name))
	if d.Name == "" || d.ProviderAccountID == 0 {
		writeErr(w, http.StatusBadRequest, "name and provider account are required")
		return
	}
	var linkHosts, mailHosts []hostEntry
	if d.LinkHosts != nil {
		linkHosts = *d.LinkHosts
	}
	if d.MailHosts != nil {
		mailHosts = *d.MailHosts
	}
	dom := models.Domain{
		OrgID:             h.orgID(r),
		Name:              d.Name,
		ProviderAccountID: d.ProviderAccountID,
		ZoneID:            d.ZoneID,
		Note:              d.Note,
		LinkHosts:         normalizeHosts(linkHosts),
		MailHosts:         normalizeHosts(mailHosts),
	}
	// On creation, derive master switches from host presence unless explicitly set.
	if d.ForLink != nil {
		dom.ForLink = *d.ForLink
	} else {
		dom.ForLink = len(dom.LinkHosts) > 0
	}
	if d.ForMail != nil {
		dom.ForMail = *d.ForMail
	} else {
		dom.ForMail = len(dom.MailHosts) > 0
	}
	// Best-effort credential check.
	if prov, err := h.providerFor(dom); err == nil && dom.ZoneID != "" {
		if name, err := prov.VerifyZone(r.Context(), dom.ZoneID); err != nil {
			writeErr(w, http.StatusBadRequest, "provider verification failed: "+err.Error())
			return
		} else if dom.Name == "" {
			dom.Name = name
		}
	}
	if err := h.db.Create(&dom).Error; err != nil {
		writeErr(w, http.StatusConflict, "domain already exists")
		return
	}
	h.audit(r, "domain.create", "domain", dom.ID, map[string]any{"name": dom.Name})
	writeJSON(w, http.StatusCreated, dom)
}

func (h *Handler) updateDomain(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, "bad id")
		return
	}
	var dom models.Domain
	if h.db.Where("id = ? AND owner_id = ?", id, h.orgID(r)).First(&dom).Error != nil {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	var d domainDTO
	if err := readJSON(r, &d); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	dom.Note = d.Note
	dom.ZoneID = d.ZoneID
	// Apply master switches when explicitly provided.
	if d.ForLink != nil {
		dom.ForLink = *d.ForLink
	}
	if d.ForMail != nil {
		dom.ForMail = *d.ForMail
	}
	// Only overwrite host lists when they were present in the payload.
	if d.LinkHosts != nil {
		dom.LinkHosts = normalizeHosts(*d.LinkHosts)
	}
	if d.MailHosts != nil {
		dom.MailHosts = normalizeHosts(*d.MailHosts)
	}
	if d.ProviderAccountID != 0 {
		dom.ProviderAccountID = d.ProviderAccountID
	}
	h.db.Save(&dom)
	h.audit(r, "domain.update", "domain", dom.ID, map[string]any{"name": dom.Name})
	writeJSON(w, http.StatusOK, dom)
}

func (h *Handler) deleteDomain(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, "bad id")
		return
	}
	if res := h.db.Where("id = ? AND owner_id = ?", id, h.orgID(r)).Delete(&models.Domain{}); res.RowsAffected == 0 {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	h.audit(r, "domain.delete", "domain", id, nil)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// validateRecord catches the most common reasons Cloudflare rejects a record
// before we make the API call, returning a friendly message (or "" if valid).
func validateRecord(rec dnsprovider.Record) string {
	if strings.TrimSpace(rec.Type) == "" {
		return "record type is required"
	}
	if strings.TrimSpace(rec.Content) == "" {
		return "content is required (e.g. an IP for A, a hostname for CNAME, a value for TXT)"
	}
	switch strings.ToUpper(rec.Type) {
	case "MX", "SRV", "URI":
		if rec.Priority == nil {
			return "priority is required for " + strings.ToUpper(rec.Type) + " records"
		}
	}
	return ""
}

// --- DNS records (live via provider) ---

func (h *Handler) recordsProvider(r *http.Request) (dnsprovider.Provider, *models.Domain, error) {
	id, _ := strconv.ParseUint(r.PathValue("id"), 10, 64)
	var dom models.Domain
	if h.db.Where("id = ? AND owner_id = ?", id, h.orgID(r)).First(&dom).Error != nil {
		return nil, nil, errNotFound
	}
	prov, err := h.providerFor(dom)
	return prov, &dom, err
}

func (h *Handler) listRecords(w http.ResponseWriter, r *http.Request) {
	prov, dom, err := h.recordsProvider(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	recs, err := prov.ListRecords(r.Context(), dom.ZoneID)
	if err != nil {
		h.providerErr(w, "list records", err)
		return
	}
	writeJSON(w, http.StatusOK, recs)
}

func (h *Handler) createRecord(w http.ResponseWriter, r *http.Request) {
	prov, dom, err := h.recordsProvider(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if dom.ZoneID == "" {
		writeErr(w, http.StatusBadRequest, "this domain has no Zone ID — sync from Cloudflare or set it in the domain settings")
		return
	}
	var rec dnsprovider.Record
	if err := readJSON(r, &rec); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	if msg := validateRecord(rec); msg != "" {
		writeErr(w, http.StatusBadRequest, msg)
		return
	}
	out, err := prov.CreateRecord(r.Context(), dom.ZoneID, rec)
	if err != nil {
		h.providerErr(w, "create record", err)
		return
	}
	writeJSON(w, http.StatusCreated, out)
}

func (h *Handler) updateRecord(w http.ResponseWriter, r *http.Request) {
	prov, dom, err := h.recordsProvider(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	var rec dnsprovider.Record
	if err := readJSON(r, &rec); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	rec.ID = r.PathValue("rid")
	if msg := validateRecord(rec); msg != "" {
		writeErr(w, http.StatusBadRequest, msg)
		return
	}
	out, err := prov.UpdateRecord(r.Context(), dom.ZoneID, rec)
	if err != nil {
		h.providerErr(w, "update record", err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *Handler) deleteRecord(w http.ResponseWriter, r *http.Request) {
	prov, dom, err := h.recordsProvider(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := prov.DeleteRecord(r.Context(), dom.ZoneID, r.PathValue("rid")); err != nil {
		h.providerErr(w, "delete record", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// GET /api/domains/{id}/verify-dns
func (h *Handler) verifyDomainDNS(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, "bad id")
		return
	}
	var dom models.Domain
	if err := h.db.Where("id = ? AND owner_id = ?", id, h.orgID(r)).First(&dom).Error; err != nil {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}

	domainName := dom.Name

	// 1. SPF record check
	spfSet := false
	spfHealthy := false
	spfValue := ""
	spfRecords, _ := h.lookupTXT(domainName)
	for _, record := range spfRecords {
		lower := strings.ToLower(strings.TrimSpace(record))
		if strings.Contains(lower, "v=spf1") {
			spfSet = true
			spfValue = record
			if strings.HasPrefix(strings.ReplaceAll(lower, " ", ""), "v=spf1") {
				spfHealthy = true
			}
			break
		}
	}

	// 2. DMARC record check
	dmarcSet := false
	dmarcHealthy := false
	dmarcValue := ""
	dmarcRecords, _ := h.lookupTXT("_dmarc." + domainName)
	for _, record := range dmarcRecords {
		lower := strings.ToLower(strings.TrimSpace(record))
		if strings.Contains(lower, "v=dmarc1") {
			dmarcSet = true
			dmarcValue = record
			if strings.Contains(lower, "p=none") || strings.Contains(lower, "p=quarantine") || strings.Contains(lower, "p=reject") {
				dmarcHealthy = true
			}
			break
		}
	}

	// 3. DKIM record check (probing common selectors)
	selectors := []string{"default", "led", "google", "mail", "k1", "sig1"}
	type dkimResult struct {
		selector string
		value    string
		set      bool
		healthy  bool
	}
	resultChan := make(chan dkimResult, len(selectors))
	for _, sel := range selectors {
		go func(s string) {
			records, err := h.lookupTXT(s + "._domainkey." + domainName)
			if err == nil {
				for _, r := range records {
					lower := strings.ToLower(strings.TrimSpace(r))
					if strings.Contains(lower, "v=dkim1") || strings.Contains(lower, "k=rsa") || strings.Contains(lower, "p=") {
						resultChan <- dkimResult{
							selector: s,
							value:    r,
							set:      true,
							healthy:  strings.Contains(lower, "p="),
						}
						return
					}
				}
			}
			resultChan <- dkimResult{}
		}(sel)
	}

	var dkimRes dkimResult
	for i := 0; i < len(selectors); i++ {
		res := <-resultChan
		if res.set {
			dkimRes = res
		}
	}

	type recordStatus struct {
		Set     bool   `json:"set"`
		Healthy bool   `json:"healthy"`
		Value   string `json:"value,omitempty"`
	}

	type dkimStatus struct {
		recordStatus
		Selector string `json:"selector,omitempty"`
	}

	response := map[string]any{
		"spf": recordStatus{
			Set:     spfSet,
			Healthy: spfHealthy,
			Value:   spfValue,
		},
		"dmarc": recordStatus{
			Set:     dmarcSet,
			Healthy: dmarcHealthy,
			Value:   dmarcValue,
		},
		"dkim": dkimStatus{
			recordStatus: recordStatus{
				Set:     dkimRes.set,
				Healthy: dkimRes.healthy,
				Value:   dkimRes.value,
			},
			Selector: dkimRes.selector,
		},
	}

	writeJSON(w, http.StatusOK, response)
}
