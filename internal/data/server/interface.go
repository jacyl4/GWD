package server

import "context"

// Repository describes the persistence contract for server-scoped data.
// Methods will be added as the server functionality grows.
type Repository interface {
	// Bootstrap prepares the backing store (e.g. run migrations, seed data).
	Bootstrap(ctx context.Context) error
}
