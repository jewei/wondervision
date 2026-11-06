package documents

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"unicode/utf8"

	"github.com/egoist/mygo/ui/editor"
)

type RecoveryStore struct{ Dir string }

type RecoveryRecord struct {
	Path          string      `json:"path"`
	Area          string      `json:"area"`
	Base          Fingerprint `json:"base"`
	SavedRevision Revision    `json:"saved_revision"`
	BOM           bool        `json:"bom"`
	Newline       string      `json:"newline"`
	Size          int64       `json:"size"`
	BodyHash      [32]byte    `json:"body_hash"`
	Format        int         `json:"format"`
}

type RecoveryPlan struct {
	record   RecoveryRecord
	snapshot editor.Snapshot
}

func Sensitive(path string) bool {
	name := strings.ToLower(filepath.Base(path))
	if name == ".env" || strings.HasPrefix(name, ".env.") {
		return true
	}
	for _, ext := range []string{".key", ".pem", ".p12", ".pfx", ".crt", ".cer", ".der"} {
		if strings.HasSuffix(name, ext) {
			return true
		}
	}
	for _, part := range strings.FieldsFunc(strings.ToLower(path), func(r rune) bool { return r == '/' || r == '\\' }) {
		if part == ".ssh" || part == ".aws" || strings.HasPrefix(part, "id_rsa") || strings.HasPrefix(part, "id_ed25519") {
			return true
		}
		for _, word := range []string{"secret", "credential", "password", "private-key", "private_key", "token"} {
			if strings.Contains(part, word) {
				return true
			}
		}
	}
	return false
}

func (d *Document) RecoverySnapshot() (RecoveryPlan, error) {
	if !d.Dirty() {
		return RecoveryPlan{}, errors.New("clean documents have no recovery draft")
	}
	if Sensitive(d.Path) {
		return RecoveryPlan{}, errors.New("recovery is disabled for sensitive filenames")
	}
	s := d.Buffer.Snapshot()
	return RecoveryPlan{record: RecoveryRecord{Path: d.Path, Area: d.Area, Base: d.saved, SavedRevision: d.state.saved, BOM: d.BOM, Newline: d.Buffer.LineEnding(), Format: 1}, snapshot: s}, nil
}

func recoveryName(path string) string {
	sum := sha256.Sum256([]byte(path))
	return hex.EncodeToString(sum[:]) + ".draft"
}

