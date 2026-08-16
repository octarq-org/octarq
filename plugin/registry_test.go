package plugin

import (
	"strings"
	"testing"
)

type testGreeter interface{ Greet() string }

type greeterImpl struct{}

func (greeterImpl) Greet() string { return "hi" }

func testContext(reg *Registry) *Context {
	return &Context{Provide: reg.Provide, Lookup: reg.Lookup}
}

func TestRegistryProvideLookupRoundtrip(t *testing.T) {
	reg := NewRegistry()
	ctx := testContext(reg)

	ctx.Provide("hello.greeter", greeterImpl{})

	svc, ok := ctx.Lookup("hello.greeter")
	if !ok {
		t.Fatal("Lookup returned false for a provided service")
	}
	if _, isGreeter := svc.(testGreeter); !isGreeter {
		t.Fatalf("Lookup returned %T, want greeterImpl", svc)
	}
	if err := reg.Err(); err != nil {
		t.Fatalf("Err() = %v, want nil", err)
	}
}

func TestLookupBeforeProvideReturnsFalse(t *testing.T) {
	ctx := testContext(NewRegistry())

	if _, ok := ctx.Lookup("billing.issuer"); ok {
		t.Fatal("Lookup returned true for a never-provided name")
	}
	if _, ok := LookupAs[testGreeter](ctx, "billing.issuer"); ok {
		t.Fatal("LookupAs returned true for a never-provided name")
	}
}

func TestLookupAsSuccessAndTypeMismatch(t *testing.T) {
	ctx := testContext(NewRegistry())
	ctx.Provide("hello.greeter", greeterImpl{})

	g, ok := LookupAs[testGreeter](ctx, "hello.greeter")
	if !ok {
		t.Fatal("LookupAs[testGreeter] returned false for a provided service")
	}
	if got := g.Greet(); got != "hi" {
		t.Fatalf("Greet() = %q, want %q", got, "hi")
	}

	// Same name, wrong type: must be (zero, false), not a panic.
	if s, ok := LookupAs[string](ctx, "hello.greeter"); ok || s != "" {
		t.Fatalf("LookupAs[string] = (%q, %v), want (\"\", false)", s, ok)
	}
}

func TestLookupAsNilContextAndNilLookup(t *testing.T) {
	if _, ok := LookupAs[testGreeter](nil, "x"); ok {
		t.Fatal("LookupAs on nil Context returned true")
	}
	// A host that predates the registry leaves Lookup nil; consumers must
	// see "absent", not a nil-func panic.
	if _, ok := LookupAs[testGreeter](&Context{}, "x"); ok {
		t.Fatal("LookupAs on Context without Lookup returned true")
	}
}

func TestDuplicateProvideFailsLoudly(t *testing.T) {
	reg := NewRegistry()
	ctx := testContext(reg)

	ctx.Provide("hello.greeter", greeterImpl{})
	ctx.Provide("hello.greeter", "impostor")

	err := reg.Err()
	if err == nil {
		t.Fatal("Err() = nil after duplicate Provide, want error")
	}
	if !strings.Contains(err.Error(), `"hello.greeter"`) {
		t.Fatalf("Err() = %v, want it to name the colliding service", err)
	}

	// First registration wins; the duplicate must not overwrite it.
	svc, ok := ctx.Lookup("hello.greeter")
	if !ok {
		t.Fatal("service vanished after duplicate Provide")
	}
	if _, isGreeter := svc.(testGreeter); !isGreeter {
		t.Fatalf("duplicate Provide overwrote the original: got %T", svc)
	}
}

func TestLookupServiceAsAbsentMismatchAndSuccess(t *testing.T) {
	reg := NewRegistry()
	reg.Provide(ServiceMailSend, MailSender(func(orgID uint, to, subject, htmlBody, textBody string) error {
		return nil
	}))

	// Absent service: (zero, false).
	if fn, ok := LookupServiceAs[MailSender](reg.Lookup, ServiceMailDispatcher); ok || fn != nil {
		t.Fatalf("LookupServiceAs for absent service = (%v, %v), want (nil, false)", fn, ok)
	}
	// Service present under a different contract type: (zero, false), not a panic.
	if fn, ok := LookupServiceAs[UsageMeter](reg.Lookup, ServiceMailSend); ok || fn != nil {
		t.Fatalf("LookupServiceAs with mismatched type = (%v, %v), want (nil, false)", fn, ok)
	}
	// Matching named contract type: value returned.
	fn, ok := LookupServiceAs[MailSender](reg.Lookup, ServiceMailSend)
	if !ok || fn == nil {
		t.Fatal("LookupServiceAs[MailSender] returned false for a MailSender-provided service")
	}
	// A nil lookup func reads as absent, not a panic.
	if fn, ok := LookupServiceAs[MailSender](nil, ServiceMailSend); ok || fn != nil {
		t.Fatalf("LookupServiceAs with nil lookup = (%v, %v), want (nil, false)", fn, ok)
	}
}

// TestLookupAsMatchesLookupServiceAs pins that LookupAs delegates to
// LookupServiceAs, so the two share one assertion implementation and one
// behavior for absent / mismatched / present services.
func TestLookupAsMatchesLookupServiceAs(t *testing.T) {
	ctx := testContext(NewRegistry())
	ctx.Provide(ServiceMailSend, MailSender(func(orgID uint, to, subject, htmlBody, textBody string) error {
		return nil
	}))

	if fn, ok := LookupAs[MailSender](ctx, ServiceMailSend); !ok || fn == nil {
		t.Fatal("LookupAs[MailSender] failed for a MailSender-provided service")
	}
	if fn, ok := LookupAs[MailSender](ctx, ServiceMailDispatcher); ok || fn != nil {
		t.Fatalf("LookupAs for absent service = (%v, %v), want (nil, false)", fn, ok)
	}
	if fn, ok := LookupAs[UsageMeter](ctx, ServiceMailSend); ok || fn != nil {
		t.Fatalf("LookupAs with mismatched type = (%v, %v), want (nil, false)", fn, ok)
	}
}
