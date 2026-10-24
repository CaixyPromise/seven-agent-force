package datasource

import (
	"fmt"
	"path/filepath"

	"github.com/CaixyPromise/seven-agent/force/internal/infrastructure/datasource/mysql"
	"github.com/CaixyPromise/seven-agent/force/internal/infrastructure/datasource/postgres"
	"github.com/CaixyPromise/seven-agent/force/internal/infrastructure/datasource/store"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/config"
	"go.uber.org/zap"
)

func NewProvider(cfg config.DatasourceConfig, log *zap.Logger) (store.Provider, error) {
	switch cfg.Driver {
	case "mysql":
		return mysql.NewProvider(cfg.MySQL, log)
	case "postgres":
		return postgres.NewProvider(cfg.Postgres, log)
	default:
		return nil, fmt.Errorf("unsupported datasource driver: %s", cfg.Driver)
	}
}

func MigrationDir(driver string) string {
	return filepath.Join("migrations", driver)
}
