# -*- coding: utf-8 -*-
"""Search inside the shell: query field, on-screen keyboard, scope, results.

Searches the selected portal and section only (never TMDB). The status line
says what was actually searched: the portal itself, the full channel list,
or — when the portal ignores searches — only the pages already opened.
"""
from __future__ import absolute_import, division

from enigma import eTimer
from Components.Label import Label
from Components.Pixmap import Pixmap

try:
    from . import ui_api, ui_theme as T
    from .ui_models import EDGE_DOWN, EDGE_LEFT, EDGE_RIGHT, EDGE_UP, MOVED, Keyboard, SPECIAL
    from .ui_widgets import ChannelList, PosterGrid, color, font, move, visible
except (ImportError, ValueError):
    import ui_api
    import ui_theme as T
    from ui_models import EDGE_DOWN, EDGE_LEFT, EDGE_RIGHT, EDGE_UP, MOVED, Keyboard, SPECIAL
    from ui_widgets import ChannelList, PosterGrid, color, font, move, visible

K = T.KEYBOARD
KEY_ROWS = 5
WIDE_W = 179
SCOPE_Y = 712
DEBOUNCE_MS = 700
TYPE = {"vod": "vod", "series": "series", "live": "itv"}


def key_pos(col, row):
    return K["x"] + col * (K["key_w"] + K["gap"]), K["y"] + row * (K["key_h"] + K["gap"])


def wide_pos(index):
    return K["x"] + index * (WIDE_W + K["gap"]), K["y"] + KEY_ROWS * (K["key_h"] + K["gap"])


