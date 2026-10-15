// Package enigma writes Enigma2 bouquets and EPGImport configuration and
// talks to the receiver through OpenWebif.
package enigma

import (
	"fmt"
	"hash/fnv"
	"os"
	"path/filepath"
	"strings"
)

const (
	filePrefix = "userbouquet.dreaminator_"
	indexFile  = "bouquets.tv"
	backupExt  = ".dreaminator-backup"
	// onid marks our fake DVB triplets so they never clash with satellite
	// services; together with sid/tsid it is what EPGImport maps XMLTV onto.
	onid = 0x1DE
)

type Service struct {
	Key  string // stable "<source>/<channel>" key; drives the service ids
	URL  string
	Name string
}

type Bouquet struct {
	ID       string
	Name     string
	Services []Service
}

func ids(key string) (sid, tsid uint16) {
	h := fnv.New32a()
	h.Write([]byte(key))
	v := h.Sum32()
	sid, tsid = uint16(v>>16), uint16(v)
	if sid == 0 {
		sid = 1
	}
	return sid, tsid
}

func cleanName(s string) string {
	return strings.TrimSpace(strings.NewReplacer(":", " ", "\n", " ", "\r", " ").Replace(s))
}

// ServiceRef builds an IPTV service reference such as
// 4097:0:1:1A2B:3C4D:1DE:0:0:0:0:http%3a//host/stream.ts:Name
func ServiceRef(serviceType int, s Service) string {
	sid, tsid := ids(s.Key)
	return fmt.Sprintf("%d:0:1:%X:%X:%X:0:0:0:0:%s:%s",
		serviceType, sid, tsid, onid, strings.ReplaceAll(s.URL, ":", "%3a"), cleanName(s.Name))
}

// EPGRef is the reference EPGImport uses to attach programmes to a service.
func EPGRef(key string) string {
	sid, tsid := ids(key)
	return fmt.Sprintf("1:0:1:%X:%X:%X:0:0:0:0:", sid, tsid, onid)
}

func safeID(id string) string {
	return strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '_' {
			return r
		}
		return -1
	}, strings.ToLower(id))
}

// WriteBouquets replaces our userbouquet files in dir and registers them in
// bouquets.tv. Bouquets that are not ours are never touched.
func WriteBouquets(dir string, serviceType int, bouquets []Bouquet, top bool) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	keep := map[string]bool{}
	var refs []string
	for _, b := range bouquets {
		if len(b.Services) == 0 {
			continue
		}
		name := filePrefix + safeID(b.ID) + ".tv"
		var sb strings.Builder
		fmt.Fprintf(&sb, "#NAME %s\n", cleanName(b.Name))
		for _, s := range b.Services {
			fmt.Fprintf(&sb, "#SERVICE %s\n#DESCRIPTION %s\n", ServiceRef(serviceType, s), cleanName(s.Name))
		}
		if err := WriteFileAtomic(filepath.Join(dir, name), []byte(sb.String())); err != nil {
			return err
		}
		keep[name] = true
		refs = append(refs, fmt.Sprintf(`#SERVICE 1:7:1:0:0:0:0:0:0:0:FROM BOUQUET "%s" ORDER BY bouquet`, name))
	}

	stale, _ := filepath.Glob(filepath.Join(dir, filePrefix+"*.tv"))
	for _, p := range stale {
		if !keep[filepath.Base(p)] {
			os.Remove(p)
		}
	}
	return updateIndex(filepath.Join(dir, indexFile), refs, top)
}

func updateIndex(path string, refs []string, top bool) error {
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if err == nil {
		// Keep the receiver's original list once, before we ever change it.
		if _, statErr := os.Stat(path + backupExt); os.IsNotExist(statErr) {
			if err := os.WriteFile(path+backupExt, data, 0o644); err != nil {
				return err
			}
		}
	}

	header := ""
	var rest []string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimRight(line, "\r")
		t := strings.TrimSpace(line)
		switch {
		case t == "":
		case header == "" && strings.HasPrefix(t, "#NAME"):
			header = t
		case strings.Contains(t, `"`+filePrefix):
		default:
			rest = append(rest, line)
		}
	}
	if header == "" {
		header = "#NAME User - bouquets (TV)"
	}
	out := []string{header}
	if top {
		out = append(append(out, refs...), rest...)
	} else {
		out = append(append(out, rest...), refs...)
	}
	return WriteFileAtomic(path, []byte(strings.Join(out, "\n")+"\n"))
}

// WriteFileAtomic replaces path with data so that a power cut leaves either
// the old or the new file. The data reaches the flash before the rename:
// UBIFS may otherwise commit the rename first, and a reboot then leaves a
// cut-off file (it happened to state.json on a DM520).
func WriteFileAtomic(path string, data []byte) error {
	return replaceFile(path, data, "")
}

// WriteFileAtomicBackup is WriteFileAtomic that keeps the previous version
// as path+".bak" (one rename, no extra writes) for readers to fall back on.
func WriteFileAtomicBackup(path string, data []byte) error {
	return replaceFile(path, data, path+".bak")
}

func replaceFile(path string, data []byte, backup string) error {
	tmp := path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	_, err = f.Write(data)
	if err == nil {
		err = f.Sync()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(tmp)
		return err
	}
	if backup != "" {
		if err := os.Rename(path, backup); err != nil && !os.IsNotExist(err) {
			os.Remove(tmp)
			return err
		}
	}
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	// Make the renames durable too.
	if dir, err := os.Open(filepath.Dir(path)); err == nil {
		dir.Sync()
		dir.Close()
	}
	return nil
}
