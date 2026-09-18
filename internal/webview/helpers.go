package webview

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/kumbuka-me/kumbuka/pkg/domain"
)

// webhookContext combines one webhook with template metadata required by the form.
func webhookContext(item domain.Webhook, events []string, encryptionKeyConfigured bool) webhookView {
	return webhookView{
		Webhook:                 item,
		AvailableEvents:         events,
		EncryptionKeyConfigured: encryptionKeyConfigured,
	}
}

// pageTemplateContext combines one blueprint with groups and lifecycle choices.
func pageTemplateContext(item domain.PageTemplate, groups []domain.Group, statuses []string) pageTemplateView {
	return pageTemplateView{
		PageTemplate: item,
		Groups:       groups,
		PageStatuses: statuses,
	}
}

// blankPageTemplate returns the defaults used for a new blueprint form.
func blankPageTemplate() domain.PageTemplate {
	return domain.PageTemplate{Status: "verified", Properties: map[string]string{}}
}

// templatePropertiesText serializes blueprint properties for the administration form.
func templatePropertiesText(properties map[string]string) string {
	keys := make([]string, 0, len(properties))
	for key := range properties {
		keys = append(keys, key)
	}

	slices.SortFunc(keys, func(left, right string) int {
		return strings.Compare(strings.ToLower(left), strings.ToLower(right))
	})

	lines := make([]string, 0, len(keys))
	for _, key := range keys {
		lines = append(lines, key+"="+properties[key])
	}
	return strings.Join(lines, "\n")
}

// templateFieldsText serializes blueprint fields for the administration form.
func templateFieldsText(fields []domain.PageTemplateField) string {
	lines := make([]string, 0, len(fields))
	for _, field := range fields {
		required := ""
		if field.Required {
			required = "required"
		}
		lines = append(lines, strings.Join([]string{field.Name, field.Label, field.Default, required}, " | "))
	}
	return strings.Join(lines, "\n")
}

// timeAgo formats recent timestamps as compact relative ages.
func timeAgo(value time.Time) string {
	duration := time.Since(value)
	switch {
	case duration < time.Minute:
		return "just now"
	case duration < time.Hour:
		return strconv.Itoa(int(duration.Minutes())) + "m ago"
	case duration < 24*time.Hour:
		return strconv.Itoa(int(duration.Hours())) + "h ago"
	default:
		return value.Format("2006-01-02")
	}
}

// fileSize formats byte counts for compact template display.
func fileSize(size int64) string {
	const unit = 1024
	if size < unit {
		return strconv.FormatInt(size, 10) + " B"
	}

	divisor := int64(unit)
	exponent := 0

	for value := size / unit; value >= unit && exponent < 3; value /= unit {
		divisor *= unit
		exponent++
	}

	return fmt.Sprintf("%.1f %ciB", float64(size)/float64(divisor), "KMGT"[exponent])
}

// fingerprintAssets returns a stable short hash for the complete embedded web filesystem.
func fingerprintAssets(appFS fs.FS) (string, error) {
	hash := sha256.New()
	err := fs.WalkDir(appFS, ".", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}

		data, err := fs.ReadFile(appFS, path)
		if err != nil {
			return err
		}

		_, _ = hash.Write([]byte(path))
		_, _ = hash.Write([]byte{0})
		_, _ = hash.Write(data)
		_, _ = hash.Write([]byte{0})

		return nil
	})
	if err != nil {
		return "", err
	}

	sum := hash.Sum(nil)

	return hex.EncodeToString(sum[:8]), nil
}

// hasGroup reports whether a group name appears in a user's group list.
func hasGroup(groups []string, name string) bool {
	return slices.Contains(groups, name)
}

// hasGroupID reports whether a group identifier appears in a page group list.
func hasGroupID(groups []domain.Group, id int64) bool {
	return slices.ContainsFunc(groups, func(group domain.Group) bool {
		return group.ID == id
	})
}
