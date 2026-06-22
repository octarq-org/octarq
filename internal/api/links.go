package api

import (
	"crypto/rand"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jungley/led/internal/models"
	qrcode "github.com/skip2/go-qrcode"
)

const slugAlphabet = "abcdefghijkmnpqrstuvwxyz23456789"

func randomSlug(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	for i := range b {
		b[i] = slugAlphabet[int(b[i])%len(slugAlphabet)]
	}
	return string(b)
}

func idParam(r *http.Request) (uint, bool) {
	v, err := strconv.ParseUint(r.PathValue("id"), 10, 64)
	if err != nil {
		return 0, false
	}
	return uint(v), true
}

// linkDTO is the create/update payload.
type linkDTO struct {
	Host      string     `json:"host"`
	Slug      string     `json:"slug"`
	Target    string     `json:"target"`
	Password  string     `json:"password"`
	Note      string     `json:"note"`
	Title     string     `json:"title"`
	ExpiresAt *time.Time `json:"expiresAt"`
	Enabled   *bool      `json:"enabled"`
}

type linkView struct {
	models.Link
	HasPassword bool `json:"hasPassword"`
}

func view(l models.Link) linkView {
	return linkView{Link: l, HasPassword: l.Password != ""}
}

func (h *Handler) listLinks(w http.ResponseWriter, r *http.Request) {
	var links []models.Link
	q := h.db.Order("created_at DESC")
	if s := r.URL.Query().Get("q"); s != "" {
		like := "%" + s + "%"
		q = q.Where("slug LIKE ? OR target LIKE ? OR note LIKE ? OR title LIKE ?", like, like, like, like)
	}
	q.Find(&links)
	out := make([]linkView, len(links))
	for i, l := range links {
		out[i] = view(l)
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *Handler) getLink(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, "bad id")
		return
	}
	var l models.Link
	if h.db.First(&l, id).Error != nil {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	writeJSON(w, http.StatusOK, view(l))
}

func (h *Handler) createLink(w http.ResponseWriter, r *http.Request) {
	var d linkDTO
	if err := readJSON(r, &d); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	d.Target = strings.TrimSpace(d.Target)
	if d.Target == "" {
		writeErr(w, http.StatusBadRequest, "target is required")
		return
	}
	if !strings.Contains(d.Target, "://") {
		d.Target = "https://" + d.Target
	}
	slug := strings.TrimSpace(d.Slug)
	if slug == "" {
		slug = randomSlug(6)
	}
	enabled := true
	if d.Enabled != nil {
		enabled = *d.Enabled
	}
	l := models.Link{
		OwnerID: models.SingleUserID,
		Host:    strings.TrimSpace(d.Host), Slug: slug, Target: d.Target,
		Password: d.Password, Note: d.Note, Title: d.Title,
		ExpiresAt: d.ExpiresAt, Enabled: enabled,
	}
	if err := h.db.Create(&l).Error; err != nil {
		writeErr(w, http.StatusConflict, "slug already exists on this host")
		return
	}
	writeJSON(w, http.StatusCreated, view(l))
}

func (h *Handler) updateLink(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, "bad id")
		return
	}
	var l models.Link
	if h.db.First(&l, id).Error != nil {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	var d linkDTO
	if err := readJSON(r, &d); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	if d.Slug != "" {
		l.Slug = strings.TrimSpace(d.Slug)
	}
	if d.Target != "" {
		t := strings.TrimSpace(d.Target)
		if !strings.Contains(t, "://") {
			t = "https://" + t
		}
		l.Target = t
	}
	l.Host = strings.TrimSpace(d.Host)
	l.Note = d.Note
	l.Title = d.Title
	l.Password = d.Password
	l.ExpiresAt = d.ExpiresAt
	if d.Enabled != nil {
		l.Enabled = *d.Enabled
	}
	if err := h.db.Save(&l).Error; err != nil {
		writeErr(w, http.StatusConflict, "slug already exists on this host")
		return
	}
	writeJSON(w, http.StatusOK, view(l))
}

func (h *Handler) deleteLink(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, "bad id")
		return
	}
	h.db.Where("link_id = ?", id).Delete(&models.LinkEvent{})
	h.db.Delete(&models.Link{}, id)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// linkStats returns basic analytics: totals, a daily time series, and the
// top referers / countries / devices / browsers over the requested window.
func (h *Handler) linkStats(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, "bad id")
		return
	}
	days := 30
	if d := r.URL.Query().Get("days"); d != "" {
		if n, err := strconv.Atoi(d); err == nil && n > 0 && n <= 365 {
			days = n
		}
	}
	since := time.Now().AddDate(0, 0, -days)

	top := func(col string) []statKV {
		var rows []statKV
		h.db.Model(&models.LinkEvent{}).
			Select(col+" as key, count(*) as count").
			Where("link_id = ? AND created_at >= ? AND "+col+" <> ''", id, since).
			Group(col).Order("count DESC").Limit(10).Scan(&rows)
		return rows
	}

	var total int64
	h.db.Model(&models.LinkEvent{}).Where("link_id = ?", id).Count(&total)

	var series []statKV
	h.db.Model(&models.LinkEvent{}).
		Select("strftime('%Y-%m-%d', created_at) as key, count(*) as count").
		Where("link_id = ? AND created_at >= ?", id, since).
		Group("key").Order("key ASC").Scan(&series)
	// Postgres uses to_char; fall back when sqlite strftime yields nothing.
	if len(series) == 0 && h.cfg.DBDriver == "postgres" {
		h.db.Model(&models.LinkEvent{}).
			Select("to_char(created_at, 'YYYY-MM-DD') as key, count(*) as count").
			Where("link_id = ? AND created_at >= ?", id, since).
			Group("key").Order("key ASC").Scan(&series)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"total":     total,
		"windowed":  sum(series),
		"days":      days,
		"series":    series,
		"referers":  top("referer"),
		"countries": top("country"),
		"devices":   top("device"),
		"browsers":  top("browser"),
	})
}

// statKV is a key/count pair used across link analytics responses.
type statKV struct {
	Key   string `json:"key"`
	Count int64  `json:"count"`
}

func sum(kvs []statKV) int64 {
	var t int64
	for _, k := range kvs {
		t += k.Count
	}
	return t
}

func (h *Handler) linkQR(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, "bad id")
		return
	}
	var l models.Link
	if h.db.First(&l, id).Error != nil {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	target := h.shortURL(l)
	png, err := qrcode.Encode(target, qrcode.Medium, 320)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "qr failed")
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Write(png)
}

// shortURL builds the public URL for a link.
func (h *Handler) shortURL(l models.Link) string {
	if l.Host != "" {
		return "https://" + l.Host + "/" + l.Slug
	}
	if h.cfg.BaseURL != "" {
		return h.cfg.BaseURL + "/" + l.Slug
	}
	return "/" + l.Slug
}
