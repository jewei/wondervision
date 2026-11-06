package app

import (
	"bytes"
	"context"
	"errors"
	"image"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"
	"github.com/jewei/wondervision/internal/documents"
	"github.com/jewei/wondervision/internal/workspace"
)

type previewState struct {
	doc     *documents.Document
	version uint64
	source  string
	loaded  bool
	options *markdown.Options
	cancel  context.CancelFunc
}

func baseName(path string) string { return filepath.Base(path) }

func (a *App) releasePreview() {
	if a.preview != nil {
		a.preview.cancel()
		a.preview = nil
	}
}

func (a *App) preparePreview(d *documents.Document) *previewState {
	if a.preview == nil || a.preview.doc != d {
		a.releasePreview()
		ctx, cancel := context.WithCancel(a.ctx)
		options := &markdown.Options{Anchor: d.Anchor}
		a.preview = &previewState{doc: d, options: options, cancel: cancel}
		slots := make(chan struct{}, 2)
		area, path := d.Area, d.Path
		options.LoadImage = func(raw string) (ui.ImageSource, error) {
			select {
			case slots <- struct{}{}:
			case <-ctx.Done():
				return nil, ctx.Err()
			}
			defer func() { <-slots }()
			return loadLocalImage(ctx, area, path, raw)
		}
		options.OnLink = func(raw string) { a.followLink(d, raw) }
	}
	p := a.preview
	if v := d.Buffer.Version(); !p.loaded || p.version != v {
		snapshot := d.Buffer.Snapshot()
		p.source, p.version = snapshot.String(), snapshot.Version()
		p.loaded = true
	}
	return p
}

func (a *App) markdownView(c *ui.Context, d *documents.Document) {
	p := a.preparePreview(d)
	ui.Scroll(c.Key("preview")).TrackScroll(a.scroll[d]).Grow(1).FillWidth().Padding(20, 28).Children(func() {
		markdown.View(c, p.source, p.options).FillWidth()
	})
}

func (a *App) setAnchor(d *documents.Document, anchor string) {
	if anchor != "" && d.Markdown {
		d.Anchor = anchor
		if p := a.preview; p != nil && p.doc == d {
			p.options.Anchor = anchor
			p.options.AnchorVersion++
		}
	}
}

func (a *App) followLink(d *documents.Document, raw string) {
	u, err := url.Parse(raw)
	if err != nil {
		a.notify(err)
		return
	}
	if u.Scheme != "" {
		if ui.SafeToOpen(raw) {
			go func() {
				if err := mygo.Shell.OpenExternal(raw); err != nil {
					a.win.Update(func() { a.notify(err) })
				}
			}()
		}
		return
	}
	if u.Path == "" && u.Host == "" && u.RawQuery == "" {
		a.setAnchor(d, u.Fragment)
		return
	}
	// Resolve symlinks on the I/O worker, not in the link's UI callback.
	id, epoch := a.openID, a.epoch
	area, document := d.Area, d.Path
	a.enqueue(func() func() {
		path, anchor, err := workspace.ResolveLocal(area, document, raw)
		return func() {
			if a.active() != d || id != a.openID || epoch != a.epoch {
				return
			}
			if err != nil {
				a.notify(err)
				return
			}
			ext := strings.ToLower(filepath.Ext(path))
			if ext != ".md" && ext != ".markdown" {
				a.notify(errors.New("preview links open Markdown files only"))
				return
			}
			a.openFile(path, area, anchor)
		}
	})
}

func loadLocalImage(ctx context.Context, area, document, raw string) (ui.ImageSource, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	path, _, err := workspace.ResolveLocal(area, document, raw)
	if err != nil {
		return nil, err
	}
	before, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !before.Mode().IsRegular() {
		return nil, errors.New("only regular image files can be loaded")
	}
	f, err := workspace.OpenWithin(area, path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > documents.MaxFileSize {
		return nil, errors.New("unsupported image size or type")
	}
	data, err := io.ReadAll(io.LimitReader(f, documents.MaxFileSize+1))
	if err != nil {
		return nil, err
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	if len(data) > documents.MaxFileSize {
		return nil, errors.New("image exceeds loading limit")
	}
	if strings.EqualFold(filepath.Ext(path), ".svg") {
		return ui.ParseSVG(data)
	}
	config, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	if config.Width <= 0 || config.Height <= 0 || int64(config.Width)*int64(config.Height) > 16_000_000 {
		return nil, errors.New("image dimensions exceed the preview limit")
	}
	return ui.DecodeBitmap(data)
}
