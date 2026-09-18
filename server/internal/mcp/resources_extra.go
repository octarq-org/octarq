package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	// ResourceURILinksTrending ranks the workspace's short links by clicks.
	ResourceURILinksTrending = "octarq://links/trending"
	// ResourceTemplateMailboxLatest addresses the newest message of one mailbox.
	ResourceTemplateMailboxLatest = "octarq://mailboxes/{id}/latest"
)

// TrendingLinksResource is the payload of octarq://links/trending.
type TrendingLinksResource struct {
	OrgID       uint            `json:"org_id"`
	Links       []TopLinkMetric `json:"links"`
	GeneratedAt time.Time       `json:"generated_at"`
}

// MailboxLatestResource is the payload of octarq://mailboxes/{id}/latest: the
// newest message metadata plus the unread count. Bodies stay behind the tools.
type MailboxLatestResource struct {
	OrgID       uint                `json:"org_id"`
	MailboxID   uint                `json:"mailbox_id"`
	Address     string              `json:"address"`
	Unread      int64               `json:"unread"`
	Latest      *MailboxLatestEmail `json:"latest"`
	GeneratedAt time.Time           `json:"generated_at"`
}

// MailboxLatestEmail carries one message's metadata (never the raw body).
type MailboxLatestEmail struct {
	ID         uint      `json:"id"`
	From       string    `json:"from"`
	Subject    string    `json:"subject"`
	ReceivedAt time.Time `json:"received_at"`
}

// subscriptionTracker records resources/subscribe interest so push
// notifications only fan out to URIs a client actually watches.
type subscriptionTracker struct {
	mu   sync.Mutex
	subs map[string]int
}

func (t *subscriptionTracker) add(uri string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.subs == nil {
		t.subs = map[string]int{}
	}
	t.subs[uri]++
}

func (t *subscriptionTracker) remove(uri string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.subs == nil {
		return
	}
	if t.subs[uri] <= 1 {
		delete(t.subs, uri)
		return
	}
	t.subs[uri]--
}

func (t *subscriptionTracker) watched(uri string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.subs[uri] > 0
}

// registerExtraResources wires the trending + mailbox-latest endpoints and the
// resources/subscribe capability onto the server.
func (s *server) registerExtraResources(srv *mcp.Server) {
	srv.AddResource(&mcp.Resource{
		URI:         ResourceURILinksTrending,
		Name:        "links_trending",
		Title:       "Trending Links",
		Description: "Short links of the current workspace ranked by clicks. Poll or subscribe for traffic spikes.",
		MIMEType:    "application/json",
	}, s.handleResourceLinksTrending)

	srv.AddResourceTemplate(&mcp.ResourceTemplate{
		URITemplate: ResourceTemplateMailboxLatest,
		Name:        "mailbox_latest",
		Title:       "Mailbox Latest",
		Description: "Newest message metadata of one mailbox (no bodies). Subscribe to stop polling for arrivals.",
		MIMEType:    "application/json",
	}, s.handleResourceMailboxLatest)
}

// handleSubscribe/handleUnsubscribe back the resources/subscribe capability.
func (s *server) handleSubscribe(_ context.Context, req *mcp.SubscribeRequest) error {
	if req == nil || req.Params == nil || req.Params.URI == "" {
		return errors.New("missing subscribe URI")
	}
	if s.subs == nil {
		s.subs = &subscriptionTracker{}
	}
	s.subs.add(req.Params.URI)
	return nil
}

func (s *server) handleUnsubscribe(_ context.Context, req *mcp.UnsubscribeRequest) error {
	if req == nil || req.Params == nil || req.Params.URI == "" {
		return errors.New("missing unsubscribe URI")
	}
	if s.subs == nil {
		return nil
	}
	s.subs.remove(req.Params.URI)
	return nil
}

// notifyResourceUpdated pushes notifications/resources/updated to subscribed
// sessions. It is a no-op when nobody watches the URI.
func (s *server) notifyResourceUpdated(ctx context.Context, uri string) {
	if s.subs == nil || !s.subs.watched(uri) {
		return
	}
	if s.srv == nil {
		return
	}
	_ = s.srv.ResourceUpdated(ctx, &mcp.ResourceUpdatedNotificationParams{URI: uri})
}

