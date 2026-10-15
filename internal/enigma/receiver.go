package enigma

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

var ErrReceiverConflict = errors.New("букет изменился на ресивере; откройте его заново, прежде чем сохранять")

// ErrReceiverFileLeft means Delete unregistered the bouquet but could not
// remove its file. The bouquet is gone from the TV; the file is an orphan.
var ErrReceiverFileLeft = errors.New("букет убран из списка, но его файл не удалён")

// removeFile is replaced in tests to simulate a failed unlink.
var removeFile = os.Remove
var bouquetReference = regexp.MustCompile(`FROM BOUQUET "([^"]+)"`)

// ReceiverStore edits only registered, non-managed TV bouquets. Callers must
// serialize writes with the same mutex used by WriteBouquets.
type ReceiverStore struct{ Dir, BackupDir string }
type ReceiverChannel struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Kind      string `json:"kind"`
	Reference string `json:"-"`
	raw       string
}
type ReceiverBouquet struct {
	ID       string            `json:"id"`
	Name     string            `json:"name"`
	Count    int               `json:"count"`
	Revision string            `json:"revision,omitempty"`
	Channels []ReceiverChannel `json:"channels,omitempty"`
	CanUndo  bool              `json:"canUndo"`
	Error    string            `json:"error,omitempty"`
	header   string
	data     []byte
	mode     os.FileMode
}
type ReceiverPatch struct {
	Revision string   `json:"revision"`
	Name     string   `json:"name"`
	Order    []string `json:"order"`
}
type receiverUndo struct {
	Before []byte `json:"before"`
	After  string `json:"after"`
}

