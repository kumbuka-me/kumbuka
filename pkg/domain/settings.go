package domain

import "strings"

// RenderingSettings contains application-wide content presentation defaults.
type RenderingSettings struct {
	// DefaultTypographySize is used when a user has not selected a personal content size.
	DefaultTypographySize TypographySize
}

// AuthenticationSettings controls browser authentication without storing secrets.
type AuthenticationSettings struct {
	// Mode selects local, trusted-proxy, or OIDC authentication.
	Mode AuthMode
	// OIDCIssuer is the OIDC discovery issuer URL.
	OIDCIssuer string
	// OIDCClientID is the public OIDC client identifier.
	OIDCClientID string
	// OIDCGroupClaim is the top-level claim containing external group names.
	OIDCGroupClaim string
	// OIDCGroupSync enables synchronization of explicitly mapped OIDC groups.
	OIDCGroupSync bool
	// OIDCGroupsAuthoritative removes mapped memberships that disappear from the claim.
	OIDCGroupsAuthoritative bool
	// OIDCGroupMappings maps external group values to Kumbuka groups.
	OIDCGroupMappings []OIDCGroupMapping
	// OIDCAdminGroup is the claim value that grants session-scoped administrator access.
	OIDCAdminGroup string
	// TrustedUsernameHeaders lists trusted-proxy username headers in priority order.
	TrustedUsernameHeaders []string
	// TrustedEmailHeaders lists trusted-proxy email headers in priority order.
	TrustedEmailHeaders []string
	// TrustedDisplayNameHeaders lists trusted-proxy display-name headers in priority order.
	TrustedDisplayNameHeaders []string
	// TrustedGroupHeaders lists trusted-proxy group header candidates.
	TrustedGroupHeaders []string
	// TrustedAdminGroup is the trusted group value that grants administrator access.
	TrustedAdminGroup string
}

// ExternalLink describes one configurable top-bar link.
type ExternalLink struct {
	// Label is the display label for external link.
	Label string `json:"label" toml:"label"`
	// URL is the target URL for external link.
	URL string `json:"url" toml:"url"`
	// Icon names the icon used for external link.
	Icon string `json:"icon,omitempty" toml:"icon"`
	// Description describes external link.
	Description string `json:"description,omitempty" toml:"description"`
	// HoverEffect controls visual feedback when a pointer hovers over the link.
	HoverEffect ExternalLinkHoverEffect `json:"hover_effect,omitempty" toml:"hover_effect"`
	// HoverText is an optional title template supporting {{label}} and {{description}}.
	HoverText string `json:"hover_text,omitempty" toml:"hover_text"`
}

// ExternalLinkHoverEffects returns the supported external-link hover presentations.
func ExternalLinkHoverEffects() []string {
	return []string{string(ExternalLinkHoverHighlight), string(ExternalLinkHoverLift), string(ExternalLinkHoverNone)}
}

// ValidExternalLinkHoverEffect reports whether value is a supported hover presentation. Empty selects the default highlight presentation.
func ValidExternalLinkHoverEffect(value ExternalLinkHoverEffect) bool {
	switch value {
	case "", ExternalLinkHoverHighlight, ExternalLinkHoverLift, ExternalLinkHoverNone:
		return true
	default:
		return false
	}
}

// EffectiveExternalLinkHoverEffect returns the visual hover presentation used for a link.
func EffectiveExternalLinkHoverEffect(value ExternalLinkHoverEffect) ExternalLinkHoverEffect {
	if value == "" {
		return ExternalLinkHoverHighlight
	}
	return value
}

// ExternalLinkHoverTitle resolves a link's hover text template.
func ExternalLinkHoverTitle(link ExternalLink) string {
	label := strings.TrimSpace(link.Label)
	description := strings.TrimSpace(link.Description)
	template := strings.TrimSpace(link.HoverText)

	if template == "" && description == "" {
		return label
	}
	if template == "" {
		return label + " — " + description
	}

	return strings.TrimSpace(expandExternalLinkHoverTemplate(template, link))
}

// expandExternalLinkHoverTemplate replaces supported placeholders and preserves unknown ones.
func expandExternalLinkHoverTemplate(value string, link ExternalLink) string {
	var output strings.Builder

	for value != "" {
		start := strings.Index(value, "{{")
		if start < 0 {
			output.WriteString(value)
			break
		}

		output.WriteString(value[:start])

		rest := value[start+2:]
		end := strings.Index(rest, "}}")
		if end < 0 {
			output.WriteString(value[start:])
			break
		}

		placeholderEnd := start + 2 + end
		name := strings.TrimSpace(value[start+2 : placeholderEnd])

		switch name {
		case "label":
			output.WriteString(link.Label)
		case "description":
			output.WriteString(link.Description)
		default:
			output.WriteString(value[start : placeholderEnd+2])
		}

		value = value[placeholderEnd+2:]
	}

	return output.String()
}

// PDFHeader describes one configurable request header sent to the external PDF service.
type PDFHeader struct {
	// ID identifies PDF header.
	ID int64
	// Name is the name of PDF header.
	Name string
	// Value contains the value represented by PDF header.
	Value string
	// Sensitive marks a PDF request header value as secret.
	Sensitive bool
	// Configured reports whether a sensitive header has a stored value without exposing it.
	Configured bool
}

// ApplicationSettings contains mutable application-wide settings.
type ApplicationSettings struct {
	// AllowUserRegistration permits new OIDC and trusted-proxy identities to create wiki accounts.
	AllowUserRegistration bool
	// DiscussionsEnabled enables page comments and anchored discussions.
	DiscussionsEnabled bool
	// ContentLanguage is the BCP 47 language tag applied to wiki content and the editor.
	ContentLanguage string
	// PDFURL is the persisted HTML-to-PDF rendering endpoint.
	PDFURL string
	// ExternalLinks contains configurable links rendered beside global search.
	ExternalLinks []ExternalLink
	// RobotsPolicy controls whether robots.txt allows, disallows, or omits crawler guidance.
	RobotsPolicy RobotsPolicy
	// Authentication contains non-secret browser authentication settings.
	Authentication AuthenticationSettings
	// Rendering contains application-wide content presentation defaults.
	Rendering RenderingSettings
	// EditorToolbarOverrides contains global administrator overrides keyed by stable contribution ID.
	EditorToolbarOverrides []EditorToolbarOverride
}

// EditorToolbarOverride customizes one plugin toolbar contribution without changing its manifest default.
type EditorToolbarOverride struct {
	// ID is the stable plugin ID and contribution ID pair.
	ID string `json:"id"`
	// Group moves the contribution to one plugin-allowed host group; empty preserves the default.
	Group string `json:"group,omitempty"`
	// Hidden removes the contribution from both editor modes.
	Hidden bool `json:"hidden,omitempty"`
	// Order overrides the plugin's default ordering hint.
	Order int `json:"order,omitempty"`
}
