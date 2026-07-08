package api

import (
	"context"
	"crypto/rand"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/octarq-org/octarq/internal/models"
	qrcode "github.com/skip2/go-qrcode"
)

var (
	reTitle    = regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`)
	reDesc     = regexp.MustCompile(`(?is)<meta[^>]+name=["']description["'][^>]+content=["'](.*?)["']`)
	reOgTitle  = regexp.MustCompile(`(?is)<meta[^>]+property=["']og:title["'][^>]+content=["'](.*?)["']`)
	reOgTitle2 = regexp.MustCompile(`(?is)<meta[^>]+content=["'](.*?)["'][^>]+property=["']og:title["']`)
)

// fetchPageMeta does a best-effort GET and extracts <title> and meta description.
func fetchPageMeta(ctx context.Context, rawURL string) (title, desc string) {
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	// Guarded fetch: blocks internal/cloud-metadata IPs and redirect-based SSRF.
	resp, err := safeGet(ctx, rawURL)
	if err != nil {
		return "", ""
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 256<<10)) // 256 KiB is plenty for <head>
	if m := reOgTitle.FindSubmatch(body); m != nil {
		title = strings.TrimSpace(html.UnescapeString(string(m[1])))
	} else if m := reOgTitle2.FindSubmatch(body); m != nil {
		title = strings.TrimSpace(html.UnescapeString(string(m[1])))
	} else if m := reTitle.FindSubmatch(body); m != nil {
		title = strings.TrimSpace(html.UnescapeString(string(m[1])))
	}
	if m := reDesc.FindSubmatch(body); m != nil {
		desc = strings.TrimSpace(html.UnescapeString(string(m[1])))
	}
	return title, desc
}

const slugAlphabet = "abcdefghijkmnpqrstuvwxyz23456789"

func randomSlug(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	for i := range b {
		b[i] = slugAlphabet[int(b[i])%len(slugAlphabet)]
	}
	return string(b)
}

// linkDTO is the create/update payload.
type linkDTO struct {
	Host       string     `json:"host"`
	Slug       string     `json:"slug"`
	Target     string     `json:"target"`
	Password   string     `json:"password"`
	Note       string     `json:"note"`
	Title      string     `json:"title"`
	Tags       string     `json:"tags"`
	ExpiresAt  *time.Time `json:"expiresAt"`
	ExpiredURL string     `json:"expiredUrl"`
	ClickLimit int64      `json:"clickLimit"`
	Archived   *bool      `json:"archived"`
	Enabled    *bool      `json:"enabled"`
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
	q := h.orgDB(r).Order("created_at DESC")
	// Archived links are hidden unless explicitly requested (?archived=1).
	if r.URL.Query().Get("archived") == "1" {
		q = q.Where("archived = ?", true)
	} else {
		q = q.Where("archived = ?", false)
	}
	if s := r.URL.Query().Get("q"); s != "" {
		like := "%" + s + "%"
		q = q.Where("slug LIKE ? OR target LIKE ? OR note LIKE ? OR title LIKE ? OR tags LIKE ?", like, like, like, like, like)
	}
	if tag := r.URL.Query().Get("tag"); tag != "" {
		q = q.Where("tags LIKE ?", "%"+tag+"%")
	}
	if host := r.URL.Query().Get("host"); host != "" {
		q = q.Where("host = ?", host)
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
	q.Find(&links)
	out := make([]linkView, len(links))
	for i, l := range links {
		out[i] = view(l)
	}
	writeJSON(w, http.StatusOK, out)
}

// linkMetadata fetches the target page's <title>, description, and favicon so
// the dashboard can prefill a link's title (dub-style). Best-effort.
func (h *Handler) linkMetadata(w http.ResponseWriter, r *http.Request) {
	raw := strings.TrimSpace(r.URL.Query().Get("url"))
	if raw == "" {
		writeErr(w, http.StatusBadRequest, "url required")
		return
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		writeErr(w, http.StatusBadRequest, "invalid url")
		return
	}
	title, desc := fetchPageMeta(r.Context(), raw)
	favicon := u.Scheme + "://" + u.Host + "/favicon.ico"
	writeJSON(w, http.StatusOK, map[string]any{
		"title": title, "description": desc, "favicon": favicon,
	})
}

func (h *Handler) getLink(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, "bad id")
		return
	}
	var l models.Link
	if h.db.Where("id = ? AND owner_id = ?", id, h.orgID(r)).First(&l).Error != nil {
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
	if h.isReservedSlug(slug) {
		writeErr(w, http.StatusConflict, "slug is reserved")
		return
	}
	enabled := true
	if d.Enabled != nil {
		enabled = *d.Enabled
	}
	l := models.Link{
		OrgID: h.orgID(r),
		Host:  strings.TrimSpace(d.Host), Slug: slug, Target: d.Target,
		Password: d.Password, Note: d.Note, Title: d.Title, Tags: d.Tags,
		ExpiresAt: d.ExpiresAt, ExpiredURL: d.ExpiredURL, ClickLimit: d.ClickLimit,
		Enabled: enabled,
	}
	if err := h.db.Create(&l).Error; err != nil {
		writeErr(w, http.StatusConflict, "slug already exists on this host")
		return
	}
	h.audit(r, "link.create", "link", l.ID, map[string]any{"slug": l.Slug, "target": l.Target})

	if l.Title == "" {
		payload, _ := json.Marshal(map[string]any{
			"id":     l.ID,
			"target": l.Target,
		})
		_ = h.queue.Enqueue(r.Context(), "link.crawl", payload)
	}

	_ = h.auth.Cache().Delete(r.Context(), "link:redirect:"+l.Host+":"+l.Slug)
	writeJSON(w, http.StatusCreated, view(l))
}

