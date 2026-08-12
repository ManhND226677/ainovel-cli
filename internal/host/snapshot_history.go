package host

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type ManuscriptSnapshot struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	CreatedAt time.Time `json:"created_at"`
	SizeKB    int64     `json:"size_kb"`
	Files     int       `json:"files"`
}

const snapshotIndexFile = "index.json"

func (h *Host) snapshotsDir() string {
	return filepath.Join(h.store.Dir(), ".ainovel", "web-snapshots")
}

// ListManuscriptSnapshots returns a durable, newest-first history of archives
// created from the real output directory.
func (h *Host) ListManuscriptSnapshots() ([]ManuscriptSnapshot, error) {
	path := filepath.Join(h.snapshotsDir(), snapshotIndexFile)
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return []ManuscriptSnapshot{}, nil
	}
	if err != nil {
		return nil, err
	}
	var snapshots []ManuscriptSnapshot
	if err := json.Unmarshal(data, &snapshots); err != nil {
		return nil, fmt.Errorf("read snapshot index: %w", err)
	}
	sort.Slice(snapshots, func(i, j int) bool { return snapshots[i].CreatedAt.After(snapshots[j].CreatedAt) })
	return snapshots, nil
}

// CreateManuscriptSnapshot archives all user-visible output artifacts while
// excluding the snapshot store itself. It is safe to call while the engine is
// idle; callers must avoid invoking it during an exclusive output operation.
func (h *Host) CreateManuscriptSnapshot(title string) (ManuscriptSnapshot, error) {
	if h.store == nil {
		return ManuscriptSnapshot{}, fmt.Errorf("manuscript store is unavailable")
	}
	now := time.Now().UTC()
	cleanTitle := strings.TrimSpace(title)
	if cleanTitle == "" {
		cleanTitle = "Sao lưu thủ công"
	}
	seed := fmt.Sprintf("%s:%s", now.Format(time.RFC3339Nano), cleanTitle)
	sum := sha256.Sum256([]byte(seed))
	id := fmt.Sprintf("snap-%s", hex.EncodeToString(sum[:])[:12])
	if err := os.MkdirAll(h.snapshotsDir(), 0o755); err != nil {
		return ManuscriptSnapshot{}, err
	}
	archivePath := filepath.Join(h.snapshotsDir(), id+".zip")
	archive, err := os.Create(archivePath)
	if err != nil {
		return ManuscriptSnapshot{}, err
	}
	zw := zip.NewWriter(archive)
	files := 0
	root := h.store.Dir()
	walkErr := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		// Never archive archives; logs and lock data are runtime concerns rather
		// than manuscript facts.
		if rel == ".ainovel" || strings.HasPrefix(rel, ".ainovel"+string(filepath.Separator)) || strings.HasPrefix(rel, "logs"+string(filepath.Separator)) {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		defer in.Close()
		out, err := zw.Create(filepath.ToSlash(rel))
		if err != nil {
			return err
		}
		if _, err := io.Copy(out, in); err != nil {
			return err
		}
		files++
		return nil
	})
	closeErr := zw.Close()
	fileCloseErr := archive.Close()
	if walkErr != nil {
		_ = os.Remove(archivePath)
		return ManuscriptSnapshot{}, walkErr
	}
	if closeErr != nil || fileCloseErr != nil {
		_ = os.Remove(archivePath)
		if closeErr != nil {
			return ManuscriptSnapshot{}, closeErr
		}
		return ManuscriptSnapshot{}, fileCloseErr
	}
	info, err := os.Stat(archivePath)
	if err != nil {
		return ManuscriptSnapshot{}, err
	}
	snapshot := ManuscriptSnapshot{ID: id, Title: cleanTitle, CreatedAt: now, SizeKB: (info.Size() + 1023) / 1024, Files: files}
	list, err := h.ListManuscriptSnapshots()
	if err != nil {
		return ManuscriptSnapshot{}, err
	}
	list = append(list, snapshot)
	data, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return ManuscriptSnapshot{}, err
	}
	if err := os.WriteFile(filepath.Join(h.snapshotsDir(), snapshotIndexFile), data, 0o600); err != nil {
		return ManuscriptSnapshot{}, err
	}
	h.emitEvent(Event{Time: now, Category: "SYSTEM", Level: "info", Summary: "Đã tạo snapshot bản thảo: " + cleanTitle})
	return snapshot, nil
}

// RestoreManuscriptSnapshot overlays archived creative artifacts after taking a
// mandatory pre-restore snapshot. It rejects path traversal in archive entries.
func (h *Host) RestoreManuscriptSnapshot(id string) error {
	if !strings.HasPrefix(id, "snap-") || strings.ContainsAny(id, `\\/:`) {
		return fmt.Errorf("snapshot id is invalid")
	}
	if _, err := h.CreateManuscriptSnapshot("Tự động trước khôi phục " + id); err != nil {
		return fmt.Errorf("create pre-restore snapshot: %w", err)
	}
	archive, err := zip.OpenReader(filepath.Join(h.snapshotsDir(), id+".zip"))
	if err != nil {
		return fmt.Errorf("open snapshot: %w", err)
	}
	defer archive.Close()
	root := h.store.Dir()
	for _, entry := range archive.File {
		target := filepath.Join(root, filepath.FromSlash(entry.Name))
		rel, err := filepath.Rel(root, target)
		if err != nil || strings.HasPrefix(rel, "..") || filepath.IsAbs(rel) {
			return fmt.Errorf("invalid snapshot entry %q", entry.Name)
		}
		if entry.FileInfo().IsDir() {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		in, err := entry.Open()
		if err != nil {
			return err
		}
		out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, entry.Mode())
		if err != nil {
			in.Close()
			return err
		}
		_, copyErr := io.Copy(out, in)
		closeErr := out.Close()
		in.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
	}
	h.emitEvent(Event{Time: time.Now().UTC(), Category: "SYSTEM", Level: "warn", Summary: "Đã khôi phục snapshot bản thảo: " + id})
	return nil
}