class SearchPanel(object):
    """Owns the search components of the shell screen (prefix 'q')."""

    def __init__(self, shell):
        self.shell = shell
        sc, s = shell, shell.s
        self.kb = Keyboard(K["cols"])
        self.scope = "vod"
        self.scope_focus = 0
        self.query = u""
        self.generation = 0
        self.state = "idle"  # idle | short | loading | done | error
        self.result = None
        self.grid = PosterGrid(sc, s, "s", T.SEARCH_GRID, "ring_search", shell.images)
        self.live = ChannelList(sc, s, "sl", T.SEARCH_LIVE, shell.images, preview=False)
        sc["q_field"] = Pixmap()
        sc["q_text"] = Label("")
        sc["q_status"] = Label("")
        sc["q_status2"] = Label("")
        for i in range(K["cols"] * KEY_ROWS):
            sc["k_%d" % i] = Label("")
            sc["kp_%d" % i] = Pixmap()
        for i in range(len(SPECIAL)):
            sc["kw_%d" % i] = Label("")
            sc["kwp_%d" % i] = Pixmap()
        sc["k_focus"] = Pixmap()
        sc["kw_focus"] = Pixmap()
        for i in range(len(T.SEARCH_SCOPES)):
            sc["sc_%d" % i] = Label("")
            sc["scp_%d" % i] = Pixmap()
            sc["scf_%d" % i] = Pixmap()
        self.timer = eTimer()
        self.timer.callback.append(self.run)

    @staticmethod
    def skin(s):
        out = [s.pixmap("q_field", K["x"], 168, 552, 76, z=1, asset="field"),
               s.label("q_text", K["x"] + 24, 168, 504, 76, 34, T.TEXT, z=2),
               s.label("q_status", 820, 152, 1020, 40, 26, T.TEXT, semi=True),
               s.label("q_status2", 820, 190, 1020, 32, 22, T.MUTED)]
        for i in range(K["cols"] * KEY_ROWS):
            x, y = key_pos(i % K["cols"], i // K["cols"])
            out.append(s.pixmap("kp_%d" % i, x, y, K["key_w"], K["key_h"], z=1, asset="plate_key"))
            out.append(s.label("k_%d" % i, x, y, K["key_w"], K["key_h"], 26, T.TEXT, halign="center", z=3))
        for i in range(len(SPECIAL)):
            x, y = wide_pos(i)
            out.append(s.pixmap("kwp_%d" % i, x, y, WIDE_W, K["key_h"], z=1, asset="plate_key_wide"))
            out.append(s.label("kw_%d" % i, x, y, WIDE_W, K["key_h"], 24, T.TEXT, halign="center", z=3))
        out.append(s.pixmap("k_focus", K["x"], K["y"], K["key_w"], K["key_h"], z=2, asset="pill_key"))
        out.append(s.pixmap("kw_focus", K["x"], K["y"], WIDE_W, K["key_h"], z=2, asset="pill_key_wide"))
        for i, (_, label) in enumerate(T.SEARCH_SCOPES):
            x = K["x"] + i * (176 + 12)
            out.append(s.pixmap("scp_%d" % i, x, SCOPE_Y, 176, 52, z=1, asset="chip_scope"))
            out.append(s.pixmap("scf_%d" % i, x, SCOPE_Y, 176, 52, z=2, asset="chip_scope_focus"))
            out.append(s.label("sc_%d" % i, x, SCOPE_Y, 176, 52, 22, T.TEXT_2, halign="center", z=3))
        out.extend(PosterGrid.skin(s, "s", T.SEARCH_GRID, "ring_search"))
        out.extend(ChannelList.skin(s, "sl", T.SEARCH_LIVE, preview=False))
        return out

    # -------------------------------------------------------------- visibility

    def components(self):
        names = ["q_field", "q_text", "q_status", "q_status2", "k_focus", "kw_focus"]
        names += ["k_%d" % i for i in range(K["cols"] * KEY_ROWS)] + ["kp_%d" % i for i in range(K["cols"] * KEY_ROWS)]
        names += ["kw_%d" % i for i in range(len(SPECIAL))] + ["kwp_%d" % i for i in range(len(SPECIAL))]
        for i in range(len(T.SEARCH_SCOPES)):
            names += ["sc_%d" % i, "scp_%d" % i, "scf_%d" % i]
        return names

    def hide(self):
        self.timer.stop()
        for name in self.components():
            self.shell[name].hide()
        self.grid.clear()
        self.live.clear()

    def scopes(self):
        """The sections of the current source: a playlist has Live TV only."""
        return [(key, label) for key, label in T.SEARCH_SCOPES if self.shell.has(key)]

    def source_changed(self):
        keys = [key for key, _ in self.scopes()]
        if self.scope not in keys:
            self.scope = keys[0]
            self.reset_results()

    def open(self, scope):
        if scope in TYPE and scope != self.scope and self.shell.has(scope):
            self.scope = scope
            self.reset_results()
        for name in ("q_field", "q_text", "q_status", "q_status2"):
            self.shell[name].show()
        self.render_keys()
        self.render_scope()
        self.render_query()
        self.render_status()
        self.content().render(True, self.shell.zone == "content")

    def content(self):
        return self.live if self.scope == "live" else self.grid

    def other(self):
        return self.grid if self.scope == "live" else self.live

    # -------------------------------------------------------------- rendering

    def render_keys(self):
        sc = self.shell
        keys = self.kb.keys()
        focused = sc.zone == "keys"
        col, row = self.kb.position()
        special = row == self.kb.special_row()
        for i in range(K["cols"] * KEY_ROWS):
            if i < len(keys):
                sc["k_%d" % i].setText(ui_api.text(keys[i].upper()))
                sc["k_%d" % i].show()
                sc["kp_%d" % i].show()
                on = focused and not special and i == self.kb.focus
                color(sc, "k_%d" % i, T.ON_ACCENT if on else T.TEXT)
            else:
                sc["k_%d" % i].hide()
                sc["kp_%d" % i].hide()
        labels = {"space": "Пробел", "erase": "Стереть", "layout": self.kb.layout_label()}
        for i, (kind, _) in enumerate(SPECIAL):
            sc["kw_%d" % i].setText(ui_api.text(labels[kind]))
            sc["kw_%d" % i].show()
            sc["kwp_%d" % i].show()
            color(sc, "kw_%d" % i, T.ON_ACCENT if focused and special and col == i else T.TEXT)
        if focused and not special:
            x, y = key_pos(col, row)
            move(sc, sc.s, "k_focus", x, y)
            sc["k_focus"].show()
        else:
            sc["k_focus"].hide()
        if focused and special:
            x, y = wide_pos(col)
            move(sc, sc.s, "kw_focus", x, y)
            sc["kw_focus"].show()
        else:
            sc["kw_focus"].hide()

    def render_scope(self):
        sc = self.shell
        scopes = self.scopes()
        for i in range(len(T.SEARCH_SCOPES)):
            if i >= len(scopes):
                for name in ("sc_%d", "scf_%d", "scp_%d"):
                    sc[name % i].hide()
                continue
            key, label = scopes[i]
            focused = sc.zone == "scope" and i == self.scope_focus
            sc["sc_%d" % i].setText(label)
            sc["sc_%d" % i].show()
            visible(sc["scf_%d" % i], focused)
            visible(sc["scp_%d" % i], not focused)
            color(sc, "sc_%d" % i, T.ON_ACCENT if focused else (T.ACCENT if key == self.scope else T.TEXT_2))
            font(sc, sc.s, "sc_%d" % i, 22, focused or key == self.scope)

    def render_query(self):
        sc = self.shell
        if self.kb.text:
            sc["q_text"].setText(ui_api.text(self.kb.text + u"|"))
            color(sc, "q_text", T.TEXT)
        else:
            sc["q_text"].setText("Название…")
            color(sc, "q_text", T.MUTED)

    def render_status(self):
        sc = self.shell
        r = self.result or {}
        line, hint = "", ""
        if self.state == "idle":
            line, hint = "Наберите название", "Ищем на выбранном портале, в разделе ниже"
        elif self.state == "short":
            line = "Введите хотя бы две буквы"
        elif self.state == "loading":
            line = "Ищу…"
        elif self.state == "error":
            line, hint = self.error_text, "Жёлтая — повторить"
        else:
            count = self.content().count()
            more = "" if self.content().listing.complete() else "+"
            scope = r.get("scope")
            query = ui_api.text(r.get("query") or self.query)
            if not count:
                line = "Ничего не найдено по «%s»" % query
            elif scope == "channels":
                line = "Найдено каналов: %d%s" % (count, more)
            else:
                line = "Найдено: %d%s" % (count, more)
            if scope == "server":
                hint = "Поиск на сервере портала"
            elif scope == "channels":
                hint = "Поиск по всем телеканалам портала"
            elif scope == "loaded":
                hint = "Портал не умеет искать — проверены только открытые страницы (%d)" % int(r.get("searched") or 0)
        sc["q_status"].setText(line)
        sc["q_status2"].setText(hint)
        color(sc, "q_status", T.ERROR if self.state == "error" else T.TEXT)
        color(sc, "q_status2", T.WARN if r.get("scope") == "loaded" and self.state == "done" else T.MUTED)

    # -------------------------------------------------------------- search

    def reset_results(self):
        self.generation += 1
        self.result = None
        self.grid.reset(self.shell.source_id())
        self.live.reset(self.shell.source_id())

    def changed(self):
        self.render_query()
        self.timer.stop()
        text = self.kb.text.strip()
        if len(text) < 2:
            self.reset_results()
            self.state = "short" if text else "idle"
            self.render_status()
            self.content().render(True, False)
            return
        self.timer.start(DEBOUNCE_MS, True)

    def run(self):
        """Start a new search; answers to older queries are ignored."""
        text = self.kb.text.strip()
        if len(text) < 2 or not self.shell.source:
            return
        self.query = text
        self.reset_results()
        self.state = "loading"
        self.render_status()
        self.content().render(True, False)
        self.load_page(1)

    def load_page(self, n):
        content = self.content()
        content.listing.loading.add(n)
        generation = self.generation
        path = ui_api.catalog_path(self.shell.source_id(), "search", type=TYPE[self.scope], q=self.query, page=n)

        def ok(result):
            if generation != self.generation:
                return
            content.listing.store(n, result)
            if n == 1:
                self.result = result
                self.state = "done"
            self.render_status()
            focused = self.shell.zone == "content"
            content.model.clamp(content.count())
            content.render(True, focused)
            self.shell.schedule_images()

        def fail(problem):
            if generation != self.generation:
                return
            content.listing.fail(n, problem)
            if n == 1:
                self.state = "error"
                self.error_text = {"network": "Нет связи с порталом", "service": problem.message}.get(problem.kind, problem.message)
                self.render_status()

        self.shell.api(path, ok, fail, guard=False)

    def ensure_pages(self):
        content = self.content()
        if self.state != "done":
            return
        start, stop = content.window()
        for n in content.listing.missing(start, stop)[:1]:
            self.load_page(n)

    def retry(self):
        if self.kb.text.strip():
            self.run()

    # -------------------------------------------------------------- keys

    def move_keys(self, dx, dy):
        result = self.kb.move(dx, dy)
        if result == EDGE_LEFT:
            self.shell.enter_rail()
            return
        if result == EDGE_RIGHT and self.content().count():
            self.shell.set_zone("content")
            return
        if result == EDGE_DOWN:
            self.scope_focus = [k for k, _ in self.scopes()].index(self.scope)
            self.shell.set_zone("scope")
            return
        self.render_keys()

    def move_scope(self, dx, dy):
        if dy < 0:
            self.shell.set_zone("keys")
        elif dx:
            target = self.scope_focus + dx
            if target < 0:
                self.shell.enter_rail()
            elif target >= len(self.scopes()):
                if self.content().count():
                    self.shell.set_zone("content")
            else:
                self.scope_focus = target
                self.render_scope()

    def press(self):
        """OK on the keyboard or the scope row."""
        if self.shell.zone == "scope":
            key = self.scopes()[self.scope_focus][0]
            if key != self.scope:
                self.other().clear()
                self.scope = key
                self.reset_results()
                self.render_scope()
                self.shell.update_subtitle()
                self.changed()
                if len(self.kb.text.strip()) >= 2:
                    self.timer.stop()
                    self.run()
            return
        before = self.kb.layout
        if self.kb.press():
            self.changed()
        if before != self.kb.layout:
            self.render_keys()

    def type_digit(self, digit):
        if self.kb.type_digit(digit.decode("utf-8") if isinstance(digit, bytes) else u"%s" % digit):
            self.changed()

    def erase(self):
        if self.kb.erase():
            self.changed()

    def zone_changed(self):
        self.render_keys()
        self.render_scope()
        self.content().render(False, self.shell.zone == "content")


__all__ = ["SearchPanel", "MOVED", "EDGE_UP"]
