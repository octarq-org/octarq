package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/octarq-org/octarq/server/internal/models"
	"github.com/octarq-org/octarq/server/plugin"
)

const (
	// ResourceURITrafficMetrics is the canonical MCP resource URI for workspace traffic metrics.
	ResourceURITrafficMetrics = "octarq://metrics/traffic"
	// ResourceURIDomainsHealth is the canonical MCP resource URI for workspace domain health status.
	ResourceURIDomainsHealth = "octarq://domains/health"
)

// TrafficMetricsResource represents the traffic metrics returned by octarq://metrics/traffic.
type TrafficMetricsResource struct {
	OrgID       uint            `json:"org_id"`
	TotalLinks  int64           `json:"total_links"`
	ActiveLinks int64           `json:"active_links"`
	TotalClicks int64           `json:"total_clicks"`
	Clicks7d    int64           `json:"clicks_7d"`
	Clicks30d   int64           `json:"clicks_30d"`
	TopLinks    []TopLinkMetric `json:"top_links"`
	GeneratedAt time.Time       `json:"generated_at"`
}

// TopLinkMetric represents a top-performing short link.
type TopLinkMetric struct {
	ID     uint   `json:"id"`
	Host   string `json:"host,omitempty"`
	Slug   string `json:"slug"`
	Title  string `json:"title,omitempty"`
	Clicks int64  `json:"clicks"`
}

// DomainsHealthResource represents the domain health overview returned by octarq://domains/health.
type DomainsHealthResource struct {
	OrgID             uint                 `json:"org_id"`
	TotalDomains      int                  `json:"total_domains"`
	ActiveMailDomains int                  `json:"active_mail_domains"`
	ActiveLinkDomains int                  `json:"active_link_domains"`
	Domains           []DomainHealthDetail `json:"domains"`
	GeneratedAt       time.Time            `json:"generated_at"`
}

// DomainHealthDetail represents health and routing status for one domain.
type DomainHealthDetail struct {
	ID                uint            `json:"id"`
	Name              string          `json:"name"`
	Status            string          `json:"status"` // "healthy", "pending_provider", "pending_zone", "configured"
	ForMail           bool            `json:"for_mail"`
	ForLink           bool            `json:"for_link"`
	ProviderAccountID uint            `json:"provider_account_id"`
	ZoneID            string          `json:"zone_id,omitempty"`
	LinkHosts         models.HostList `json:"link_hosts,omitempty"`
	MailHosts         models.HostList `json:"mail_hosts,omitempty"`
	CreatedAt         time.Time       `json:"created_at"`
	UpdatedAt         time.Time       `json:"updated_at"`
}

// domainQueryRow holds internal query projection from the domains table.
type domainQueryRow struct {
	ID                uint            `gorm:"primaryKey"`
	OrgID             uint            `gorm:"column:owner_id"`
	Name              string          `gorm:"column:name"`
	ProviderAccountID uint            `gorm:"column:provider_account_id"`
	ZoneID            string          `gorm:"column:zone_id"`
	ForMail           bool            `gorm:"column:for_mail"`
	ForLink           bool            `gorm:"column:for_link"`
	LinkHosts         models.HostList `gorm:"column:link_hosts;type:text"`
	MailHosts         models.HostList `gorm:"column:mail_hosts;type:text"`
	CreatedAt         time.Time       `gorm:"column:created_at"`
	UpdatedAt         time.Time       `gorm:"column:updated_at"`
}

// registerResources wires standard read-only MCP resources onto the server.
func (s *server) registerResources(srv *mcp.Server) {
	srv.AddResource(&mcp.Resource{
		URI:         ResourceURITrafficMetrics,
		Name:        "traffic_metrics",
		Title:       "Traffic Metrics",
		Description: "Read-only traffic and short-link redirection analytics for the current workspace.",
		MIMEType:    "application/json",
	}, s.handleResourceTraffic)

	srv.AddResource(&mcp.Resource{
		URI:         ResourceURIDomainsHealth,
		Name:        "domains_health",
		Title:       "Domains Health",
		Description: "Read-only DNS, routing capability, and health status for domains managed in the current workspace.",
		MIMEType:    "application/json",
	}, s.handleResourceDomainsHealth)
}

func (s *server) resolveOrg(ctx context.Context) (uint, error) {
	orgID := plugin.OrgIDFromContext(ctx)
	if orgID == 0 {
		orgID = s.orgID
	}
	if orgID == 0 {
		return 0, errNoOrgInContext
	}
	return orgID, nil
}

