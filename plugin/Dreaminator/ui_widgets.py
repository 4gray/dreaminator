# -*- coding: utf-8 -*-
"""Reusable parts of the TV screens: poster grid and channel list.

Each part owns a set of named components inside a Screen (Enigma2 screens
are dictionaries of components) and renders a PagedList through a focus
model. Rules learned on a DM520: nothing larger than a few hundred pixels
may be alpha-blended on top of a decoded picture, and a hidden component
costs nothing to draw.
"""
from __future__ import absolute_import, division

import time

from enigma import eLabel, ePoint, eSize, gFont, gRGB
from Components.Label import Label
from Components.Pixmap import Pixmap
from Components.ProgressBar import ProgressBar

try:
    from . import ui_api, ui_theme as T
    from .ui_models import GridModel, ListModel, PagedList
except (ImportError, ValueError):
    import ui_api
    import ui_theme as T
    from ui_models import GridModel, ListModel, PagedList

RING_PARTS = ("top", "bottom", "left", "right")
RATING_LABEL = {"imdb": "IMDb", "kinopoisk": "КП", "tmdb": "TMDB"}


def move(screen, skin, name, x, y):
    screen[name].instance.move(ePoint(skin.px(x), skin.px(y)))


def color(screen, name, hex_value):
    screen[name].instance.setForegroundColor(gRGB(int(hex_value[1:], 16)))


def font(screen, skin, name, size, semi):
    family = skin.font_semi if semi else skin.font_regular
    screen[name].instance.setFont(gFont(family, max(12, skin.px(size))))


def visible(widget, on):
    if on:
        widget.show()
    else:
        widget.hide()


