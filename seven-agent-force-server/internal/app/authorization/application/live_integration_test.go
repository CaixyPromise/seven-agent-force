package application

import (
	"context"
	"os"
	"testing"

	authdomain "github.com/CaixyPromise/seven-agent/force/internal/app/authorization/domain"
	authinfra "github.com/CaixyPromise/seven-agent/force/internal/app/authorization/infrastructure"
	"github.com/CaixyPromise/seven-agent/force/internal/infrastructure/datasource"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/config"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/xid"
)

func TestBuildUserContextAgainstLiveSchema(t *testing.T) {
	dsn := os.Getenv("SEVEN_AUTH_LIVE_DSN")
	if dsn == "" {
		t.Skip("SEVEN_AUTH_LIVE_DSN is not set")
	}
	userID := int64(1819823136159395842)

	cfg, err := config.Load("../../../../configs")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	cfg.Datasource.Driver = "mysql"
	cfg.Datasource.MySQL.Enabled = true
	cfg.Datasource.MySQL.DSN = dsn

	provider, err := datasource.NewProvider(cfg.Datasource, nil)
	if err != nil {
		t.Fatalf("build provider: %v", err)
	}
	defer provider.Close()

	repository, err := authinfra.NewRepository(provider, nil)
	if err != nil {
		t.Fatalf("build repository: %v", err)
	}
	idGen, err := xid.New(cfg.ID.Node)
	if err != nil {
		t.Fatalf("build id generator: %v", err)
	}

	service := NewService(
		cfg.Authorization,
		nil,
		provider.Transactor(),
		repository,
		authdomain.NewService(),
		idGen,
		nil,
		nil,
		nil,
		nil,
	)

	userContext, err := service.BuildUserContext(context.Background(), userID, "probe-session", nil, nil, "integration-test")
	if err != nil {
		t.Fatalf("BuildUserContext() error = %v", err)
	}
	if userContext == nil || userContext.UserID != userID {
		t.Fatalf("BuildUserContext() user = %#v", userContext)
	}

	userVO, err := service.GetUserVO(context.Background(), userID)
	if err != nil {
		t.Fatalf("GetUserVO() error = %v", err)
	}
	if userVO == nil || userVO.UserID != userID {
		t.Fatalf("GetUserVO() user = %#v", userVO)
	}

	menus, err := service.GetCurrentUserMenus(context.Background(), userID)
	if err != nil {
		t.Fatalf("GetCurrentUserMenus() error = %v", err)
	}
	if len(menus) == 0 {
		t.Fatalf("GetCurrentUserMenus() returned no menus")
	}
}
