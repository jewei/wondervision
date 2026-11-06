package documents

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo/ui/editor"
)

type Fingerprint struct {
	Hash   [32]byte `json:"hash"`
	Exists bool     `json:"exists"`
}

var ErrConflict = errors.New("the disk file changed; reload, compare, save as, or explicitly overwrite")
var ErrDestinationExists = errors.New("the destination already exists")

type SavePlan struct {
	token    uint64
	source   Revision
	snapshot editor.Snapshot
	path     string
	area     string
	bom      bool
	expected Fingerprint
	saveAs   bool
}

type SaveResult struct {
	token       uint64
	source      Revision
	Path        string
	Fingerprint Fingerprint
	Info        os.FileInfo
	Warning     error
}

type contextWriter struct {
	ctx context.Context
	w   io.Writer
}

func (w contextWriter) Write(p []byte) (int, error) {
	if err := w.ctx.Err(); err != nil {
		return 0, err
	}
	return w.w.Write(p)
}

func snapshotRevision(s editor.Snapshot) Revision {
	h := sha256.New()
	_, _ = s.WriteTo(h)
	var revision Revision
	copy(revision[:], h.Sum(nil))
	return revision
}

func (d *Document) revision() Revision {
	s := d.Buffer.Snapshot()
	if !d.revisionKnown || d.revisionVersion != s.Version() {
		d.revisionValue = snapshotRevision(s)
		d.revisionVersion = s.Version()
		d.revisionKnown = true
	}
	return d.revisionValue
}

func (d *Document) Dirty() bool                   { return d.state.dirty(d.revision()) }
func (d *Document) Saving() bool                  { return d.state.pending != nil }
func (d *Document) SavedFingerprint() Fingerprint { return d.saved }

func (d *Document) PrepareSave() (SavePlan, error) { return d.plan(d.Path, d.saved, false) }
func (d *Document) PrepareSaveAs(path string) (SavePlan, error) {
	return d.plan(path, Fingerprint{}, true)
}
func (d *Document) PrepareOverwrite(expected Fingerprint) (SavePlan, error) {
	return d.plan(d.Path, expected, false)
}

func (d *Document) plan(path string, expected Fingerprint, saveAs bool) (SavePlan, error) {
	s := d.Buffer.Snapshot()
	source := snapshotRevision(s)
	token, err := d.state.begin(source)
	if err != nil {
		return SavePlan{}, err
	}
	return SavePlan{token: token, source: source, snapshot: s, path: path, area: d.Area, bom: d.BOM, expected: expected, saveAs: saveAs}, nil
}

// CompleteSave runs on the UI thread. The baseline is the saved snapshot,
// including when a later edit arrived while the write was in flight.
func (d *Document) CompleteSave(result SaveResult, err error) bool {
	if !d.state.complete(result.token, err == nil) {
		return false
	}
	if err != nil {
		return true
	}
	d.Path = result.Path
	d.Info = result.Info
	d.saved = result.Fingerprint
	if d.revision() == result.source {
		d.Buffer.MarkSaved()
	}
	return true
}

func (p SavePlan) Write(ctx context.Context) (SaveResult, error) { return p.write(ctx, nil) }