def duration(minutes):
    try:
        m = int(minutes)
    except (TypeError, ValueError):
        return ""
    if m <= 0:
        return ""
    if m < 60:
        return "%d мин" % m
    return "%d ч %02d мин" % (m // 60, m % 60)


def days_text(n):
    """'1 день', '3 дня', '7 дней'."""
    n = int(n)
    if n % 10 == 1 and n % 100 != 11:
        word = "день"
    elif 2 <= n % 10 <= 4 and not 12 <= n % 100 <= 14:
        word = "дня"
    else:
        word = "дней"
    return "%d %s" % (n, word)


def rating_text(rating):
    if not rating or not rating.get("value"):
        return ""
    source = rating.get("source")
    return "%s %s" % (RATING_LABEL.get(source) or ui_api.text(source), ui_api.text(rating["value"]))


def meta_line(item):
    """'2013 · IMDb 5.7' or '2013 · 1 ч 28 мин' (display text for this Python)."""
    parts = []
    if item.get("year"):
        parts.append(ui_api.text(item["year"]))
    rating = rating_text(item.get("rating"))
    if rating:
        parts.append(rating)
    elif duration(item.get("duration")):
        parts.append(duration(item.get("duration")))
    return " · ".join(parts)


def focus_meta(item):
    """Header line for the focused card: year · rating with source · length · type."""
    parts = []
    if item.get("year"):
        parts.append(ui_api.text(item["year"]))
    rating = rating_text(item.get("rating"))
    if rating:
        parts.append(rating)
    if duration(item.get("duration")):
        parts.append(duration(item.get("duration")))
    if item.get("kind") == "series":
        parts.append("Сериал")
    elif item.get("kind") == "channel" and item.get("number"):
        parts.append("Канал %s" % ui_api.text(item["number"]))
    if item.get("missing"):
        parts.append("Нет в плейлисте")
    elif item.get("fav"):
        parts.append("В избранном")
    return " · ".join(parts)


def clipped(value, limit):
    return ui_api.text(T.clip(ui_api.unicode_text(value or ""), limit))


def row_source(item, default):
    """Library rows (favorites, recents) come from several sources."""
    return item.get("source") or default


# ---------------------------------------------------------------- programmes

def hhmm(ts):
    try:
        return time.strftime("%H:%M", time.localtime(int(ts)))
    except (TypeError, ValueError, OverflowError):
        return ""


def on_air(entry, t):
    """The programme is on now (the service's entries carry unix times)."""
    try:
        return bool(entry) and int(entry["start"]) <= t < int(entry["stop"])
    except (KeyError, TypeError, ValueError):
        return False


def progress(entry, t):
    try:
        start, stop = int(entry["start"]), int(entry["stop"])
    except (KeyError, TypeError, ValueError):
        return 0
    if stop <= start:
        return 0
    return max(0, min(100, int((t - start) * 100 // (stop - start))))


def span_text(entry):
    """'18:00 – 19:30'."""
    return "%s – %s" % (hhmm(entry.get("start")), hhmm(entry.get("stop")))


def length_text(entry):
    try:
        return duration((int(entry["stop"]) - int(entry["start"])) // 60)
    except (KeyError, TypeError, ValueError):
        return ""


def set_valign(widget, top):
    """Channel names sit on the upper line when programmes are shown under
    them, centred otherwise. Old images without setVAlign keep the skin's."""
    try:
        widget.instance.setVAlign(eLabel.alignTop if top else eLabel.alignCenter)
    except (AttributeError, TypeError):
        pass


def poster_cell_skin(s, prefix, i, x, y, w, h, corners="corner", r=T.CORNER, title=False):
    """A poster slot built from cheap parts: the picture over a flat
    placeholder, and four tiny corner masks.

    Grid slots use one placeholder label that carries the title; it stays
    shown under the (opaque) poster, because on a DM520 every show or hide
    costs ~3 ms (Enigma2 recomputes the window's visible regions)."""
    if title:
        out = [s.label("%spt_%d" % (prefix, i), x, y, w, h, 22, T.TEXT_2, halign="center", valign="center", bg=T.SURFACE,
                       z=2, wrap=True)]
    else:
        out = [s.label("%spe_%d" % (prefix, i), x, y, w, h, bg=T.SURFACE, z=1),
               s.pixmap("%spi_%d" % (prefix, i), x + (w - 64) // 2, y + (h - 64) // 2, 64, 64, z=2, asset="icon_film_empty")]
    out.append(s.pixmap("%sp_%d" % (prefix, i), x, y, w, h, z=3, alpha="off"))
    for part, cx, cy in T.corner_rects(x, y, w, h, r):
        out.append(s.pixmap("%sc%s_%d" % (prefix, part, i), cx, cy, r, r, z=4, asset="%s_%s" % (corners, part)))
    return out


CORNER_PARTS = ("tl", "tr", "bl", "br")


def add_cell_components(screen, prefix, i, title=False):
    if title:
        screen[prefix + "pt_%d" % i] = Label("")
    else:
        screen[prefix + "pe_%d" % i] = Label("")
        screen[prefix + "pi_%d" % i] = Pixmap()
    screen[prefix + "p_%d" % i] = Pixmap()
    for part in CORNER_PARTS:
        screen["%sc%s_%d" % (prefix, part, i)] = Pixmap()


def show_corners(screen, prefix, i, on):
    """Corner masks are only right over a slot: elsewhere they would notch
    whatever another view draws there."""
    for part in CORNER_PARTS:
        visible(screen["%sc%s_%d" % (prefix, part, i)], on)


class WidgetState(object):
    """Skips widget calls that change nothing.

    Every Python-to-Enigma2 call costs a fraction of a millisecond on a
    DM520; a grid redraw makes hundreds of them, most repeating the current
    state (a hidden placeholder hidden again, the same title set again).
    """

    def __init__(self, screen):
        self.screen = screen
        self.shown = {}
        self.texts = {}

    def visible(self, name, on):
        if self.shown.get(name) is not on:
            self.shown[name] = on
            visible(self.screen[name], on)

    def text(self, name, value):
        if self.texts.get(name) != value:
            self.texts[name] = value
            self.screen[name].setText(value)

    def forget(self):
        self.shown.clear()
        self.texts.clear()


class PosterGrid(object):
    """A rows x cols window over a catalog list, with a focus ring."""

    def __init__(self, screen, skin, prefix, geometry, ring, images):
        self.screen, self.s, self.p, self.g, self.ring, self.images = screen, skin, prefix, geometry, ring, images
        self.size = geometry["cols"] * geometry["rows"]
        self.text = bool(geometry.get("text"))
        self.model = GridModel(geometry["cols"], geometry["rows"])
        self.listing = PagedList()
        self.source_id = ""
        self.cell_keys = [None] * self.size
        self.cell_pixmaps = [None] * self.size
        self.focused_slot = None
        self.w = WidgetState(screen)
        for i in range(self.size):
            add_cell_components(screen, prefix, i, title=True)
            screen[prefix + "b_%d" % i] = Label("")
            screen[prefix + "f_%d" % i] = Pixmap()
            if self.text:
                screen[prefix + "t_%d" % i] = Label("")
                screen[prefix + "m_%d" % i] = Label("")
        for part in RING_PARTS:
            screen[prefix + "ring_" + part] = Pixmap()

    @staticmethod
    def skin(s, prefix, g, ring):
        out = []
        for i in range(g["cols"] * g["rows"]):
            x, y = T.grid_cell(i, g)
            out.extend(poster_cell_skin(s, prefix, i, x, y, g["pw"], g["ph"], title=True))
            out.append(s.label(prefix + "b_%d" % i, x + 10, y + 10, 104, 30, 18, T.TEXT, bg=T.BG, z=5, halign="center"))
            # Favorite: an opaque tile in the other top corner (no blending over the poster).
            out.append(s.pixmap(prefix + "f_%d" % i, x + g["pw"] - 10 - T.STAR_BADGE, y + 10, T.STAR_BADGE, T.STAR_BADGE, z=5,
                                alpha="off", asset="star_badge"))
            if g.get("text"):
                ty = y + g["ph"] + g["title_gap"]
                out.append(s.label(prefix + "t_%d" % i, x, ty, g["pw"], g["title_h"], 24, T.TEXT_2, valign="top", wrap=True))
                out.append(s.label(prefix + "m_%d" % i, x, ty + g["title_h"] + 2, g["pw"], g["meta_h"], 22, T.MUTED))
        x, y = T.grid_cell(0, g)
        for part, rx, ry, rw, rh in T.ring_rects(x, y, g):
            out.append(s.pixmap(prefix + "ring_" + part, rx, ry, rw, rh, z=6, asset="%s_%s" % (ring, part)))
        return out

    def reset(self, source_id, state=None):
        self.source_id = source_id
        self.listing = PagedList()
        self.model = GridModel(self.g["cols"], self.g["rows"])
        if state:
            self.model.restore(state)
        self.clear()

    def count(self):
        return self.listing.count()

    def item(self):
        return self.listing.get(self.model.focus) if self.count() else None

    def window(self):
        m = self.model
        return max(0, m.first_visible() - m.cols), m.first_visible() + m.cols * (m.rows + 2)

    def clear(self):
        for slot in range(self.size):
            self.fill(slot, None, False)
        self.render_focus(False)

    def show_poster(self, slot, pixmap):
        """The poster covers the placeholder; only its own visibility changes."""
        p = self.p
        if pixmap is not None and self.cell_pixmaps[slot] is not pixmap:
            self.cell_pixmaps[slot] = pixmap
            self.screen[p + "p_%d" % slot].instance.setPixmap(pixmap)
        self.w.visible(p + "p_%d" % slot, pixmap is not None)

    def corners(self, slot, on):
        for part in CORNER_PARTS:
            self.w.visible("%sc%s_%d" % (self.p, part, slot), on)

    def fill(self, slot, item, loading):
        p, w = self.p, self.w
        # The picture this slot waits for (rows may come from several sources).
        self.cell_keys[slot] = self.image_key(item) if item else None
        if item is None:
            w.visible(p + "p_%d" % slot, False)
            if self.text:
                w.text(p + "t_%d" % slot, "")
                w.text(p + "m_%d" % slot, "")
            w.text(p + "pt_%d" % slot, "")
            w.visible(p + "pt_%d" % slot, loading)
            w.visible(p + "b_%d" % slot, False)
            w.visible(p + "f_%d" % slot, False)
            self.corners(slot, loading)
            return
        self.corners(slot, True)
        w.visible(p + "pt_%d" % slot, True)
        # A slot without artwork still says what it is.
        title = clipped(item.get("name"), 60)
        if item.get("missing"):
            title += "\n(нет в плейлисте)"
        w.text(p + "pt_%d" % slot, title)
        if self.text:
            w.text(p + "t_%d" % slot, clipped(item.get("name"), 34))
            w.text(p + "m_%d" % slot, meta_line(item))
        badge = "Сериал" if item.get("kind") == "series" else ""
        if badge:
            w.text(p + "b_%d" % slot, badge)
        w.visible(p + "b_%d" % slot, bool(badge))
        w.visible(p + "f_%d" % slot, bool(item.get("fav")))
        self.show_poster(slot, self.images.get(self.image_key(item)))

    def redraw_item(self, item, focused=True):
        """Draw the card of item again if it is on screen (its favorite mark changed)."""
        start = self.model.first_visible()
        for slot in range(self.size):
            if item is not None and self.listing.get(start + slot) is item:
                self.fill(slot, item, False)
        self.render_focus(focused)

    def image_key(self, item):
        return "%s:%s:poster" % (row_source(item, self.source_id), item.get("key"))

    def render(self, full, focused):
        m = self.model
        count = self.count()
        if full:
            start = m.first_visible()
            for slot in range(self.size):
                index = start + slot
                self.fill(slot, self.listing.get(index) if index < count else None, index < count)
            self.focused_slot = None
        self.render_focus(focused)

    def render_focus(self, focused):
        p, sc = self.p, self.screen
        slot = self.model.slot() if focused and self.count() else None
        if self.text and self.focused_slot is not None and self.focused_slot != slot:
            color(sc, p + "t_%d" % self.focused_slot, T.TEXT_2)
            font(sc, self.s, p + "t_%d" % self.focused_slot, 24, False)
        if slot is not None:
            x, y = T.grid_cell(slot, self.g)
            for part, rx, ry, rw, rh in T.ring_rects(x, y, self.g):
                move(sc, self.s, p + "ring_" + part, rx, ry)
                sc[p + "ring_" + part].show()
            if self.text and slot != self.focused_slot:
                color(sc, p + "t_%d" % slot, T.TEXT)
                font(sc, self.s, p + "t_%d" % slot, 24, True)
        else:
            for part in RING_PARTS:
                sc[p + "ring_" + part].hide()
        self.focused_slot = slot

    def wanted(self, path_for):
        """Picture requests: the focused card first, the screen, one row ahead."""
        m = self.model
        count = self.count()
        start = m.first_visible()
        order = [m.focus] + [i for i in range(start, start + self.size) if i != m.focus]
        order += list(range(start + self.size, start + self.size + m.cols))
        w, h = self.s.px(self.g["pw"]), self.s.px(self.g["ph"])
        out = []
        for index in order:
            item = self.listing.get(index) if index < count else None
            if item and item.get("hasPoster"):
                out.append((self.image_key(item), path_for(item), w, h, self.poster_ready, "cover"))
        return out

    def poster_ready(self, key, pixmap):
        for slot, wanted in enumerate(self.cell_keys):
            if wanted == key:
                self.show_poster(slot, pixmap)


class ChannelList(object):
    """Live TV: compact rows with logos and what is on now, and an optional
    panel with the focused channel's programme (now and next)."""

    def __init__(self, screen, skin, prefix, geometry, images, preview=True):
        self.screen, self.s, self.p, self.g, self.images, self.preview = screen, skin, prefix, geometry, images, preview
        self.rows = geometry["rows"]
        self.model = ListModel(self.rows)
        self.listing = PagedList()
        self.source_id = ""
        self.row_keys = [None] * self.rows
        self.preview_key = None
        self.guide_key = None
        self.guide = None
        self.two_line = False
        self.lit_row = None  # the row drawn in focus colours
        self.values = {}
        self.name_colors = {}  # row -> colour of its name (cached, see paint_name)
        self.w = WidgetState(screen)
        for i in range(self.rows):
            screen[prefix + "n_%d" % i] = Label("")
            screen[prefix + "lp_%d" % i] = Label("")
            screen[prefix + "li_%d" % i] = Pixmap()
            screen[prefix + "l_%d" % i] = Pixmap()
            screen[prefix + "c_%d" % i] = Label("")
            screen[prefix + "f_%d" % i] = Pixmap()
            screen[prefix + "e_%d" % i] = Label("")
            screen[prefix + "g_%d" % i] = ProgressBar()
        screen[prefix + "pill"] = Pixmap()
        if preview:
            for name in PREVIEW_TEXTS:
                screen[prefix + name] = Label("")
            screen[prefix + "pv_bar"] = ProgressBar()
            screen[prefix + "pv_logo"] = Pixmap()
            screen[prefix + "pv_icon"] = Pixmap()
            screen[prefix + "pv_panel"] = Pixmap()

    @staticmethod
    def skin(s, prefix, g, preview=True):
        out = [s.pixmap(prefix + "pill", g["x"], g["y"], g["w"], g["row"], z=1, asset=g.get("pill", "pill_live"))]
        lw, lh = g["logo_w"], g["logo_h"]
        text_x, text_w = g["x"] + 88 + lw + 24, g["w"] - lw - 136
        for i in range(g["rows"]):
            y = g["y"] + i * g["pitch"]
            out.append(s.label(prefix + "n_%d" % i, g["x"] + 12, y, 60, g["row"], 24, T.MUTED, halign="right"))
            ly = y + (g["row"] - lh) // 2
            out.append(s.label(prefix + "lp_%d" % i, g["x"] + 88, ly, lw, lh, bg=T.SURFACE, z=2))
            out.append(s.pixmap(prefix + "li_%d" % i, g["x"] + 88 + (lw - 32) // 2, ly + (lh - 32) // 2, 32, 32, z=3, asset="icon_live_muted"))
            out.append(s.pixmap(prefix + "l_%d" % i, g["x"] + 88, ly, lw, lh, z=3, alpha="off"))
            # Centred alone; on the upper line when a programme is shown under it.
            out.append(s.label(prefix + "c_%d" % i, text_x, y + 4, text_w - T.STAR_ROW - 16, g["row"] - 8, 28, T.TEXT))
            out.append(s.label(prefix + "e_%d" % i, text_x, y + 40, text_w - 132, 28, 22, T.MUTED))
            out.append(s.progress(prefix + "g_%d" % i, g["x"] + g["w"] - 136, y + 51, 112, 6))
            # Favorite: a small star at the end of the row, above the progress bar
            # (over the row, never over a picture).
            out.append(s.pixmap(prefix + "f_%d" % i, g["x"] + g["w"] - 24 - T.STAR_ROW, y + 14, T.STAR_ROW, T.STAR_ROW, z=3,
                                asset="star_row"))
        if preview:
            v = T.LIVE_PREVIEW
            x, w = v["x"] + 20, v["w"] - 40
            out.append(s.pixmap(prefix + "pv_panel", v["x"], v["y"], v["w"], v["h"], z=1, asset="panel_live"))
            lx, ly = v["x"] + (v["w"] - v["logo_w"]) // 2, v["y"] + 40
            out.append(s.label(prefix + "pv_plate", lx, ly, v["logo_w"], v["logo_h"], bg=T.SURFACE, z=2))
            out.append(s.pixmap(prefix + "pv_icon", lx + (v["logo_w"] - 64) // 2, ly + (v["logo_h"] - 64) // 2, 64, 64, z=3, asset="icon_live_empty"))
            out.append(s.pixmap(prefix + "pv_logo", lx, ly, v["logo_w"], v["logo_h"], z=3, alpha="off"))
            # Two lines: long playlist names ("Setanta Sports Qazaqstan HD") stay whole.
            y = ly + v["logo_h"] + 14
            out.append(s.label(prefix + "pv_name", x, y, w, 84, 32, T.TEXT, semi=True, halign="center", valign="center", wrap=True, z=2))
            out.append(s.label(prefix + "pv_info", x, y + 86, w, 30, 22, T.MUTED, halign="center", z=2))
            y += 142
            out.append(s.label(prefix + "pv_now_time", x, y, w, 28, 22, T.ACCENT, z=2))
            out.append(s.label(prefix + "pv_now_title", x, y + 30, w, 70, 28, T.TEXT, semi=True, valign="top", wrap=True, z=2))
            out.append(s.progress(prefix + "pv_bar", x, y + 106, w, 6))
            out.append(s.label(prefix + "pv_now_desc", x, y + 124, w, 108, 22, T.TEXT_2, valign="top", wrap=True, z=2))
            y += 254
            out.append(s.label(prefix + "pv_next_time", x, y, w, 28, 22, T.MUTED, z=2))
            out.append(s.label(prefix + "pv_next_title", x, y + 30, w, 36, 26, T.TEXT, z=2))
            out.append(s.label(prefix + "pv_next_desc", x, y + 68, w, 56, 20, T.MUTED, valign="top", wrap=True, z=2))
            # Two lines: OK and EPG, then the favorites key.
            out.append(s.label(prefix + "pv_hint", x, v["y"] + v["h"] - 90, w, 64, 22, T.TEXT_2, halign="center", wrap=True, z=2))
        return out

    def reset(self, source_id, state=None):
        self.source_id = source_id
        self.listing = PagedList()
        self.model = ListModel(self.rows)
        if state:
            self.model.restore(state)
        self.set_two_line(False)
        self.clear()

    def count(self):
        return self.listing.count()

    def item(self):
        return self.listing.get(self.model.focus) if self.count() else None

    def window(self):
        m = self.model
        return max(0, m.top - self.rows), m.top + 2 * self.rows

    def visible_items(self):
        m, count = self.model, self.count()
        return [self.listing.get(i) for i in range(m.top, min(count, m.top + self.rows))]

    def clear(self):
        for i in range(self.rows):
            self.fill(i, None)
        self.screen[self.p + "pill"].hide()
        if self.preview:
            self.show_preview(None, False)

    def set_two_line(self, on):
        if on == self.two_line:
            return
        self.two_line = on
        for i in range(self.rows):
            set_valign(self.screen[self.p + "c_%d" % i], on)

    def fill(self, row, item, t=None):
        p, sc = self.p, self.screen
        self.row_keys[row] = self.image_key(item, "row") if item else None
        if item is None:
            for name in ("n_%d", "c_%d"):
                sc[p + name % row].setText("")
            sc[p + "lp_%d" % row].hide()
            sc[p + "li_%d" % row].hide()
            sc[p + "l_%d" % row].hide()
            self.w.visible(p + "f_%d" % row, False)
            self.fill_now(row, None, 0)
            return
        sc[p + "n_%d" % row].setText(ui_api.text(item.get("number") or ""))
        if item.get("missing"):
            sc[p + "c_%d" % row].setText(clipped(item.get("name"), 22) + " · нет в плейлисте")
        else:
            sc[p + "c_%d" % row].setText(clipped(item.get("name"), 38))
        # Cached: a show or hide costs ~3 ms on a DM520, most rows keep their state.
        self.w.visible(p + "f_%d" % row, bool(item.get("fav")))
        self.paint_name(row, item)
        self.show_logo(row, self.images.get(self.image_key(item, "row")) if item.get("hasPoster") else None)
        self.fill_now(row, item, time.time() if t is None else t)

    def paint_name(self, row, item):
        """The name's colour: focus, grey when gone from the playlist, or plain."""
        if row == self.lit_row:
            want = T.ON_ACCENT
        else:
            want = T.MUTED if item and item.get("missing") else T.TEXT
        if self.name_colors.get(row) != want:
            self.name_colors[row] = want
            color(self.screen, self.p + "c_%d" % row, want)

    def fill_now(self, row, item, t):
        """What is on now under the name, with its progress."""
        p, w = self.p, self.w
        now = item.get("now") if item else None
        if not on_air(now, t):
            w.text(p + "e_%d" % row, "")
            w.visible(p + "g_%d" % row, False)
            return
        w.text(p + "e_%d" % row, clipped(now.get("title"), 34))
        self.set_value(p + "g_%d" % row, progress(now, t))
        w.visible(p + "g_%d" % row, True)

    def set_value(self, name, value):
        if self.values.get(name) != value:
            self.values[name] = value
            self.screen[name].setValue(value)

    def show_logo(self, row, pixmap):
        p, sc = self.p, self.screen
        logo = sc[p + "l_%d" % row]
        if pixmap is not None:
            logo.instance.setPixmap(pixmap)
            logo.show()
        else:
            logo.hide()
        visible(sc[p + "lp_%d" % row], pixmap is None)
        visible(sc[p + "li_%d" % row], pixmap is None)

    def image_key(self, item, size):
        return "%s:%s:logo-%s" % (row_source(item, self.source_id), item.get("key"), size)

    def redraw_item(self, item, focused=True):
        """Draw the row of item again if it is on screen (its favorite mark changed)."""
        m = self.model
        for row in range(self.rows):
            if item is not None and self.listing.get(m.top + row) is item:
                self.fill(row, item)
        self.preview_key = None
        self.render(False, focused)

    def render(self, full, focused):
        m = self.model
        count = self.count()
        p, sc = self.p, self.screen
        t = time.time()
        if full:
            if not self.two_line and any(item and item.get("now") for item in self.visible_items()):
                self.set_two_line(True)
            for i in range(self.rows):
                index = m.top + i
                self.fill(i, self.listing.get(index) if index < count else None, t)
        slot = m.focus - m.top if count else None
        # Only the rows whose focus changed are recoloured: every call into
        # Enigma2 counts on a DM520, and a list has 11 rows of 3 texts.
        lit = slot if focused else None
        if lit != self.lit_row:
            old, self.lit_row = self.lit_row, lit
            for i in (old, lit):
                if i is not None:
                    on = i == lit
                    self.paint_name(i, self.listing.get(m.top + i) if m.top + i < count else None)
                    color(sc, p + "n_%d" % i, T.ON_ACCENT if on else T.MUTED)
                    color(sc, p + "e_%d" % i, T.ON_ACCENT if on else T.MUTED)
        if focused and slot is not None:
            move(sc, self.s, p + "pill", self.g["x"], self.g["y"] + slot * self.g["pitch"])
            sc[p + "pill"].show()
        else:
            sc[p + "pill"].hide()
        if self.preview:
            self.show_preview(self.item(), focused, t)

    def tick(self, t):
        """Once a minute: move the progress bars on. Returns the keys of
        rows whose programme has ended, so the screen can ask what is next."""
        ended = []
        for row, item in enumerate(self.visible_items()):
            now = item.get("now") if item else None
            if now and not on_air(now, t):
                ended.append(item["key"])
            self.fill_now(row, item, t)
        if self.preview:
            item = self.item()
            if item is not None:
                self.show_programme(item, t)
        return ended

    def update_now(self, keys, answer):
        """Fresh 'now' for the rows asked about (absent: nothing known)."""
        asked = set(keys)
        for row, item in enumerate(self.visible_items()):
            if item and item.get("key") in asked:
                if item["key"] in answer:
                    item["now"] = answer[item["key"]]
                else:
                    item.pop("now", None)
        if not self.two_line and any(item and item.get("now") for item in self.visible_items()):
            self.set_two_line(True)
        t = time.time()
        for row, item in enumerate(self.visible_items()):
            self.fill_now(row, item, t)

    def show_preview(self, item, focused, t=None):
        p, sc = self.p, self.screen
        if item is None:
            for name in PREVIEW_TEXTS:
                self.w.text(p + name, "")
            self.w.visible(p + "pv_bar", False)
            sc[p + "pv_logo"].hide()
            sc[p + "pv_icon"].hide()
            visible(sc[p + "pv_plate"], False)
            visible(sc[p + "pv_panel"], False)
            self.preview_key = None
            return
        sc[p + "pv_panel"].show()
        self.w.text(p + "pv_name", clipped(item.get("name"), 44))
        archive = 0
        if item.get("missing"):
            info = "Нет в плейлисте"
        else:
            info = "Канал %s" % ui_api.text(item.get("number") or "") if item.get("number") else ""
            try:
                archive = int(item.get("archive") or 0)
            except (TypeError, ValueError):
                pass
            if archive > 0:
                info += (" · " if info else "") + "Архив " + days_text(archive)
        self.w.text(p + "pv_info", info)
        hint = ""
        if focused:
            held = "OK долго — убрать из избранного" if item.get("fav") else "OK долго — в избранное"
            guide = "EPG — программа и архив" if archive > 0 else "EPG — программа"
            hint = held if item.get("missing") else "OK — смотреть · " + guide + "\n" + held
        self.w.text(p + "pv_hint", hint)
        key = self.image_key(item, "big")
        if key != self.preview_key:
            self.preview_key = key
            self.show_big_logo(self.images.get(key) if item.get("hasPoster") else None)
        self.show_programme(item, time.time() if t is None else t)

    def show_guide(self, key, result):
        """The focused channel's programme arrived (None: it failed)."""
        self.guide_key, self.guide = key, result
        item = self.item()
        if self.preview and item is not None and item.get("key") == key:
            self.show_programme(item, time.time())

    def show_programme(self, item, t):
        """Now and next of the focused channel: from its row at once, with
        descriptions when the service answered for this channel."""
        p, w = self.p, self.w
        guide = self.guide if self.guide_key == item.get("key") else {}
        failed = self.guide_key == item.get("key") and guide is None
        guide = guide or {}
        now = guide.get("now") if guide.get("now") and on_air(guide.get("now"), t) else item.get("now")
        if not on_air(now, t):
            now = None
        upcoming = guide.get("next")
        if now and upcoming and int(upcoming.get("start", 0)) < int(now.get("stop", 0)):
            upcoming = None
        if now is None and guide.get("now") and not on_air(guide["now"], t) and upcoming and on_air(upcoming, t):
            now, upcoming = upcoming, None  # the answer is a little old: next is on now
        if now:
            w.text(p + "pv_now_time", "Сейчас · " + span_text(now))
            w.text(p + "pv_now_title", clipped(now.get("title"), 60))
            self.set_value(p + "pv_bar", progress(now, t))
            w.visible(p + "pv_bar", True)
            w.text(p + "pv_now_desc", clipped(now.get("desc"), 150))
        else:
            if failed:
                status = "Программа не загрузилась"
            elif self.guide_key == item.get("key"):
                status = "Нет программы"
            else:
                status = ""
            w.text(p + "pv_now_time", status)
            w.text(p + "pv_now_title", "")
            w.visible(p + "pv_bar", False)
            w.text(p + "pv_now_desc", "")
        if upcoming:
            w.text(p + "pv_next_time", "Далее · " + hhmm(upcoming.get("start")))
            w.text(p + "pv_next_title", clipped(upcoming.get("title"), 34))
            w.text(p + "pv_next_desc", clipped(upcoming.get("desc"), 80))
        else:
            for name in ("pv_next_time", "pv_next_title", "pv_next_desc"):
                w.text(p + name, "")

    def show_big_logo(self, pixmap):
        p, sc = self.p, self.screen
        if pixmap is not None:
            sc[p + "pv_logo"].instance.setPixmap(pixmap)
            sc[p + "pv_logo"].show()
        else:
            sc[p + "pv_logo"].hide()
        visible(sc[p + "pv_plate"], pixmap is None)
        visible(sc[p + "pv_icon"], pixmap is None)

    def wanted(self, path_for):
        m = self.model
        count = self.count()
        out = []
        focused = self.item()
        if self.preview and focused and focused.get("hasPoster"):
            v = T.LIVE_PREVIEW
            out.append((self.image_key(focused, "big"), path_for(focused), self.s.px(v["logo_w"]), self.s.px(v["logo_h"]), self.big_ready))
        w, h = self.s.px(self.g["logo_w"]), self.s.px(self.g["logo_h"])
        for index in range(m.top, min(count, m.top + self.rows + 3)):
            item = self.listing.get(index)
            if item and item.get("hasPoster"):
                out.append((self.image_key(item, "row"), path_for(item), w, h, self.logo_ready))
        return out

    def logo_ready(self, key, pixmap):
        for row, wanted in enumerate(self.row_keys):
            if wanted == key:
                self.show_logo(row, pixmap)

    def big_ready(self, key, pixmap):
        if key == self.preview_key:
            self.show_big_logo(pixmap)


PREVIEW_TEXTS = ("pv_plate", "pv_name", "pv_info", "pv_hint", "pv_now_time", "pv_now_title", "pv_now_desc", "pv_next_time",
                 "pv_next_title", "pv_next_desc")


class OverlayMenu(object):
    """A short choice drawn inside the screen (sort order, portal).

    A separate Enigma2 screen would hide the catalog and show live TV
    behind the dialog. The panel is a flat label: a translucent PNG over
    posters would be slow to blend on a DM520.
    """
    ROWS = 6
    X, Y, W = 640, 300, 640

    def __init__(self, screen, skin):
        self.screen, self.s = screen, skin
        self.options = []
        self.focus = 0
        self.top = 0
        self.callback = None
        for name in ("om_panel", "om_title", "om_sub", "om_pill", "om_hint"):
            screen[name] = Label("")
        for i in range(self.ROWS):
            screen["om_row_%d" % i] = Label("")
            screen["om_det_%d" % i] = Label("")

    @classmethod
    def skin(cls, s):
        x, y, w = cls.X, cls.Y, cls.W
        h = 140 + cls.ROWS * 66 + 70
        out = ['<widget name="om_panel" position="%s" size="%s" backgroundColor="%s" borderWidth="%d" borderColor="%s" zPosition="9" />'
               % (s.pos(x, y), s.size(w, h), T.RAIL, max(1, s.px(2)), T.LINE),
               s.label("om_title", x + 40, y + 28, w - 80, 48, 34, T.TEXT, semi=True, bg=T.RAIL, z=10),
               s.label("om_sub", x + 40, y + 78, w - 80, 30, 22, T.MUTED, bg=T.RAIL, z=10),
               s.label("om_pill", x + 32, y + 130, w - 64, 60, bg=T.ACCENT, z=10)]
        for i in range(cls.ROWS):
            out.append(s.label("om_row_%d" % i, x + 56, y + 130 + i * 66, w - 112, 60, 28, T.TEXT, z=11))
            out.append(s.label("om_det_%d" % i, x + w - 276, y + 130 + i * 66, 220, 60, 22, T.MUTED, halign="right", z=11))
        out.append(s.label("om_hint", x + 40, y + h - 56, w - 80, 30, 22, T.MUTED, bg=T.RAIL, z=10))
        return out

    def is_open(self):
        return self.callback is not None

    def open(self, title, subtitle, options, current, callback):
        self.options, self.callback = options, callback
        ids = [o.get("id") for o in options]
        self.focus = ids.index(current) if current in ids else 0
        self.top = max(0, self.focus - self.ROWS + 1)
        sc = self.screen
        sc["om_title"].setText(ui_api.text(title))
        sc["om_sub"].setText(ui_api.text(subtitle))
        sc["om_hint"].setText("OK — выбрать · Back — отмена")
        # The panel fits the choice: four orders need no empty rows.
        rows = min(self.ROWS, len(options))
        height = 140 + rows * 66 + 70
        sc["om_panel"].instance.resize(eSize(self.s.px(self.W), self.s.px(height)))
        move(sc, self.s, "om_hint", self.X + 40, self.Y + height - 56)
        for name in ("om_panel", "om_title", "om_sub", "om_pill", "om_hint"):
            sc[name].show()
        self.render()

    def render(self):
        sc = self.screen
        if self.focus < self.top:
            self.top = self.focus
        elif self.focus >= self.top + self.ROWS:
            self.top = self.focus - self.ROWS + 1
        for i in range(self.ROWS):
            index = self.top + i
            name, det = "om_row_%d" % i, "om_det_%d" % i
            if index < len(self.options):
                option = self.options[index]
                # A right-hand note (the kind of source) leaves less room for the name.
                sc[name].setText(clipped(option.get("name"), 24 if option.get("detail") else 34))
                sc[det].setText(ui_api.text(option.get("detail") or ""))
                color(sc, name, T.ON_ACCENT if index == self.focus else T.TEXT)
                color(sc, det, T.ON_ACCENT if index == self.focus else T.MUTED)
                sc[name].show()
                sc[det].show()
            else:
                sc[name].hide()
                sc[det].hide()
        move(sc, self.s, "om_pill", self.X + 32, self.Y + 130 + (self.focus - self.top) * 66)

    def move(self, dy):
        self.focus = max(0, min(len(self.options) - 1, self.focus + dy))
        self.render()

    def close(self, choose=False):
        callback, option = self.callback, self.options[self.focus] if choose and self.options else None
        self.callback = None
        sc = self.screen
        for name in ("om_panel", "om_title", "om_sub", "om_pill", "om_hint"):
            sc[name].hide()
        for i in range(self.ROWS):
            sc["om_row_%d" % i].hide()
            sc["om_det_%d" % i].hide()
        if callback and option:
            callback(option)
