package app

import (
	"context"
	"fmt"
	"strings"

	"github.com/slhmy/identra/internal/config"
	"github.com/slhmy/identra/internal/identra"
	"github.com/slhmy/identra/internal/serviceaccount"
	"github.com/slhmy/identra/internal/store/sqlite"
)

func buildStores(_ context.Context, cfg config.PersistenceConfig) (identra.UserStore, identra.ExternalIdentityStore, serviceaccount.Store, func(context.Context) error, error) {
	repoType := strings.ToLower(strings.TrimSpace(cfg.Type))
	if repoType != "" && repoType != "sqlite" {
		return nil, nil, nil, nil, fmt.Errorf("unsupported user repository type: %s", cfg.Type)
	}
	db, err := sqlite.Open(cfg.SQLite)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("failed to initialize sqlite database: %w", err)
	}
	userStore, extStore := sqlite.NewStores(db)
	return userStore, extStore, sqlite.NewServiceAccountStore(db), func(context.Context) error { return db.Close() }, nil
}