func (p SavePlan) write(ctx context.Context, beforeReplace func()) (result SaveResult, err error) {
	result = SaveResult{token: p.token, source: p.source}
	if err = ctx.Err(); err != nil {
		return
	}
	absolute, e := filepath.Abs(p.path)
	if e != nil {
		err = e
		return
	}

	parent := filepath.Dir(absolute)
	area := p.area
	if p.saveAs {
		area = parent
	}
	scope, e := os.OpenRoot(area)
	if e != nil {
		err = e
		return
	}
	defer scope.Close()
	rel, e := filepath.Rel(area, parent)
	if e != nil || !filepath.IsLocal(rel) {
		err = errors.New("save destination is outside the authorized folder")
		return
	}
	root, e := scope.OpenRoot(rel)
	if e != nil {
		err = e
		return
	}
	defer root.Close()

	name := filepath.Base(absolute)
	if name == "." || name == string(filepath.Separator) {
		err = errors.New("choose a file destination")
		return
	}
	result.Path = filepath.Join(parent, name)
	before, info, e := fingerprintAt(ctx, root, name)
	if e != nil {
		err = e
		return
	}
	if before != p.expected {
		if p.saveAs && before.Exists {
			err = ErrDestinationExists
		} else {
			err = ErrConflict
		}
		return
	}
	mode := os.FileMode(0600)
	if before.Exists {
		if info.Mode().Perm()&0222 == 0 {
			err = fmt.Errorf("file is read-only: %w", os.ErrPermission)
			return
		}
		writable, e := root.OpenFile(name, os.O_WRONLY, 0)
		if e != nil {
			err = e
			return
		}
		writable.Close()
		mode = info.Mode().Perm()
	}
	temp := ".wondervision-save-" + rand.Text()
	f, e := root.OpenFile(temp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if e != nil {
		err = e
		return
	}
	defer root.Remove(temp)
	defer f.Close()
	h := sha256.New()
	w := contextWriter{ctx, io.MultiWriter(f, h)}
	if p.bom {
		if _, e = w.Write([]byte{0xef, 0xbb, 0xbf}); e != nil {
			err = e
			return
		}
	}
	if _, e = p.snapshot.WriteTo(w); e != nil {
		err = e
		return
	}
	if e = f.Chmod(mode); e != nil {
		err = e
		return
	}
	if e = f.Sync(); e != nil {
		err = e
		return
	}
	if e = f.Close(); e != nil {
		err = e
		return
	}
	if e = ctx.Err(); e != nil {
		err = e
		return
	}
	now, nowInfo, e := fingerprintAt(ctx, root, name)
	if e != nil {
		err = e
		return
	}
	if now != before || (before.Exists && nowInfo.Mode().Perm() != info.Mode().Perm()) {
		err = ErrConflict
		return
	}
	if beforeReplace != nil {
		beforeReplace()
	}
	if before.Exists {
		e = root.Rename(temp, name)
	} else {
		e = root.Link(temp, name)
	}
	if e != nil {
		err = fmt.Errorf("replace file: %w", e)
		return
	}
	result.Fingerprint.Exists = true
	copy(result.Fingerprint.Hash[:], h.Sum(nil))
	result.Info, e = root.Stat(name)
	if e != nil {
		result.Warning = fmt.Errorf("saved, but could not read file metadata: %w", e)
	}
	return result, nil
}

func fingerprintAt(ctx context.Context, root *os.Root, name string) (Fingerprint, os.FileInfo, error) {
	info, err := root.Lstat(name)
	if errors.Is(err, os.ErrNotExist) {
		return Fingerprint{}, nil, nil
	}
	if err != nil {
		return Fingerprint{}, nil, err
	}
	if !info.Mode().IsRegular() {
		return Fingerprint{}, nil, errors.New("destination must be a regular file, without a final symlink")
	}
	f, err := root.Open(name)
	if err != nil {
		return Fingerprint{}, nil, err
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.CopyBuffer(contextWriter{ctx, h}, io.LimitReader(f, MaxFileSize+1), make([]byte, 64<<10))
	if err != nil {
		return Fingerprint{}, nil, err
	}
	if n > MaxFileSize {
		return Fingerprint{}, nil, errors.New("disk file exceeds the supported size")
	}
	var result Fingerprint
	result.Exists = true
	copy(result.Hash[:], h.Sum(nil))
	return result, info, nil
}

func Destination(path string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	parent, err := filepath.EvalSymlinks(filepath.Dir(absolute))
	if err != nil {
		return "", err
	}
	return filepath.Join(parent, filepath.Base(absolute)), nil
}

func InspectDestination(ctx context.Context, path string) (Fingerprint, error) {
	root, err := os.OpenRoot(filepath.Dir(path))
	if err != nil {
		return Fingerprint{}, err
	}
	defer root.Close()
	fp, _, err := fingerprintAt(ctx, root, filepath.Base(path))
	return fp, err
}

func (d *Document) PrepareSaveAsReplace(path string, expected Fingerprint) (SavePlan, error) {
	return d.plan(path, expected, true)
}
func (d *Document) CancelSave(plan SavePlan) { d.state.complete(plan.token, false) }

func (d *Document) Relocate(path, area string, info os.FileInfo) {
	d.Path = path
	d.Area = area
	d.Info = info
	d.Buffer.SetLanguage(language(path))
	ext := strings.ToLower(filepath.Ext(path))
	wasMarkdown := d.Markdown
	d.Markdown = ext == ".md" || ext == ".markdown"
	d.Large = d.Markdown && d.Buffer.Len() > LargeMarkdown
	if !d.Markdown || !wasMarkdown {
		d.Mode = Edit
	}
}