func (s *server) handleResourceTraffic(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
	if req == nil || req.Params == nil {
		return nil, errors.New("missing request params")
	}
	if req.Params.URI != ResourceURITrafficMetrics {
		return nil, mcp.ResourceNotFoundError(req.Params.URI)
	}

	orgID, err := s.resolveOrg(ctx)
	if err != nil {
		return nil, err
	}

	res := TrafficMetricsResource{
		OrgID:       orgID,
		TopLinks:    []TopLinkMetric{},
		GeneratedAt: time.Now(),
	}

	if s.gdb != nil {
		hasLinks := s.gdb.Migrator().HasTable("links")
		if hasLinks {
			s.gdb.WithContext(ctx).Table("links").
				Where("owner_id = ?", orgID).
				Count(&res.TotalLinks)

			s.gdb.WithContext(ctx).Table("links").
				Where("owner_id = ? AND archived = ? AND enabled = ?", orgID, false, true).
				Count(&res.ActiveLinks)

			s.gdb.WithContext(ctx).Table("links").
				Where("owner_id = ?", orgID).
				Select("COALESCE(SUM(clicks), 0)").
				Scan(&res.TotalClicks)

			var top []TopLinkMetric
			s.gdb.WithContext(ctx).Table("links").
				Select("id, host, slug, title, clicks").
				Where("owner_id = ? AND archived = ?", orgID, false).
				Order("clicks DESC").
				Limit(5).
				Scan(&top)
			if len(top) > 0 {
				res.TopLinks = top
			}

			if s.gdb.Migrator().HasTable("link_events") {
				sevenDaysAgo := time.Now().AddDate(0, 0, -7)
				thirtyDaysAgo := time.Now().AddDate(0, 0, -30)

				s.gdb.WithContext(ctx).Table("link_events").
					Joins("JOIN links ON links.id = link_events.link_id").
					Where("links.owner_id = ? AND link_events.is_bot = ? AND link_events.created_at >= ?", orgID, false, sevenDaysAgo).
					Count(&res.Clicks7d)

				s.gdb.WithContext(ctx).Table("link_events").
					Joins("JOIN links ON links.id = link_events.link_id").
					Where("links.owner_id = ? AND link_events.is_bot = ? AND link_events.created_at >= ?", orgID, false, thirtyDaysAgo).
					Count(&res.Clicks30d)
			}
		}
	}

	buf, err := json.MarshalIndent(res, "", "  ")
	if err != nil {
		return nil, err
	}

	return &mcp.ReadResourceResult{
		Contents: []*mcp.ResourceContents{
			{
				URI:      req.Params.URI,
				MIMEType: "application/json",
				Text:     string(buf),
			},
		},
	}, nil
}

func (s *server) handleResourceDomainsHealth(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
	if req == nil || req.Params == nil {
		return nil, errors.New("missing request params")
	}
	if req.Params.URI != ResourceURIDomainsHealth {
		return nil, mcp.ResourceNotFoundError(req.Params.URI)
	}

	orgID, err := s.resolveOrg(ctx)
	if err != nil {
		return nil, err
	}

	res := DomainsHealthResource{
		OrgID:       orgID,
		Domains:     []DomainHealthDetail{},
		GeneratedAt: time.Now(),
	}

	if s.gdb != nil && s.gdb.Migrator().HasTable("domains") {
		var rows []domainQueryRow
		if err := s.gdb.WithContext(ctx).Table("domains").
			Where("owner_id = ?", orgID).
			Order("name ASC").
			Find(&rows).Error; err == nil {

			res.TotalDomains = len(rows)
			for _, r := range rows {
				status := "configured"
				if r.ZoneID != "" && r.ProviderAccountID != 0 {
					status = "healthy"
				} else if r.ProviderAccountID == 0 {
					status = "pending_provider"
				} else if r.ZoneID == "" {
					status = "pending_zone"
				}

				if r.ForMail {
					res.ActiveMailDomains++
				}
				if r.ForLink {
					res.ActiveLinkDomains++
				}

				res.Domains = append(res.Domains, DomainHealthDetail{
					ID:                r.ID,
					Name:              r.Name,
					Status:            status,
					ForMail:           r.ForMail,
					ForLink:           r.ForLink,
					ProviderAccountID: r.ProviderAccountID,
					ZoneID:            r.ZoneID,
					LinkHosts:         r.LinkHosts,
					MailHosts:         r.MailHosts,
					CreatedAt:         r.CreatedAt,
					UpdatedAt:         r.UpdatedAt,
				})
			}
		}
	}

	buf, err := json.MarshalIndent(res, "", "  ")
	if err != nil {
		return nil, err
	}

	return &mcp.ReadResourceResult{
		Contents: []*mcp.ResourceContents{
			{
				URI:      req.Params.URI,
				MIMEType: "application/json",
				Text:     string(buf),
			},
		},
	}, nil
}
