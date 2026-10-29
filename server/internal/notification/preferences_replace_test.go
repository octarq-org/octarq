package notification

import (
	"context"
	"testing"
)

// ReplacePreferences writes the whole routing table, so a rule the user removed
// in the preferences page is deleted. SavePreferences alone only upserts: the
// page reported "saved" and the rule kept routing after a reload.
func TestReplacePreferencesDeletesAbsentRows(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()
	const userID = "user_replace"

	seed := []PreferenceItem{
		{EventPattern: "system.*", Channels: []string{"in_app", "email"}},
		{EventPattern: "billing.*", Channels: []string{"in_app"}},
		{EventPattern: "security.*", Channels: []string{"email"}},
	}
	if err := SavePreferences(ctx, db, userID, seed); err != nil {
		t.Fatalf("seed SavePreferences failed: %v", err)
	}

	// The page submits the matrix without billing.*, and with system.* muted.
	err := ReplacePreferences(ctx, db, userID, []PreferenceItem{
		{EventPattern: "system.*", Channels: []string{"in_app"}},
		{EventPattern: "security.*", Channels: []string{"email"}},
	})
	if err != nil {
		t.Fatalf("ReplacePreferences failed: %v", err)
	}

	prefs, err := GetPreferences(ctx, db, userID)
	if err != nil {
		t.Fatalf("GetPreferences failed: %v", err)
	}
	if len(prefs) != 2 {
		t.Fatalf("expected 2 stored preferences, got %d: %+v", len(prefs), prefs)
	}
	for _, p := range prefs {
		if p.EventPattern == "billing.*" {
			t.Fatalf("billing.* survived a replace that did not include it")
		}
	}

	// The deleted pattern falls back to the default routing again.
	channels, err := ResolveChannels(ctx, db, userID, "billing.invoice_paid")
	if err != nil {
		t.Fatalf("ResolveChannels failed: %v", err)
	}
	if len(channels) != 2 || channels[0] != "in_app" || channels[1] != "email" {
		t.Fatalf("expected default channels after deletion, got %v", channels)
	}

	// The surviving row was updated, not duplicated.
	var systemRows int
	for _, p := range prefs {
		if p.EventPattern == "system.*" {
			systemRows++
			if len(p.Channels) != 1 || p.Channels[0] != "in_app" {
				t.Fatalf("system.* channels not updated: %v", p.Channels)
			}
		}
	}
	if systemRows != 1 {
		t.Fatalf("expected one system.* row, got %d", systemRows)
	}
}

// A replace never touches another user's rows.
func TestReplacePreferencesIsScopedToUser(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	if err := SavePreferences(ctx, db, "user_a", []PreferenceItem{
		{EventPattern: "system.*", Channels: []string{"email"}},
	}); err != nil {
		t.Fatalf("seed user_a failed: %v", err)
	}
	if err := SavePreferences(ctx, db, "user_b", []PreferenceItem{
		{EventPattern: "billing.*", Channels: []string{"email"}},
	}); err != nil {
		t.Fatalf("seed user_b failed: %v", err)
	}

	if err := ReplacePreferences(ctx, db, "user_a", []PreferenceItem{
		{EventPattern: "security.*", Channels: []string{"in_app"}},
	}); err != nil {
		t.Fatalf("ReplacePreferences failed: %v", err)
	}

	bRows, err := GetPreferences(ctx, db, "user_b")
	if err != nil {
		t.Fatalf("GetPreferences failed: %v", err)
	}
	if len(bRows) != 1 || bRows[0].EventPattern != "billing.*" {
		t.Fatalf("user_b rows changed by another user's replace: %+v", bRows)
	}
}
