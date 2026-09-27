package authentication

import (
	"context"

	"github.com/kumbuka-me/kumbuka/pkg/domain"
)

// registrationSettingsRepository loads the persisted external-registration policy.
type registrationSettingsRepository interface {
	ApplicationSettings(context.Context) (domain.ApplicationSettings, error)
}

// registrationPolicy resolves deployment and persisted external-registration settings.
type registrationPolicy struct {
	// repository loads persisted application settings when no deployment override exists.
	repository registrationSettingsRepository
	// override replaces the persisted registration setting when configured by deployment policy.
	override *bool
}

// newRegistrationPolicy constructs a shared external-registration policy resolver.
func newRegistrationPolicy(repository registrationSettingsRepository, override *bool) registrationPolicy {
	return registrationPolicy{repository: repository, override: override}
}

// enabled resolves the deployment override before the persisted registration setting.
func (p registrationPolicy) enabled(ctx context.Context) (bool, error) {
	if p.override != nil {
		return *p.override, nil
	}

	settings, err := p.repository.ApplicationSettings(ctx)
	if err != nil {
		return false, err
	}

	return settings.AllowUserRegistration, nil
}
