# -*- coding: utf-8 -*-
"""The Dreaminator TV look: colours, fonts, geometry and skin helpers.

Everything is designed on a 1920x1080 canvas and scaled for HD desktops.
This module is pure Python: the plugin, the test harness and the artwork
generator (tools/genui) all read it, so the PNGs always match the skin.
"""
from __future__ import absolute_import, division

import os

PLUGIN_DIR = os.path.dirname(os.path.abspath(__file__))

# Palette (IPTVnator dark graphite; one accent for focus everywhere).
BG = "#0b0d11"
RAIL = "#111318"
SURFACE = "#151a24"
SURFACE_2 = "#1a1f2b"
ACTIVE = "#171c27"
LINE = "#2a3040"
TEXT = "#eef1f7"
TEXT_2 = "#b3bbcc"
MUTED = "#7d869b"
ACCENT = "#78adff"
ON_ACCENT = "#071225"
WARN = "#f5c84c"
ERROR = "#ff8f8f"
KEY_COLORS = {"red": "#e5484d", "green": "#3dd68c", "yellow": "#f5c84c", "blue": "#4c8dff"}

FONT = "DreamSans"
FONT_SEMI = "DreamSansSemi"
FONT_FILES = {FONT: "Geist-Regular.ttf", FONT_SEMI: "Geist-SemiBold.ttf"}

# Safe area: backgrounds bleed to the edges, content stays inside.
SAFE_X = 80
SAFE_Y = 48

RAIL_W = 152
RAIL_OPEN_W = 440
RAIL_ITEMS = [  # key, label, y
    ("favorites", "Избранное", 180),
    ("recent", "Недавние", 268),
    ("portal", "Портал", 356),
    ("live", "Live TV", 444),
    ("vod", "Фильмы", 532),
    ("series", "Сериалы", 620),
    ("search", "Поиск", 708),
    ("settings", "Настройки", 880),
    ("about", "О приложении", 968),
]
RAIL_ICON = {"favorites": "star", "recent": "recent", "portal": "portal", "live": "live", "vod": "film", "series": "series",
             "search": "search", "settings": "settings", "about": "info"}
# Favorite marks: on a channel row (blended over the row, not over a
# picture) and on a poster (an opaque tile, like the "Сериал" badge: no
# alpha-blending over a decoded picture).
STAR_ROW = 30
STAR_BADGE = 34

CONTENT_X = 200
HEADER_Y = 44
BODY_Y = 168

CATS = {"x": 200, "y": BODY_Y, "w": 320, "row": 60, "rows": 14}

# Poster grid: 18 posters, no text under them. The focused title, year and
# rating sit in the header (one place to read on a TV, as on Netflix and
# Google TV); posters keep the portal's shape (~1:1.46, no bars). Measured
# on a DM520 (OpenHDF 6.4) with live IPTV playing; see ROADMAP.
GRID = {"x": 568, "y": BODY_Y, "cols": 6, "rows": 3, "pw": 190, "ph": 276, "gap_x": 26, "gap_y": 18,
        "text": False, "title_gap": 0, "title_h": 0, "meta_h": 0, "radius": 10, "ring_gap": 6, "ring": 4}

LIVE = {"x": 568, "y": BODY_Y, "w": 780, "row": 72, "pitch": 76, "rows": 11, "logo_w": 96, "logo_h": 54}
# The panel beside the channel list: logo, name, and what is on now and next.
LIVE_PREVIEW = {"x": 1400, "y": BODY_Y, "w": 440, "h": 832, "logo_w": 288, "logo_h": 162}
# Progress of the programme on air: a light bar on a dark track.
PROGRESS = TEXT_2
PROGRESS_TRACK = LINE

# Programme of a channel for a day: list on the left, the focused programme
# on the right.
GUIDE = {"x": 200, "y": 216, "w": 980, "row": 62, "pitch": 68, "rows": 11, "info_x": 1240, "info_w": 600}


def grid_cell(index, grid=GRID):
    """Top-left corner of a grid cell's poster on the 1920 canvas."""
    col, row = index % grid["cols"], index // grid["cols"]
    pitch_y = grid["ph"] + grid["gap_y"]
    if grid.get("text"):
        pitch_y += grid["title_gap"] + grid["title_h"] + 2 + grid["meta_h"]
    return grid["x"] + col * (grid["pw"] + grid["gap_x"]), grid["y"] + row * pitch_y


def ring_size(grid=GRID):
    pad = grid["ring_gap"] + grid["ring"]
    return grid["pw"] + 2 * pad, grid["ph"] + 2 * pad


