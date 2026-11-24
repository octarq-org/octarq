package plugin

import (
	"context"
	"encoding/json"
	"errors"
)

const ServiceMessagingSend = "messaging.send"

const ServiceMessagingReady = "messaging.ready"

const ServiceMessagingSendSystem = "messaging.send.system"

const ServiceEventsDispatch = "events.dispatch"

const ServiceCloudUsage = "cloud.usage"

const ServiceStorage = "storage.service"

const ServiceCredentialResolver = "credential.resolver"

const ServiceNotificationRouter = "notification.router"

const ServiceTrustProxy = "net.trust_proxy"

const ServiceCron = "cron.service"

const ServiceHealthProvider = "health.provider"

const ServiceHealthProvidersAll = "health.providers.all"

func HealthProviderServiceName(name string) string {
	return "health.provider." + name
}

func CleanupServiceName(pluginName string) string {
	return pluginName + ".cleanup"
}

func ExportServiceName(pluginName string) string {
	return pluginName + ".export"
}

func PurgeServiceName(pluginName string) string {
	return pluginName + ".purge"
}

func MCPExportServiceName(resource string) string {
	return resource + ".mcp_export"
}

func OverviewServiceName(pluginName string) string {
	return pluginName + ".overview"
}

type MessagingSender func(orgID uint, to, subject, htmlBody, textBody string) error

type SystemMessagingSender func(to, subject, htmlBody, textBody string) error

type MessagingReady func() bool

type EventDispatcher func(handler func(topic string, payload json.RawMessage))

type UsageMeter func(orgID uint, metric string, n int64)

type TrustProxyProvider func(bool)

const (
	MetricClicks   = "clicks"
	MetricMailIn   = "mailIn"
	MetricMailOut  = "mailOut"
	MetricRawBytes = "mail.raw_bytes"
)

type TierResolver func(orgID uint) string

type CleanupFunc func(ctx context.Context, retentionDays int)

type ExportFunc func(orgID uint) map[string]any

type PurgeFunc func(orgID uint) error

type OverviewFunc func(orgID uint) map[string]any

type MCPExporter func(ctx context.Context, orgID uint) (any, error)

const ServiceExportSignedURL = "export.signed_url"

type ExportSignedURLFunc func(orgID uint) (string, error)

const ServiceDNSManager = "dns.manager"

const ServiceMailStorageProvider = "mail.storage_provider"

var ErrStorageNotFound = errors.New("storage object not found")

type StorageProvider interface {
	Put(ctx context.Context, key string, data []byte) error
	Get(ctx context.Context, key string) ([]byte, error)
	Delete(ctx context.Context, key string) error
	Stat(ctx context.Context, key string) (int64, error)
}

const ServiceQuotaChecker = "quota.checker"

type QuotaChecker interface {
	Check(ctx context.Context, orgID uint, metric string, n int64) error
}

var (
	ErrQuotaExceeded    = errors.New("quota exceeded")
	ErrQuotaUnavailable = errors.New("capability not included in plan")
)

type DNSRecord struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Name     string `json:"name"`
	Content  string `json:"content"`
	TTL      int    `json:"ttl"`
	Proxied  bool   `json:"proxied"`
	Comment  string `json:"comment"`
	Priority *int   `json:"priority,omitempty"`
}

type DNSManager interface {
	List(ctx context.Context, orgID, domainID uint) ([]DNSRecord, error)
	Set(ctx context.Context, orgID, domainID uint, r DNSRecord) (DNSRecord, error)
	Delete(ctx context.Context, orgID, domainID uint, recordID string) error
}

type LinkCreator interface {
	CreateLink(ctx context.Context, orgID uint, targetURL string) (slug string, err error)
}

type CredentialResolver interface {
	LookupCredential(ctx context.Context, orgID uint, query CredentialQuery) (*Credential, error)
}
