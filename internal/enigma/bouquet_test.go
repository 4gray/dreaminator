package enigma

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestServiceRef(t *testing.T) {
	ref := ServiceRef(4097, Service{Key: "src/1", URL: "http://host:8080/live/u/p/1.ts", Name: "Канал: один"})
	parts := strings.Split(ref, ":")
	if len(parts) != 12 {
		t.Fatalf("ref must have 12 colon-separated fields, got %d: %s", len(parts), ref)
	}
	if parts[0] != "4097" || parts[5] != "1DE" {
		t.Errorf("unexpected ref %s", ref)
	}
	if parts[10] != "http%3a//host%3a8080/live/u/p/1.ts" || parts[11] != "Канал  один" {
		t.Errorf("url/name not escaped: %s", ref)
	}
	epg := EPGRef("src/1")
	if !strings.HasPrefix(epg, "1:0:1:"+parts[3]+":"+parts[4]+":1DE:") {
		t.Errorf("epg ref %s does not match service %s", epg, ref)
	}
}

func TestWriteBouquets(t *testing.T) {
	dir := t.TempDir()
	index := "#NAME User - bouquets (TV)\n" +
		"#SERVICE 1:7:1:0:0:0:0:0:0:0:FROM BOUQUET \"userbouquet.favourites.tv\" ORDER BY bouquet\n" +
		"#SERVICE 1:7:1:0:0:0:0:0:0:0:FROM BOUQUET \"userbouquet.dreaminator_old.tv\" ORDER BY bouquet\n"
	os.WriteFile(filepath.Join(dir, "bouquets.tv"), []byte(index), 0o644)
	os.WriteFile(filepath.Join(dir, "userbouquet.favourites.tv"), []byte("#NAME Спутник\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "userbouquet.dreaminator_old.tv"), []byte("#NAME old\n"), 0o644)

	bouquets := []Bouquet{
		{ID: "ab12", Name: "Избранное", Services: []Service{{Key: "s/1", URL: "http://h/1.ts", Name: "Один"}}},
		{ID: "empty", Name: "Пусто"},
	}
	if err := WriteBouquets(dir, 4097, bouquets, true); err != nil {
		t.Fatal(err)
	}

	got, _ := os.ReadFile(filepath.Join(dir, "bouquets.tv"))
	lines := strings.Split(strings.TrimSpace(string(got)), "\n")
	if len(lines) != 3 || !strings.Contains(lines[1], "userbouquet.dreaminator_ab12.tv") || !strings.Contains(lines[2], "favourites") {
		t.Errorf("bouquets.tv:\n%s", got)
	}
	if _, err := os.Stat(filepath.Join(dir, "userbouquet.dreaminator_old.tv")); !os.IsNotExist(err) {
		t.Error("stale bouquet must be removed")
	}
	if _, err := os.Stat(filepath.Join(dir, "userbouquet.favourites.tv")); err != nil {
		t.Error("foreign bouquet must be kept")
	}
	if backup, _ := os.ReadFile(filepath.Join(dir, "bouquets.tv"+backupExt)); string(backup) != index {
		t.Error("original bouquets.tv must be backed up")
	}
	b, _ := os.ReadFile(filepath.Join(dir, "userbouquet.dreaminator_ab12.tv"))
	if !strings.HasPrefix(string(b), "#NAME Избранное\n#SERVICE 4097:0:1:") || !strings.Contains(string(b), "#DESCRIPTION Один") {
		t.Errorf("bouquet file:\n%s", b)
	}

	// Moving our bouquets to the bottom must not duplicate lines.
	if err := WriteBouquets(dir, 4097, bouquets, false); err != nil {
		t.Fatal(err)
	}
	got, _ = os.ReadFile(filepath.Join(dir, "bouquets.tv"))
	lines = strings.Split(strings.TrimSpace(string(got)), "\n")
	if len(lines) != 3 || !strings.Contains(lines[2], "dreaminator_ab12") {
		t.Errorf("bottom placement:\n%s", got)
	}
}

func TestParseServiceRef(t *testing.T) {
	svc := Service{Key: "src/1", URL: "http://host:8080/iptv/T/1/index.m3u8#User-Agent=A b", Name: "Один"}
	sid, tsid := ServiceIDs("src/1")
	// Enigma2 prints a played reference in lower case hex and escapes ':'
	// and '%' in the path.
	played := strings.ToLower(ServiceRef(4097, svc)[:strings.Index(ServiceRef(4097, svc), "http")]) +
		"http%3a//host%3a8080/iptv/T/1/index.m3u8#User-Agent=A b:Один"
	for _, ref := range []string{ServiceRef(4097, svc), played} {
		p, ok := ParseServiceRef(ref)
		if !ok || !p.Ours || p.SID != sid || p.TSID != tsid || p.Stream != svc.URL {
			t.Errorf("%s: %+v %v", ref, p, ok)
		}
	}
	p, ok := ParseServiceRef("4097:0:1:0:0:0:0:0:0:0:http%3a//h/a%2520b.ts")
	if !ok || p.Ours || p.Stream != "http://h/a%20b.ts" {
		t.Errorf("foreign reference without a name: %+v %v", p, ok)
	}
	for _, ref := range []string{"1:0:19:283D:3FB:1:C00000:0:0:0:", "1:7:1:0:0:0:0:0:0:0:FROM BOUQUET \"x.tv\" ORDER BY bouquet", ""} {
		if _, ok := ParseServiceRef(ref); ok {
			t.Errorf("%q is not a stream", ref)
		}
	}
}
