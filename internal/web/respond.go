package web

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"pressure-tap-qualification/internal/domain"
)

type apiError struct {
	Error       string            `json:"error"`
	Code        domain.ErrorCode  `json:"code"`
	FieldErrors map[string]string `json:"field_errors,omitempty"`
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, err error) {
	status := http.StatusBadRequest
	code := domain.ErrorCodeOf(err)
	switch code {
	case domain.CodeNotFound:
		status = http.StatusNotFound
	case domain.CodeConflict:
		status = http.StatusConflict
	case domain.CodeForbidden:
		status = http.StatusForbidden
	case domain.CodeState, domain.CodeUnqualified:
		status = http.StatusUnprocessableEntity
	}
	writeJSON(w, status, apiError{Error: err.Error(), Code: code, FieldErrors: domain.FieldErrorsOf(err)})
}

func decodeJSON(r *http.Request, target any) error {
	r.Body = http.MaxBytesReader(nil, r.Body, 1<<20)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return domain.NewError(domain.CodeInvalid, "JSON 请求无效：%v", err)
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return domain.NewError(domain.CodeInvalid, "请求只能包含一个 JSON 对象")
	}
	return nil
}
