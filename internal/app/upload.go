package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"dreaminator/internal/fetch"
	"dreaminator/internal/playlist"
)

// MaxUpload caps an uploaded playlist; live-TV playlists are far smaller.
const MaxUpload = 50 << 20

func (a *App) uploadPath(sourceID string) string {
	return filepath.Join(a.cfg.DataDir, "uploads", sourceID+".m3u")
}

// stageUpload stores r next to the source's playlist file and checks that it
// parses, so a broken upload never replaces a working one.
func (a *App) stageUpload(id string, r io.Reader) (string, error) {
	path := a.uploadPath(id)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	tmp := path + ".new"
	f, err := os.Create(tmp)
	if err != nil {
		return "", err
	}
	n, err := io.Copy(f, io.LimitReader(r, MaxUpload+1))
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	switch {
	case err != nil:
		os.Remove(tmp)
		return "", fmt.Errorf("не удалось сохранить файл: %w", err)
	case n == 0:
		os.Remove(tmp)
		return "", errors.New("файл пустой")
	case n > MaxUpload:
		os.Remove(tmp)
		return "", fmt.Errorf("файл больше %d МБ", MaxUpload>>20)
	}

	rc, err := fetch.Open(context.Background(), fetch.Options{}, "file://"+tmp)
	if err == nil {
		_, err = playlist.ParseM3U(rc)
		rc.Close()
	}
	if err != nil {
		os.Remove(tmp)
		return "", err
	}
	return tmp, nil
}

// AddUploadedSource creates a source from an uploaded M3U file. Unlike a
// link it is never re-downloaded: the nightly refresh just re-reads it.
func (a *App) AddUploadedSource(name, epgURL, filename string, r io.Reader) (*Source, error) {
	if name = strings.TrimSpace(name); name == "" {
		name = strings.TrimSuffix(filepath.Base(filename), filepath.Ext(filename))
	}
	in := SourceInput{Type: "file", Name: name, EPGURL: epgURL}
	if err := in.validate(); err != nil {
		return nil, err
	}
	id := newID()
	tmp, err := a.stageUpload(id, r)
	if err != nil {
		return nil, err
	}
	if err := os.Rename(tmp, a.uploadPath(id)); err != nil {
		return nil, err
	}

	a.mu.Lock()
	s := &Source{ID: id}
	applyInput(s, in)
	a.st.Sources = append(a.st.Sources, s)
	a.save()
	a.mu.Unlock()

	ctx, cancel := context.WithTimeout(a.bg, time.Minute)
	defer cancel()
	err = a.RefreshSource(ctx, id)

	a.mu.Lock()
	defer a.mu.Unlock()
	cp := *s
	return &cp, err
}

// ReplaceUpload swaps in a new file for an uploaded source.
func (a *App) ReplaceUpload(id string, r io.Reader) (*Source, error) {
	a.mu.Lock()
	s := a.source(id)
	var typ string
	if s != nil {
		typ = s.Type
	}
	a.mu.Unlock()
	if s == nil {
		return nil, ErrNotFound
	}
	if typ != "file" {
		return nil, errors.New("этот плейлист подключён по ссылке, а не файлом")
	}

	tmp, err := a.stageUpload(id, r)
	if err != nil {
		return nil, err
	}
	if err := os.Rename(tmp, a.uploadPath(id)); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(a.bg, time.Minute)
	defer cancel()
	err = a.RefreshSource(ctx, id)

	a.mu.Lock()
	defer a.mu.Unlock()
	if s = a.source(id); s == nil {
		return nil, ErrNotFound
	}
	cp := *s
	return &cp, err
}
