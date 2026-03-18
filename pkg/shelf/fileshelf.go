package shelf

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"cube-adk/pkg/core"
)

// FileShelf persists artifacts as files on disk.
// Each artifact is stored as {id}.meta.json (metadata) + {id}.data (raw bytes).
type FileShelf struct {
	root string
	mu   sync.RWMutex
}

func NewFileShelf(root string) (*FileShelf, error) {
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, fmt.Errorf("fileshelf: create dir: %w", err)
	}
	return &FileShelf{root: root}, nil
}

type fileMeta struct {
	ID   string            `json:"id"`
	Name string            `json:"name"`
	MIME string            `json:"mime"`
	Meta map[string]string `json:"meta,omitempty"`
}

func (s *FileShelf) Store(_ context.Context, a core.ArtifactDetail) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	meta := fileMeta{ID: a.ID, Name: a.Name, MIME: a.MIME, Meta: a.Meta}
	metaData, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(s.root, a.ID+".meta.json"), metaData, 0o644); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(s.root, a.ID+".data"), a.Data, 0o644)
}

func (s *FileShelf) Fetch(_ context.Context, id string) (*core.ArtifactDetail, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	metaData, err := os.ReadFile(filepath.Join(s.root, id+".meta.json"))
	if err != nil {
		return nil, fmt.Errorf("shelf: artifact %q not found", id)
	}
	var meta fileMeta
	if err := json.Unmarshal(metaData, &meta); err != nil {
		return nil, err
	}
	data, err := os.ReadFile(filepath.Join(s.root, id+".data"))
	if err != nil {
		return nil, err
	}
	return &core.ArtifactDetail{
		ID: meta.ID, Name: meta.Name, MIME: meta.MIME, Data: data, Meta: meta.Meta,
	}, nil
}

func (s *FileShelf) List(_ context.Context, mime string, limit int) ([]core.ArtifactDetail, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	entries, err := os.ReadDir(s.root)
	if err != nil {
		return nil, err
	}
	var out []core.ArtifactDetail
	for _, de := range entries {
		if !strings.HasSuffix(de.Name(), ".meta.json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(s.root, de.Name()))
		if err != nil {
			continue
		}
		var meta fileMeta
		if json.Unmarshal(data, &meta) != nil {
			continue
		}
		if mime != "" && meta.MIME != mime {
			continue
		}
		out = append(out, core.ArtifactDetail{ID: meta.ID, Name: meta.Name, MIME: meta.MIME, Meta: meta.Meta})
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out, nil
}

func (s *FileShelf) Discard(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	os.Remove(filepath.Join(s.root, id+".meta.json"))
	os.Remove(filepath.Join(s.root, id+".data"))
	return nil
}
