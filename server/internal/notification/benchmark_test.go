package notification

import (
	"context"
	"fmt"
	"testing"
)

func BenchmarkSavePreferences(b *testing.B) {
	db := setupTestDB(b)
	ctx := context.Background()
	userID := "user-benchmark"

	// Create 100 items
	var items []PreferenceItem
	for i := 0; i < 100; i++ {
		items = append(items, PreferenceItem{
			EventPattern: fmt.Sprintf("event.%d", i),
			Channels:     []string{"email", "in_app"},
		})
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := SavePreferences(ctx, db, userID, items); err != nil {
			b.Fatal(err)
		}
	}
}
