package enigma

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const receiverID = "userbouquet.provider_Россия.tv"
const dvbBlock = "#SERVICE 1:0:1:123:456:789:0:0:0:0:\n#DESCRIPTION Satellite\n#CUSTOM preserve this\n"
const iptvBlock = "#SERVICE 5002:0:1:42:0:0:0:0:0:0:http%3a//provider/secret.ts:IPTV\n#DESCRIPTION IPTV\n"
const markerBlock = "#SERVICE 1:64:0:0:0:0:0:0:0:0:\n#DESCRIPTION Group\n"

func receiverFixture(t *testing.T) (ReceiverStore, string) {
	t.Helper()
	dir := t.TempDir()
	s := ReceiverStore{Dir: dir, BackupDir: filepath.Join(t.TempDir(), "undo")}
	data := "#NAME Original\n#CUSTOM header\n" + dvbBlock + markerBlock + iptvBlock + dvbBlock
	put := func(name, value string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(value), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	put(receiverID, data)
	put("bouquets.tv", "#NAME TV\n#SERVICE 1:7:1:0:0:0:0:0:0:0:FROM BOUQUET \""+receiverID+"\" ORDER BY bouquet\n#SERVICE 1:7:1:0:0:0:0:0:0:0:FROM BOUQUET \"userbouquet.missing.tv\" ORDER BY bouquet\n#SERVICE 1:7:1:0:0:0:0:0:0:0:FROM BOUQUET \"userbouquet.dreaminator_abc.tv\" ORDER BY bouquet\n")
	return s, data
}

func TestReceiverReadAndSavePreservesServiceBlocks(t *testing.T) {
	s, original := receiverFixture(t)
	listed, err := s.List()
	if err != nil || len(listed) != 2 {
		t.Fatalf("list: %+v %v", listed, err)
	}
	if listed[1].Error == "" {
		t.Fatal("missing file was not explained")
	}
	b, err := s.Read(receiverID)
	if err != nil {
		t.Fatal(err)
	}
	if b.Name != "Original" || len(b.Channels) != 4 || b.Channels[1].Kind != "marker" {
		t.Fatalf("read: %+v", b)
	}
	if b.Channels[0].ID == b.Channels[3].ID {
		t.Fatal("duplicate refs need distinct entry ids")
	}
	patch := ReceiverPatch{Revision: b.Revision, Name: "Edited", Order: []string{b.Channels[2].ID, b.Channels[0].ID, b.Channels[1].ID}}
	updated, err := s.Save(receiverID, patch)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(filepath.Join(s.Dir, receiverID))
	want := "#NAME Edited\n#CUSTOM header\n" + iptvBlock + dvbBlock + markerBlock
	if string(got) != want {
		t.Fatalf("service data changed:\n%s", got)
	}
	if !updated.CanUndo {
		t.Fatal("no undo after save")
	}
	if _, err := s.Save(receiverID, patch); !errors.Is(err, ErrReceiverConflict) {
		t.Fatalf("stale save: %v", err)
	}
	if _, err := s.Undo(receiverID, updated.Revision); err != nil {
		t.Fatal(err)
	}
	got, _ = os.ReadFile(filepath.Join(s.Dir, receiverID))
	if string(got) != original {
		t.Fatal("undo did not restore exact bytes")
	}
}

func TestReceiverRejectsInvalidDraftAndExternalChanges(t *testing.T) {
	s, original := receiverFixture(t)
	b, _ := s.Read(receiverID)
	for _, order := range [][]string{{"unknown"}, {b.Channels[0].ID, b.Channels[0].ID}, nil} {
		_, err := s.Save(receiverID, ReceiverPatch{Revision: b.Revision, Name: "Invalid", Order: order})
		if err == nil {
			t.Fatalf("accepted invalid order %v", order)
		}
	}
	for _, name := range []string{"", "name\n#SERVICE injected", "name\x00"} {
		_, err := s.Save(receiverID, ReceiverPatch{Revision: b.Revision, Name: name, Order: []string{}})
		if err == nil {
			t.Fatalf("accepted invalid name %q", name)
		}
	}
	updated, err := s.Save(receiverID, ReceiverPatch{Revision: b.Revision, Name: "Empty", Order: []string{}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(s.Dir, receiverID), []byte(original+"#EXTERNAL change\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Undo(receiverID, updated.Revision); !errors.Is(err, ErrReceiverConflict) {
		t.Fatalf("external undo: %v", err)
	}
	current, _ := s.Read(receiverID)
	if current.CanUndo {
		t.Fatal("offered stale undo")
	}
}

func TestReceiverOnlyEditsRegisteredRegularFiles(t *testing.T) {
	s, _ := receiverFixture(t)
	for _, id := range []string{"../bouquets.tv", "bouquets.tv", "userbouquet.unlisted.tv", "userbouquet.dreaminator_abc.tv"} {
		if _, err := s.Read(id); err == nil {
			t.Fatalf("accepted %q", id)
		}
	}
	outside := filepath.Join(t.TempDir(), "outside")
	os.WriteFile(outside, []byte("private"), 0o600)
	os.Remove(filepath.Join(s.Dir, receiverID))
	os.Symlink(outside, filepath.Join(s.Dir, receiverID))
	if _, err := s.Read(receiverID); err == nil {
		t.Fatal("followed symlink")
	}
}

func TestReceiverCopySurvivesManagedApply(t *testing.T) {
	s, original := receiverFixture(t)
	index, _ := os.ReadFile(filepath.Join(s.Dir, indexFile))
	b, _ := s.Read(receiverID)
	copy, err := s.Copy(receiverID, b.Revision, "My copy")
	if err != nil {
		t.Fatal(err)
	}
	if copy.ID == receiverID || strings.HasPrefix(copy.ID, filePrefix) {
		t.Fatal("copy namespace collides")
	}
	after, _ := os.ReadFile(filepath.Join(s.Dir, indexFile))
	if !strings.HasPrefix(string(after), string(index)) {
		t.Fatal("copy replaced original index entries")
	}
	if err := WriteBouquets(s.Dir, 4097, []Bouquet{{ID: "managed", Name: "New", Services: []Service{{Key: "1", URL: "http://example/test", Name: "Test"}}}}, true); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Read(copy.ID); err != nil {
		t.Fatalf("managed apply removed copy: %v", err)
	}
	got, _ := os.ReadFile(filepath.Join(s.Dir, receiverID))
	if string(got) != original {
		t.Fatal("copy changed original bouquet")
	}
}

func TestReceiverFailedWriteKeepsPreviousUndo(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("permission failure requires an unprivileged user")
	}
	s, original := receiverFixture(t)
	b, _ := s.Read(receiverID)
	p := ReceiverPatch{Revision: b.Revision, Name: "Saved", Order: []string{b.Channels[0].ID}}
	b, err := s.Save(receiverID, p)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(s.Dir, 0500); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(s.Dir, 0700)
	p.Revision = b.Revision
	p.Name = "Fails"
	if _, err := s.Save(receiverID, p); err == nil {
		t.Fatal("write unexpectedly succeeded")
	}
	os.Chmod(s.Dir, 0700)
	b, err = s.Read(receiverID)
	if err != nil || !b.CanUndo {
		t.Fatalf("failed write lost prior undo: %+v %v", b, err)
	}
	if _, err := s.Undo(receiverID, b.Revision); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(filepath.Join(s.Dir, receiverID))
	if string(got) != original {
		t.Fatal("previous undo did not restore original")
	}
}

func TestReceiverLargeSnapshotCanUndo(t *testing.T) {
	s, _ := receiverFixture(t)
	original := "#NAME Large\n" + iptvBlock + "#COMMENT " + strings.Repeat("x", 13<<20) + "\n"
	if err := os.WriteFile(filepath.Join(s.Dir, receiverID), []byte(original), 0600); err != nil {
		t.Fatal(err)
	}
	b, err := s.Read(receiverID)
	if err != nil {
		t.Fatal(err)
	}
	b, err = s.Save(receiverID, ReceiverPatch{Revision: b.Revision, Name: "Renamed", Order: []string{b.Channels[0].ID}})
	if err != nil || !b.CanUndo {
		t.Fatalf("large snapshot unreadable: canUndo=%v err=%v", b.CanUndo, err)
	}
	if _, err := s.Undo(receiverID, b.Revision); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(filepath.Join(s.Dir, receiverID))
	if string(got) != original {
		t.Fatal("large undo did not restore original")
	}
}

func TestReceiverSavePreservesPrivateFilePermissions(t *testing.T) {
	s, _ := receiverFixture(t)
	path := filepath.Join(s.Dir, receiverID)
	os.Chmod(path, 0600)
	b, _ := s.Read(receiverID)
	b, err := s.Save(receiverID, ReceiverPatch{Revision: b.Revision, Name: "Private", Order: []string{b.Channels[0].ID}})
	if err != nil {
		t.Fatal(err)
	}
	fi, _ := os.Stat(path)
	if fi.Mode().Perm() != 0600 {
		t.Fatalf("save changed permissions to %o", fi.Mode().Perm())
	}
	if _, err := s.Undo(receiverID, b.Revision); err != nil {
		t.Fatal(err)
	}
	fi, _ = os.Stat(path)
	if fi.Mode().Perm() != 0600 {
		t.Fatalf("undo changed permissions to %o", fi.Mode().Perm())
	}
}

func TestReceiverRenameKeepsEntryIDs(t *testing.T) {
	s, original := receiverFixture(t)
	b, _ := s.Read(receiverID)
	renamed, err := s.Rename(receiverID, b.Revision, "Renamed")
	if err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(filepath.Join(s.Dir, receiverID))
	if string(got) != strings.Replace(original, "#NAME Original", "#NAME Renamed", 1) {
		t.Fatalf("rename changed more than the name:\n%s", got)
	}
	if renamed.Name != "Renamed" || !renamed.CanUndo || len(renamed.Channels) != len(b.Channels) {
		t.Fatalf("renamed: %+v", renamed)
	}
	for i, c := range renamed.Channels {
		if c.ID != b.Channels[i].ID {
			t.Fatal("rename changed entry ids, an open draft would no longer match")
		}
	}
	if _, err := s.Rename(receiverID, b.Revision, "Stale"); !errors.Is(err, ErrReceiverConflict) {
		t.Fatalf("stale rename: %v", err)
	}
	if _, err := s.Rename(receiverID, renamed.Revision, "bad\nname"); err == nil {
		t.Fatal("accepted a name with a line break")
	}
}

func TestReceiverDeleteUnregistersAndRemovesFile(t *testing.T) {
	s, original := receiverFixture(t)
	indexPath := filepath.Join(s.Dir, indexFile)
	index, _ := os.ReadFile(indexPath)
	b, _ := s.Read(receiverID)
	if err := s.Delete(receiverID, "stale"); !errors.Is(err, ErrReceiverConflict) {
		t.Fatalf("stale delete: %v", err)
	}
	if err := s.Delete(receiverID, b.Revision); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(s.Dir, receiverID)); !os.IsNotExist(err) {
		t.Fatalf("bouquet file left behind: %v", err)
	}
	after, _ := os.ReadFile(indexPath)
	want := strings.Replace(string(index), "#SERVICE 1:7:1:0:0:0:0:0:0:0:FROM BOUQUET \""+receiverID+"\" ORDER BY bouquet\n", "", 1)
	if string(after) != want {
		t.Fatalf("index changed beyond the deleted entry:\n%s", after)
	}
	listed, _ := s.List()
	if len(listed) != 1 || listed[0].ID == receiverID {
		t.Fatalf("deleted bouquet still listed: %+v", listed)
	}
	saved, err := os.ReadFile(filepath.Join(s.BackupDir, "deleted-"+receiverID+"-"+b.Revision[:12]))
	if err != nil || string(saved) != original {
		t.Fatalf("no recoverable copy of the deleted bouquet: %v", err)
	}
	if err := s.Delete(receiverID, b.Revision); err == nil {
		t.Fatal("deleted an unregistered bouquet twice")
	}
}

func TestReceiverDeleteMissingFileOnlyUnregisters(t *testing.T) {
	s, _ := receiverFixture(t)
	if err := s.Delete("userbouquet.missing.tv", ""); err != nil {
		t.Fatal(err)
	}
	index, _ := os.ReadFile(filepath.Join(s.Dir, indexFile))
	if strings.Contains(string(index), "userbouquet.missing.tv") || !strings.Contains(string(index), receiverID) || !strings.Contains(string(index), "userbouquet.dreaminator_abc.tv") {
		t.Fatalf("index: %s", index)
	}
}

func TestReceiverDeleteKeepsOtherEntriesAndDescriptions(t *testing.T) {
	s, _ := receiverFixture(t)
	indexPath := filepath.Join(s.Dir, indexFile)
	other := "userbouquet.other.tv"
	os.WriteFile(filepath.Join(s.Dir, other), []byte("#NAME Other\n"), 0o644)
	index := "#NAME TV\r\n" +
		"#SERVICE 1:7:1:0:0:0:0:0:0:0:FROM BOUQUET \"" + receiverID + "\" ORDER BY bouquet\r\n#DESCRIPTION Provider\r\n" +
		"#SERVICE 1:7:1:0:0:0:0:0:0:0:FROM BOUQUET \"" + other + "\" ORDER BY bouquet\r\n#DESCRIPTION Other\r\n" +
		"#SERVICE 1:7:1:0:0:0:0:0:0:0:FROM BOUQUET \"" + receiverID + "\" ORDER BY bouquet\r\n"
	os.WriteFile(indexPath, []byte(index), 0o644)
	b, _ := s.Read(receiverID)
	if err := s.Delete(receiverID, b.Revision); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(indexPath)
	want := "#NAME TV\r\n#SERVICE 1:7:1:0:0:0:0:0:0:0:FROM BOUQUET \"" + other + "\" ORDER BY bouquet\r\n#DESCRIPTION Other\r\n"
	if string(got) != want {
		t.Fatalf("index:\n%q", got)
	}
	for _, id := range []string{"bouquets.tv", "../" + other, "userbouquet.dreaminator_abc.tv", "userbouquet.unlisted.tv"} {
		if err := s.Delete(id, ""); err == nil {
			t.Fatalf("deleted %q", id)
		}
	}
}

func TestReceiverDeleteGuardsAndBackups(t *testing.T) {
	s, original := receiverFixture(t)
	indexPath := filepath.Join(s.Dir, indexFile)
	index, _ := os.ReadFile(indexPath)
	if err := s.Delete(receiverID, ""); !errors.Is(err, ErrReceiverConflict) {
		t.Fatalf("delete without revision: %v", err)
	}
	b, _ := s.Read(receiverID)
	b, err := s.Save(receiverID, ReceiverPatch{Revision: b.Revision, Name: "Edited", Order: entryIDs(b)})
	if err != nil || !b.CanUndo {
		t.Fatalf("save: %v", err)
	}
	if err := s.Delete(receiverID, b.Revision); err != nil {
		t.Fatal(err)
	}
	if saved, err := os.ReadFile(filepath.Join(s.BackupDir, "index-"+revision(index))); err != nil || string(saved) != string(index) {
		t.Fatalf("index backup: %v", err)
	}
	if left, _ := filepath.Glob(filepath.Join(s.BackupDir, revision([]byte(receiverID))+"-*.json")); len(left) != 0 {
		t.Fatalf("undo snapshots kept for a deleted bouquet: %v", left)
	}

	// A provider recreates the bouquet; deleting it again keeps both copies.
	os.WriteFile(filepath.Join(s.Dir, receiverID), []byte(original), 0o644)
	os.WriteFile(indexPath, index, 0o644)
	b, _ = s.Read(receiverID)
	if err := s.Delete(receiverID, b.Revision); err != nil {
		t.Fatal(err)
	}
	if copies, _ := filepath.Glob(filepath.Join(s.BackupDir, "deleted-"+receiverID+"-*")); len(copies) != 2 {
		t.Fatalf("second delete replaced the first copy: %v", copies)
	}
}

func TestReceiverDeleteRefusesSymlink(t *testing.T) {
	s, _ := receiverFixture(t)
	indexPath := filepath.Join(s.Dir, indexFile)
	index, _ := os.ReadFile(indexPath)
	outside := filepath.Join(t.TempDir(), "outside")
	os.WriteFile(outside, []byte("private"), 0o600)
	os.Remove(filepath.Join(s.Dir, receiverID))
	os.Symlink(outside, filepath.Join(s.Dir, receiverID))
	if err := s.Delete(receiverID, ""); err == nil {
		t.Fatal("deleted through a symlink")
	}
	if got, _ := os.ReadFile(indexPath); string(got) != string(index) {
		t.Fatal("refused delete changed the index")
	}
	if got, _ := os.ReadFile(outside); string(got) != "private" {
		t.Fatal("symlink target changed")
	}
}

func TestReceiverDeleteReportsLeftoverFile(t *testing.T) {
	s, _ := receiverFixture(t)
	removeFile = func(string) error { return os.ErrPermission }
	t.Cleanup(func() { removeFile = os.Remove })
	b, _ := s.Read(receiverID)
	if err := s.Delete(receiverID, b.Revision); !errors.Is(err, ErrReceiverFileLeft) {
		t.Fatalf("failed unlink: %v", err)
	}
	if index, _ := os.ReadFile(filepath.Join(s.Dir, indexFile)); strings.Contains(string(index), receiverID) {
		t.Fatal("leftover file must already be unregistered")
	}
}

func TestReceiverUnregisterSkipsBlankLineBeforeDescription(t *testing.T) {
	index := "#NAME TV\n#SERVICE 1:7:1:0:0:0:0:0:0:0:FROM BOUQUET \"" + receiverID + "\" ORDER BY bouquet\n\n#DESCRIPTION Gone\n" +
		"#SERVICE 1:7:1:0:0:0:0:0:0:0:FROM BOUQUET \"userbouquet.other.tv\" ORDER BY bouquet\n"
	want := "#NAME TV\n#SERVICE 1:7:1:0:0:0:0:0:0:0:FROM BOUQUET \"userbouquet.other.tv\" ORDER BY bouquet\n"
	if got := string(unregister([]byte(index), receiverID)); got != want {
		t.Fatalf("unregister:\n%q", got)
	}
}
