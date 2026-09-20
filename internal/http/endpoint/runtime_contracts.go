package endpoint

import (
	"context"

	apppages "github.com/kumbuka-me/kumbuka/internal/application/pages"
	appsettings "github.com/kumbuka-me/kumbuka/internal/application/settings"
	appwebhooks "github.com/kumbuka-me/kumbuka/internal/application/webhooks"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
)

// settingsService owns persisted application, PDF, and authentication settings.
type settingsService interface {
	ApplicationSettings(context.Context) (domain.ApplicationSettings, error)
	PDFHeaders(context.Context) ([]domain.PDFHeader, error)
	PDFRequestHeaders(context.Context) ([]domain.PDFHeader, error)
	ResolvePDFRequestHeaders(context.Context, []appsettings.PDFHeaderInput) ([]domain.PDFHeader, error)
	RevealPDFHeader(context.Context, int64) (string, error)
	SaveApplicationSettings(context.Context, domain.ApplicationSettings, int64) error
	SavePDFSettings(context.Context, string, []appsettings.PDFHeaderInput, int64) error
	SaveAuthenticationSettings(context.Context, domain.AuthenticationSettings, int64) error
	RecordLocalPasswordUpdated(context.Context, domain.User)
}

// databaseInfoService exposes administrator-safe database information.
type databaseInfoService interface {
	DatabaseSize(context.Context) (int64, error)
}

// sharingService owns page-share creation and anonymous share resolution.
type sharingService interface {
	CreatePageShareLink(context.Context, string, domain.User) (apppages.IssuedPageShareLink, error)
	PageShareLink(context.Context, string) (domain.PageShareLink, error)
}

// systemService owns health and initial-setup application state.
type systemService interface {
	Ping(context.Context) error
	SetupRequired(context.Context) (bool, error)
	RecordSetupCompleted(context.Context, domain.User)
}

// webhookAdminService owns webhook configuration and delivery administration.
type webhookAdminService interface {
	Webhooks(context.Context) ([]domain.Webhook, error)
	WebhookDeliveries(context.Context, int) ([]domain.WebhookDelivery, error)
	SaveWebhook(context.Context, int64, appwebhooks.WebhookInput) (domain.Webhook, error)
	DeleteWebhook(context.Context, int64) error
	TestWebhook(context.Context, int64) error
	RevealWebhookHeader(context.Context, int64, int64) (string, error)
}
