package documents

import (
	"context"
	"errors"
	"os"
	"path/filepath"
)

type DiskVersion struct {
	Known       bool
	Fingerprint Fingerprint
	Document    *Document
}

func ReadDisk(ctx context.Context, path, area string) (DiskVersion, error) {
	root, err := os.OpenRoot(area)
	if err != nil {
		return DiskVersion{}, err
	}
	defer root.Close()
	rel, err := filepath.Rel(area, path)
	if err != nil || !filepath.IsLocal(rel) {
		return DiskVersion{}, errors.New("file is outside the authorized folder")
	}
	fp, _, err := fingerprintAt(ctx, root, rel)
	if err != nil {
		return DiskVersion{}, err
	}
	disk := DiskVersion{Fingerprint: fp, Known: true}
	if !fp.Exists {
		return disk, nil
	}
	d, err := Open(ctx, path, area, Edit)
	if err != nil {
		return disk, err
	}
	if d.saved != fp {
		return disk, errors.New("file changed while reading; try Reload again")
	}
	disk.Document = d
	return disk, nil
}

func (d *Document) acceptDisk(other *Document) {
	position, scroll := d.Editor.CursorPos(), d.Editor.ScrollY()
	d.Buffer.Reset(other.Buffer.String())
	d.Buffer.SetLineEnding(other.Buffer.LineEnding())
	d.BOM = other.BOM
	d.Info = other.Info
	d.saved = other.saved
	d.state.saved = snapshotRevision(d.Buffer.Snapshot())
	d.revisionKnown = false
	d.Editor.GoTo(position)
	d.Editor.SetScrollY(scroll)
}

func (d *Document) AcceptExternal(other *Document, version uint64) bool {
	if d.Saving() || d.Dirty() || d.Buffer.Version() != version {
		return false
	}
	d.acceptDisk(other)
	return true
}
func (d *Document) ReloadDiscarding(other *Document, confirmedVersion uint64) bool {
	if d.Saving() || d.Buffer.Version() != confirmedVersion {
		return false
	}
	d.acceptDisk(other)
	return true
}
