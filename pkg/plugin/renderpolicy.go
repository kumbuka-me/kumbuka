package plugin

import (
	"maps"

	"github.com/kumbuka-me/sdk"
)

// RenderFeatures merges request-scoped feature flags with semantic rendering policies contributed by active plugins. Policy names remain opaque to core.
func (p *RenderPlan) RenderFeatures(features map[string]bool) map[string]bool {
	result := maps.Clone(features)
	if result == nil {
		result = make(map[string]bool)
	}
	if p == nil {
		return result
	}
	for _, contribution := range p.RenderPolicies {
		result[sdk.RenderPolicyFeature(contribution.Policy)] = true
	}
	return result
}
