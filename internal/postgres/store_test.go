package postgres

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestOpenRejectsNegativeMaxConns(t *testing.T) {
	t.Parallel()

	database, err := Open(
		context.Background(),
		"postgres://example/kumbuka",
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		WithMaxConns(-1),
	)

	assert.Nil(t, database)
	assert.EqualError(t, err, "database max connections must not be negative")
}

func TestOpenRejectsNegativeMinIdleConns(t *testing.T) {
	t.Parallel()

	database, err := Open(
		context.Background(),
		"postgres://example/kumbuka",
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		WithMinIdleConns(-1),
	)

	assert.Nil(t, database)
	assert.EqualError(t, err, "database minimum idle connections must not be negative")
}

func TestOpenRejectsMinIdleConnsAboveMaxConns(t *testing.T) {
	t.Parallel()

	database, err := Open(
		context.Background(),
		"postgres://example/kumbuka",
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		WithMaxConns(4),
		WithMinIdleConns(5),
	)

	assert.Nil(t, database)
	assert.EqualError(t, err, "database minimum idle connections must not exceed maximum connections")
}
