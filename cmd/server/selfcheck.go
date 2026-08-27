package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"time"
)

type checkResponse struct {
	BatchID          string   `json:"batch_id"`
	Revision         uint64   `json:"revision"`
	CreatedDefectIDs []string `json:"created_defect_ids"`
	DraftDiffSummary string   `json:"draft_diff_summary"`
	ThresholdDigest  string   `json:"threshold_digest"`
	TopologyDigest   string   `json:"topology_digest"`
	BatchInfoDigest  string   `json:"batch_info_digest"`
	FactDigest       string   `json:"fact_digest"`
	Certificate      *struct {
		CanonicalDigest string `json:"canonical_digest"`
	} `json:"certificate"`
	Valid bool   `json:"valid"`
	Error string `json:"error"`
}

func runSelfCheck(ctx context.Context, address string) error {
	dir, err := os.MkdirTemp("", "pressure-tap-self-check-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	handler, err := buildHandler(dir)
	if err != nil {
		return err
	}
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return fmt.Errorf("自检监听失败: %w", err)
	}
	server := &http.Server{Handler: handler, ReadHeaderTimeout: 2 * time.Second}
	serveDone := make(chan error, 1)
	go func() { serveDone <- server.Serve(listener) }()
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
		<-serveDone
	}()
	base := "http://" + listener.Addr().String()
	client := &http.Client{Timeout: 3 * time.Second}
	revision := uint64(0)
	request := 0
	post := func(path string, payload map[string]any) (checkResponse, error) {
		request++
		payload["request_id"] = fmt.Sprintf("SELF-%02d", request)
		payload["expected_revision"] = revision
		if _, ok := payload["actor_id"]; !ok {
			payload["actor_id"] = "engineer-a"
		}
		data, _ := json.Marshal(payload)
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+path, bytes.NewReader(data))
		if err != nil {
			return checkResponse{}, err
		}
		req.Header.Set("Content-Type", "application/json")
		res, err := client.Do(req)
		if err != nil {
			return checkResponse{}, err
		}
		defer res.Body.Close()
		var out checkResponse
		if err = json.NewDecoder(res.Body).Decode(&out); err != nil {
			return out, err
		}
		if res.StatusCode >= 300 {
			return out, fmt.Errorf("%s 返回 %d: %s", path, res.StatusCode, out.Error)
		}
		if out.Revision > 0 {
			revision = out.Revision
		}
		return out, nil
	}
	get := func(path string) (checkResponse, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+path, nil)
		if err != nil {
			return checkResponse{}, err
		}
		res, err := client.Do(req)
		if err != nil {
			return checkResponse{}, err
		}
		defer res.Body.Close()
		var out checkResponse
		if err = json.NewDecoder(res.Body).Decode(&out); err != nil {
			return out, err
		}
		if res.StatusCode >= 300 {
			return out, fmt.Errorf("%s 返回 %d: %s", path, res.StatusCode, out.Error)
		}
		return out, nil
	}
	batchID := "SELF-CHECK"
	validUntil := time.Now().UTC().Add(24 * time.Hour).Format(time.RFC3339Nano)
	created, err := post("/api/batches", map[string]any{"batch_id": batchID, "model_code": "WT-SC-01", "test_objective": "验证压力测孔闭环自检", "taps": []map[string]any{{"tap_id": "T01", "label": "上翼面 01", "surface_zone": "wing-upper", "nominal_diameter_mm": 1.2, "neighbor_tap_ids": []string{}}}})
	if err != nil {
		return err
	}
	if _, err = post("/api/batches/"+batchID+"/freeze", map[string]any{"confirmed_diff_summary": created.DraftDiffSummary, "threshold_digest": created.ThresholdDigest, "topology_digest": created.TopologyDigest, "batch_info_digest": created.BatchInfoDigest, "warning_acknowledgements": []string{"isolated:T01:"}}); err != nil {
		return err
	}
	if _, err = post("/api/batches/"+batchID+"/calibration", map[string]any{"reference": "CAL-SC", "instrument_summary": "自检校准器具", "valid_until": validUntil}); err != nil {
		return err
	}
	measured, err := post("/api/batches/"+batchID+"/measurements", map[string]any{"round_id": "ROUND-FAIL", "tap_id": "T01", "round_kind": "initial", "calibration_ref": "CAL-SC", "supply_pressure_pa": 1000, "steady_pressure_pa": 1100, "decay_seconds": 1, "neighbor_responses": []map[string]any{}})
	if err != nil {
		return err
	}
	if len(measured.CreatedDefectIDs) != 1 {
		return fmt.Errorf("自检预期产生一个泄漏缺陷")
	}
	defectID := measured.CreatedDefectIDs[0]
	if _, err = post("/api/batches/"+batchID+"/defects/"+defectID+"/treatment", map[string]any{"actor_id": "technician-b", "version_id": "TREAT-1", "source_round_id": "ROUND-FAIL", "cause": "接头密封不足", "corrective_action": "重装密封并扭矩确认", "evidence_digest": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}); err != nil {
		return err
	}
	for i := 1; i <= 2; i++ {
		if _, err = post("/api/batches/"+batchID+"/measurements", map[string]any{"round_id": fmt.Sprintf("RETEST-%d", i), "tap_id": "T01", "round_kind": "retest", "source_round_id": "ROUND-FAIL", "defect_id": defectID, "treatment_version_id": "TREAT-1", "calibration_ref": "CAL-SC", "supply_pressure_pa": 1000, "steady_pressure_pa": 980, "decay_seconds": 1, "neighbor_responses": []map[string]any{}}); err != nil {
			return err
		}
	}
	if _, err = post("/api/batches/"+batchID+"/defects/"+defectID+"/close", map[string]any{}); err != nil {
		return err
	}
	preflight, err := get("/api/batches/" + batchID + "/submit/preflight")
	if err != nil {
		return err
	}
	if _, err = post("/api/batches/"+batchID+"/submit", map[string]any{"preflight_digest": preflight.FactDigest}); err != nil {
		return err
	}
	items := []map[string]any{{"item_id": "baseline", "status": "passed", "comment": "确认"}, {"item_id": "calibration", "status": "passed", "comment": "确认"}, {"item_id": "coverage", "status": "passed", "comment": "确认"}, {"item_id": "defects", "status": "passed", "comment": "确认"}, {"item_id": "evidence", "status": "passed", "comment": "确认"}}
	approved, err := post("/api/batches/"+batchID+"/review", map[string]any{"actor_id": "reviewer-c", "decision": "approved", "note": "独立复核确认全部验收依据完整", "items": items})
	if err != nil {
		return err
	}
	if approved.Certificate == nil || approved.Certificate.CanonicalDigest == "" {
		return fmt.Errorf("自检未取得资格证书")
	}
	verified, err := post("/api/certificates/verify", map[string]any{"batch_id": batchID, "digest": approved.Certificate.CanonicalDigest})
	if err != nil {
		return err
	}
	if !verified.Valid {
		return fmt.Errorf("资格证书摘要校验未通过")
	}
	return nil
}
