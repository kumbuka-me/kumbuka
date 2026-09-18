package plugin

import (
	"context"
	"fmt"
)

// RunAdminAction invokes one active administrator action while pinning its plugin lifetime.
func (m *Manager) RunAdminAction(ctx context.Context, pluginID, moduleID string) error {
	if !validID.MatchString(moduleID) {
		return fmt.Errorf("invalid admin action")
	}

	entry, release, ok := m.registry.AcquireEntry(pluginID)
	if !ok {
		return fmt.Errorf("plugin %s is not active", pluginID)
	}
	defer release()

	for _, module := range entry.Contributions.AdminActions {
		if module.ID != moduleID {
			continue
		}
		if module.Action == nil {
			return fmt.Errorf("admin action %s is unavailable", moduleID)
		}
		_, err := Guard(pluginID, func() (struct{}, error) {
			return struct{}{}, module.Action.Run(ctx)
		})
		return err
	}

	return fmt.Errorf("admin action %s is not active in plugin %s", moduleID, pluginID)
}
