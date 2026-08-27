package application

import (
	"regexp"
	"sort"
	"strings"

	"pressure-tap-qualification/internal/domain"
	"pressure-tap-qualification/internal/evidence"
)

type CertificateLedgerEntry struct {
	Certificate *domain.ReleaseCertificate `json:"certificate"`
	ModelCode   string                     `json:"model_code"`
	BatchState  domain.BatchState          `json:"batch_state"`
}

type CertificateLedgerPage struct {
	Entries  []CertificateLedgerEntry `json:"entries"`
	Total    int                      `json:"total"`
	Page     int                      `json:"page"`
	PageSize int                      `json:"page_size"`
}

type BatchCertificateVerificationItem struct {
	BatchID      string                 `json:"batch_id"`
	Status       string                 `json:"status"`
	Message      string                 `json:"message"`
	Verification *evidence.Verification `json:"verification,omitempty"`
}

type BatchCertificateVerificationSummary struct {
	Passed   int `json:"passed"`
	Failed   int `json:"failed"`
	NotFound int `json:"not_found"`
}
type BatchCertificateVerification struct {
	Items   []BatchCertificateVerificationItem  `json:"items"`
	Summary BatchCertificateVerificationSummary `json:"summary"`
}

var digestPrefixPattern = regexp.MustCompile(`^[a-fA-F0-9]{1,64}$`)

func (s *Service) ListCertificateLedger(filter CertificateLedgerFilter) (CertificateLedgerPage, error) {
	if filter.Page == 0 {
		filter.Page = 1
	}
	if filter.PageSize == 0 {
		filter.PageSize = 20
	}
	if filter.Page < 1 || filter.PageSize < 1 || filter.PageSize > 100 {
		return CertificateLedgerPage{}, domain.NewError(domain.CodeInvalid, "page 必须大于 0，page_size 必须为 1 到 100")
	}
	if filter.IssuedFrom != nil && filter.IssuedTo != nil && filter.IssuedFrom.After(*filter.IssuedTo) {
		return CertificateLedgerPage{}, domain.NewError(domain.CodeInvalid, "签发时间起点不能晚于终点")
	}
	if filter.DigestPrefix != "" && !digestPrefixPattern.MatchString(filter.DigestPrefix) {
		return CertificateLedgerPage{}, domain.NewError(domain.CodeInvalid, "摘要前缀必须为 1 到 64 位十六进制值")
	}
	batches, err := s.repo.List()
	if err != nil {
		return CertificateLedgerPage{}, err
	}
	entries := []CertificateLedgerEntry{}
	for _, batch := range batches {
		if batch.State != domain.StateApproved {
			continue
		}
		cert, loadErr := s.repo.LoadCertificate(batch.BatchID)
		if loadErr != nil {
			return CertificateLedgerPage{}, loadErr
		}
		if filter.ModelCode != "" && batch.ModelCode != filter.ModelCode {
			continue
		}
		if filter.ReviewerID != "" && cert.ReviewerID != filter.ReviewerID {
			continue
		}
		if filter.IssuedFrom != nil && cert.IssuedAt.Before(*filter.IssuedFrom) {
			continue
		}
		if filter.IssuedTo != nil && cert.IssuedAt.After(*filter.IssuedTo) {
			continue
		}
		if filter.DigestPrefix != "" && !strings.HasPrefix(strings.ToLower(cert.CanonicalDigest), strings.ToLower(filter.DigestPrefix)) {
			continue
		}
		entries = append(entries, CertificateLedgerEntry{Certificate: cert, ModelCode: batch.ModelCode, BatchState: batch.State})
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Certificate.IssuedAt.Equal(entries[j].Certificate.IssuedAt) {
			return entries[i].Certificate.CertificateID < entries[j].Certificate.CertificateID
		}
		return entries[i].Certificate.IssuedAt.After(entries[j].Certificate.IssuedAt)
	})
	total := len(entries)
	start := (filter.Page - 1) * filter.PageSize
	if start > total {
		start = total
	}
	end := start + filter.PageSize
	if end > total {
		end = total
	}
	return CertificateLedgerPage{Entries: entries[start:end], Total: total, Page: filter.Page, PageSize: filter.PageSize}, nil
}

func (s *Service) VerifyCertificates(items []CertificateVerifyItem) (BatchCertificateVerification, error) {
	if len(items) == 0 || len(items) > 50 {
		return BatchCertificateVerification{}, domain.NewError(domain.CodeInvalid, "批量核验数量必须为 1 到 50")
	}
	seen := map[string]bool{}
	for _, item := range items {
		if strings.TrimSpace(item.BatchID) == "" {
			return BatchCertificateVerification{}, domain.NewError(domain.CodeInvalid, "batch_id 不能为空")
		}
		if seen[item.BatchID] {
			return BatchCertificateVerification{}, domain.NewError(domain.CodeInvalid, "batch_id %s 重复", item.BatchID)
		}
		seen[item.BatchID] = true
		if item.Digest != "" && !domain.ValidSHA256(item.Digest) {
			return BatchCertificateVerification{}, domain.NewError(domain.CodeInvalid, "批次 %s 的输入摘要非法", item.BatchID)
		}
	}
	out := BatchCertificateVerification{Items: make([]BatchCertificateVerificationItem, 0, len(items))}
	for _, item := range items {
		verification, err := s.evidence.Verify(item.BatchID, item.Digest)
		if err != nil {
			if domain.ErrorCodeOf(err) == domain.CodeNotFound {
				out.Items = append(out.Items, BatchCertificateVerificationItem{BatchID: item.BatchID, Status: "not_found", Message: err.Error()})
				out.Summary.NotFound++
			} else {
				out.Items = append(out.Items, BatchCertificateVerificationItem{BatchID: item.BatchID, Status: "failed", Message: err.Error()})
				out.Summary.Failed++
			}
			continue
		}
		status := "failed"
		if verification.Valid {
			status = "passed"
			out.Summary.Passed++
		} else {
			out.Summary.Failed++
		}
		copy := verification
		out.Items = append(out.Items, BatchCertificateVerificationItem{BatchID: item.BatchID, Status: status, Message: verification.Message, Verification: &copy})
	}
	return out, nil
}
