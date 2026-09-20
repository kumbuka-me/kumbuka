package postgres

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
)

// integrationDatabase isolates each test in a disposable schema. The supplied
// database must be a development/test database with schema creation privileges.
func integrationDatabase(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("KUMBUKA_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set KUMBUKA_TEST_DATABASE_URL for PostgreSQL integration tests")
	}
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, dsn)
	require.NoError(t, err)
	schema := fmt.Sprintf("kumbuka_test_%d", time.Now().UnixNano())
	_, err = conn.Exec(ctx, "CREATE SCHEMA "+pgx.Identifier{schema}.Sanitize())
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = conn.Exec(ctx, "DROP SCHEMA "+pgx.Identifier{schema}.Sanitize()+" CASCADE")
		_ = conn.Close(ctx)
	})
	parsed, err := url.Parse(dsn)
	require.NoError(t, err)
	query := parsed.Query()
	query.Set("search_path", schema)
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

func TestConcurrentStartupMigrations(t *testing.T) {
	dsn := integrationDatabase(t)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	const count = 6
	errors := make(chan error, count)
	start := make(chan struct{})
	var workers sync.WaitGroup
	for range count {
		workers.Go(func() {
			<-start
			database, err := Open(context.Background(), dsn, logger)
			if database != nil {
				database.Close()
			}
			errors <- err
		})
	}
	close(start)
	workers.Wait()
	close(errors)
	for err := range errors {
		require.NoError(t, err)
	}
}

func TestOIDCIdentitySurvivesDatabaseReopen(t *testing.T) {
	dsn := integrationDatabase(t)
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	database, err := Open(ctx, dsn, logger)
	require.NoError(t, err)
	user, err := database.LoginOIDCUser(ctx, "https://issuer.example", "subject-7", "alice", "alice@example.com", "Alice")
	require.NoError(t, err)
	require.NoError(t, database.SetExternalAdminStatus(ctx, user.ID, "oidc", true))
	database.Close()
	restarted, err := Open(ctx, dsn, logger)
	require.NoError(t, err)
	defer restarted.Close()
	restored, err := restarted.OIDCUser(ctx, "https://issuer.example", "subject-7")
	require.NoError(t, err)
	require.Equal(t, user.ID, restored.ID)
	require.Equal(t, user.SessionVersion, restored.SessionVersion)
	require.True(t, restored.ExternalAdmin)
	require.NoError(t, restarted.RevokeUserSessions(ctx, user.ID))
	revoked, err := restarted.OIDCUser(ctx, "https://issuer.example", "subject-7")
	require.NoError(t, err)
	require.Greater(t, revoked.SessionVersion, user.SessionVersion)
}
