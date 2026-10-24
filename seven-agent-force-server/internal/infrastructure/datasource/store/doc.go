// Package store provides database-agnostic execution and transaction helpers.
//
// Recommended repository shape:
//
//	type Repository struct {
//	    db store.DBTX
//	}
//
//	func NewRepository(db store.DBTX) *Repository {
//	    return &Repository{db: db}
//	}
//
//	func (r *Repository) ListUsers(ctx context.Context) error {
//	    rows, err := store.Executor(ctx, r.db).QueryContext(ctx, "SELECT id FROM sys_user")
//	    if err != nil {
//	        return err
//	    }
//	    defer rows.Close()
//	    return nil
//	}
//
// Application services should depend on a store.Transactor and call
// WithinTransaction when a use case needs a transaction boundary. Repositories
// stay unaware of whether the current executor is a plain *sql.DB or a *sql.Tx.
//
// For sqlx-based query repositories, prefer store.SQLXExecutor(ctx, r.db).
package store
