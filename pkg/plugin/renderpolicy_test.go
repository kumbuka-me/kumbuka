package plugin

import (
	"testing"

	"github.com/kumbuka-me/sdk"
	"github.com/stretchr/testify/assert"
)

func TestRenderPoliciesBecomeOpaqueRequestFeatures(t *testing.T) {
	plan := &RenderPlan{RenderPolicies: []RenderPolicy{{ID: "operators", Policy: "preserve-programming-operators"}}}
	features := plan.RenderFeatures(map[string]bool{"example.setting": true})

	assert.True(t, features["example.setting"])
	assert.True(t, features[sdk.RenderPolicyFeature("preserve-programming-operators")])
}