# Search keyboard and result grid.
KEYBOARD = {"x": 200, "y": 272, "cols": 7, "key_w": 72, "key_h": 60, "gap": 8}
SEARCH_GRID = dict(GRID, x=820, y=232, cols=5, rows=2, pw=184, ph=268, gap_x=24, gap_y=12, text=True,
                   title_gap=10, title_h=64, meta_h=28)
SEARCH_LIVE = dict(LIVE, x=820, y=232, w=1020, rows=10, pill="pill_search_live")
SEARCH_SCOPES = [("vod", "Фильмы"), ("series", "Сериалы"), ("live", "Live TV")]

# Detail screen.
DETAIL = {"backdrop_x": 640, "backdrop_w": 1280, "backdrop_h": 720, "poster_x": 96, "poster_y": 120,
          "poster_w": 300, "poster_h": 438, "info_x": 444, "info_w": 1000, "button_y": 880}
CAST = {"x": 444, "y": 640, "w": 96, "h": 140, "pitch": 128, "count": 8}
STILL = {"x": 1220, "y": 690, "w": 320, "h": 180}


RING_EDGE = 20  # focus-ring strips: thick enough to hold the rounded corners
CORNER = 10


def ring_parts(name, grid):
    """Four strips of a poster focus ring (blending a whole ring is slow)."""
    rw, rh = ring_size(grid)
    base = {"kind": "ring", "w": rw, "h": rh, "r": grid["radius"] + grid["ring_gap"] + grid["ring"], "stroke": grid["ring"],
            "color": ACCENT}
    return [dict(base, name="%s_%s" % (name, part), part=part, side=RING_EDGE) for part in ("top", "bottom", "left", "right")]


def ring_rects(x, y, grid=GRID):
    """(part, x, y, w, h) of the ring strips around the poster at x, y."""
    pad = grid["ring_gap"] + grid["ring"]
    rw, rh = ring_size(grid)
    x0, y0, e = x - pad, y - pad, RING_EDGE
    return [("top", x0, y0, rw, e), ("bottom", x0, y0 + rh - e, rw, e),
            ("left", x0, y0 + e, e, rh - 2 * e), ("right", x0 + rw - e, y0 + e, e, rh - 2 * e)]


def corner_parts(name, w, h, r):
    base = {"kind": "mask", "w": w, "h": h, "r": r, "color": BG}
    return [dict(base, name="%s_%s" % (name, part), part=part, side=r) for part in ("tl", "tr", "bl", "br")]


def corner_rects(x, y, w, h, r=CORNER):
    return [("tl", x, y), ("tr", x + w - r, y), ("bl", x, y + h - r), ("br", x + w - r, y + h - r)]


