package store

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"pressure-tap-qualification/internal/domain"
)

func eventDigest(e domain.AuditEvent) (string, error) {
	e.Digest = ""
	data, err := json.Marshal(e)
	if err != nil {
		return "", err
	}
	return digest(data), nil
}

func appendEvent(path string, event domain.AuditEvent) error {
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0640)
	if err != nil {
		return err
	}
	defer f.Close()
	frame := make([]byte, 0, len(data)+10)
	frame = append(frame, []byte(fmt.Sprintf("%08x ", len(data)))...)
	frame = append(frame, data...)
	frame = append(frame, '\n')
	if _, err = f.Write(frame); err != nil {
		return err
	}
	return f.Sync()
}

func loadEvents(path string) ([]domain.AuditEvent, error) {
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	r := bufio.NewReader(f)
	events := []domain.AuditEvent{}
	previous := ""
	var sequence uint64 = 1
	for {
		header, err := r.ReadString(' ')
		if err == io.EOF && header == "" {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("事件帧头损坏: %w", err)
		}
		header = strings.TrimSuffix(header, " ")
		if len(header) != 8 {
			return nil, fmt.Errorf("事件长度帧无效")
		}
		n, err := strconv.ParseUint(header, 16, 32)
		if err != nil {
			return nil, fmt.Errorf("事件长度无效: %w", err)
		}
		data := make([]byte, n)
		if _, err = io.ReadFull(r, data); err != nil {
			return nil, fmt.Errorf("事件帧被截断: %w", err)
		}
		ending, err := r.ReadByte()
		if err != nil || ending != '\n' {
			return nil, fmt.Errorf("事件帧终止符无效")
		}
		var event domain.AuditEvent
		if err = json.Unmarshal(data, &event); err != nil {
			return nil, fmt.Errorf("事件 JSON 损坏: %w", err)
		}
		if event.Sequence != sequence {
			return nil, fmt.Errorf("事件序号不连续: 期望 %d，得到 %d", sequence, event.Sequence)
		}
		if event.PreviousDigest != previous {
			return nil, fmt.Errorf("事件前序摘要不连续")
		}
		got := event.Digest
		calculated, err := eventDigest(event)
		if err != nil {
			return nil, err
		}
		if got != calculated {
			return nil, fmt.Errorf("事件摘要校验失败")
		}
		events = append(events, event)
		previous = event.Digest
		sequence++
	}
	return events, nil
}

func loadValidEventPrefix(path string) ([]domain.AuditEvent, int64, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, 0, nil
	}
	if err != nil {
		return nil, 0, err
	}
	events := []domain.AuditEvent{}
	previous := ""
	offset := 0
	for offset < len(data) {
		frameStart := offset
		if len(data)-offset < 9 || data[offset+8] != ' ' {
			return events, int64(frameStart), nil
		}
		n, parseErr := strconv.ParseUint(string(data[offset:offset+8]), 16, 32)
		if parseErr != nil {
			return events, int64(frameStart), nil
		}
		offset += 9
		end := offset + int(n)
		if end >= len(data) || data[end] != '\n' {
			return events, int64(frameStart), nil
		}
		payload := data[offset:end]
		var event domain.AuditEvent
		if json.Unmarshal(payload, &event) != nil {
			return events, int64(frameStart), nil
		}
		if event.Sequence != uint64(len(events)+1) || event.PreviousDigest != previous {
			return events, int64(frameStart), nil
		}
		got := event.Digest
		calculated, digestErr := eventDigest(event)
		if digestErr != nil {
			return nil, 0, digestErr
		}
		if !bytes.Equal([]byte(got), []byte(calculated)) {
			return events, int64(frameStart), nil
		}
		events = append(events, event)
		previous = event.Digest
		offset = end + 1
	}
	return events, int64(offset), nil
}
