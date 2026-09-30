package postgres

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/kumbuka-me/kumbuka/pkg/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func profileTestStore(t *testing.T) (*Store, context.Context) {
	t.Helper()
	ctx := context.Background()
	database, err := Open(ctx, integrationDatabase(t), slog.New(slog.NewTextHandler(io.Discard, nil)))
	require.NoError(t, err)
	t.Cleanup(database.Close)
	return database, ctx
}

func TestOIDCProfileSyncOverrideAndRevert(t *testing.T) {
	database, ctx := profileTestStore(t)
	user, err := database.ResolveOIDCLogin(ctx, "https://issuer.example", "stable-subject", "alice", "alice@example.test", "Alice", true)
	require.NoError(t, err)

	profile, err := database.UserProfile(ctx, user.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.ProfileSourceOIDC, profile.Source)
	assert.False(t, profile.UsernameOverridden)
	assert.False(t, profile.EmailOverridden)
	assert.False(t, profile.DisplayNameOverridden)

	require.NoError(t, database.UpdateUserAccount(ctx, domain.UserAccountUpdate{
		UserID: user.ID, Email: utils.ToPtr("local@example.test"), Role: user.Role, Enabled: true,
	}))
	user, err = database.ResolveOIDCLogin(ctx, "https://issuer.example", "stable-subject", "alice-new", "provider-new@example.test", "Alice New", true)
	require.NoError(t, err)
	assert.Equal(t, "alice-new", user.Username)
	assert.Equal(t, "local@example.test", user.Email)
	assert.Equal(t, "Alice New", user.DisplayName)

	profile, err = database.UserProfile(ctx, user.ID)
	require.NoError(t, err)
	assert.Equal(t, "provider-new@example.test", profile.ProviderEmail)
	assert.True(t, profile.EmailOverridden)
	assert.False(t, profile.UsernameOverridden)
	assert.False(t, profile.DisplayNameOverridden)

	require.NoError(t, database.UpdateUserAccount(ctx, domain.UserAccountUpdate{
		UserID: user.ID, RevertEmail: true, Role: user.Role, Enabled: true,
	}))
	profile, err = database.UserProfile(ctx, user.ID)
	require.NoError(t, err)
	assert.Equal(t, "provider-new@example.test", profile.Email)
	assert.False(t, profile.EmailOverridden)
}

func TestTrustedProxyProfileSyncOverrideAndRevert(t *testing.T) {
	database, ctx := profileTestStore(t)
	_, err := database.CreateTrustedProxyUser(ctx, "proxy-alice", "alice@example.test", "Alice", false, false)
	require.NoError(t, err)
	user, err := database.RefreshTrustedProxyUser(ctx, "proxy-alice", "alice-new@example.test", "Alice New", false, false)
	require.NoError(t, err)
	assert.Equal(t, "alice-new@example.test", user.Email)
	assert.Equal(t, "Alice New", user.DisplayName)

	require.NoError(t, database.UpdateUserAccount(ctx, domain.UserAccountUpdate{
		UserID: user.ID, Email: utils.ToPtr("local@example.test"), DisplayName: utils.ToPtr("Local Alice"), Role: user.Role, Enabled: true,
	}))
	user, err = database.RefreshTrustedProxyUser(ctx, "proxy-alice", "provider@example.test", "Provider Alice", false, false)
	require.NoError(t, err)
	assert.Equal(t, "local@example.test", user.Email)
	assert.Equal(t, "Local Alice", user.DisplayName)

	profile, err := database.UserProfile(ctx, user.ID)
	require.NoError(t, err)
	assert.Equal(t, "provider@example.test", profile.ProviderEmail)
	assert.Equal(t, "Provider Alice", profile.ProviderDisplayName)
	assert.True(t, profile.EmailOverridden)
	assert.True(t, profile.DisplayNameOverridden)

	require.NoError(t, database.UpdateUserAccount(ctx, domain.UserAccountUpdate{
		UserID: user.ID, RevertEmail: true, RevertDisplayName: true, Role: user.Role, Enabled: true,
	}))
	profile, err = database.UserProfile(ctx, user.ID)
	require.NoError(t, err)
	assert.Equal(t, "provider@example.test", profile.Email)
	assert.Equal(t, "Provider Alice", profile.DisplayName)
	assert.False(t, profile.EmailOverridden)
	assert.False(t, profile.DisplayNameOverridden)
}

func TestTrustedProxyRelinkRejectsDuplicatesAndUsesOnlyIdentityKey(t *testing.T) {
	database, ctx := profileTestStore(t)
	alice, err := database.CreateTrustedProxyUser(ctx, "proxy-carol", "carol@example.test", "Carol", false, false)
	require.NoError(t, err)
	bob, err := database.CreateTrustedProxyUser(ctx, "proxy-dave", "dave@example.test", "Dave", false, false)
	require.NoError(t, err)

	assert.ErrorIs(t, database.RelinkTrustedProxyIdentity(ctx, alice.ID, "proxy-dave"), domain.ErrAlreadyExists)
	require.NoError(t, database.RelinkTrustedProxyIdentity(ctx, alice.ID, "proxy-carol-new"))

	_, err = database.RefreshTrustedProxyUser(ctx, "proxy-carol", "dave@example.test", "Spoof", false, false)
	assert.ErrorIs(t, err, domain.ErrRegistrationDisabled)
	_, err = database.CreateTrustedProxyUser(ctx, "proxy-carol", "dave@example.test", "Spoof", false, false)
	assert.ErrorIs(t, err, domain.ErrRegistrationDisabled)
	resolved, err := database.RefreshTrustedProxyUser(ctx, "proxy-carol-new", "dave@example.test", "Carol Again", false, false)
	require.NoError(t, err)
	assert.Equal(t, alice.ID, resolved.ID)
	assert.NotEqual(t, bob.ID, resolved.ID)
	assert.Equal(t, "dave@example.test", resolved.Email, "email is mutable metadata, not an account lookup key")
}

func TestLocalProfileChangesRemainStraightforward(t *testing.T) {
	database, ctx := profileTestStore(t)
	var userID int64
	require.NoError(t, database.pool.QueryRow(ctx, `
INSERT INTO users(username,email,display_name) VALUES('local-old','old@example.test','Old') RETURNING id`).Scan(&userID))
	require.NoError(t, database.UpdateUserAccount(ctx, domain.UserAccountUpdate{
		UserID: userID, Username: utils.ToPtr("local-new"), Email: utils.ToPtr("new@example.test"),
		DisplayName: utils.ToPtr("New"), Role: domain.UserRoleEditor, Enabled: true,
	}))
	profile, err := database.UserProfile(ctx, userID)
	require.NoError(t, err)
	assert.Equal(t, domain.ProfileSourceLocal, profile.Source)
	assert.Equal(t, "local-new", profile.Username)
	assert.Equal(t, "new@example.test", profile.Email)
	assert.Equal(t, "New", profile.DisplayName)
	assert.False(t, profile.UsernameOverridden)
	assert.False(t, profile.EmailOverridden)
	assert.False(t, profile.DisplayNameOverridden)
}
