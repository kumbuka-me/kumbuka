package store

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/kumbuka-me/kumbuka/pkg/domain"
)

// ApplicationSettings returns the persisted application-wide settings.
func (s *Store) ApplicationSettings(ctx context.Context) (domain.ApplicationSettings, error) {
	var settings domain.ApplicationSettings
	var externalLinks json.RawMessage
	err := s.pool.QueryRow(ctx, `
SELECT
  allow_user_registration,
  discussions_enabled,
  pdf_url,
  external_links,
  default_typography_size,
	  content_language,
  robots_policy,
  auth_mode,
  oidc_issuer,
  oidc_client_id,
  oidc_group_claim,
  oidc_group_sync,
  oidc_groups_authoritative,
	oidc_admin_group,
  trusted_username_headers,
  trusted_email_headers,
  trusted_display_name_headers,
	trusted_group_headers,
	trusted_admin_group
FROM application_settings
WHERE singleton=true`).Scan(
		&settings.AllowUserRegistration,
		&settings.DiscussionsEnabled,
		&settings.PDFURL,
		&externalLinks,
		&settings.Rendering.DefaultTypographySize,
		&settings.ContentLanguage,
		&settings.RobotsPolicy,
		&settings.Authentication.Mode,
		&settings.Authentication.OIDCIssuer,
		&settings.Authentication.OIDCClientID,
		&settings.Authentication.OIDCGroupClaim,
		&settings.Authentication.OIDCGroupSync,
		&settings.Authentication.OIDCGroupsAuthoritative,
		&settings.Authentication.OIDCAdminGroup,
		&settings.Authentication.TrustedUsernameHeaders,
		&settings.Authentication.TrustedEmailHeaders,
		&settings.Authentication.TrustedDisplayNameHeaders,
		&settings.Authentication.TrustedGroupHeaders,
		&settings.Authentication.TrustedAdminGroup,
	)
	if err != nil {
		return domain.ApplicationSettings{}, err
	}
	if err := json.Unmarshal(externalLinks, &settings.ExternalLinks); err != nil {
		return domain.ApplicationSettings{}, err
	}

	return settings, nil
}

// SaveApplicationSettings updates mutable application-wide settings.
func (s *Store) SaveApplicationSettings(ctx context.Context, settings domain.ApplicationSettings) error {
	externalLinks, err := json.Marshal(settings.ExternalLinks)
	if err != nil {
		return err
	}

	_, err = s.pool.Exec(ctx, `
INSERT INTO application_settings(singleton,allow_user_registration,discussions_enabled,external_links,default_typography_size,content_language,robots_policy,updated_at)
VALUES(true,$1,$2,$3::jsonb,$4,$5,$6,now())
ON CONFLICT(singleton) DO UPDATE
SET allow_user_registration=EXCLUDED.allow_user_registration,
    discussions_enabled=EXCLUDED.discussions_enabled,
    external_links=EXCLUDED.external_links,
    default_typography_size=EXCLUDED.default_typography_size,
    content_language=EXCLUDED.content_language,
    robots_policy=EXCLUDED.robots_policy,
    updated_at=now()`, settings.AllowUserRegistration, settings.DiscussionsEnabled, string(externalLinks), settings.Rendering.DefaultTypographySize, settings.ContentLanguage, settings.RobotsPolicy)
	return err
}

// PDFHeaders returns the configured request headers for the external PDF service.
func (s *Store) PDFHeaders(ctx context.Context) ([]domain.PDFHeader, error) {
	rows, err := s.pool.Query(ctx, `
SELECT id,name,value,sensitive
FROM pdf_headers
ORDER BY lower(name),id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	headers := make([]domain.PDFHeader, 0)
	for rows.Next() {
		var header domain.PDFHeader
		if err := rows.Scan(&header.ID, &header.Name, &header.Value, &header.Sensitive); err != nil {
			return nil, err
		}

		headers = append(headers, header)
	}

	return headers, rows.Err()
}

// SavePDFSettings updates the persisted HTML-to-PDF endpoint and request headers atomically.
func (s *Store) SavePDFSettings(ctx context.Context, pdfURL string, headers []domain.PDFHeader) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `
UPDATE application_settings
SET pdf_url=$1,
    updated_at=now()
WHERE singleton=true`, pdfURL); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM pdf_headers`); err != nil {
		return err
	}

	for _, header := range headers {
		if _, err := tx.Exec(ctx, `
INSERT INTO pdf_headers(name,value,sensitive)
VALUES($1,$2,$3)`, header.Name, header.Value, header.Sensitive); err != nil {
			return err
		}
	}

	return tx.Commit(ctx)
}

// SaveAuthenticationSettings updates non-secret browser authentication settings.
func (s *Store) SaveAuthenticationSettings(ctx context.Context, settings domain.AuthenticationSettings) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return mutationError(err)
	}

	defer func() { _ = tx.Rollback(ctx) }()

	// A changed assertion source invalidates previously observed external role state.
	if _, err := tx.Exec(ctx, `
UPDATE users
SET oidc_admin_observed=false,oidc_external_admin=false
WHERE EXISTS (
  SELECT 1
  FROM application_settings
  WHERE singleton=true
    AND (oidc_admin_group IS DISTINCT FROM $1 OR oidc_group_claim IS DISTINCT FROM $2)
)`, settings.OIDCAdminGroup, settings.OIDCGroupClaim); err != nil {
		return mutationError(err)
	}
	if _, err := tx.Exec(ctx, `
UPDATE users
SET trusted_proxy_admin_observed=false,trusted_proxy_external_admin=false
WHERE EXISTS (
  SELECT 1
  FROM application_settings
  WHERE singleton=true
    AND (trusted_admin_group IS DISTINCT FROM $1 OR trusted_group_headers IS DISTINCT FROM $2)
)`, settings.TrustedAdminGroup, settings.TrustedGroupHeaders); err != nil {
		return mutationError(err)
	}

	if _, err := tx.Exec(ctx, `
UPDATE application_settings
SET auth_mode=$1,
    oidc_issuer=$2,
    oidc_client_id=$3,
    oidc_group_claim=$4,
    oidc_group_sync=$5,
    oidc_groups_authoritative=$6,
	oidc_admin_group=$7,
    trusted_username_headers=$8,
    trusted_email_headers=$9,
    trusted_display_name_headers=$10,
	trusted_group_headers=$11,
	trusted_admin_group=$12,
    updated_at=now()
WHERE singleton=true`,
		settings.Mode,
		settings.OIDCIssuer,
		settings.OIDCClientID,
		settings.OIDCGroupClaim,
		settings.OIDCGroupSync,
		settings.OIDCGroupsAuthoritative,
		settings.OIDCAdminGroup,
		settings.TrustedUsernameHeaders,
		settings.TrustedEmailHeaders,
		settings.TrustedDisplayNameHeaders,
		settings.TrustedGroupHeaders,
		settings.TrustedAdminGroup,
	); err != nil {
		return mutationError(err)
	}

	if _, err := tx.Exec(ctx, `
DELETE FROM oidc_group_mappings`); err != nil {
		return mutationError(err)
	}

	for _, mapping := range settings.OIDCGroupMappings {
		if _, err := tx.Exec(ctx, `
INSERT INTO oidc_group_mappings(oidc_group,group_id)
VALUES($1,$2)`, strings.TrimSpace(mapping.OIDCGroup), mapping.GroupID); err != nil {
			return mutationError(err)
		}
	}

	return mutationError(tx.Commit(ctx))
}
