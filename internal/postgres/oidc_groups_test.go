package postgres

import (
	"testing"

	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/stretchr/testify/require"
)

func TestBuildOIDCGroupMembershipPlan(t *testing.T) {
	t.Run("maps claimed groups", func(t *testing.T) {
		plan := buildOIDCGroupMembershipPlan([]string{"engineering"}, []domain.OIDCGroupMapping{{OIDCGroup: "engineering", GroupID: 10}, {OIDCGroup: "support", GroupID: 20}})
		require.Equal(t, map[int64]bool{10: true, 20: true}, plan.managed)
		require.Equal(t, map[int64]bool{10: true}, plan.desired)
	})

	t.Run("ignores blank claims and invalid mappings", func(t *testing.T) {
		plan := buildOIDCGroupMembershipPlan([]string{"  "}, []domain.OIDCGroupMapping{{OIDCGroup: "engineering", GroupID: 0}})
		require.Empty(t, plan.managed)
		require.Empty(t, plan.desired)
	})
}
