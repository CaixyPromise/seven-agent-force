package store

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/jmoiron/sqlx"
)

type exampleDB struct{}

func (exampleDB) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return nil, nil
}

func (exampleDB) PrepareContext(ctx context.Context, query string) (*sql.Stmt, error) {
	return nil, nil
}

func (exampleDB) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	return nil, nil
}

func (exampleDB) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	return nil
}

type exampleRepository struct {
	db DBTX
}

func (r exampleRepository) executorName(ctx context.Context) string {
	return fmt.Sprintf("%T", Executor(ctx, r.db))
}

func ExampleExecutor_repositoryPattern() {
	repo := exampleRepository{db: exampleDB{}}

	fmt.Println(repo.executorName(context.Background()))

	txCtx := context.WithValue(context.Background(), txContextKey{}, &sql.Tx{})
	fmt.Println(repo.executorName(txCtx))

	// Output:
	// store.exampleDB
	// *sql.Tx
}

func ExampleSQLXExecutor_queryRepositoryPattern() {
	base := sqlx.NewDb(&sql.DB{}, "mysql")

	fmt.Println(fmt.Sprintf("%T", SQLXExecutor(context.Background(), base)))

	queryCtx := context.WithValue(context.Background(), sqlxContextKey{}, base)
	fmt.Println(SQLXExecutor(queryCtx, base) == base)

	// Output:
	// *sqlx.DB
	// true
}
