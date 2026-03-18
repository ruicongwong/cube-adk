package vault

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"cube-adk/pkg/core"
)

// FileVault persists memory entries as JSON files on disk.
type FileVault struct {
	root string
	mu   sync.RWMutex
}

func NewFileVault(root string) (*FileVault, error) {
	for _, dir := range []string{"working", "short", "long"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
			return nil, fmt.Errorf("filevault: create dir %s: %w", dir, err)
		}
	}
	return &FileVault{root: root}, nil
}

type fileEntry struct {
	Tag     string            `json:"tag"`
	Content string            `json:"content"`
	Meta    map[string]string `json:"meta,omitempty"`
	Time    time.Time         `json:"time"`
}

func (v *FileVault) scopeDir(s core.Scope) string {
	switch s {
	case core.ScopeShort:
		return filepath.Join(v.root, "short")
	case core.ScopeLong:
		return filepath.Join(v.root, "long")
	default:
		return filepath.Join(v.root, "working")
	}
}

func (v *FileVault) Append(_ context.Context, entry core.Entry) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	fe := fileEntry{
		Tag:     entry.Tag,
		Content: entry.Content,
		Meta:    entry.Meta,
		Time:    time.Now(),
	}
	data, err := json.Marshal(fe)
	if err != nil {
		return err
	}
	name := fmt.Sprintf("%d.json", time.Now().UnixNano())
	return os.WriteFile(filepath.Join(v.scopeDir(entry.Scope), name), data, 0o644)
}

func (v *FileVault) Recall(_ context.Context, query string, limit int) ([]core.Fragment, error) {
	v.mu.RLock()
	defer v.mu.RUnlock()

	q := strings.ToLower(query)
	var results []core.Fragment

	for _, scope := range []core.Scope{core.ScopeWorking, core.ScopeShort, core.ScopeLong} {
		dir := v.scopeDir(scope)
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, de := range entries {
			if !strings.HasSuffix(de.Name(), ".json") {
				continue
			}
			data, err := os.ReadFile(filepath.Join(dir, de.Name()))
			if err != nil {
				continue
			}
			var fe fileEntry
			if json.Unmarshal(data, &fe) != nil {
				continue
			}
			if strings.Contains(strings.ToLower(fe.Content), q) {
				results = append(results, core.Fragment{
					Content: fe.Content,
					Score:   1.0,
					Source:  fe.Tag,
				})
				if len(results) >= limit {
					return results, nil
				}
			}
		}
	}
	return results, nil
}

func (v *FileVault) Forget(_ context.Context, filter core.Filter) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	scopes := []core.Scope{core.ScopeWorking, core.ScopeShort, core.ScopeLong}
	if filter.Scope != nil {
		scopes = []core.Scope{*filter.Scope}
	}

	for _, scope := range scopes {
		dir := v.scopeDir(scope)
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, de := range entries {
			if !strings.HasSuffix(de.Name(), ".json") {
				continue
			}
			path := filepath.Join(dir, de.Name())
			if filter.Tag != "" {
				data, err := os.ReadFile(path)
				if err != nil {
					continue
				}
				var fe fileEntry
				if json.Unmarshal(data, &fe) != nil {
					continue
				}
				if fe.Tag != filter.Tag {
					continue
				}
			}
			os.Remove(path)
		}
	}
	return nil
}
