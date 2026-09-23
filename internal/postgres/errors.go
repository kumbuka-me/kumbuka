package postgres

import (
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
)

var uniqueConstraintErrors = map[string]error{
	"pages_slug_key":                   domain.ErrAlreadyExists,
	"page_aliases_pkey":                domain.ErrAlreadyExists,
	"wiki_groups_name_key":             domain.ErrAlreadyExists,
	"wiki_groups_name_ci_idx":          domain.ErrAlreadyExists,
	"page_templates_name_key":          domain.ErrAlreadyExists,
	"page_templates_name_ci_idx":       domain.ErrAlreadyExists,
	"saved_searches_user_id_name_key":  domain.ErrAlreadyExists,
	"saved_searches_user_name_ci_idx":  domain.ErrAlreadyExists,
	"page_review_requests_pending_idx": domain.ErrReviewPending,
}

var foreignKeyValidationFields = map[string]string{
	"user_groups_group_id_fkey":                   "group_ids",
	"page_groups_group_id_fkey":                   "group_id",
	"pages_owner_group_id_fkey":                   "owner_group_id",
	"oidc_group_mappings_group_id_fkey":           "oidc_group_mappings",
	"page_review_requests_reviewer_group_id_fkey": "reviewer_group_id",
	"page_review_request_reviewers_user_id_fkey":  "reviewers",
}

// mutationError translates only constraints with an established domain meaning. Unknown constraints remain infrastructure errors, and known errors retain their cause.
func mutationError(err error) error {
	databaseError, ok := errors.AsType[*pgconn.PgError](err)
	if !ok {
		return err
	}
	if translated := uniqueConstraintError(databaseError, err); translated != nil {
		return translated
	}
	if translated := foreignKeyValidationError(databaseError, err); translated != nil {
		return translated
	}
	return err
}

// uniqueConstraintError translates known uniqueness constraints into domain conflicts.
func uniqueConstraintError(databaseError *pgconn.PgError, cause error) error {
	if databaseError.Code != "23505" {
		return nil
	}
	translated := uniqueConstraintErrors[databaseError.ConstraintName]
	if translated == nil {
		return nil
	}
	return errors.Join(translated, cause)
}

// foreignKeyValidationError translates known group-reference failures into field validation errors.
func foreignKeyValidationError(databaseError *pgconn.PgError, cause error) error {
	if databaseError.Code != "23503" {
		return nil
	}
	field := foreignKeyValidationFields[databaseError.ConstraintName]
	if field == "" {
		return nil
	}
	validation := domain.NewValidationError(field, "One or more selected groups no longer exist.")
	validation.Cause = cause
	return validation
}
