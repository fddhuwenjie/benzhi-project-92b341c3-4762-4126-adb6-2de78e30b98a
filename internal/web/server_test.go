package web

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"pressure-tap-qualification/internal/application"
	"pressure-tap-qualification/internal/evidence"
	"pressure-tap-qualification/internal/store"
)

func testServer(t *testing.T) *Server {
	t.Helper()
	repo, err := store.NewFileRepository(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return New(application.New(repo, evidence.New(repo)))
}

func TestWorkbenchAndCreateValidation(t *testing.T) {
	s := testServer(t)
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	response := httptest.NewRecorder()
	s.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !bytes.Contains(response.Body.Bytes(), []byte("<body>")) {
		t.Fatal("工作台 HTML 未提供完整 body")
	}
	request = httptest.NewRequest(http.MethodPost, "/api/batches", bytes.NewBufferString(`{"unknown":true}`))
	request.Header.Set("Content-Type", "application/json")
	response = httptest.NewRecorder()
	s.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("未知字段应返回 400，得到 %d", response.Code)
	}
}