func (h *Handler) exportLinksCSV(w http.ResponseWriter, r *http.Request) {
	var links []models.Link
	h.orgDB(r).Order("created_at DESC").Find(&links)

	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename=\"links.csv\"")

	cw := csv.NewWriter(w)
	cw.Write([]string{"ID", "Host", "Slug", "Target", "Title", "Clicks", "CreatedAt"})
	for _, l := range links {
		cw.Write([]string{
			fmt.Sprintf("%d", l.ID),
			l.Host,
			l.Slug,
			l.Target,
			l.Title,
			fmt.Sprintf("%d", l.Clicks),
			l.CreatedAt.Format(time.RFC3339),
		})
	}
	cw.Flush()
}

func (h *Handler) updateLink(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, "bad id")
		return
	}
	var l models.Link
	if h.db.Where("id = ? AND owner_id = ?", id, h.orgID(r)).First(&l).Error != nil {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	var d linkDTO
	if err := readJSON(r, &d); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	if d.Slug != "" {
		slug := strings.TrimSpace(d.Slug)
		if slug != l.Slug && h.isReservedSlug(slug) {
			writeErr(w, http.StatusConflict, "slug is reserved")
			return
		}
		l.Slug = slug
	}
	if d.Target != "" {
		t := strings.TrimSpace(d.Target)
		if !strings.Contains(t, "://") {
			t = "https://" + t
		}
		l.Target = t
	}
	oldHost := l.Host
	oldSlug := l.Slug

	l.Host = strings.TrimSpace(d.Host)
	l.Note = d.Note
	l.Title = d.Title
	l.Tags = d.Tags
	l.Password = d.Password
	l.ExpiresAt = d.ExpiresAt
	l.ExpiredURL = d.ExpiredURL
	l.ClickLimit = d.ClickLimit
	if d.Archived != nil {
		l.Archived = *d.Archived
	}
	if d.Enabled != nil {
		l.Enabled = *d.Enabled
	}
	if err := h.db.Save(&l).Error; err != nil {
		writeErr(w, http.StatusConflict, "slug already exists on this host")
		return
	}

	_ = h.auth.Cache().Delete(r.Context(), "link:redirect:"+oldHost+":"+oldSlug)
	if oldHost != l.Host || oldSlug != l.Slug {
		_ = h.auth.Cache().Delete(r.Context(), "link:redirect:"+l.Host+":"+l.Slug)
	}

	h.audit(r, "link.update", "link", l.ID, map[string]any{"slug": l.Slug, "target": l.Target})
	writeJSON(w, http.StatusOK, view(l))
}

func (h *Handler) deleteLink(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, "bad id")
		return
	}
	var l models.Link
	if h.db.Where("id = ? AND owner_id = ?", id, h.orgID(r)).First(&l).Error != nil {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	h.db.Delete(&l)
	_ = h.auth.Cache().Delete(r.Context(), "link:redirect:"+l.Host+":"+l.Slug)

	h.db.Where("link_id = ?", id).Delete(&models.LinkEvent{})
	h.audit(r, "link.delete", "link", id, nil)
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
	// Ensure the link belongs to the caller's org before exposing its analytics.
	var l models.Link
	if h.db.Where("id = ? AND owner_id = ?", id, h.orgID(r)).First(&l).Error != nil {
		writeErr(w, http.StatusNotFound, "not found")
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
		rows := make([]statKV, 0)
		q := h.db.Model(&models.LinkEvent{}).
			Where("link_id = ? AND created_at >= ? AND "+col+" <> ''", id, since)
		if col == "device" {
			q = q.Select(col + " as key, count(distinct(ip || ' ' || ua)) as count")
		} else {
			q = q.Select(col + " as key, count(*) as count")
		}
		q.Group(col).Order("count DESC").Limit(10).Scan(&rows)
		return rows
	}

	var total int64
	h.db.Model(&models.LinkEvent{}).Where("link_id = ?", id).Count(&total)

	series := make([]statKV, 0)
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
		"regions":   top("region"),
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
	if h.db.Where("id = ? AND owner_id = ?", id, h.orgID(r)).First(&l).Error != nil {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	target := shortURL(r, l)
	png, err := qrcode.Encode(target, qrcode.Medium, 320)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "qr failed")
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Write(png)
}

// shortURL builds the public URL for a link. When the link has its own host it
// is used; otherwise the URL is derived from the incoming request so no extra
// configuration is needed.
func shortURL(r *http.Request, l models.Link) string {
	host := l.Host
	scheme := "https"
	if host == "" {
		host = r.Host
		if r.TLS == nil && r.Header.Get("X-Forwarded-Proto") != "https" {
			scheme = "http"
		}
	}
	return scheme + "://" + host + "/" + l.Slug
}
