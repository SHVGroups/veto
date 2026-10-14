// Package store is Veto's persistence layer on PostgreSQL. All SQL lives
// here. Methods are defined on Queries, which wraps either the connection
// pool or a transaction, so the same code runs inside and outside of a
// transaction.
package store

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrNotFound is returned when a looked-up record does not exist in the
// workspace.
var ErrNotFound = errors.New("store: not found")

// ErrConflict is returned when a write violates a uniqueness rule.
var ErrConflict = errors.New("store: already exists")

type querier interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	SendBatch(ctx context.Context, b *pgx.Batch) pgx.BatchResults
}

// Queries runs SQL against a pool or a transaction.
type Queries struct {
	db querier
}

// Store owns the connection pool.
type Store struct {
	*Queries
	pool *pgxpool.Pool
}

// Open connects to Postgres and verifies the connection.
func Open(ctx context.Context, url string) (*Store, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, fmt.Errorf("store: parse database url: %w", err)
	}
	if cfg.MaxConns < 10 {
		cfg.MaxConns = 10
	}
	cfg.MaxConnIdleTime = 5 * time.Minute
	cfg.AfterConnect = func(_ context.Context, conn *pgx.Conn) error {
		// Scan timestamps as UTC so API output does not depend on the
		// host's time zone.
		conn.TypeMap().RegisterType(&pgtype.Type{
			Name: "timestamptz", OID: pgtype.TimestamptzOID,
			Codec: &pgtype.TimestamptzCodec{ScanLocation: time.UTC},
		})
		return nil
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("store: connect: %w", err)
	}
	pingCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("store: ping: %w", err)
	}
	return &Store{Queries: &Queries{db: pool}, pool: pool}, nil
}

// Close releases all connections.
func (s *Store) Close() { s.pool.Close() }

// Pool exposes the pool for components that need a dedicated connection,
// such as the LISTEN loop.
func (s *Store) Pool() *pgxpool.Pool { return s.pool }

// Ping checks that the database answers.
func (s *Store) Ping(ctx context.Context) error { return s.pool.Ping(ctx) }

// Tx runs fn in a transaction, committing if it returns nil.
func (s *Store) Tx(ctx context.Context, fn func(q *Queries) error) error {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck // Rollback after Commit is a no-op.
	if err := fn(&Queries{db: tx}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

//go:embed migrations/*.sql
var migrationsFS embed.FS

// migrationLockID is an arbitrary constant for pg_advisory_lock so that only
// one replica migrates at a time.
const migrationLockID = 0x7665746f00000001

// Migrate applies any migrations that have not run yet. It is safe to call
// from several replicas at once.
func (s *Store) Migrate(ctx context.Context) ([]string, error) {
	conn, err := s.pool.Acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Release()
	if _, err := conn.Exec(ctx, "SELECT pg_advisory_lock($1)", int64(migrationLockID)); err != nil {
		return nil, fmt.Errorf("store: migration lock: %w", err)
	}
	defer conn.Exec(context.WithoutCancel(ctx), "SELECT pg_advisory_unlock($1)", int64(migrationLockID)) //nolint:errcheck

	if _, err := conn.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version text PRIMARY KEY,
		applied_at timestamptz NOT NULL DEFAULT now()
	)`); err != nil {
		return nil, err
	}
	applied := map[string]bool{}
	rows, err := conn.Query(ctx, "SELECT version FROM schema_migrations")
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			rows.Close()
			return nil, err
		}
		applied[v] = true
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	files, err := fs.Glob(migrationsFS, "migrations/*.sql")
	if err != nil {
		return nil, err
	}
	sort.Strings(files)
	var ran []string
	for _, f := range files {
		version := strings.TrimSuffix(strings.TrimPrefix(f, "migrations/"), ".sql")
		if applied[version] {
			continue
		}
		body, err := migrationsFS.ReadFile(f)
		if err != nil {
			return ran, err
		}
		tx, err := conn.Begin(ctx)
		if err != nil {
			return ran, err
		}
		if _, err := tx.Exec(ctx, string(body)); err != nil {
			tx.Rollback(ctx) //nolint:errcheck
			return ran, fmt.Errorf("store: migration %s: %w", version, err)
		}
		if _, err := tx.Exec(ctx, "INSERT INTO schema_migrations (version) VALUES ($1)", version); err != nil {
			tx.Rollback(ctx) //nolint:errcheck
			return ran, err
		}
		if err := tx.Commit(ctx); err != nil {
			return ran, err
		}
		ran = append(ran, version)
	}
	return ran, nil
}

// SchemaVersion returns the latest applied migration.
func (s *Store) SchemaVersion(ctx context.Context) (string, error) {
	var v string
	err := s.pool.QueryRow(ctx, "SELECT coalesce(max(version), '') FROM schema_migrations").Scan(&v)
	return v, err
}

// TryAdvisoryLock takes a session-level advisory lock on a dedicated
// connection and returns a function that releases it. ok is false if another
// session holds the lock. Workers use this so that only one replica runs each
// background job.
func (s *Store) TryAdvisoryLock(ctx context.Context, key int64) (release func(), ok bool, err error) {
	conn, err := s.pool.Acquire(ctx)
	if err != nil {
		return nil, false, err
	}
	if err := conn.QueryRow(ctx, "SELECT pg_try_advisory_lock($1)", key).Scan(&ok); err != nil {
		conn.Release()
		return nil, false, err
	}
	if !ok {
		conn.Release()
		return nil, false, nil
	}
	return func() {
		conn.Exec(context.Background(), "SELECT pg_advisory_unlock($1)", key) //nolint:errcheck
		conn.Release()
	}, true, nil
}

// isUniqueViolation reports whether err is a unique-constraint failure.
func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

func notFound(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

// Notify sends a notification on a channel. Inside a transaction it is
// delivered only when the transaction commits.
func (q *Queries) Notify(ctx context.Context, channel, payload string) error {
	_, err := q.db.Exec(ctx, "SELECT pg_notify($1, $2)", channel, payload)
	return err
}

// LockForTransaction takes a transaction-scoped advisory lock, serializing
// transactions that use the same key. Call it inside Tx.
func (q *Queries) LockForTransaction(ctx context.Context, key int64) error {
	_, err := q.db.Exec(ctx, "SELECT pg_advisory_xact_lock($1)", key)
	return err
}

// Advisory lock keys used across the application.
const (
	LockSetup int64 = 0x7665746f00000002
)
