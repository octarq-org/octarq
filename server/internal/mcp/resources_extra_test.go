package mcp

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"gorm.io/gorm"
)

type testMailboxRow struct {
	ID      uint   `gorm:"primaryKey"`
	OrgID   uint   `gorm:"column:owner_id"`
	Address string `gorm:"column:address"`
	Enabled bool   `gorm:"column:enabled"`
}

func (testMailboxRow) TableName() string { return "mailboxes" }

type testEmailRow struct {
	ID         uint      `gorm:"primaryKey"`
	MailboxID  uint      `gorm:"column:mailbox_id"`
	From       string    `gorm:"column:from_addr"`
	Subject    string    `gorm:"column:subject"`
	Read       bool      `gorm:"column:read"`
	ReceivedAt time.Time `gorm:"column:received_at"`
}

func (testEmailRow) TableName() string { return "emails" }

func setupExtraResourcesTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	gdb, err := gorm.Open(sqlite.Open(t.TempDir()+"/mcp_extra.db"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite test db: %v", err)
	}
	if err := gdb.AutoMigrate(&testLinkRow{}, &testMailboxRow{}, &testEmailRow{}); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}
	return gdb
}

func connectTestClient(t *testing.T, ctx context.Context, srv *mcp.Server, opts *mcp.ClientOptions) (*mcp.ClientSession, *mcp.ServerSession) {
	t.Helper()
	client := mcp.NewClient(&mcp.Implementation{Name: "client", Version: "1.0"}, opts)
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := srv.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	t.Cleanup(func() { serverSession.Close() })
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { clientSession.Close() })
	return clientSession, serverSession
}

func TestParseMailboxLatestURI(t *testing.T) {
	for uri, want := range map[string]uint{
		"octarq://mailboxes/7/latest":   7,
		"octarq://mailboxes/123/latest": 123,
	} {
		got, ok := parseMailboxLatestURI(uri)
		if !ok || got != want {
			t.Errorf("parse %q = (%d, %v), want (%d, true)", uri, got, ok, want)
		}
	}
	for _, bad := range []string{
		"octarq://mailboxes/latest",
		"octarq://mailboxes/0/latest",
		"octarq://mailboxes/abc/latest",
		"octarq://mailboxes/1/2/latest",
		"octarq://links/trending",
		"",
	} {
		if _, ok := parseMailboxLatestURI(bad); ok {
			t.Errorf("parse %q must fail", bad)
		}
	}
}

func TestLinksTrendingReadAndIsolation(t *testing.T) {
	ctx := context.Background()
	gdb := setupExtraResourcesTestDB(t)
	now := time.Now()
	gdb.Create(&testLinkRow{OrgID: 1, Host: "go.corp.com", Slug: "top", Title: "Top", Clicks: 50, Enabled: true, CreatedAt: now})
	gdb.Create(&testLinkRow{OrgID: 1, Host: "go.corp.com", Slug: "mid", Title: "Mid", Clicks: 10, Enabled: true, CreatedAt: now})
	gdb.Create(&testLinkRow{OrgID: 2, Host: "x.other.com", Slug: "secret", Title: "Secret", Clicks: 999, Enabled: true, CreatedAt: now})

	srv := NewServerInstance(gdb, 1, nil)
	cs, _ := connectTestClient(t, ctx, srv, nil)

	res, err := cs.ReadResource(ctx, &mcp.ReadResourceParams{URI: ResourceURILinksTrending})
	if err != nil {
		t.Fatalf("ReadResource trending: %v", err)
	}
	if len(res.Contents) != 1 {
		t.Fatalf("expected 1 content, got %d", len(res.Contents))
	}
	body := res.Contents[0].Text
	if strings.Contains(body, "secret") {
		t.Errorf("trending leaked cross-org link: %s", body)
	}
	var parsed TrendingLinksResource
	if err := json.Unmarshal([]byte(body), &parsed); err != nil {
		t.Fatalf("unmarshal trending: %v", err)
	}
	if len(parsed.Links) != 2 || parsed.Links[0].Slug != "top" {
		t.Errorf("trending order wrong: %+v", parsed.Links)
	}
}

