package epg

import (
	"encoding/xml"
	"strings"
	"testing"
	"time"
)

const guide = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE tv SYSTEM "xmltv.dtd">
<tv generator-info-name="test">
  <channel id="1tv"><display-name lang="ru">Первый канал</display-name><icon src="x"/></channel>
  <channel id="rtr"><display-name>Россия 1</display-name></channel>
  <channel id="other"><display-name>Другой</display-name></channel>
  <programme start="20260927100000 +0300" stop="20260927110000 +0300" channel="1tv">
    <title lang="ru">Новости &amp; погода</title><desc lang="ru">Описание</desc><rating><value>16+</value></rating>
  </programme>
  <programme start="20260920100000 +0300" stop="20260920110000 +0300" channel="1tv"><title>Старое</title></programme>
  <programme start="20260927100000 +0300" stop="20260927110000 +0300" channel="rtr"><title>Вести</title></programme>
  <programme start="20260927100000 +0300" stop="20260927110000 +0300" channel="other"><title>Лишнее</title></programme>
</tv>`

func TestFilter(t *testing.T) {
	wants := []Want{
		{Ref: "REF1", TvgID: "1TV", Names: []string{"Первый HD"}},
		{Ref: "REF2", Names: []string{"R1 Moscow", "Россия-1 HD"}}, // tvg-name matches
		{Ref: "REF3", Names: []string{"Нет в программе"}},
	}
	var out strings.Builder
	out.WriteString(Header)
	now := time.Date(2026, 9, 27, 9, 0, 0, 0, time.UTC)
	res, err := Filter(strings.NewReader(guide), "e0.", wants, &out, now)
	if err != nil {
		t.Fatal(err)
	}
	out.WriteString(Footer)

	if res.Channels != 2 || res.Programmes != 2 {
		t.Errorf("channels=%d programmes=%d\n%s", res.Channels, res.Programmes, out.String())
	}
	if got := res.Map["e0.1tv"]; len(got) != 1 || got[0] != "REF1" {
		t.Errorf("1tv mapping = %v", got)
	}
	if got := res.Map["e0.rtr"]; len(got) != 1 || got[0] != "REF2" {
		t.Errorf("name match mapping = %v", got)
	}
	s := out.String()
	if strings.Contains(s, "Лишнее") || strings.Contains(s, "Старое") {
		t.Errorf("unwanted programmes leaked:\n%s", s)
	}
	if !strings.Contains(s, "Новости &amp; погода") {
		t.Errorf("escaping broken:\n%s", s)
	}
	if err := xml.Unmarshal([]byte(s), new(struct{})); err != nil {
		t.Errorf("output is not valid XML: %v\n%s", err, s)
	}
}

func TestNormalize(t *testing.T) {
	for in, want := range map[string]string{
		"Первый канал HD":           "первыйканал",
		"Россия-1 (FHD)":            "россия1",
		"Ёлки +2":                   "елки+2",
		"Mir 24 (1080p) [Not 24/7]": "mir24",
		"NTV HD 720p":               "ntv",
	} {
		if got := Normalize(in); got != want {
			t.Errorf("Normalize(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestFilterShift(t *testing.T) {
	wants := []Want{
		{Ref: "ORIG", TvgID: "1tv"},
		{Ref: "PLUS2", TvgID: "1tv", Shift: 2 * time.Hour},
	}
	var out strings.Builder
	now := time.Date(2026, 9, 27, 9, 0, 0, 0, time.UTC)
	res, err := Filter(strings.NewReader(guide), "e0.", wants, &out, now)
	if err != nil {
		t.Fatal(err)
	}
	if got := res.Map["e0.1tv"]; len(got) != 1 || got[0] != "ORIG" {
		t.Errorf("original mapping = %v", got)
	}
	if got := res.Map["e0.1tv@+2"]; len(got) != 1 || got[0] != "PLUS2" {
		t.Errorf("shifted mapping = %v (all: %v)", got, res.Map)
	}
	s := out.String()
	if !strings.Contains(s, `start="20260927100000 +0300" stop="20260927110000 +0300" channel="e0.1tv"`) ||
		!strings.Contains(s, `start="20260927120000 +0300" stop="20260927130000 +0300" channel="e0.1tv@+2"`) {
		t.Errorf("shifted programme missing:\n%s", s)
	}
	if res.Programmes != 2 {
		t.Errorf("programmes = %d", res.Programmes)
	}
}
