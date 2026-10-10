package postgres

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
)

// oidcGroupMembershipPlan separates mapped memberships from memberships currently desired.
type oidcGroupMembershipPlan struct {
	// managed contains every Kumbuka group controlled by the supplied mappings.
	managed map[int64]bool
	// desired contains mapped Kumbuka groups asserted by the current identity claim.
	desired map[int64]bool
}

// buildOIDCGroupMembershipPlan resolves provider claims through the configured group mappings.
func buildOIDCGroupMembershipPlan(claimedGroups []string, mappings []domain.OIDCGroupMapping) oidcGroupMembershipPlan {
	claimed := make(map[string]bool, len(claimedGroups))
	for _, group := range claimedGroups {
		if group = strings.TrimSpace(group); group != "" {
			claimed[group] = true
		}
	}
	plan := oidcGroupMembershipPlan{managed: make(map[int64]bool, len(mappings)), desired: make(map[int64]bool, len(mappings))}
	for _, mapping := range mappings {
		if mapping.GroupID <= 0 {
			continue
		}
		plan.managed[mapping.GroupID] = true
		if claimed[strings.TrimSpace(mapping.OIDCGroup)] {
			plan.desired[mapping.GroupID] = true
		}
	}
	return plan
}

// applyOIDCGroupMembershipPlan updates mapped group memberships inside an existing transaction.
func applyOIDCGroupMembershipPlan(ctx context.Context, tx pgx.Tx, userID int64, plan oidcGroupMembershipPlan, authoritative bool) error {
	if authoritative {
		for groupID := range plan.managed {
			if plan.desired[groupID] {
				continue
			}
			if _, err := tx.Exec(ctx, `
DELETE FROM user_groups
WHERE user_id=$1 AND group_id=$2`, userID, groupID); err != nil {
				return err
			}
		}
	}
	for groupID := range plan.desired {
		if _, err := tx.Exec(ctx, `
INSERT INTO user_groups(user_id,group_id)
VALUES($1,$2)
ON CONFLICT DO NOTHING`, userID, groupID); err != nil {
			return err
		}
	}
	return nil
}
