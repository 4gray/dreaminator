// mkipk builds the opkg package for the receiver without needing Linux tools:
// an ipk is an ar archive of debian-binary, control.tar.gz and data.tar.gz.
package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"debug/elf"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// extensionsDest is where Enigma2 looks for extension plugins; every folder
// of the plugin directory (Dreaminator, DreaminatorEPG) becomes one there.
const extensionsDest = "./usr/lib/enigma2/python/Plugins/Extensions/"

type entry struct {
	name string
	mode int64
	data []byte // nil for directories
}

const postinst = `#!/bin/sh
[ -n "$D" ] && exit 0
PATH=/usr/local/sbin:/usr/local/bin:/sbin:/bin:/usr/sbin:/usr/bin
export PATH
update-rc.d dreaminator defaults 95 >/dev/null 2>&1 || true
/etc/init.d/dreaminator restart
IP=$(ip -4 addr show eth0 2>/dev/null | sed -n 's/.*inet \([0-9.]*\).*/\1/p' | head -n1)
echo ""
echo "Dreaminator установлен. Откройте на планшете: http://${IP:-<адрес ресивера>}:8088"
echo "После перезапуска интерфейса адрес и QR-код будут в меню «Плагины» → Dreaminator."
exit 0
`

const prerm = `#!/bin/sh
[ -n "$D" ] && exit 0
/etc/init.d/dreaminator stop
exit 0
`

const postrm = `#!/bin/sh
[ -n "$D" ] && exit 0
PATH=/usr/local/sbin:/usr/local/bin:/sbin:/bin:/usr/sbin:/usr/bin
export PATH
update-rc.d -f dreaminator remove >/dev/null 2>&1 || true
exit 0
`

func main() {
	version := flag.String("version", "0.8.0-r1", "package version")
	binary := flag.String("binary", "dist/dreaminator", "static receiver executable")
	arch := flag.String("arch", "all", "package architecture: all (legacy DM520 MIPS) or vuuno4kse (ARMv7)")
	initScript := flag.String("init", "packaging/dreaminator.init", "init script")
	pluginDir := flag.String("plugins", "plugin", "directory of the Enigma2 plugin folders")
	out := flag.String("out", "dist/dreaminator.ipk", "output file")
	flag.Parse()

	bin, err := os.ReadFile(*binary)
	if err != nil {
		log.Fatal(err)
	}
	if err := validateBinary(bin, *arch); err != nil {
		log.Fatal(err)
	}
	initData, err := os.ReadFile(*initScript)
	if err != nil {
		log.Fatal(err)
	}

	control := fmt.Sprintf(`Package: dreaminator
Version: %s
Architecture: %s
Maintainer: Dreaminator
Section: extra
Priority: optional
Description: Плейлисты IPTV и избранное для Enigma2 с планшета (веб-интерфейс на порту 8088)
`, *version, *arch)

	controlTar := tarGz([]entry{
		{name: "./", mode: 0o755},
		{name: "./control", mode: 0o644, data: []byte(control)},
		{name: "./postinst", mode: 0o755, data: []byte(postinst)},
		{name: "./prerm", mode: 0o755, data: []byte(prerm)},
		{name: "./postrm", mode: 0o755, data: []byte(postrm)},
	})
	data := []entry{
		{name: "./", mode: 0o755},
		{name: "./usr/", mode: 0o755},
		{name: "./usr/bin/", mode: 0o755},
		{name: "./usr/bin/dreaminator", mode: 0o755, data: bin},
		{name: "./etc/", mode: 0o755},
		{name: "./etc/init.d/", mode: 0o755},
		{name: "./etc/init.d/dreaminator", mode: 0o755, data: initData},
	}
	// Binary distributions retain the project's and dependencies' notices.
	data = append(data, entry{name: "./usr/share/", mode: 0o755},
		entry{name: "./usr/share/doc/", mode: 0o755},
		entry{name: "./usr/share/doc/dreaminator/", mode: 0o755})
	for _, name := range []string{"LICENSE", "THIRD_PARTY_NOTICES.md"} {
		notice, err := os.ReadFile(name)
		if err != nil {
			log.Fatal(err)
		}
		data = append(data, entry{name: "./usr/share/doc/dreaminator/" + name, mode: 0o644, data: notice})
	}
	for _, dir := range []string{"./usr/lib/", "./usr/lib/enigma2/", "./usr/lib/enigma2/python/",
		"./usr/lib/enigma2/python/Plugins/", extensionsDest} {
		data = append(data, entry{name: dir, mode: 0o755})
	}
	folders, err := os.ReadDir(*pluginDir)
	if err != nil {
		log.Fatal(err)
	}
	for _, f := range folders {
		if !f.IsDir() || strings.HasPrefix(f.Name(), ".") {
			continue
		}
		dest := extensionsDest + f.Name() + "/"
		plugin, err := pluginEntries(filepath.Join(*pluginDir, f.Name()), dest)
		if err != nil {
			log.Fatal(err)
		}
		data = append(append(data, entry{name: dest, mode: 0o755}), plugin...)
	}
	dataTar := tarGz(data)

	var ar bytes.Buffer
	ar.WriteString("!<arch>\n")
	arMember(&ar, "debian-binary", []byte("2.0\n"))
	arMember(&ar, "control.tar.gz", controlTar)
	arMember(&ar, "data.tar.gz", dataTar)

	if err := os.MkdirAll(filepath.Dir(*out), 0o755); err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile(*out, ar.Bytes(), 0o644); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("%s (%d KB)\n", *out, ar.Len()/1024)
}

