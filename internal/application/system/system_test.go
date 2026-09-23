package system

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// systemRepositoryStub provides controllable system repository behavior for tests.
type systemRepositoryStub struct {
	// databaseSize configures or records the database size value used by the fixture.
	databaseSize int64
	// databaseErr configures the error returned by the test double.
	databaseErr error
}

func (s systemRepositoryStub) DatabaseSize(context.Context) (int64, error) {
	return s.databaseSize, s.databaseErr
}

func (systemRepositoryStub) Ping(context.Context) error { return nil }

func (systemRepositoryStub) SetupRequired(context.Context) (bool, error) { return false, nil }

func (systemRepositoryStub) LogAudit(context.Context, int64, string, string, string, string) error {
	return nil
}

func TestDatabaseSize(t *testing.T) {
	t.Parallel()

	t.Run("returns repository size", func(t *testing.T) {
		t.Parallel()

		system := NewSystem(systemRepositoryStub{databaseSize: 192 * 1024 * 1024})

		size, err := system.DatabaseSize(context.Background())

		require.NoError(t, err)
		assert.Equal(t, int64(192*1024*1024), size)
	})
}
