package postgres

import (
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMutationErrorPreservesKnownConflictCauses(t *testing.T) {
	t.Parallel()
	t.Run("pages_slug_key", func(t *testing.T) {
		t.Parallel()
		cause := &pgconn.PgError{Code: "23505", ConstraintName: "pages_slug_key"}
		err := mutationError(fmt.Errorf("persist: %w", cause))
		assert.ErrorIs(t, err, domain.ErrAlreadyExists)
		assert.ErrorIs(t, err, cause)
		databaseError, ok := errors.AsType[*pgconn.PgError](err)
		require.True(t, ok)
		assert.Same(t, cause, databaseError)
	})

	t.Run("page_aliases_pkey", func(t *testing.T) {
		t.Parallel()
		cause := &pgconn.PgError{Code: "23505", ConstraintName: "page_aliases_pkey"}
		err := mutationError(fmt.Errorf("persist: %w", cause))
		assert.ErrorIs(t, err, domain.ErrAlreadyExists)
		assert.ErrorIs(t, err, cause)
		databaseError, ok := errors.AsType[*pgconn.PgError](err)
		require.True(t, ok)
		assert.Same(t, cause, databaseError)
	})

	t.Run("wiki_groups_name_key", func(t *testing.T) {
		t.Parallel()
		cause := &pgconn.PgError{Code: "23505", ConstraintName: "wiki_groups_name_key"}
		err := mutationError(fmt.Errorf("persist: %w", cause))
		assert.ErrorIs(t, err, domain.ErrAlreadyExists)
		assert.ErrorIs(t, err, cause)
		databaseError, ok := errors.AsType[*pgconn.PgError](err)
		require.True(t, ok)
		assert.Same(t, cause, databaseError)
	})

	t.Run("wiki_groups_name_ci_idx", func(t *testing.T) {
		t.Parallel()
		cause := &pgconn.PgError{Code: "23505", ConstraintName: "wiki_groups_name_ci_idx"}
		err := mutationError(fmt.Errorf("persist: %w", cause))
		assert.ErrorIs(t, err, domain.ErrAlreadyExists)
		assert.ErrorIs(t, err, cause)
		databaseError, ok := errors.AsType[*pgconn.PgError](err)
		require.True(t, ok)
		assert.Same(t, cause, databaseError)
	})

	t.Run("page_templates_name_key", func(t *testing.T) {
		t.Parallel()
		cause := &pgconn.PgError{Code: "23505", ConstraintName: "page_templates_name_key"}
		err := mutationError(fmt.Errorf("persist: %w", cause))
		assert.ErrorIs(t, err, domain.ErrAlreadyExists)
		assert.ErrorIs(t, err, cause)
		databaseError, ok := errors.AsType[*pgconn.PgError](err)
		require.True(t, ok)
		assert.Same(t, cause, databaseError)
	})

	t.Run("page_templates_name_ci_idx", func(t *testing.T) {
		t.Parallel()
		cause := &pgconn.PgError{Code: "23505", ConstraintName: "page_templates_name_ci_idx"}
		err := mutationError(fmt.Errorf("persist: %w", cause))
		assert.ErrorIs(t, err, domain.ErrAlreadyExists)
		assert.ErrorIs(t, err, cause)
		databaseError, ok := errors.AsType[*pgconn.PgError](err)
		require.True(t, ok)
		assert.Same(t, cause, databaseError)
	})

	t.Run("saved_searches_user_id_name_key", func(t *testing.T) {
		t.Parallel()
		cause := &pgconn.PgError{Code: "23505", ConstraintName: "saved_searches_user_id_name_key"}
		err := mutationError(fmt.Errorf("persist: %w", cause))
		assert.ErrorIs(t, err, domain.ErrAlreadyExists)
		assert.ErrorIs(t, err, cause)
		databaseError, ok := errors.AsType[*pgconn.PgError](err)
		require.True(t, ok)
		assert.Same(t, cause, databaseError)
	})

	t.Run("saved_searches_user_name_ci_idx", func(t *testing.T) {
		t.Parallel()
		cause := &pgconn.PgError{Code: "23505", ConstraintName: "saved_searches_user_name_ci_idx"}
		err := mutationError(fmt.Errorf("persist: %w", cause))
		assert.ErrorIs(t, err, domain.ErrAlreadyExists)
		assert.ErrorIs(t, err, cause)
		databaseError, ok := errors.AsType[*pgconn.PgError](err)
		require.True(t, ok)
		assert.Same(t, cause, databaseError)
	})
}

func TestMutationErrorPreservesMissingGroupFields(t *testing.T) {
	t.Parallel()

	t.Run("user_groups_group_id_fkey", func(t *testing.T) {
		t.Parallel()

		cause := &pgconn.PgError{Code: "23503", ConstraintName: "user_groups_group_id_fkey", Detail: "private database detail"}
		err := mutationError(cause)
		validation, ok := errors.AsType[*domain.ValidationError](err)
		require.True(t, ok)
		require.Len(t, validation.Fields, 1)
		assert.Equal(t, "group_ids", validation.Fields[0].Field)
		assert.NotContains(t, validation.Fields[0].Message, cause.Detail)
		assert.ErrorIs(t, err, cause)
	})

	t.Run("page_groups_group_id_fkey", func(t *testing.T) {
		t.Parallel()

		cause := &pgconn.PgError{Code: "23503", ConstraintName: "page_groups_group_id_fkey", Detail: "private database detail"}
		err := mutationError(cause)
		validation, ok := errors.AsType[*domain.ValidationError](err)
		require.True(t, ok)
		require.Len(t, validation.Fields, 1)
		assert.Equal(t, "group_id", validation.Fields[0].Field)
		assert.NotContains(t, validation.Fields[0].Message, cause.Detail)
		assert.ErrorIs(t, err, cause)
	})

	t.Run("pages_owner_group_id_fkey", func(t *testing.T) {
		t.Parallel()

		cause := &pgconn.PgError{Code: "23503", ConstraintName: "pages_owner_group_id_fkey", Detail: "private database detail"}
		err := mutationError(cause)
		validation, ok := errors.AsType[*domain.ValidationError](err)
		require.True(t, ok)
		require.Len(t, validation.Fields, 1)
		assert.Equal(t, "owner_group_id", validation.Fields[0].Field)
		assert.NotContains(t, validation.Fields[0].Message, cause.Detail)
		assert.ErrorIs(t, err, cause)
	})

	t.Run("oidc_group_mappings_group_id_fkey", func(t *testing.T) {
		t.Parallel()

		cause := &pgconn.PgError{Code: "23503", ConstraintName: "oidc_group_mappings_group_id_fkey", Detail: "private database detail"}
		err := mutationError(cause)
		validation, ok := errors.AsType[*domain.ValidationError](err)
		require.True(t, ok)
		require.Len(t, validation.Fields, 1)
		assert.Equal(t, "oidc_group_mappings", validation.Fields[0].Field)
		assert.NotContains(t, validation.Fields[0].Message, cause.Detail)
		assert.ErrorIs(t, err, cause)
	})
}

func TestMutationErrorDoesNotReclassifyInfrastructureFailures(t *testing.T) {
	t.Parallel()

	t.Run("nil error", func(t *testing.T) {
		t.Parallel()

		assert.NoError(t, mutationError(nil))
	})

	t.Run("connection lost", func(t *testing.T) {
		t.Parallel()

		err := errors.New("connection lost")
		assert.Equal(t, err, mutationError(err))
	})

	t.Run("unknown unique constraint", func(t *testing.T) {
		t.Parallel()

		err := &pgconn.PgError{Code: "23505", ConstraintName: "api_tokens_token_hash_key"}
		assert.Equal(t, err, mutationError(err))
	})

	t.Run("unknown foreign key constraint", func(t *testing.T) {
		t.Parallel()

		err := &pgconn.PgError{Code: "23503", ConstraintName: "page_revisions_page_id_fkey"}
		assert.Equal(t, err, mutationError(err))
	})

	t.Run("serialization failure", func(t *testing.T) {
		t.Parallel()

		err := &pgconn.PgError{Code: "40001", ConstraintName: "pages_slug_key"}
		assert.Equal(t, err, mutationError(err))
	})
}

func TestMutationErrorPreservesReviewConflict(t *testing.T) {
	t.Parallel()

	cause := &pgconn.PgError{Code: "23505", ConstraintName: "page_review_requests_pending_idx"}
	err := mutationError(fmt.Errorf("persist: %w", cause))

	assert.ErrorIs(t, err, domain.ErrReviewPending)
	assert.ErrorIs(t, err, cause)
}
