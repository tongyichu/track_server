package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/tongyichu/track_server/internal/maparea"
)

const sourceBase = "https://geo.datav.aliyun.com/"

type sourceRecord struct {
	URL    string `json:"url"`
	SHA256 string `json:"sha256"`
}

type sourceClient struct {
	cache   string
	offline bool
	client  *http.Client
}

// Cached source files make a snapshot reproducible without calling DataV again.
// Use a new cache directory when collecting a new upstream snapshot.
func (s *sourceClient) get(path string) ([]byte, sourceRecord, error) {
	file := filepath.Join(s.cache, filepath.FromSlash(path))
	raw, err := os.ReadFile(file)
	if os.IsNotExist(err) && !s.offline {
		for attempt := 0; attempt < 3; attempt++ {
			raw, err = s.download(sourceBase + path)
			if err == nil {
				err = writeAtomic(file, raw)
				break
			}
			if strings.Contains(err.Error(), "HTTP 404") {
				break
			}
			time.Sleep(time.Duration(attempt+1) * time.Second)
		}
	}
	if err != nil {
		return nil, sourceRecord{}, fmt.Errorf("%s: %w", path, err)
	}
	if !json.Valid(raw) {
		return nil, sourceRecord{}, fmt.Errorf("%s: invalid JSON", path)
	}
	hash := sha256.Sum256(raw)
	return raw, sourceRecord{URL: sourceBase + path, SHA256: hex.EncodeToString(hash[:])}, nil
}

func (s *sourceClient) download(url string) ([]byte, error) {
	response, err := s.client.Get(url)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", response.StatusCode)
	}
	const maxBytes = 32 << 20
	raw, err := io.ReadAll(io.LimitReader(response.Body, maxBytes+1))
	if err == nil && len(raw) > maxBytes {
		return nil, fmt.Errorf("response exceeds %d bytes", maxBytes)
	}
	if err == nil && !json.Valid(raw) {
		return nil, fmt.Errorf("response is not JSON")
	}
	return raw, err
}

type feature struct {
	Properties struct {
		ADCode json.RawMessage `json:"adcode"`
		Name   string          `json:"name"`
	} `json:"properties"`
	Geometry *maparea.Geometry `json:"geometry"`
}

func (f feature) code() int {
	code, _ := strconv.Atoi(strings.Trim(string(f.Properties.ADCode), `"`))
	return code // Non-administrative features such as 100000_JD are excluded.
}

type boundary struct {
	feature
	source sourceRecord
}

func (s *sourceClient) boundaries(code int, full bool) (map[int]boundary, error) {
	suffix := ".json"
	if full {
		suffix = "_full.json"
	}
	var failures []string
	for _, version := range []string{"areas_v3", "areas"} {
		path := fmt.Sprintf("%s/bound/%d%s", version, code, suffix)
		raw, source, err := s.get(path)
		if err != nil {
			failures = append(failures, err.Error())
			continue
		}
		var collection struct {
			Type     string    `json:"type"`
			Features []feature `json:"features"`
		}
		if err := json.Unmarshal(raw, &collection); err != nil {
			return nil, fmt.Errorf("decode %s: %w", path, err)
		}
		if collection.Type != "FeatureCollection" || len(collection.Features) == 0 {
			return nil, fmt.Errorf("%s: empty or invalid FeatureCollection", path)
		}
		result := make(map[int]boundary)
		for _, f := range collection.Features {
			if code := f.code(); code > 0 && f.Geometry != nil {
				if _, exists := result[code]; exists {
					return nil, fmt.Errorf("%s: duplicate adcode %d", path, code)
				}
				result[code] = boundary{feature: f, source: source}
			}
		}
		return result, nil
	}
	return nil, fmt.Errorf("%s", strings.Join(failures, "; "))
}

func writeAtomic(path string, raw []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".maparea-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(raw); err != nil {
		f.Close()
		return err
	}
	if err = f.Chmod(0644); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}
