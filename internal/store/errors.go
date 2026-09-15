package store

import (
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/kumbuka-me/kumbuka/internal/domain"
)

// mutationError translates only constraints with an established domain meaning.
// Unknown constraints remain infrastructure errors, and known errors retain their cause.
func mutationError(err error) error {
	databaseError, ok := errors.AsType[*pgconn.PgError](err)
	if !ok {
		return err
	}
	if databaseError.Code == "23505" {
		switch databaseError.ConstraintName {
		case "pages_slug_key", "page_aliases_pkey", "wiki_groups_name_key", "wiki_groups_name_ci_idx",
			"page_templates_name_key", "page_templates_name_ci_idx", "saved_searches_user_id_name_key", "saved_searches_user_name_ci_idx":
			return errors.Join(domain.ErrAlreadyExists, err)
		case "page_review_requests_pending_idx":
			return errors.Join(domain.ErrReviewPending, err)
		}
	}
	if databaseError.Code == "23503" {
		var field string
		switch databaseError.ConstraintName {
		case "user_groups_group_id_fkey":
			field = "group_ids"
		case "page_groups_group_id_fkey":
			field = "group_id"
		case "pages_owner_group_id_fkey":
			field = "owner_group_id"
		case "oidc_group_mappings_group_id_fkey":
			field = "oidc_group_mappings"
		case "page_review_requests_reviewer_group_id_fkey":
			field = "reviewer_group_id"
		case "page_review_request_reviewers_user_id_fkey":
			field = "reviewers"
		}
		if field != "" {
			validation := domain.NewValidationError(field, "One or more selected groups no longer exist.")
			validation.Cause = err
			return validation
		}
	}
	return err
}
