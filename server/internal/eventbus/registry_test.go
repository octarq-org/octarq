package eventbus

import "testing"

func TestRegistryCoreEventsPresent(t *testing.T) {
	groups := EventGroups()
	byName := map[string][]EventDef{}
	for _, g := range groups {
		byName[g.Group] = g.Events
	}
	if len(byName["Member"]) != 3 {
		t.Fatalf("Member group = %v, want 3 events", byName["Member"])
	}
	if len(byName["Auth"]) != 2 {
		t.Fatalf("Auth group = %v, want 2 events", byName["Auth"])
	}
}

func TestRegisterEventDefDedupAndOrder(t *testing.T) {
	RegisterEventDef(EventDef{Key: "test.one", Group: "TestGroup", Title: "One"})
	RegisterEventDef(EventDef{Key: "test.one", Group: "TestGroup", Title: "Duplicate"})
	RegisterEventDef(EventDef{Key: "test.two", Group: "TestGroup", Title: "Two"})
	RegisterEventDef(EventDef{Key: ""}) // ignored

	var got []EventDef
	for _, g := range EventGroups() {
		if g.Group == "TestGroup" {
			got = g.Events
		}
	}
	if len(got) != 2 || got[0].Key != "test.one" || got[0].Title != "One" || got[1].Key != "test.two" {
		t.Fatalf("TestGroup events = %v, want [test.one(One) test.two]", got)
	}
}