// NotifyMailboxLatest announces a new arrival without polling.
func (s *server) NotifyMailboxLatest(ctx context.Context, mailboxID uint) {
	s.notifyResourceUpdated(ctx, fmt.Sprintf("octarq://mailboxes/%d/latest", mailboxID))
}

// NotifyLinksTrending announces a traffic spike.
func (s *server) NotifyLinksTrending(ctx context.Context) {
	s.notifyResourceUpdated(ctx, ResourceURILinksTrending)
}

func (s *server) handleResourceLinksTrending(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
	if req == nil || req.Params == nil {
		return nil, errors.New("missing request params")
	}
	if req.Params.URI != ResourceURILinksTrending {
		return nil, mcp.ResourceNotFoundError(req.Params.URI)
	}
	orgID, err := s.resolveOrg(ctx)
	if err != nil {
		return nil, err
	}

	res := TrendingLinksResource{OrgID: orgID, Links: []TopLinkMetric{}, GeneratedAt: time.Now()}
	if s.gdb != nil && s.gdb.Migrator().HasTable("links") {
		var top []TopLinkMetric
		s.gdb.WithContext(ctx).Table("links").
			Select("id, host, slug, title, clicks").
			Where("owner_id = ? AND archived = ?", orgID, false).
			Order("clicks DESC").
			Limit(10).
			Scan(&top)
		if len(top) > 0 {
			res.Links = top
		}
	}
	return marshalResource(req.Params.URI, res)
}

func (s *server) handleResourceMailboxLatest(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
	if req == nil || req.Params == nil {
		return nil, errors.New("missing request params")
	}
	mailboxID, ok := parseMailboxLatestURI(req.Params.URI)
	if !ok {
		return nil, mcp.ResourceNotFoundError(req.Params.URI)
	}
	orgID, err := s.resolveOrg(ctx)
	if err != nil {
		return nil, err
	}

	res := MailboxLatestResource{OrgID: orgID, MailboxID: mailboxID, GeneratedAt: time.Now()}
	if s.gdb == nil || !s.gdb.Migrator().HasTable("mailboxes") {
		return marshalResource(req.Params.URI, res)
	}
	var mb struct {
		ID      uint
		Address string
	}
	if err := s.gdb.WithContext(ctx).Table("mailboxes").
		Select("id, address").
		Where("id = ? AND owner_id = ?", mailboxID, orgID).
		First(&mb).Error; err != nil {
		return nil, mcp.ResourceNotFoundError(req.Params.URI)
	}
	res.Address = mb.Address

	if s.gdb.Migrator().HasTable("emails") {
		s.gdb.WithContext(ctx).Table("emails").
			Where("mailbox_id = ? AND read = ?", mailboxID, false).
			Count(&res.Unread)
		var latest struct {
			ID         uint
			From       string `gorm:"column:from_addr"`
			Subject    string
			ReceivedAt time.Time `gorm:"column:received_at"`
		}
		if err := s.gdb.WithContext(ctx).Table("emails").
			Select("id, from_addr, subject, received_at").
			Where("mailbox_id = ?", mailboxID).
			Order("received_at DESC").
			First(&latest).Error; err == nil {
			res.Latest = &MailboxLatestEmail{
				ID:         latest.ID,
				From:       latest.From,
				Subject:    latest.Subject,
				ReceivedAt: latest.ReceivedAt,
			}
		}
	}
	return marshalResource(req.Params.URI, res)
}

func parseMailboxLatestURI(uri string) (uint, bool) {
	const prefix = "octarq://mailboxes/"
	const suffix = "/latest"
	if !strings.HasPrefix(uri, prefix) || !strings.HasSuffix(uri, suffix) {
		return 0, false
	}
	raw := strings.TrimSuffix(strings.TrimPrefix(uri, prefix), suffix)
	if raw == "" || strings.Contains(raw, "/") {
		return 0, false
	}
	id, err := strconv.ParseUint(raw, 10, 64)
	if err != nil || id == 0 {
		return 0, false
	}
	return uint(id), true
}

func marshalResource(uri string, v any) (*mcp.ReadResourceResult, error) {
	buf, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return nil, err
	}
	return &mcp.ReadResourceResult{
		Contents: []*mcp.ResourceContents{
			{
				URI:      uri,
				MIMEType: "application/json",
				Text:     string(buf),
			},
		},
	}, nil
}
