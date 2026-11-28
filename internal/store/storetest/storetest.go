// Package storetest holds helpers shared by the Postgres-backed tests.
package storetest

import (
	"context"
	"os"
	"testing"

	"ellestudio/internal/store"
)

// TestDB opens TEST_DATABASE_URL or skips. The Postgres tests use negative
// user IDs and "test-" resource IDs (never real data) and remove them.
func TestDB(t *testing.T) *store.DB {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("set TEST_DATABASE_URL to test the Postgres backend")
	}
	db, err := store.OpenDatabase(context.Background(), url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
	return db
}
