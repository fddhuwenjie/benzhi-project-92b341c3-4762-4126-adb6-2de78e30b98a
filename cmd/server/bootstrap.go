package main

import (
	"fmt"
	"net/http"

	"pressure-tap-qualification/internal/application"
	"pressure-tap-qualification/internal/evidence"
	"pressure-tap-qualification/internal/store"
	"pressure-tap-qualification/internal/web"
)

func buildHandler(dataDir string) (http.Handler, error) {
	repository, err := store.NewFileRepository(dataDir)
	if err != nil {
		return nil, fmt.Errorf("打开数据目录: %w", err)
	}
	evidenceService := evidence.New(repository)
	app := application.New(repository, evidenceService)
	return web.New(app), nil
}
