package endpoint

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	appwebhooks "github.com/kumbuka-me/kumbuka/internal/application/webhooks"
	httpresponse "github.com/kumbuka-me/kumbuka/internal/http/response"
	"github.com/kumbuka-me/kumbuka/internal/webview"
)

// AdminWebhooks renders outgoing webhook configuration and recent deliveries.
func AdminWebhooks(viewDataUseCases viewDataService, webhookUseCases webhookAdminService, views *webview.Views) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data, err := administrationData(r, viewDataUseCases, views, "Webhooks", "webhooks")
		if err != nil {
			httpresponse.InternalServerError(views.Logger(), w, err)
			return
		}
		data.Webhooks, err = webhookUseCases.Webhooks(r.Context())
		if err != nil {
			httpresponse.InternalServerError(views.Logger(), w, err)
			return
		}
		data.WebhookDeliveries, err = webhookUseCases.WebhookDeliveries(r.Context(), 50)
		if err != nil {
			httpresponse.InternalServerError(views.Logger(), w, err)
			return
		}
		data.WebhookEvents = appwebhooks.WebhookEvents()
		data.WebhookDraft = appwebhooks.DefaultWebhook()
		views.Render(w, "admin_webhooks", data)
	}
}

// SaveAdminWebhook creates or updates one outgoing webhook.
func SaveAdminWebhook(webhookUseCases webhookAdminService, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			httpresponse.Problem(w, http.StatusBadRequest, "Invalid webhook form.")
			return
		}

		id, err := optionalPositivePathID(r.PathValue("id"))
		if err != nil {
			httpresponse.Problem(w, http.StatusBadRequest, "Invalid webhook identifier.")
			return
		}

		headers, err := webhookHeadersFromForm(r)
		if err != nil {
			if tryWriteRequestProblem(w, http.StatusBadRequest, "Invalid webhook form.", "headers", err) {
				return
			}
			httpresponse.InternalServerError(logger, w, err)
			return
		}

		retryCount, retryBackoff, retryMaxBackoff, err := webhookRetryFromForm(r)
		if err != nil {
			if tryWriteRequestProblem(w, http.StatusBadRequest, "Invalid webhook form.", "retry", err) {
				return
			}
			httpresponse.InternalServerError(logger, w, err)
			return
		}

		_, err = webhookUseCases.SaveWebhook(r.Context(), id, appwebhooks.WebhookInput{
			Name:            r.FormValue("name"),
			URL:             r.FormValue("url"),
			Events:          r.Form["event"],
			BodyTemplate:    r.FormValue("body_template"),
			Headers:         headers,
			RetryEnabled:    r.FormValue("retry_enabled") == "on",
			RetryCount:      retryCount,
			RetryBackoff:    retryBackoff,
			RetryMaxBackoff: retryMaxBackoff,
			RetryJitter:     r.FormValue("retry_jitter") == "on",
			Enabled:         r.FormValue("enabled") == "on",
		})
		if err != nil {
			writeAdminProblem(logger, w, err, "Webhook")
			return
		}
		http.Redirect(w, r, "/admin/webhooks", http.StatusSeeOther)
	}
}

// optionalPositivePathID parses an optional positive identifier from a route value.
func optionalPositivePathID(raw string) (int64, error) {
	if raw == "" {
		return 0, nil
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		return 0, errors.New("identifier must be a positive integer")
	}
	return id, nil
}

// webhookRetryFromForm parses persisted duration-based retry settings.
func webhookRetryFromForm(r *http.Request) (int, time.Duration, time.Duration, error) {
	retryCount, err := strconv.Atoi(strings.TrimSpace(r.FormValue("retry_count")))
	if err != nil {
		return 0, 0, 0, newRequestError("retry_count", "Retries must be a number.", err)
	}
	retryBackoff, err := time.ParseDuration(strings.TrimSpace(r.FormValue("retry_backoff")))
	if err != nil {
		return 0, 0, 0, newRequestError("retry_backoff", "Initial backoff must be a duration such as 1s.", err)
	}
	retryMaxBackoff, err := time.ParseDuration(strings.TrimSpace(r.FormValue("retry_max_backoff")))
	if err != nil {
		return 0, 0, 0, newRequestError("retry_max_backoff", "Maximum backoff must be a duration such as 30s.", err)
	}

	return retryCount, retryBackoff, retryMaxBackoff, nil
}

