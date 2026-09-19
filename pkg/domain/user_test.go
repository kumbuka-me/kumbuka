package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestUserAuthorizationSemantics(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		user          User
		administrator bool
		canEdit       bool
	}{
		{name: "administrator role", user: User{Role: UserRoleAdmin}, administrator: true, canEdit: true},
		{name: "external administrator", user: User{Role: UserRoleViewer, ExternalAdmin: true}, administrator: true, canEdit: true},
		{name: "editor", user: User{Role: UserRoleEditor}, administrator: false, canEdit: true},
		{name: "viewer", user: User{Role: UserRoleViewer}, administrator: false, canEdit: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, test.administrator, test.user.IsAdministrator())
			assert.Equal(t, test.canEdit, test.user.CanEditContent())
		})
	}
}

func TestValidUserRoleUsesCanonicalRoles(t *testing.T) {
	t.Parallel()

	assert.True(t, ValidUserRole(UserRoleAdmin))
	assert.True(t, ValidUserRole(UserRoleEditor))
	assert.True(t, ValidUserRole(UserRoleViewer))
	assert.False(t, ValidUserRole("owner"))
}
