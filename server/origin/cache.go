package origin

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/octarq-org/octarq/server/internal/models"
	"gorm.io/gorm"
)

const ttl = 5 * time.Minute

var (
	globalCacheMu sync.RWMutex
	globalCache   = make(map[string]entry)
)

type entry struct {
	ok     bool
	org    uint
	name   string
	list   []string
	expiry time.Time
}

func namespace(db *gorm.DB) string {
	if db == nil {
		return "nodb|"
	}
	sqlDB, err := db.DB()
	if err != nil || sqlDB == nil {
		return "nodb|"
	}
	return fmt.Sprintf("%p|", sqlDB)
}

func ClearDomainCache(domain string) {
	domain = NormalizeHost(domain)
	if domain == "" {
		return
	}
	globalCacheMu.Lock()
	defer globalCacheMu.Unlock()
	for k := range globalCache {
		if strings.HasSuffix(k, "|any") {
			delete(globalCache, k)
			continue
		}
		idx := strings.LastIndexByte(k, ':')
		if idx != -1 {
			host := k[idx+1:]
			if host == domain || strings.HasSuffix(host, "."+domain) {
				delete(globalCache, k)
			}
		}
	}
}

func baseDomainKey(db *gorm.DB) string {
	return namespace(db) + "base|"
}

func cachedBaseDomain(db *gorm.DB) string {
	key := baseDomainKey(db)
	globalCacheMu.RLock()
	e, hit := globalCache[key]
	globalCacheMu.RUnlock()
	if hit && time.Now().Before(e.expiry) {
		return e.name
	}
	base := models.BaseDomain(db)
	globalCacheMu.Lock()
	if len(globalCache) > 1024 {
		globalCache = make(map[string]entry)
	}
	globalCache[key] = entry{ok: true, name: base, expiry: time.Now().Add(ttl)}
	globalCacheMu.Unlock()
	return base
}

func ClearBaseDomainCache(db *gorm.DB) {
	ns := namespace(db)
	globalCacheMu.Lock()
	defer globalCacheMu.Unlock()
	for k := range globalCache {
		if strings.HasPrefix(k, ns) {
			delete(globalCache, k)
		}
	}
}

func sharedHostsKey(db *gorm.DB) string {
	return namespace(db) + "shared_hosts|"
}

func cachedSharedHosts(db *gorm.DB) []string {
	key := sharedHostsKey(db)
	globalCacheMu.RLock()
	e, hit := globalCache[key]
	globalCacheMu.RUnlock()
	if hit && time.Now().Before(e.expiry) {
		return e.list
	}
	hosts := models.SharedHosts(db)
	globalCacheMu.Lock()
	if len(globalCache) > 1024 {
		globalCache = make(map[string]entry)
	}
	globalCache[key] = entry{ok: true, list: hosts, expiry: time.Now().Add(ttl)}
	globalCacheMu.Unlock()
	return hosts
}

func ClearSharedHostsCache(db *gorm.DB) {
	ns := namespace(db)
	globalCacheMu.Lock()
	defer globalCacheMu.Unlock()
	for k := range globalCache {
		if strings.HasPrefix(k, ns) {
			delete(globalCache, k)
		}
	}
}

func HasSharedHosts(db *gorm.DB) bool {
	return len(cachedSharedHosts(db)) > 0
}

func clearAllCache() {
	globalCacheMu.Lock()
	defer globalCacheMu.Unlock()
	globalCache = make(map[string]entry)
}