// The historical "all" package is MIPS, despite its architecture label. Keep
// that contract for the DM520, but do not allow ARM and MIPS to be mixed up.
func validateBinary(data []byte, arch string) error {
	var machine elf.Machine
	switch arch {
	case "all":
		machine = elf.EM_MIPS
	case "vuuno4kse":
		machine = elf.EM_ARM
	default:
		return fmt.Errorf("unsupported package architecture %q", arch)
	}
	f, err := elf.NewFile(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("receiver binary must be ELF: %w", err)
	}
	defer f.Close()
	if f.Class != elf.ELFCLASS32 {
		return fmt.Errorf("%s requires a 32-bit ELF executable", arch)
	}
	if f.Data != elf.ELFDATA2LSB {
		return fmt.Errorf("%s requires a little-endian ELF executable", arch)
	}
	if f.Machine != machine {
		return fmt.Errorf("%s requires %s, got %s", arch, machine, f.Machine)
	}
	if f.Type != elf.ET_EXEC {
		return fmt.Errorf("%s requires a static ELF executable", arch)
	}
	for _, p := range f.Progs {
		if p.Type == elf.PT_INTERP || p.Type == elf.PT_DYNAMIC {
			return fmt.Errorf("%s requires a static executable without shared library dependencies", arch)
		}
	}
	return nil
}

// pluginEntries packs a plugin folder's sources into dest, skipping
// compiled leftovers.
func pluginEntries(dir, dest string) ([]entry, error) {
	var out []entry
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		name := d.Name()
		if d.IsDir() {
			if name == "__pycache__" {
				return filepath.SkipDir
			}
			// Fonts and UI artwork live in subfolders; old opkg builds do not
			// create missing parents while unpacking.
			if rel, _ := filepath.Rel(dir, path); rel != "." {
				out = append(out, entry{name: dest + filepath.ToSlash(rel) + "/", mode: 0o755})
			}
			return nil
		}
		if strings.HasSuffix(name, ".pyc") || strings.HasSuffix(name, ".pyo") || strings.HasPrefix(name, ".") {
			return nil
		}
		rel, _ := filepath.Rel(dir, path)
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		out = append(out, entry{name: dest + filepath.ToSlash(rel), mode: 0o644, data: b})
		return nil
	})
	if len(out) == 0 && err == nil {
		err = fmt.Errorf("в %s нет файлов плагина", dir)
	}
	return out, err
}

func tarGz(entries []entry) []byte {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	now := time.Now()
	for _, e := range entries {
		hdr := &tar.Header{Name: e.name, Mode: e.mode, ModTime: now, Uname: "root", Gname: "root", Format: tar.FormatGNU}
		if e.data == nil {
			hdr.Typeflag = tar.TypeDir
		} else {
			hdr.Typeflag = tar.TypeReg
			hdr.Size = int64(len(e.data))
		}
		if err := tw.WriteHeader(hdr); err != nil {
			log.Fatal(err)
		}
		if e.data != nil {
			if _, err := tw.Write(e.data); err != nil {
				log.Fatal(err)
			}
		}
	}
	if err := tw.Close(); err != nil {
		log.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		log.Fatal(err)
	}
	return buf.Bytes()
}

func arMember(w *bytes.Buffer, name string, data []byte) {
	fmt.Fprintf(w, "%-16s%-12d%-6d%-6d%-8o%-10d`\n", name, time.Now().Unix(), 0, 0, 0o100644, len(data))
	w.Write(data)
	if len(data)%2 == 1 {
		w.WriteByte('\n')
	}
}
