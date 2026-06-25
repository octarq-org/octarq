package api

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/jungley/led/internal/dnsprovider"
	"github.com/jungley/led/internal/models"
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

// normalizeHosts cleans and de-duplicates a host list, dropping it entirely
// when the matching service is disabled.
func normalizeHosts(hosts []string, enabled bool) models.StringList {
	if !enabled {
		return nil
	}
	seen := map[string]bool{}
	var out models.StringList
	for _, h := range hosts {
		h = normalizeHost(h)
		if h != "" && !seen[h] {
			seen[h] = true
			out = append(out, h)
		}
	}
	return out
}

// syncDomains imports every zone the given credentials can access, creating a
// Domain for each new zone and refreshing the zone id / stored credentials on
// existing ones. User flags (forLink/forMail/note) on existing domains are kept.
func (h *Handler) syncDomains(w http.ResponseWriter, r *http.Request) {
	var d struct {
		Provider string         `json:"provider"`
		Config   map[string]any `json:"config"`
	}
	if err := readJSON(r, &d); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	if d.Provider == "" {
		d.Provider = "cloudflare"
	}
	// Fall back to the global Cloudflare token from Settings when none is given.
	if len(d.Config) == 0 && d.Provider == "cloudflare" {
		if tok := h.cloudflareToken(); tok != "" {
			d.Config = map[string]any{"apiToken": tok}
		}
	}
	enc, err := h.encryptConfig(d.Config)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "encrypt config")
		return
	}
	credsJSON, _ := json.Marshal(d.Config)
	prov, err := dnsprovider.New(d.Provider, credsJSON)
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
			dom.Provider = d.Provider
			if enc != "" {
				dom.Config = enc
			}
			h.db.Save(&dom)
			updated++
		} else {
			h.db.Create(&models.Domain{
				OwnerID: models.SingleUserID,
				Name:    name, Provider: d.Provider, ZoneID: z.ID, Config: enc,
			})
			created++
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true, "total": len(zones), "created": created, "updated": updated,
	})
}

// domainDTO is the create/update payload. Config holds the provider
// credentials (e.g. {"apiToken":"..."}) and is encrypted before storage.
type domainDTO struct {
	Name      string         `json:"name"`
	Provider  string         `json:"provider"`
	ZoneID    string         `json:"zoneId"`
	Note      string         `json:"note"`
	ForMail   bool           `json:"forMail"`
	ForLink   bool           `json:"forLink"`
	LinkHosts []string       `json:"linkHosts"`
	MailHosts []string       `json:"mailHosts"`
	Config    map[string]any `json:"config"`
}

func (h *Handler) listDomains(w http.ResponseWriter, r *http.Request) {
	var ds []models.Domain
	h.db.Order("created_at DESC").Find(&ds)
	writeJSON(w, http.StatusOK, ds)
}

func (h *Handler) createDomain(w http.ResponseWriter, r *http.Request) {
	var d domainDTO
	if err := readJSON(r, &d); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	d.Name = strings.TrimSpace(strings.ToLower(d.Name))
	if d.Name == "" || d.Provider == "" {
		writeErr(w, http.StatusBadRequest, "name and provider are required")
		return
	}
	enc, err := h.encryptConfig(d.Config)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "encrypt config")
		return
	}
	dom := models.Domain{
		OwnerID: models.SingleUserID,
		Name:    d.Name, Provider: d.Provider, ZoneID: d.ZoneID,
		Note: d.Note, ForMail: d.ForMail, ForLink: d.ForLink,
		LinkHosts: normalizeHosts(d.LinkHosts, d.ForLink),
		MailHosts: normalizeHosts(d.MailHosts, d.ForMail),
		Config:    enc,
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
	writeJSON(w, http.StatusCreated, dom)
}

func (h *Handler) updateDomain(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, "bad id")
		return
	}
	var dom models.Domain
	if h.db.First(&dom, id).Error != nil {
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
	dom.ForMail = d.ForMail
	dom.ForLink = d.ForLink
	dom.LinkHosts = normalizeHosts(d.LinkHosts, d.ForLink)
	dom.MailHosts = normalizeHosts(d.MailHosts, d.ForMail)
	if len(d.Config) > 0 {
		enc, err := h.encryptConfig(d.Config)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "encrypt config")
			return
		}
		dom.Config = enc
	}
	h.db.Save(&dom)
	writeJSON(w, http.StatusOK, dom)
}

func (h *Handler) deleteDomain(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, "bad id")
		return
	}
	h.db.Delete(&models.Domain{}, id)
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
	if h.db.First(&dom, id).Error != nil {
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
