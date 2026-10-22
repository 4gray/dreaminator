package main

import (
	"debug/elf"
	"encoding/binary"
	"os/exec"
	"strings"
	"testing"
)

func elfFixture(machine elf.Machine, class elf.Class, order elf.Data, dynamic bool) []byte {
	data := make([]byte, 120)
	copy(data, "\x7fELF")
	data[elf.EI_CLASS] = byte(class)
	data[elf.EI_DATA] = byte(order)
	data[elf.EI_VERSION] = byte(elf.EV_CURRENT)
	var endian binary.ByteOrder = binary.LittleEndian
	if order == elf.ELFDATA2MSB {
		endian = binary.BigEndian
	}
	endian.PutUint16(data[16:], uint16(elf.ET_EXEC))
	endian.PutUint16(data[18:], uint16(machine))
	endian.PutUint32(data[20:], uint32(elf.EV_CURRENT))
	if class == elf.ELFCLASS64 {
		endian.PutUint16(data[52:], 64)
		endian.PutUint16(data[54:], 56)
		endian.PutUint16(data[58:], 64)
	} else {
		endian.PutUint16(data[40:], 52)
		endian.PutUint16(data[42:], 32)
		endian.PutUint16(data[46:], 40)
	}
	if dynamic {
		endian.PutUint32(data[28:], 52)
		endian.PutUint16(data[44:], 1)
		endian.PutUint32(data[52:], uint32(elf.PT_INTERP))
	}
	return data
}

func TestValidateBinary(t *testing.T) {
	tests := []struct {
		name    string
		arch    string
		binary  []byte
		wantErr string
	}{
		{"legacy DM520", "all", elfFixture(elf.EM_MIPS, elf.ELFCLASS32, elf.ELFDATA2LSB, false), ""},
		{"Vu+ ARM", "vuuno4kse", elfFixture(elf.EM_ARM, elf.ELFCLASS32, elf.ELFDATA2LSB, false), ""},
		{"MIPS in Vu+ package", "vuuno4kse", elfFixture(elf.EM_MIPS, elf.ELFCLASS32, elf.ELFDATA2LSB, false), "EM_ARM"},
		{"ARM in DM520 package", "all", elfFixture(elf.EM_ARM, elf.ELFCLASS32, elf.ELFDATA2LSB, false), "EM_MIPS"},
		{"big endian ARM", "vuuno4kse", elfFixture(elf.EM_ARM, elf.ELFCLASS32, elf.ELFDATA2MSB, false), "little-endian"},
		{"64 bit ARM", "vuuno4kse", elfFixture(elf.EM_AARCH64, elf.ELFCLASS64, elf.ELFDATA2LSB, false), "32-bit"},
		{"dynamic ARM", "vuuno4kse", elfFixture(elf.EM_ARM, elf.ELFCLASS32, elf.ELFDATA2LSB, true), "static"},
		{"not ELF", "vuuno4kse", []byte("not an executable"), "ELF"},
		{"unknown package architecture", "arm64", elfFixture(elf.EM_ARM, elf.ELFCLASS32, elf.ELFDATA2LSB, false), "unsupported"},
		{"metadata injection", "vuuno4kse\nDepends: injected", nil, "unsupported"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateBinary(tt.binary, tt.arch)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %v; want it to contain %q", err, tt.wantErr)
			}
		})
	}
}

func TestReceiverBuildsStaySeparate(t *testing.T) {
	for _, target := range []struct {
		name, env, output, pack string
	}{
		{"ipk", "GOOS=linux GOARCH=mipsle GOMIPS=softfloat CGO_ENABLED=0", "dist/dreaminator", "dist/dreaminator_0.11.0-r3_all.ipk"},
		{"ipk-vuuno4kse", "GOOS=linux GOARCH=arm GOARM=7 CGO_ENABLED=0", "dist/vuuno4kse/dreaminator", "dist/dreaminator_0.11.0-r3_vuuno4kse.ipk"},
	} {
		t.Run(target.name, func(t *testing.T) {
			cmd := exec.Command("make", "-n", "SIM_ARCH=arm64", "VERSION=0.11.0-r3", target.name)
			cmd.Dir = "../.."
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("make: %v\n%s", err, out)
			}
			for _, want := range []string{target.env + " go build", "-o " + target.output + " ./cmd/dreaminator", "-binary " + target.output, "-out " + target.pack} {
				if !strings.Contains(string(out), want) {
					t.Errorf("build commands lack %q:\n%s", want, out)
				}
			}
			if target.name == "ipk-vuuno4kse" && !strings.Contains(string(out), "-arch vuuno4kse") {
				t.Errorf("Vu+ package must declare its receiver architecture:\n%s", out)
			}
		})
	}
}