def asset_specs():
    """Artwork drawn by tools/genui at 1920 scale (and at 2/3 for HD)."""
    g = GRID
    sg = SEARCH_GRID
    specs = ring_parts("ring_poster", g) + ring_parts("ring_search", sg)
    specs += corner_parts("corner", 100, 100, CORNER) + corner_parts("corner_big", 100, 100, 14)
    specs += [
        {"name": "icon_film_empty", "kind": "icon", "w": 64, "h": 64, "icon": "film", "color": LINE},
        {"name": "icon_live_empty", "kind": "icon", "w": 64, "h": 64, "icon": "live", "color": LINE},
        {"name": "pill_rail", "kind": "rrect", "w": 64, "h": 64, "r": 16, "color": ACCENT},
        {"name": "pill_rail_active", "kind": "rrect", "w": 64, "h": 64, "r": 16, "color": ACTIVE},
        {"name": "pill_rail_open", "kind": "rrect", "w": 328, "h": 64, "r": 16, "color": ACCENT},
        {"name": "pill_rail_open_active", "kind": "rrect", "w": 328, "h": 64, "r": 16, "color": ACTIVE},
        {"name": "rail_shadow", "kind": "hgradient", "w": 48, "h": 1080, "color": "#000000", "from": 140, "to": 0},
        {"name": "mark", "kind": "rrect", "w": 48, "h": 48, "r": 12, "color": ACCENT},
        {"name": "pill_cat", "kind": "rrect", "w": CATS["w"], "h": CATS["row"], "r": 14, "color": ACCENT},
        {"name": "pill_cat_active", "kind": "rrect", "w": CATS["w"], "h": CATS["row"], "r": 14, "color": SURFACE},
        {"name": "pill_live", "kind": "rrect", "w": LIVE["w"], "h": LIVE["row"], "r": 14, "color": ACCENT},
        {"name": "pill_search_live", "kind": "rrect", "w": 1020, "h": LIVE["row"], "r": 14, "color": ACCENT},
        {"name": "chip_scope", "kind": "rrect", "w": 176, "h": 52, "r": 12, "color": SURFACE},
        {"name": "chip_scope_focus", "kind": "rrect", "w": 176, "h": 52, "r": 12, "color": ACCENT},
        {"name": "chip_scope_active", "kind": "rrect", "w": 176, "h": 52, "r": 12, "color": ACTIVE},
        {"name": "dialog", "kind": "rrect", "w": 760, "h": 640, "r": 24, "color": RAIL},
        {"name": "pill_dialog", "kind": "rrect", "w": 680, "h": 64, "r": 14, "color": ACCENT},
        {"name": "panel_live", "kind": "rrect", "w": LIVE_PREVIEW["w"], "h": LIVE_PREVIEW["h"], "r": 20, "color": "#121620"},
        {"name": "pill_key", "kind": "rrect", "w": KEYBOARD["key_w"], "h": KEYBOARD["key_h"], "r": 12, "color": ACCENT},
        {"name": "plate_key", "kind": "rrect", "w": KEYBOARD["key_w"], "h": KEYBOARD["key_h"], "r": 12, "color": SURFACE},
        {"name": "pill_key_wide", "kind": "rrect", "w": 179, "h": KEYBOARD["key_h"], "r": 12, "color": ACCENT},
        {"name": "plate_key_wide", "kind": "rrect", "w": 179, "h": KEYBOARD["key_h"], "r": 12, "color": SURFACE},
        {"name": "field", "kind": "rrect", "w": 552, "h": 76, "r": 16, "color": SURFACE},
        {"name": "button", "kind": "rrect", "w": 300, "h": 72, "r": 14, "color": SURFACE_2},
        {"name": "button_focus", "kind": "rrect", "w": 300, "h": 72, "r": 14, "color": ACCENT},
        {"name": "chip", "kind": "rrect", "w": 200, "h": 52, "r": 12, "color": SURFACE},
        {"name": "chip_focus", "kind": "rrect", "w": 200, "h": 52, "r": 12, "color": ACCENT},
        {"name": "chip_active", "kind": "rrect", "w": 200, "h": 52, "r": 12, "color": ACTIVE},
        {"name": "pill_episode", "kind": "rrect", "w": 1080, "h": 64, "r": 14, "color": ACCENT},
        {"name": "pill_guide", "kind": "rrect", "w": GUIDE["w"], "h": GUIDE["row"], "r": 14, "color": ACCENT},
        {"name": "fade_left", "kind": "hgradient", "w": 640, "h": DETAIL["backdrop_h"], "color": BG, "from": 255, "to": 0},
        {"name": "fade_bottom", "kind": "vgradient", "w": DETAIL["backdrop_w"], "h": 280, "color": BG, "from": 0, "to": 255},
        {"name": "star_row", "kind": "rimmed", "w": STAR_ROW, "h": STAR_ROW, "icon": "star_filled", "color": WARN, "fg": BG},
        {"name": "star_badge", "kind": "badge", "w": STAR_BADGE, "h": STAR_BADGE, "r": 0, "color": BG, "fg": WARN, "icon": "star_filled"},
    ]
    for key, color in KEY_COLORS.items():
        specs.append({"name": "dot_" + key, "kind": "rrect", "w": 14, "h": 14, "r": 7, "color": color})
    for icon in sorted(set(RAIL_ICON.values()) | {"play", "text", "back", "backspace"}):
        for variant, color in (("muted", MUTED), ("accent", ACCENT), ("dark", ON_ACCENT), ("light", TEXT)):
            specs.append({"name": "icon_%s_%s" % (icon, variant), "kind": "icon", "w": 32, "h": 32, "icon": icon, "color": color})
    return specs