// webhookHeadersFromForm parses dynamic webhook request-header rows.
func webhookHeadersFromForm(r *http.Request) ([]appwebhooks.WebhookHeaderInput, error) {
	rows := r.Form["webhook_header_row"]
	if len(rows) == 0 {
		return nil, nil
	}

	seen := make(map[string]struct{}, len(rows))
	headers := make([]appwebhooks.WebhookHeaderInput, 0, len(rows))
	for _, row := range rows {
		if !validWebhookHeaderRow(row) {
			return nil, newRequestError("headers", "The webhook header form is invalid.", nil)
		}
		if _, exists := seen[row]; exists {
			return nil, newRequestError("headers", "The webhook header form contains a duplicate row.", nil)
		}
		seen[row] = struct{}{}

		prefix := "webhook_header_" + row + "_"
		id := int64(0)
		if rawID := strings.TrimSpace(r.FormValue(prefix + "id")); rawID != "" {
			parsed, err := strconv.ParseInt(rawID, 10, 64)
			if err != nil || parsed <= 0 {
				return nil, newRequestError("headers", "The webhook header form is invalid.", err)
			}
			id = parsed
		}

		headers = append(headers, appwebhooks.WebhookHeaderInput{
			ID:        id,
			Name:      r.FormValue(prefix + "name"),
			Value:     r.FormValue(prefix + "value"),
			Sensitive: r.FormValue(prefix+"sensitive") == "on",
		})
	}

	return headers, nil
}

// validWebhookHeaderRow restricts dynamic form keys to a small identifier alphabet.
func validWebhookHeaderRow(value string) bool {
	if value == "" || len(value) > 64 {
		return false
	}
	for _, character := range value {
		if character >= 'a' && character <= 'z' ||
			character >= 'A' && character <= 'Z' ||
			character >= '0' && character <= '9' ||
			character == '-' || character == '_' {
			continue
		}
		return false
	}
	return true
}

// DeleteAdminWebhook removes one configured outgoing webhook.
func DeleteAdminWebhook(webhookUseCases webhookAdminService, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil || id <= 0 {
			httpresponse.Problem(w, http.StatusBadRequest, "Invalid webhook identifier.")
			return
		}
		if err := webhookUseCases.DeleteWebhook(r.Context(), id); err != nil {
			writeAdminProblem(logger, w, err, "Webhook")
			return
		}
		http.Redirect(w, r, "/admin/webhooks", http.StatusSeeOther)
	}
}

// TestAdminWebhook sends a diagnostic delivery to one configured webhook.
func TestAdminWebhook(webhookUseCases webhookAdminService, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil || id <= 0 {
			httpresponse.Problem(w, http.StatusBadRequest, "Invalid webhook identifier.")
			return
		}
		if err := webhookUseCases.TestWebhook(r.Context(), id); err != nil {
			httpresponse.InternalServerError(logger, w, err)
			return
		}
		http.Redirect(w, r, "/admin/webhooks", http.StatusSeeOther)
	}
}

// RevealAdminWebhookHeader decrypts one sensitive webhook header after an explicit administrator action.
func RevealAdminWebhookHeader(webhookUseCases webhookAdminService, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "private, no-store")
		webhookID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil || webhookID <= 0 {
			httpresponse.Problem(w, http.StatusBadRequest, "Invalid webhook identifier.")
			return
		}
		headerID, err := strconv.ParseInt(r.PathValue("headerID"), 10, 64)
		if err != nil || headerID <= 0 {
			httpresponse.Problem(w, http.StatusBadRequest, "Invalid webhook header identifier.")
			return
		}

		value, err := webhookUseCases.RevealWebhookHeader(r.Context(), webhookID, headerID)
		if err != nil {
			writeAdminProblem(logger, w, err, "Webhook header")
			return
		}
		httpresponse.Respond(w, http.StatusOK, map[string]string{"value": value})
	}
}
