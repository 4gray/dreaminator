// Package fetch downloads playlists and EPG files, transparently unpacking gzip.
package fetch

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

type Options struct {
	UserAgent   string
	InsecureTLS bool
	Timeout     time.Duration
}

func Client(o Options) *http.Client {
	tr := http.DefaultTransport.(*http.Transport).Clone()
	if o.InsecureTLS {
		tr.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}
	return &http.Client{Transport: tr, Timeout: o.Timeout}
}

type body struct {
	io.Reader
	closers []io.Closer
}

func (b *body) Close() error {
	var err error
	for _, c := range b.closers {
		if e := c.Close(); e != nil && err == nil {
			err = e
		}
	}
	return err
}

var xzMagic = []byte{0xFD, '7', 'z', 'X', 'Z', 0x00}

// Open downloads url (or opens a file:// path, handy for development) and
// returns a reader over its content, gunzipping it when needed.
func Open(ctx context.Context, o Options, url string) (io.ReadCloser, error) {
	var rc io.ReadCloser
	if path, ok := strings.CutPrefix(url, "file://"); ok {
		f, err := os.Open(path)
		if err != nil {
			return nil, err
		}
		rc = f
	} else {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, fmt.Errorf("неверная ссылка: %w", err)
		}
		if o.UserAgent != "" {
			req.Header.Set("User-Agent", o.UserAgent)
		}
		resp, err := Client(o).Do(req)
		if err != nil {
			return nil, fmt.Errorf("не удалось подключиться: %w", err)
		}
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return nil, fmt.Errorf("сервер ответил: %s", resp.Status)
		}
		rc = resp.Body
	}

	br := bufio.NewReaderSize(rc, 64<<10)
	if m, _ := br.Peek(2); len(m) == 2 && m[0] == 0x1f && m[1] == 0x8b {
		gz, err := gzip.NewReader(br)
		if err != nil {
			rc.Close()
			return nil, fmt.Errorf("архив повреждён: %w", err)
		}
		return &body{gz, []io.Closer{gz, rc}}, nil
	}
	if m, _ := br.Peek(len(xzMagic)); bytes.Equal(m, xzMagic) {
		rc.Close()
		return nil, errors.New("архивы .xz пока не поддерживаются — нужна ссылка на .xml или .gz")
	}
	return &body{br, []io.Closer{rc}}, nil
}
