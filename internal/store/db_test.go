package store

import (
	"context"
	"os"
	"strings"
	"testing"
)

// The helper in storetest imports this package, so an in-package test opens
// the database itself.
func TestMigrationsAreIdempotent(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("set TEST_DATABASE_URL to test the Postgres backend")
	}
	ctx := context.Background()
	db, err := OpenDatabase(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
	// Two instances starting at once (as during a deploy) must both succeed.
	errs := make(chan error, 2)
	for range 2 {
		go func() { errs <- migrate(ctx, db.pool) }()
	}
	for range 2 {
		if err := <-errs; err != nil {
			t.Fatal(err)
		}
	}
	var v int
	if err := db.pool.QueryRow(ctx, `SELECT max(version) FROM ellie_schema_migrations`).Scan(&v); err != nil || v != len(migrations) {
		t.Fatalf("schema version = %d (%v), want %d", v, err, len(migrations))
	}
}

// testDB opens the test database, or skips without TEST_DATABASE_URL. It is
// storetest.TestDB for in-package tests (which would otherwise be an import
// cycle).
func testDB(t *testing.T) *DB {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("set TEST_DATABASE_URL to test the Postgres backend")
	}
	db, err := OpenDatabase(context.Background(), url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
	return db
}

func TestBadDatabaseURLHidesPassword(t *testing.T) {
	_, err := OpenDatabase(context.Background(), "postgres://u:s3cret@host:notaport/db")
	if err == nil {
		t.Fatal("expected an error")
	}
	if got := err.Error(); strings.Contains(got, "s3cret") {
		t.Errorf("password leaked: %s", got)
	}
}
