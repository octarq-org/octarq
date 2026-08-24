package plugin

import "testing"

// TestServiceNamesAreStableWireStrings pins the registry names. They are not
// internal identifiers: providers and consumers live in DIFFERENT MODULES
// (downstream distributions pin this one by commit), so a rename here is not caught by any
// compiler — the lookup just stops resolving and the capability silently turns
// off. Changing a value in this list is a breaking change to out-of-tree
// plugins and has to be done deliberately, on both sides.
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
	} {
		if c.got != c.want {
			t.Errorf("service name = %q, want %q", c.got, c.want)
		}
	}
}

// TestTierResolverRoundTripsThroughTheRegistry is the shape downstream modules
// must follow: convert at the Provide call site, resolve with the
// same named type.
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

// TestTierResolverIsNotInterchangeableWithABareFunc documents why downstream modules
// have to change the provider and the consumer in the SAME commit.
//
// Go type-asserts named func types exactly, so a bare `func(uint) string` in the
// registry is not a TierResolver and vice versa. That is the whole point — it
// is what turns a provider signature drift into a compile error at the Provide
// call site — but it also means converting only one half swaps a silent
// mismatch for a different silent mismatch.
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

// TestInterfaceContractsResolveWhenConvertedAtProvide covers the two services
// whose contract types are interfaces (QuotaChecker, StorageProvider): the
// conversion at the Provide site is what makes a drifted concrete type a
// compile error there instead of a lookup that silently finds nothing.
func TestInterfaceContractsResolveWhenConvertedAtProvide(t *testing.T) {
	reg := NewRegistry()
	reg.Provide(ServiceQuotaChecker, QuotaChecker(fakeQuotaChecker{}))
	if _, ok := LookupServiceAs[QuotaChecker](reg.Lookup, ServiceQuotaChecker); !ok {
		t.Fatal("QuotaChecker did not resolve under ServiceQuotaChecker")
	}

	// A value that does NOT satisfy the contract must not resolve — this is the
	// runtime symptom the Provide-site conversion is meant to make impossible,
	// and it is indistinguishable from "self-hosted, no quota module".
	other := NewRegistry()
	other.Provide(ServiceQuotaChecker, struct{}{})
	if _, ok := LookupServiceAs[QuotaChecker](other.Lookup, ServiceQuotaChecker); ok {
		t.Fatal("a value that does not implement QuotaChecker resolved as one")
	}
}
