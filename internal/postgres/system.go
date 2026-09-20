package postgres

import "context"

// DatabaseSize returns the size of the current PostgreSQL database in bytes.
func (s *Store) DatabaseSize(ctx context.Context) (int64, error) {
	var size int64
	if err := s.pool.QueryRow(ctx, "SELECT pg_database_size(current_database())").Scan(&size); err != nil {
		return 0, err
	}

	return size, nil
}
