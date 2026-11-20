package plugin

import "testing"

func TestServiceNamesAreStableWireStrings(t *testing.T) {
	for _, c := range []struct{ got, want string }{
		{ServiceMailSend, "mail.send"},
		{ServiceMailReady, "mail.ready"},
		{ServiceMailSendSystem, "mail.send.system"},
		{ServiceMailDispatcher, "mail.dispatcher"},
		{ServiceMailEmailGet, "mail.email.get"},
		{ServiceMailStorageProvider, "mail.storage_provider"},
		{ServiceCloudUsage, "cloud.usage"},
		{ServiceCloudTier, "cloud.tier"},
		{ServiceCloudQuota, "cloud.quota"},
		{ServiceLinkResolve, "links.resolve"},
		{ServiceQuotaChecker, "quota.checker"},
		{ServiceDNSManager, "dns.manager"},
		{ServiceDataScope, "datascope.filter"},
	} {
		if c.got != c.want {
			t.Errorf("service name = %q, want %q", c.got, c.want)
		}
	}
}

func TestTierResolverRoundTripsThroughTheRegistry(t *testing.T) {
	reg := NewRegistry()
	reg.Provide(ServiceCloudTier, TierResolver(func(orgID uint) string {
		if orgID == 7 {
			return "team"
		}
		return ""
	}))
	tier, ok := LookupServiceAs[TierResolver](reg.Lookup, ServiceCloudTier)
	if !ok {
		t.Fatal("TierResolver provided under ServiceCloudTier did not resolve as TierResolver")
	}
	if got := tier(7); got != "team" {
		t.Fatalf("tier(7) = %q, want %q", got, "team")
	}
	if got := tier(1); got != "" {
		t.Fatalf("tier(1) = %q, want \"\" (Free is not an error)", got)
	}
}

func TestTierResolverIsNotInterchangeableWithABareFunc(t *testing.T) {
	bare := NewRegistry()
	bare.Provide(ServiceCloudTier, func(orgID uint) string { return "team" })
	if _, ok := LookupServiceAs[TierResolver](bare.Lookup, ServiceCloudTier); ok {
		t.Error("a bare func(uint) string resolved as TierResolver; the named type would not be enforcing anything")
	}

	named := NewRegistry()
	named.Provide(ServiceCloudTier, TierResolver(func(orgID uint) string { return "team" }))
	if _, ok := LookupServiceAs[func(uint) string](named.Lookup, ServiceCloudTier); ok {
		t.Error("a TierResolver resolved as a bare func(uint) string; the old consumer would keep working and the conversion would be cosmetic")
	}
}

func TestInterfaceContractsResolveWhenConvertedAtProvide(t *testing.T) {
	reg := NewRegistry()
	reg.Provide(ServiceQuotaChecker, QuotaChecker(fakeQuotaChecker{}))
	if _, ok := LookupServiceAs[QuotaChecker](reg.Lookup, ServiceQuotaChecker); !ok {
		t.Fatal("QuotaChecker did not resolve under ServiceQuotaChecker")
	}
	reg.Provide(ServiceDataScope, DataScopeFilter(&mockDataScopeFilter{}))
	if _, ok := LookupServiceAs[DataScopeFilter](reg.Lookup, ServiceDataScope); !ok {
		t.Fatal("DataScopeFilter did not resolve under ServiceDataScope")
	}

	other := NewRegistry()
	other.Provide(ServiceQuotaChecker, struct{}{})
	if _, ok := LookupServiceAs[QuotaChecker](other.Lookup, ServiceQuotaChecker); ok {
		t.Fatal("a value that does not implement QuotaChecker resolved as one")
	}
	other.Provide(ServiceDataScope, struct{}{})
	if _, ok := LookupServiceAs[DataScopeFilter](other.Lookup, ServiceDataScope); ok {
		t.Fatal("a value that does not implement DataScopeFilter resolved as one")
	}
}
