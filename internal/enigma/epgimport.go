package enigma

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

const EPGSourceName = "Dreaminator"

var ErrNoEPGImport = errors.New("плагин EPGImport не установлен на ресивере")

func xmlEscape(s string) string {
	var b strings.Builder
	xml.EscapeText(&b, []byte(s))
	return b.String()
}

// WriteEPGImport publishes our filtered XMLTV as an EPGImport source.
// mapping is XMLTV channel id -> service refs.
func WriteEPGImport(dir, epgURL string, mapping map[string][]string) error {
	if st, err := os.Stat(dir); err != nil || !st.IsDir() {
		return ErrNoEPGImport
	}
	ids := make([]string, 0, len(mapping))
	for id := range mapping {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	var b strings.Builder
	b.WriteString("<?xml version=\"1.0\" encoding=\"utf-8\"?>\n<channels>\n")
	for _, id := range ids {
		seen := map[string]bool{}
		for _, ref := range mapping[id] {
			if seen[ref] {
				continue
			}
			seen[ref] = true
			fmt.Fprintf(&b, "  <channel id=\"%s\">%s</channel>\n", xmlEscape(id), xmlEscape(ref))
		}
	}
	b.WriteString("</channels>\n")
	if err := WriteFileAtomic(filepath.Join(dir, "dreaminator.channels.xml"), []byte(b.String())); err != nil {
		return err
	}

	sources := fmt.Sprintf(`<?xml version="1.0" encoding="utf-8"?>
<sources>
  <sourcecat sourcecatname="Dreaminator">
    <source type="gen_xmltv" nocheck="1" channels="dreaminator.channels.xml">
      <description>%s</description>
      <url>%s</url>
    </source>
  </sourcecat>
</sources>
`, EPGSourceName, xmlEscape(epgURL))
	return WriteFileAtomic(filepath.Join(dir, "dreaminator.sources.xml"), []byte(sources))
}

// enableScript ticks our source in EPGImport's settings file. Depending on
// the plugin version that file is a pickle or JSON; we keep whatever format
// is already there.
const enableScript = `
import sys, pickle, json
p, name = sys.argv[1], sys.argv[2]
fmt = 'pickle'
try:
    d = pickle.load(open(p, 'rb'))
except Exception:
    try:
        d = json.load(open(p))
        fmt = 'json'
    except Exception:
        d = {}
if not isinstance(d, dict):
    d = {}
s = list(d.get('sources') or [])
if name not in s:
    s.append(name)
    d['sources'] = s
    if fmt == 'json':
        json.dump(d, open(p, 'w'))
    else:
        pickle.dump(d, open(p, 'wb'), 2)
`

func EnableEPGImportSource(ctx context.Context, dir string) error {
	conf := filepath.Join(dir, "epgimport.conf")
	for _, py := range []string{"python3", "python"} {
		bin, err := exec.LookPath(py)
		if err != nil {
			continue
		}
		if out, err := exec.CommandContext(ctx, bin, "-c", enableScript, conf, EPGSourceName).CombinedOutput(); err != nil {
			return fmt.Errorf("%v: %s", err, strings.TrimSpace(string(out)))
		}
		return nil
	}
	return errors.New("python не найден")
}
