# -*- coding: utf-8 -*-
"""Navigation models of the TV catalog, free of Enigma2 and the network.

Screens own the widgets and the requests; these classes decide what is
focused, what is visible and which pages are still missing, so every
remote-control rule can be tested on a laptop.
"""
from __future__ import absolute_import, division

from collections import OrderedDict

# Results of a move: either the focus moved, or it hit an edge the screen
# may turn into a zone change (grid -> categories -> rail).
MOVED, EDGE_LEFT, EDGE_RIGHT, EDGE_UP, EDGE_DOWN, STAY = "moved", "left", "right", "up", "down", "stay"


class PagedList(object):
    """One catalog list as the service pages it (portal pages, 14-50 rows)."""

    def __init__(self):
        self.pages = {}
        self.page_size = 0
        self.total = None
        self.end = None  # page number past the last one, once known
        self.loading = set()
        self.failed = None

    def count(self):
        """Items that can be focused now (exact when the portal said)."""
        if self.total is not None:
            return self.total
        if not self.page_size:
            return 0
        n, count = 1, 0
        while n in self.pages:
            count += len(self.pages[n])
            if len(self.pages[n]) < self.page_size:
                break
            n += 1
        return count

    def complete(self):
        return self.total is not None or (self.end is not None and all(n in self.pages for n in range(1, self.end)))

    def get(self, index):
        if not self.page_size or index < 0:
            return None
        rows = self.pages.get(index // self.page_size + 1)
        offset = index % self.page_size
        if rows is None or offset >= len(rows):
            return None
        return rows[offset]

    def missing(self, start, stop):
        """Page numbers needed to show items [start, stop), first page first."""
        if not self.page_size:
            return [] if 1 in self.loading or self.failed else [1]
        last = stop - 1
        if self.total is not None:
            last = min(last, self.total - 1)
        elif self.end is None:
            # Unknown length: the page after the loaded run is the only guess.
            last = min(last, self.count())
        out = []
        for n in range(max(0, start) // self.page_size + 1, max(0, last) // self.page_size + 2):
            if self.end is not None and n >= self.end:
                break
            if n not in self.pages and n not in self.loading and n not in out:
                out.append(n)
        return out

    def store(self, n, result):
        """Keep a service page ({items, hasMore, pageSize, total})."""
        self.loading.discard(n)
        self.failed = None
        items = list(result.get("items") or [])
        size = int(result.get("pageSize") or 0)
        if not self.page_size:
            self.page_size = size or max(1, len(items))
        total = int(result.get("total") or 0)
        if total > 0:
            self.total = total
        self.pages[n] = items
        if not result.get("hasMore") or not items:
            self.end = n + 1 if items else n
            if self.total is None or not items:
                self.total = sum(len(self.pages.get(i, [])) for i in range(1, self.end))

    def fail(self, n, error):
        self.loading.discard(n)
        self.failed = error


class GridModel(object):
    """Focus and visible window of a poster grid; rows scroll one at a time."""

    def __init__(self, cols, rows):
        self.cols, self.rows = cols, rows
        self.focus = 0
        self.top = 0

    def first_visible(self):
        return self.top * self.cols

    def visible(self):
        start = self.first_visible()
        return range(start, start + self.cols * self.rows)

    def slot(self):
        return self.focus - self.first_visible()

    def _show_focus(self):
        row = self.focus // self.cols
        if row < self.top:
            self.top = row
        elif row >= self.top + self.rows:
            self.top = row - self.rows + 1

    def move(self, dx, dy, count):
        if count <= 0:
            return EDGE_LEFT if dx < 0 else STAY
        col, row = self.focus % self.cols, self.focus // self.cols
        if dx < 0:
            if col == 0:
                return EDGE_LEFT
            self.focus -= 1
        elif dx > 0:
            if col == self.cols - 1 or self.focus + 1 >= count:
                return EDGE_RIGHT
            self.focus += 1
        elif dy < 0:
            if row == 0:
                return EDGE_UP
            self.focus -= self.cols
        elif dy > 0:
            last_row = (count - 1) // self.cols
            if row >= last_row:
                return EDGE_DOWN
            self.focus = min(self.focus + self.cols, count - 1)
        self._show_focus()
        return MOVED

    def page(self, delta, count):
        """Turn a whole screen of rows, keeping the focused slot."""
        if count <= 0:
            return STAY
        last_row = (count - 1) // self.cols
        top = max(0, min(self.top + delta * self.rows, max(0, last_row - self.rows + 1)))
        focus = max(0, min(self.focus + (top - self.top) * self.cols, count - 1))
        if top == self.top and focus == self.focus:
            # Already at the edge: land on the first/last item instead.
            focus = 0 if delta < 0 else count - 1
            if focus == self.focus:
                return STAY
        self.top, self.focus = top, focus
        self._show_focus()
        return MOVED

    def clamp(self, count):
        if count <= 0:
            self.focus = self.top = 0
            return
        self.focus = min(self.focus, count - 1)
        last_row = (count - 1) // self.cols
        self.top = max(0, min(self.top, max(0, last_row - self.rows + 1)))
        self._show_focus()

    def state(self):
        return (self.focus, self.top)

    def restore(self, state):
        self.focus, self.top = state


class ListModel(GridModel):
    """A vertical list is a one-column grid."""

    def __init__(self, rows):
        GridModel.__init__(self, 1, rows)


class PixmapCache(object):
    """Decoded pictures with a byte budget; least recently shown go first."""

    def __init__(self, budget):
        self.budget = budget
        self.used = 0
        self.items = OrderedDict()

    def get(self, key):
        entry = self.items.pop(key, None)
        if entry is None:
            return None
        self.items[key] = entry
        return entry[0]

    def put(self, key, pixmap, size, keep=()):
        old = self.items.pop(key, None)
        if old is not None:
            self.used -= old[1]
        self.items[key] = (pixmap, size)
        self.used += size
        for victim in list(self.items):
            if self.used <= self.budget:
                break
            if victim == key or victim in keep:
                continue
            self.used -= self.items.pop(victim)[1]

    def clear(self):
        self.items.clear()
        self.used = 0


LAYOUTS = {
    "ru": u"абвгдеёжзийклмнопрстуфхцчшщъыьэюя-:",
    "en": u"abcdefghijklmnopqrstuvwxyz-:",
    "num": u"1234567890.,!?'&()+#№/",
}
SPECIAL = [("space", u"Пробел"), ("erase", u"⌫"), ("layout", u"ABC")]


class Keyboard(object):
    """On-screen keyboard: a letter grid plus a row of wide keys."""

    def __init__(self, cols=7):
        self.cols = cols
        self.layout = "ru"
        self.focus = 0
        self.text = u""

    def keys(self):
        return list(LAYOUTS[self.layout])

    def rows(self):
        return (len(self.keys()) + self.cols - 1) // self.cols + 1

    def special_row(self):
        return self.rows() - 1

    def current(self):
        """("char", value) or (special_id, label) under the focus."""
        keys = self.keys()
        if self.focus < len(keys):
            return ("char", keys[self.focus])
        index = min(self.focus - self.special_start(), len(SPECIAL) - 1)
        return SPECIAL[index]

    def special_start(self):
        return (self.rows() - 1) * self.cols

    def position(self):
        keys = len(self.keys())
        if self.focus < keys:
            return self.focus % self.cols, self.focus // self.cols
        return self.focus - self.special_start(), self.special_row()

    def move(self, dx, dy):
        col, row = self.position()
        keys = len(self.keys())
        if row == self.special_row():
            if dx:
                target = col + dx
                if target < 0:
                    return EDGE_LEFT
                if target >= len(SPECIAL):
                    return EDGE_RIGHT
                self.focus = self.special_start() + target
            elif dy < 0:
                # Wide keys sit under columns 0-2, 3-4 and 5-6.
                column = (0, 3, 5)[col]
                last_row_start = (keys - 1) // self.cols * self.cols
                self.focus = min(last_row_start + column, keys - 1)
            else:
                return EDGE_DOWN
            return MOVED
        if dx < 0:
            if col == 0:
                return EDGE_LEFT
            self.focus -= 1
        elif dx > 0:
            if col == self.cols - 1 or self.focus + 1 >= keys:
                return EDGE_RIGHT
            self.focus += 1
        elif dy < 0:
            if row == 0:
                return EDGE_UP
            self.focus -= self.cols
        elif dy > 0:
            if self.focus + self.cols < keys:
                self.focus += self.cols
            else:
                self.focus = self.special_start() + (0 if col < 3 else 1 if col < 5 else 2)
        return MOVED

    def press(self):
        """Apply the focused key; returns True when the text changed."""
        kind, value = self.current()
        if kind == "char":
            if len(self.text) >= 40:
                return False
            self.text += value
            return True
        if kind == "space":
            if self.text and not self.text.endswith(u" ") and len(self.text) < 40:
                self.text += u" "
                return True
            return False
        if kind == "erase":
            return self.erase()
        self.layout = {"ru": "en", "en": "num", "num": "ru"}[self.layout]
        self.focus = min(self.focus, self.special_start() + 2)
        return False

    def erase(self):
        if not self.text:
            return False
        self.text = self.text[:-1]
        return True

    def type_digit(self, digit):
        if len(self.text) < 40:
            self.text += digit
            return True
        return False

    def layout_label(self):
        return {"ru": u"ABC", "en": u"123", "num": u"АБВ"}[self.layout]