func revision(data []byte) string { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }
func nativeFilename(id string) bool {
	return strings.HasPrefix(id, "userbouquet.") && strings.HasSuffix(id, ".tv") &&
		!strings.HasPrefix(id, filePrefix) && filepath.Base(id) == id &&
		!strings.ContainsAny(id, "\\\"\x00\r\n")
}
func readRegular(path string) ([]byte, error) {
	return readRegularLimit(path, 16<<20)
}
func readRegularLimit(path string, limit int64) ([]byte, error) {
	fi, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !fi.Mode().IsRegular() {
		return nil, errors.New("ожидался обычный файл букета")
	}
	if fi.Size() > limit {
		return nil, errors.New("файл букета слишком большой")
	}
	return os.ReadFile(path)
}
func (s ReceiverStore) registered() ([]string, error) {
	data, err := readRegular(filepath.Join(s.Dir, indexFile))
	if os.IsNotExist(err) {
		return []string{}, nil
	}
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	ids := []string{}
	for _, line := range strings.Split(string(data), "\n") {
		if !strings.HasPrefix(line, "#SERVICE") {
			continue
		}
		m := bouquetReference.FindStringSubmatch(line)
		if len(m) == 2 && nativeFilename(m[1]) && !seen[m[1]] {
			ids = append(ids, m[1])
			seen[m[1]] = true
		}
	}
	return ids, nil
}
func (s ReceiverStore) read(id string) (ReceiverBouquet, error) {
	if !nativeFilename(id) {
		return ReceiverBouquet{}, errors.New("недопустимый букет ресивера")
	}
	ids, err := s.registered()
	if err != nil {
		return ReceiverBouquet{}, err
	}
	found := false
	for _, listed := range ids {
		if id == listed {
			found = true
			break
		}
	}
	if !found {
		return ReceiverBouquet{}, errors.New("букет больше не зарегистрирован на ресивере")
	}
	data, err := readRegular(filepath.Join(s.Dir, id))
	if err != nil {
		return ReceiverBouquet{}, err
	}
	b := parseReceiver(id, data)
	fi, err := os.Lstat(filepath.Join(s.Dir, id))
	if err != nil {
		return ReceiverBouquet{}, err
	}
	b.mode = fi.Mode().Perm()
	return b, nil
}
func parseReceiver(id string, data []byte) ReceiverBouquet {
	b := ReceiverBouquet{ID: id, Name: id, Revision: revision(data), Channels: []ReceiverChannel{}, data: data}
	for _, line := range strings.SplitAfter(string(data), "\n") {
		t := strings.TrimRight(line, "\r\n")
		if strings.HasPrefix(t, "#SERVICE ") || strings.HasPrefix(t, "#SERVICE:") {
			ref := strings.TrimSpace(t[len("#SERVICE")+1:])
			parts := strings.SplitN(ref, ":", 12)
			kind := "channel"
			if len(parts) > 1 && parts[1] == "64" {
				kind = "marker"
			}
			if strings.Contains(ref, "FROM BOUQUET") {
				kind = "group"
			}
			name := "Канал " + strconv.Itoa(len(b.Channels)+1)
			if len(parts) == 12 && parts[11] != "" {
				name = parts[11]
			}
			b.Channels = append(b.Channels, ReceiverChannel{ID: strconv.Itoa(len(b.Channels)), Name: name, Kind: kind, Reference: ref, raw: line})
		} else if len(b.Channels) > 0 {
			c := &b.Channels[len(b.Channels)-1]
			c.raw += line
			if strings.HasPrefix(t, "#DESCRIPTION ") {
				c.Name = strings.TrimPrefix(t, "#DESCRIPTION ")
			}
		} else {
			b.header += line
			if strings.HasPrefix(t, "#NAME ") {
				b.Name = strings.TrimPrefix(t, "#NAME ")
			}
		}
	}
	b.Count = len(b.Channels)
	return b
}
func (s ReceiverStore) undoPath(id, version string) string {
	return filepath.Join(s.BackupDir, revision([]byte(id))+"-"+version+".json")
}
func (s ReceiverStore) undo(id, version string) (receiverUndo, error) {
	var u receiverUndo
	// A 16 MiB bouquet expands to about 22 MiB as base64 in JSON.
	data, err := readRegularLimit(s.undoPath(id, version), 24<<20)
	if err == nil {
		err = json.Unmarshal(data, &u)
	}
	return u, err
}
func (s ReceiverStore) Read(id string) (ReceiverBouquet, error) {
	b, err := s.read(id)
	if err != nil {
		return b, err
	}
	u, err := s.undo(id, b.Revision)
	b.CanUndo = err == nil && u.After == b.Revision
	return b, nil
}
func (s ReceiverStore) List() ([]ReceiverBouquet, error) {
	ids, err := s.registered()
	if err != nil {
		return nil, err
	}
	out := make([]ReceiverBouquet, 0, len(ids))
	for _, id := range ids {
		b, err := s.Read(id)
		if err != nil {
			b = ReceiverBouquet{ID: id, Name: id, Error: "Файл букета недоступен"}
		}
		b.Channels = nil
		out = append(out, b)
	}
	return out, nil
}
func receiverName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 512 || strings.ContainsFunc(name, unicode.IsControl) {
		return "", errors.New("укажите название без переносов строк (до 512 байт)")
	}
	return name, nil
}
func receiverData(b ReceiverBouquet, name string, order []string) ([]byte, error) {
	name, err := receiverName(name)
	if err != nil {
		return nil, err
	}
	if order == nil {
		return nil, errors.New("не передан порядок каналов")
	}
	var out strings.Builder
	named := false
	for _, line := range strings.SplitAfter(b.header, "\n") {
		if strings.HasPrefix(line, "#NAME ") {
			if !named {
				out.WriteString("#NAME " + name + "\n")
				named = true
			}
		} else {
			out.WriteString(line)
		}
	}
	if !named {
		out.WriteString("#NAME " + name + "\n")
	}
	seen := map[string]bool{}
	for _, id := range order {
		i, err := strconv.Atoi(id)
		if err != nil || i < 0 || i >= len(b.Channels) || b.Channels[i].ID != id || seen[id] {
			return nil, errors.New("список содержит неизвестный или повторный канал")
		}
		seen[id] = true
		raw := b.Channels[i].raw
		out.WriteString(raw)
		if !strings.HasSuffix(raw, "\n") {
			out.WriteByte('\n')
		}
	}
	return []byte(out.String()), nil
}