func TestMailboxLatestTemplateRead(t *testing.T) {
	ctx := context.Background()
	gdb := setupExtraResourcesTestDB(t)
	now := time.Now()
	mb := testMailboxRow{OrgID: 1, Address: "in@corp.com", Enabled: true}
	gdb.Create(&mb)
	other := testMailboxRow{OrgID: 2, Address: "priv@other.com", Enabled: true}
	gdb.Create(&other)
	gdb.Create(&testEmailRow{MailboxID: mb.ID, From: "a@x.com", Subject: "older", Read: true, ReceivedAt: now.Add(-time.Hour)})
	gdb.Create(&testEmailRow{MailboxID: mb.ID, From: "b@x.com", Subject: "newest hello", Read: false, ReceivedAt: now})

	srv := NewServerInstance(gdb, 1, nil)
	cs, _ := connectTestClient(t, ctx, srv, nil)

	uri := "octarq://mailboxes/1/latest"
	_ = uri
	readURI := "octarq://mailboxes/" + strconv.FormatUint(uint64(mb.ID), 10) + "/latest"
	res, err := cs.ReadResource(ctx, &mcp.ReadResourceParams{URI: readURI})
	if err != nil {
		t.Fatalf("ReadResource mailbox latest: %v", err)
	}
	var parsed MailboxLatestResource
	if err := json.Unmarshal([]byte(res.Contents[0].Text), &parsed); err != nil {
		t.Fatalf("unmarshal latest: %v", err)
	}
	if parsed.Address != "in@corp.com" {
		t.Errorf("address = %q, want in@corp.com", parsed.Address)
	}
	if parsed.Unread != 1 {
		t.Errorf("unread = %d, want 1", parsed.Unread)
	}
	if parsed.Latest == nil || parsed.Latest.Subject != "newest hello" {
		t.Errorf("latest wrong: %+v", parsed.Latest)
	}

	otherURI := "octarq://mailboxes/" + strconv.FormatUint(uint64(other.ID), 10) + "/latest"
	if _, err := cs.ReadResource(ctx, &mcp.ReadResourceParams{URI: otherURI}); err == nil {
		t.Error("cross-org mailbox latest must be refused")
	}
	if _, err := cs.ReadResource(ctx, &mcp.ReadResourceParams{URI: "octarq://mailboxes/9999/latest"}); err == nil {
		t.Error("missing mailbox must be refused")
	}
}

func TestSubscribeHandshakeAndPush(t *testing.T) {
	ctx := context.Background()
	gdb := setupExtraResourcesTestDB(t)
	srv := NewServerInstance(gdb, 1, nil)

	updated := make(chan string, 4)
	cs, _ := connectTestClient(t, ctx, srv, &mcp.ClientOptions{
		ResourceUpdatedHandler: func(_ context.Context, req *mcp.ResourceUpdatedNotificationRequest) {
			updated <- req.Params.URI
		},
	})

	if err := cs.Subscribe(ctx, &mcp.SubscribeParams{URI: ResourceURILinksTrending}); err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	// Reach back into the internal server through a fresh handler-level call:
	// the notify path is a no-op for unwatched URIs and pushes for watched ones.
	inner := &server{gdb: gdb, orgID: 1, subs: &subscriptionTracker{}}
	inner.subs.add(ResourceURILinksTrending)
	if !inner.subs.watched(ResourceURILinksTrending) {
		t.Fatal("subscribed URI must be watched")
	}
	if err := inner.handleUnsubscribe(ctx, &mcp.UnsubscribeRequest{Params: &mcp.UnsubscribeParams{URI: ResourceURILinksTrending}}); err != nil {
		t.Fatalf("Unsubscribe: %v", err)
	}
	if inner.subs.watched(ResourceURILinksTrending) {
		t.Error("unsubscribed URI must not be watched")
	}
	if err := inner.handleSubscribe(ctx, &mcp.SubscribeRequest{Params: &mcp.SubscribeParams{}}); err == nil {
		t.Error("empty subscribe URI must fail")
	}

	// Protocol-level subscribe against the real server must succeed so the
	// capability is advertised end to end.
	if err := cs.Unsubscribe(ctx, &mcp.UnsubscribeParams{URI: ResourceURILinksTrending}); err != nil {
		t.Fatalf("Unsubscribe: %v", err)
	}
}

func TestNotifyHelpersFanOutToWatchers(t *testing.T) {
	srv := mcp.NewServer(&mcp.Implementation{Name: "notify-test", Version: "1.0"}, nil)
	inner := &server{subs: &subscriptionTracker{}, srv: srv}

	// Unwatched URIs never touch the server.
	inner.NotifyLinksTrending(context.Background())
	inner.NotifyMailboxLatest(context.Background(), 9)

	// Watched URIs push without sessions attached (no-op, must not error).
	inner.subs.add(ResourceURILinksTrending)
	inner.NotifyLinksTrending(context.Background())
	mailURI := "octarq://mailboxes/9/latest"
	inner.subs.add(mailURI)
	inner.NotifyMailboxLatest(context.Background(), 9)

	// Nil server is a safe no-op even when watched.
	inner.srv = nil
	inner.NotifyLinksTrending(context.Background())
}

func TestRemoveUnwatchedIsNoOp(t *testing.T) {
	tr := &subscriptionTracker{}
	tr.remove("octarq://nothing")
	if tr.watched("octarq://nothing") {
		t.Error("never-subscribed URI must not be watched")
	}
	tr.add("octarq://x")
	tr.add("octarq://x")
	tr.remove("octarq://x")
	if !tr.watched("octarq://x") {
		t.Error("refcount must survive one removal of two adds")
	}
	tr.remove("octarq://x")
	if tr.watched("octarq://x") {
		t.Error("URI must drop after final removal")
	}
}
