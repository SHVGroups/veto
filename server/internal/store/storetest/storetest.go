// Package storetest provides real PostgreSQL databases for tests. Each call
// creates a fresh, migrated database and drops it when the test ends. Tests
// that use it are skipped unless VETO_TEST_DATABASE_URL is set.
package storetest

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/shvgroups/veto/server/internal/store"
)

// EnvVar names the variable holding a connection URL with permission to
// create databases.
const EnvVar = "VETO_TEST_DATABASE_URL"

// New returns a migrated store backed by a database created for this test.
func New(t testing.TB) *store.Store {
	t.Helper()
	base := os.Getenv(EnvVar)
	if base == "" {
		t.Skipf("%s is not set; skipping database test", EnvVar)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	admin, err := pgx.Connect(ctx, base)
	if err != nil {
		t.Fatalf("storetest: connect: %v", err)
	}
	defer admin.Close(ctx)

	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	name := "veto_t_" + hex.EncodeToString(b)
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{name}.Sanitize()); err != nil {
		t.Fatalf("storetest: create database: %v", err)
	}

	u, err := url.Parse(base)
	if err != nil {
		t.Fatal(err)
	}
	u.Path = "/" + name
	s, err := store.Open(ctx, u.String())
	if err != nil {
		t.Fatalf("storetest: open: %v", err)
	}
	if _, err := s.Migrate(ctx); err != nil {
		t.Fatalf("storetest: migrate: %v", err)
	}
	t.Cleanup(func() {
		s.Close()
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		conn, err := pgx.Connect(ctx, base)
		if err != nil {
			t.Logf("storetest: cleanup connect: %v", err)
			return
		}
		defer conn.Close(ctx)
		if _, err := conn.Exec(ctx, "DROP DATABASE IF EXISTS "+pgx.Identifier{name}.Sanitize()+" WITH (FORCE)"); err != nil {
			t.Logf("storetest: drop database: %v", err)
		}
	})
	return s
}