class Skin(object):
    """Builds skin XML for one desktop width (1920 canvas scaled down for HD)."""

    def __init__(self, width=1920, fonts=True):
        self.f = 1.0 if width >= 1920 else width / 1920.0
        self.folder = "fhd" if width >= 1920 else "hd"
        self.font_regular = FONT if fonts else "Regular"
        self.font_semi = FONT_SEMI if fonts else "Regular"

    def px(self, v):
        return int(round(v * self.f))

    def pos(self, x, y):
        return "%d,%d" % (self.px(x), self.px(y))

    def size(self, w, h):
        return "%d,%d" % (self.px(w), self.px(h))

    def font(self, size, semi=False):
        return "%s;%d" % (self.font_semi if semi else self.font_regular, max(12, self.px(size)))

    def asset(self, name):
        return os.path.join(PLUGIN_DIR, "ui", self.folder, name + ".png")

    def box(self, x, y, w, h, color, z=0):
        return '<eLabel position="%s" size="%s" backgroundColor="%s" zPosition="%d" />' % (self.pos(x, y), self.size(w, h), color, z)

    def image(self, x, y, w, h, asset, z=1):
        return '<ePixmap position="%s" size="%s" pixmap="%s" alphatest="blend" zPosition="%d" />' % (
            self.pos(x, y), self.size(w, h), self.asset(asset), z)

    def pixmap(self, name, x, y, w, h, z=1, alpha="blend", asset=None):
        extra = ' pixmap="%s"' % self.asset(asset) if asset else ""
        return '<widget name="%s" position="%s" size="%s" alphatest="%s" zPosition="%d"%s />' % (
            name, self.pos(x, y), self.size(w, h), alpha, z, extra)

    def label(self, name, x, y, w, h, size=24, color=TEXT, semi=False, halign="left", valign="center", bg=None, z=2, wrap=False,
              shadow=False):
        attrs = 'name="%s" position="%s" size="%s" font="%s" foregroundColor="%s" halign="%s" valign="%s" zPosition="%d"' % (
            name, self.pos(x, y), self.size(w, h), self.font(size, semi), color, halign, valign, z)
        if shadow:
            # Text over pictures: a dark offset copy keeps it readable.
            attrs += ' shadowColor="#000000" shadowOffset="%d,%d"' % (max(1, self.px(2)), max(1, self.px(2)))
        if bg:
            attrs += ' backgroundColor="%s"' % bg
        else:
            attrs += ' transparent="1" backgroundColor="%s"' % BG
        if not wrap:
            attrs += ' noWrap="1"'
        return "<widget %s />" % attrs

    def text(self, x, y, w, h, value, size=24, color=TEXT, semi=False, halign="left", z=2):
        return ('<eLabel position="%s" size="%s" font="%s" foregroundColor="%s" backgroundColor="%s" transparent="1" '
                'halign="%s" valign="center" noWrap="1" zPosition="%d" text="%s" />' % (
                    self.pos(x, y), self.size(w, h), self.font(size, semi), color, BG, halign, z, escape(value)))

    def progress(self, name, x, y, w, h, z=3, fg=None, bg=None):
        """A programme's progress (eSlider: setValue 0-100 redraws only itself)."""
        return ('<widget name="%s" position="%s" size="%s" foregroundColor="%s" backgroundColor="%s" borderWidth="0" zPosition="%d" />'
                % (name, self.pos(x, y), self.size(w, h), fg or PROGRESS, bg or PROGRESS_TRACK, z))

    def clock(self, x, y, w, h, size=34):
        return ('<widget source="global.CurrentTime" render="Label" position="%s" size="%s" font="%s" foregroundColor="%s" '
                'backgroundColor="%s" transparent="1" halign="right" valign="center" zPosition="2">'
                '<convert type="ClockToText">Format:%%H:%%M</convert></widget>' % (
                    self.pos(x, y), self.size(w, h), self.font(size, semi=False), TEXT, BG))

    def screen(self, name, body):
        width, height = self.px(1920), self.px(1080)
        return ('<screen name="%s" position="0,0" size="%d,%d" flags="wfNoBorder" backgroundColor="%s" title="Dreaminator">%s</screen>'
                % (name, width, height, BG, "".join(body)))


def escape(value):
    return (value.replace("&", "&amp;").replace('"', "&quot;").replace("<", "&lt;").replace(">", "&gt;"))


def clip(text, limit):
    """Cut text that would not fit; eLabel does not add an ellipsis itself."""
    if not text:
        return ""
    if isinstance(text, bytes):
        decoded = text.decode("utf-8", "replace")
        return clip(decoded, limit).encode("utf-8")
    if len(text) <= limit:
        return text
    cut = text[:limit - 1].rstrip()
    space = cut.rfind(" ")
    if space > limit // 2:
        cut = cut[:space]
    return cut.rstrip(" ,.;:-–—") + u"…"


if __name__ == "__main__":
    import json
    import sys
    json.dump(asset_specs(), sys.stdout, ensure_ascii=False)