// privateAtomic uses a fresh temporary file instead of a predictable path; undo
// snapshots contain provider URLs and must not inherit public file permissions.
func privateAtomic(path string, data []byte, mode os.FileMode) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".dreaminator-edit-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err = f.Chmod(mode); err == nil {
		_, err = f.Write(data)
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}
func (s ReceiverStore) unchanged(id, expected string) error {
	b, err := s.read(id)
	if err != nil {
		return err
	}
	if expected == "" || b.Revision != expected {
		return ErrReceiverConflict
	}
	return nil
}
func (s ReceiverStore) Save(id string, p ReceiverPatch) (ReceiverBouquet, error) {
	b, err := s.read(id)
	if err != nil {
		return b, err
	}
	if p.Revision == "" || b.Revision != p.Revision {
		return b, ErrReceiverConflict
	}
	data, err := receiverData(b, p.Name, p.Order)
	if err != nil {
		return b, err
	}
	if string(data) == string(b.data) {
		return s.Read(id)
	}
	if err := os.MkdirAll(s.BackupDir, 0o700); err != nil {
		return b, err
	}
	backup, err := json.Marshal(receiverUndo{Before: b.data, After: revision(data)})
	if err != nil {
		return b, err
	}
	if err := privateAtomic(s.undoPath(id, revision(data)), backup, 0o600); err != nil {
		return b, err
	}
	if err := s.unchanged(id, p.Revision); err != nil {
		return b, err
	}
	if err := privateAtomic(filepath.Join(s.Dir, id), data, b.mode); err != nil {
		return b, err
	}
	// Only prune previous revisions after the bouquet write succeeds. A failed
	// save must leave the last successful undo available.
	s.pruneUndo(id, revision(data))
	return s.Read(id)
}
func (s ReceiverStore) pruneUndo(id, keep string) {
	paths, _ := filepath.Glob(filepath.Join(s.BackupDir, revision([]byte(id))+"-*.json"))
	for _, path := range paths {
		if keep == "" || path != s.undoPath(id, keep) {
			_ = os.Remove(path)
		}
	}
}
func (s ReceiverStore) Undo(id, expected string) (ReceiverBouquet, error) {
	b, err := s.read(id)
	if err != nil {
		return b, err
	}
	if expected == "" || b.Revision != expected {
		return b, ErrReceiverConflict
	}
	u, err := s.undo(id, b.Revision)
	if err != nil {
		return b, errors.New("нет сохранения для отмены")
	}
	if u.After != expected {
		return b, ErrReceiverConflict
	}
	if err := s.unchanged(id, expected); err != nil {
		return b, err
	}
	if err := privateAtomic(filepath.Join(s.Dir, id), u.Before, b.mode); err != nil {
		return b, err
	}
	s.pruneUndo(id, "")
	return s.Read(id)
}
func entryIDs(b ReceiverBouquet) []string {
	ids := make([]string, 0, len(b.Channels))
	for _, c := range b.Channels {
		ids = append(ids, c.ID)
	}
	return ids
}

// Rename changes only the #NAME line. Entry ids stay the same, so a draft
// opened before the rename still matches the new revision.
func (s ReceiverStore) Rename(id, expected, name string) (ReceiverBouquet, error) {
	b, err := s.read(id)
	if err != nil {
		return b, err
	}
	if expected == "" || b.Revision != expected {
		return b, ErrReceiverConflict
	}
	return s.Save(id, ReceiverPatch{Revision: expected, Name: name, Order: entryIDs(b)})
}

