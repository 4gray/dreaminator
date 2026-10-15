// mkipk builds the opkg package for the receiver without needing Linux tools:
// an ipk is an ar archive of debian-binary, control.tar.gz and data.tar.gz.
package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"
)

type entry struct {
	name string
	mode int64
	data []byte // nil for directories
}

const postinst = `#!/bin/sh
[ -n "$D" ] && exit 0
update-rc.d dreaminator defaults 95 >/dev/null 2>&1 || true
/etc/init.d/dreaminator restart
IP=$(ip -4 addr show eth0 2>/dev/null | sed -n 's/.*inet \([0-9.]*\).*/\1/p' | head -n1)
echo ""
echo "Dreaminator установлен. Откройте на планшете: http://${IP:-<адрес ресивера>}:8088"
exit 0
`

const prerm = `#!/bin/sh
[ -n "$D" ] && exit 0
/etc/init.d/dreaminator stop
exit 0
`

const postrm = `#!/bin/sh
[ -n "$D" ] && exit 0
update-rc.d -f dreaminator remove >/dev/null 2>&1 || true
exit 0
`

func main() {
	version := flag.String("version", "0.2.0", "package version")
	binary := flag.String("binary", "dist/dreaminator", "mipsel binary")
	initScript := flag.String("init", "packaging/dreaminator.init", "init script")
	out := flag.String("out", "dist/dreaminator.ipk", "output file")
	flag.Parse()

	bin, err := os.ReadFile(*binary)
	if err != nil {
		log.Fatal(err)
	}
	initData, err := os.ReadFile(*initScript)
	if err != nil {
		log.Fatal(err)
	}

	control := fmt.Sprintf(`Package: dreaminator
Version: %s
Architecture: all
Maintainer: Dreaminator
Section: extra
Priority: optional
Description: Плейлисты IPTV и избранное для Enigma2 с планшета (веб-интерфейс на порту 8088)
`, *version)

	controlTar := tarGz([]entry{
		{name: "./", mode: 0o755},
		{name: "./control", mode: 0o644, data: []byte(control)},
		{name: "./postinst", mode: 0o755, data: []byte(postinst)},
		{name: "./prerm", mode: 0o755, data: []byte(prerm)},
		{name: "./postrm", mode: 0o755, data: []byte(postrm)},
	})
	dataTar := tarGz([]entry{
		{name: "./", mode: 0o755},
		{name: "./usr/", mode: 0o755},
		{name: "./usr/bin/", mode: 0o755},
		{name: "./usr/bin/dreaminator", mode: 0o755, data: bin},
		{name: "./etc/", mode: 0o755},
		{name: "./etc/init.d/", mode: 0o755},
		{name: "./etc/init.d/dreaminator", mode: 0o755, data: initData},
	})

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