func (store RecoveryStore) Write(ctx context.Context, plan RecoveryPlan) error {
	r := plan.record
	if r.Format != 1 || !filepath.IsAbs(r.Path) || !filepath.IsAbs(r.Area) {
		return errors.New("invalid recovery plan")
	}
	if Sensitive(r.Path) {
		return errors.New("recovery is disabled for sensitive filenames")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	h := sha256.New()
	var size int64
	if r.BOM {
		h.Write([]byte{0xef, 0xbb, 0xbf})
		size = 3
	}
	n, err := plan.snapshot.WriteTo(contextWriter{ctx, h})
	if err != nil {
		return err
	}
	r.Size = size + n
	if r.Size > MaxFileSize {
		return errors.New("recovery snapshot exceeds the 20 MiB limit")
	}
	copy(r.BodyHash[:], h.Sum(nil))
	data, err := json.Marshal(r)
	if err != nil {
		return err
	}
	if err = os.MkdirAll(store.Dir, 0700); err != nil {
		return err
	}
	info, err := os.Stat(store.Dir)
	if err != nil {
		return err
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0077 != 0 {
		return errors.New("recovery directory is not private")
	}
	root, err := os.OpenRoot(store.Dir)
	if err != nil {
		return err
	}
	defer root.Close()
	temp := ".recovery-" + rand.Text()
	f, err := root.OpenFile(temp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer root.Remove(temp)
	defer f.Close()
	w := contextWriter{ctx, f}
	if _, err = w.Write(append(data, '\n')); err != nil {
		return err
	}
	if r.BOM {
		if _, err = w.Write([]byte{0xef, 0xbb, 0xbf}); err != nil {
			return err
		}
	}
	if _, err = plan.snapshot.WriteTo(w); err != nil {
		return err
	}
	if err = f.Sync(); err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	return root.Rename(temp, recoveryName(r.Path))
}

func readRecoveryHeader(f *os.File) (RecoveryRecord, error) {
	line, err := bufio.NewReader(io.LimitReader(f, 64<<10)).ReadBytes('\n')
	if err != nil {
		return RecoveryRecord{}, err
	}
	var r RecoveryRecord
	if err = json.Unmarshal(line, &r); err != nil {
		return r, errors.New("invalid recovery header")
	}
	if r.Format != 1 || r.Size < 0 || r.Size > MaxFileSize || !filepath.IsAbs(r.Path) || !filepath.IsAbs(r.Area) || (r.Newline != "\n" && r.Newline != "\r\n") {
		return r, errors.New("unsupported recovery record")
	}
	rel, err := filepath.Rel(r.Area, r.Path)
	if err != nil || !filepath.IsLocal(rel) {
		return r, errors.New("recovery path is outside its original area")
	}
	_, err = f.Seek(int64(len(line)), io.SeekStart)
	return r, err
}

func (store RecoveryStore) List(ctx context.Context) ([]RecoveryRecord, error) {
	dir, err := os.Open(store.Dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer dir.Close()
	root, err := os.OpenRoot(store.Dir)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	records := make([]RecoveryRecord, 0)
	for {
		if err = ctx.Err(); err != nil {
			return records, err
		}
		entries, readErr := dir.ReadDir(32)
		for _, entry := range entries {
			if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 || !strings.HasSuffix(entry.Name(), ".draft") {
				continue
			}
			if len(records) >= 128 {
				return records, errors.New("additional recovery copies remain beyond the current display limit")
			}
			info, e := root.Lstat(entry.Name())
			if e != nil || !info.Mode().IsRegular() {
				continue
			}
			f, e := root.Open(entry.Name())
			if e != nil {
				continue
			}
			r, e := readRecoveryHeader(f)
			f.Close()
			if e != nil || recoveryName(r.Path) != entry.Name() {
				continue
			}
			if Sensitive(r.Path) || (r.Base.Exists && r.BodyHash == r.Base.Hash) {
				root.Remove(entry.Name())
				continue
			}
			records = append(records, r)
		}
		if readErr == io.EOF {
			return records, nil
		}
		if readErr != nil {
			return records, readErr
		}
	}
}

func (store RecoveryStore) Load(ctx context.Context, record RecoveryRecord) (*Document, error) {
	if Sensitive(record.Path) {
		return nil, errors.New("sensitive recovery records are refused")
	}
	root, err := os.OpenRoot(store.Dir)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	f, err := root.Open(recoveryName(record.Path))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	r, err := readRecoveryHeader(f)
	if err != nil {
		return nil, err
	}
	if r != record {
		return nil, errors.New("recovery copy changed; review the list again")
	}
	data, err := io.ReadAll(io.LimitReader(f, MaxFileSize+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) != r.Size || sha256.Sum256(data) != r.BodyHash {
		return nil, errors.New("recovery copy is incomplete or damaged")
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	if r.BOM {
		if len(data) < 3 || string(data[:3]) != "\xef\xbb\xbf" {
			return nil, errors.New("invalid recovery BOM")
		}
		data = data[3:]
	}
	if !utf8.Valid(data) || strings.ContainsRune(string(data), 0) {
		return nil, errors.New("invalid UTF-8 recovery copy")
	}
	d, err := Open(ctx, r.Path, r.Area, Edit)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		b := editor.NewBuffer("")
		b.SetLanguage(language(r.Path))
		d = &Document{Path: r.Path, Area: r.Area, Buffer: b, Editor: &editor.State{Buffer: b}}
	}
	d.Buffer.Set(string(data))
	d.Buffer.SetLineEnding(r.Newline)
	d.BOM = r.BOM
	d.saved = r.Base
	d.state.saved = r.SavedRevision
	d.revisionKnown = false
	d.Relocate(d.Path, d.Area, d.Info)
	d.Mode = Edit
	return d, nil
}

func (store RecoveryStore) Remove(ctx context.Context, path string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	root, err := os.OpenRoot(store.Dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer root.Close()
	err = root.Remove(recoveryName(path))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func (d *Document) RestoreDraft(other *Document) bool {
	if d.Saving() || d.Dirty() {
		return false
	}
	d.Buffer.Set(other.Buffer.String())
	d.Buffer.SetLineEnding(other.Buffer.LineEnding())
	d.BOM = other.BOM
	d.saved = other.saved
	d.state.saved = other.state.saved
	d.revisionKnown = false
	return true
}