// Delete unregisters a bouquet from bouquets.tv and removes its file, like
// removing it from the remote. Both are first copied to BackupDir so a wrong
// deletion can be restored by hand; the copy is keyed by revision so deleting
// a recreated bouquet keeps the earlier one. A registered entry whose file is
// already gone is only unregistered.
func (s ReceiverStore) Delete(id, expected string) error {
	b, err := s.read(id)
	gone := errors.Is(err, fs.ErrNotExist)
	if err != nil && !gone {
		return err
	}
	if !gone && (expected == "" || b.Revision != expected) {
		return ErrReceiverConflict
	}
	indexPath := filepath.Join(s.Dir, indexFile)
	index, err := readRegular(indexPath)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(s.BackupDir, 0o700); err != nil {
		return err
	}
	if err := privateAtomic(filepath.Join(s.BackupDir, "index-"+revision(index)), index, 0o600); err != nil {
		return err
	}
	if !gone {
		if err := privateAtomic(filepath.Join(s.BackupDir, "deleted-"+id+"-"+b.Revision[:12]), b.data, 0o600); err != nil {
			return err
		}
		if err := s.unchanged(id, expected); err != nil {
			return err
		}
	}
	current, err := readRegular(indexPath)
	if err != nil {
		return err
	}
	if string(current) != string(index) {
		return ErrReceiverConflict
	}
	if err := privateAtomic(indexPath, unregister(index, id), 0o644); err != nil {
		return err
	}
	s.pruneUndo(id, "")
	if gone {
		return nil
	}
	if err := removeFile(filepath.Join(s.Dir, id)); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("%w: %v", ErrReceiverFileLeft, err)
	}
	return nil
}

// unregister drops every index entry for id together with its #DESCRIPTION
// lines and leaves all other bytes as they are.
func unregister(index []byte, id string) []byte {
	var out strings.Builder
	drop := false
	for _, line := range strings.SplitAfter(string(index), "\n") {
		t := strings.TrimRight(line, "\r\n")
		switch {
		case strings.HasPrefix(t, "#SERVICE"):
			m := bouquetReference.FindStringSubmatch(t)
			drop = len(m) == 2 && m[1] == id
		case strings.HasPrefix(t, "#DESCRIPTION"), strings.TrimSpace(t) == "":
			// A description belongs to the #SERVICE line above it.
		default:
			drop = false
		}
		if !drop {
			out.WriteString(line)
		}
	}
	return []byte(out.String())
}

func (s ReceiverStore) Copy(id, expected, name string) (ReceiverBouquet, error) {
	b, err := s.read(id)
	if err != nil {
		return b, err
	}
	if expected == "" || b.Revision != expected {
		return b, ErrReceiverConflict
	}
	data, err := receiverData(b, name, entryIDs(b))
	if err != nil {
		return b, err
	}
	indexPath := filepath.Join(s.Dir, indexFile)
	index, err := readRegular(indexPath)
	if err != nil {
		return b, err
	}
	token := make([]byte, 8)
	if _, err := rand.Read(token); err != nil {
		return b, err
	}
	copyID := "userbouquet.receiver_copy_" + hex.EncodeToString(token) + ".tv"
	// Reserve an unguessable new file without ever overwriting an existing one.
	f, err := os.OpenFile(filepath.Join(s.Dir, copyID), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return b, err
	}
	_, err = f.Write(data)
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	committed := false
	defer func() {
		if !committed {
			os.Remove(filepath.Join(s.Dir, copyID))
		}
	}()
	if err != nil {
		return b, err
	}
	if err := os.MkdirAll(s.BackupDir, 0o700); err != nil {
		return b, err
	}
	if err := privateAtomic(filepath.Join(s.BackupDir, "index-"+revision(index)), index, 0o600); err != nil {
		return b, err
	}
	if err := s.unchanged(id, expected); err != nil {
		return b, err
	}
	current, err := readRegular(indexPath)
	if err != nil {
		return b, err
	}
	if string(current) != string(index) {
		return b, ErrReceiverConflict
	}
	next := string(index)
	if !strings.HasSuffix(next, "\n") {
		next += "\n"
	}
	next += fmt.Sprintf("#SERVICE 1:7:1:0:0:0:0:0:0:0:FROM BOUQUET \"%s\" ORDER BY bouquet\n", copyID)
	if err := privateAtomic(indexPath, []byte(next), 0o644); err != nil {
		return b, err
	}
	committed = true
	return s.Read(copyID)
}
