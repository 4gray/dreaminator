# -*- coding: utf-8 -*-
"""Full-screen TV catalog: navigation rail, categories and content.

Focus lives in one zone at a time: rail, categories, content (poster grid
or channel list) and, in search, the keyboard and the scope row. Arrows
only move focus; CH+/CH- and the < > keys turn pages; Back climbs one
level and keeps every position. Network and picture decoding never run on
the GUI thread, and late answers are dropped by generation counters.
"""
from __future__ import absolute_import, division

import time
from collections import OrderedDict

from enigma import eServiceReference, eSize, eTimer, getDesktop
from twisted.internet import threads
from Components.ActionMap import ActionMap
from Components.Label import Label
from Components.Pixmap import Pixmap
from Screens.Screen import Screen

try:
    from . import ui_api, ui_library, ui_theme as T, ui_trace
    from .ui_images import ImageLoader
    from .ui_models import EDGE_LEFT, MOVED, ListModel
    from .ui_search import SearchPanel
    from .ui_widgets import ChannelList, OverlayMenu, PosterGrid, color, focus_meta, font, move, on_air, visible
    from .catalog_state import playback_url, service_type
except (ImportError, ValueError):
    import ui_api
    import ui_library
    import ui_theme as T
    import ui_trace
    from ui_images import ImageLoader
    from ui_models import EDGE_LEFT, MOVED, ListModel
    from ui_search import SearchPanel
    from ui_widgets import ChannelList, OverlayMenu, PosterGrid, color, focus_meta, font, move, on_air, visible
    from catalog_state import playback_url, service_type

SECTION_TYPE = {"live": "itv", "vod": "vod", "series": "series"}
SORTS = [("added", "Новые"), ("top", "Популярные"), ("rating", "По рейтингу"), ("name", "По названию")]
# For the context line; written out because Python 2 cannot lower() UTF-8 bytes.
SORT_CONTEXT = {"added": "новые", "top": "популярные", "rating": "по рейтингу", "name": "по названию"}
# The chosen order per section lasts for this Enigma2 run (no flash writes).
SORT_MEMORY = {}
SECTION_TITLE = {"live": "Live TV", "vod": "Фильмы", "series": "Сериалы", "search": "Поиск", "favorites": "Избранное",
                 "recent": "Недавние"}
# Favorites and recents: rows of every source, from the service's library.
LIBRARY = ui_library.SECTIONS
RAIL_KEYS = [k for k, _, _ in T.RAIL_ITEMS]
RAIL_PITCH = T.RAIL_ITEMS[1][2] - T.RAIL_ITEMS[0][2]
# The source and section the viewer last browsed: reopening the catalog
# returns there for this Enigma2 run (no flash writes).
LAST_PLACE = {}
SOURCE_KIND = {"stalker": "Портал", "m3u": "M3U", "file": "M3U", "xtream": "Xtream"}
# Notes over a filled list: centred under the content, inside the safe area.
TOAST = ((T.GRID["x"] + 1840) // 2 - 480, 1032 - 112, 960, 112)
KEYMAP_CONTEXT = "DreaminatorActions"
# OK acts on release ("ok"); holding it ("okLong") adds to or removes from
# the favorites; "okDown" (the press) clears the hold of the previous one.
ACTIONS = ("ok", "okDown", "okLong", "back", "up", "down", "left", "right", "pageNext", "pagePrev", "red", "green", "yellow",
           "blue", "menu", "info", "erase", "0", "1", "2", "3", "4", "5", "6", "7", "8", "9")
# The focused channel's programme is asked once the remote is quiet.
GUIDE_DELAY_MS = 400
GUIDE_TTL = 120  # seconds an answer serves when the viewer comes back
GUIDE_KEEP = 40


def plugin_module():
    try:
        from . import plugin
    except (ImportError, ValueError):
        import plugin
    return plugin


def number(n):
    """16644 -> '16 644' (grouping that reads well on a TV)."""
    s = str(n)
    out = []
    while len(s) > 3:
        out.insert(0, s[-3:])
        s = s[:-3]
    out.insert(0, s)
    return " ".join(out)


def build_skin(width):
    s = T.Skin(width, plugin_module().FONTS_READY)
    b = []
    # Navigation rail; the open panel is drawn above everything else.
    b.append(s.box(0, 0, T.RAIL_W, 1080, T.RAIL, z=0))
    b.append(s.label("rail_panel", 0, 0, T.RAIL_OPEN_W, 1080, bg=T.RAIL, z=5))
    b.append(s.pixmap("rail_shadow", T.RAIL_OPEN_W, 0, 48, 1080, z=5, asset="rail_shadow"))
    b.append(s.image(72, 48, 48, 48, "mark", z=6))
    b.append(s.text(72, 48, 48, 48, "D", 26, T.ON_ACCENT, semi=True, halign="center", z=7))
    b.append(s.label("rail_brand", 136, 44, 280, 40, 30, T.TEXT, semi=True, z=7))
    b.append(s.label("rail_brand_sub", 136, 84, 280, 26, 20, T.MUTED, z=7))
    for name, asset, w in (("rail_active_s", "pill_rail_active", 64), ("rail_active_w", "pill_rail_open_active", 328),
                           ("rail_focus_s", "pill_rail", 64), ("rail_focus_w", "pill_rail_open", 328)):
        b.append(s.pixmap(name, 64, 180, w, 64, z=6, asset=asset))
    for i, (key, label, y) in enumerate(T.RAIL_ITEMS):
        b.append(s.pixmap("rail_icon_%d" % i, 80, y + 16, 32, 32, z=7))
        b.append(s.label("rail_label_%d" % i, 132, y, 248, 64, 28, T.TEXT_2, z=7))
    # Header: where we are (small), then the focused card's title and facts —
    # one place to read on a TV instead of text under every poster.
    b.append(s.label("context", T.CONTENT_X, 40, 836, 30, 22, T.MUTED))
    b.append(s.label("title", T.CONTENT_X, 70, 836, 52, 40, T.TEXT, semi=True))
    b.append(s.label("meta", T.CONTENT_X, 122, 836, 32, 24, T.TEXT_2))
    x = 1840
    for kind, label, width in (("kbd", "Страницы", 106), ("blue", "Поиск", 70), ("yellow", "Обновить", 108), ("green", "Источники", 116),
                               ("red", "Сортировка", 128)):
        x -= width
        if kind == "red":
            b.append(s.label("hint_red", x, 104, width + 8, 36, 22, T.TEXT_2))
        else:
            b.append(s.text(x, 104, width + 8, 36, label, 22, T.TEXT_2))
        if kind == "kbd":
            x -= 62
            b.append('<eLabel position="%s" size="%s" font="%s" foregroundColor="%s" backgroundColor="%s" borderWidth="2" borderColor="%s" '
                     'halign="center" valign="center" zPosition="2" text="CH±" />' % (s.pos(x, 107), s.size(54, 30), s.font(18), T.TEXT_2, T.BG, T.LINE))
        elif kind == "red":
            x -= 22
            b.append(s.pixmap("hint_red_dot", x, 115, 14, 14, z=2, asset="dot_red"))
        else:
            x -= 22
            b.append(s.image(x, 115, 14, 14, "dot_" + kind, z=2))
        x -= 24
    b.append(s.clock(1720, 40, 120, 44))
    # Categories.
    c = T.CATS
    b.append(s.pixmap("cat_active", c["x"], c["y"], c["w"], c["row"], z=1, asset="pill_cat_active"))
    b.append(s.pixmap("cat_focus", c["x"], c["y"], c["w"], c["row"], z=1, asset="pill_cat"))
    for i in range(c["rows"]):
        b.append(s.label("cat_%d" % i, c["x"] + 24, c["y"] + i * c["row"], c["w"] - 40, c["row"], 26, T.TEXT_2))
    b.append(s.label("cat_track", c["x"] + c["w"] + 4, c["y"], 4, c["rows"] * c["row"], bg="#171b24", z=1))
    b.append(s.label("cat_thumb", c["x"] + c["w"] + 4, c["y"], 4, 60, bg="#3a4152", z=2))
    # Content views.
    b.extend(PosterGrid.skin(s, "", T.GRID, "ring_poster"))
    b.extend(ChannelList.skin(s, "lv", T.LIVE))
    b.extend(SearchPanel.skin(s))
    b.extend(OverlayMenu.skin(s))
    b.append(s.label("msg", T.GRID["x"], 380, 1272, 60, 32, T.TEXT_2, halign="center", z=6))
    b.append(s.label("msg2", T.GRID["x"], 448, 1272, 40, 24, T.MUTED, halign="center", z=6))
    # Over a filled list a note gets a solid panel: text drawn over rows is unreadable.
    x, y, w, h = TOAST
    b.append('<widget name="toast" position="%s" size="%s" backgroundColor="%s" borderWidth="%d" borderColor="%s" zPosition="9" />'
             % (s.pos(x, y), s.size(w, h), T.RAIL, max(1, s.px(2)), T.LINE))
    b.append(s.label("toast_text", x + 32, y + 14, w - 64, 46, 30, T.TEXT, semi=True, halign="center", bg=T.RAIL, z=10))
    b.append(s.label("toast_hint", x + 32, y + 62, w - 64, 34, 22, T.MUTED, halign="center", bg=T.RAIL, z=10))
    return s, s.screen("DreaminatorShell", b)


SKIN, SKIN_XML = build_skin(getDesktop(0).size().width())


class Shell(Screen):
    skin = SKIN_XML

    def __init__(self, session):
        Screen.__init__(self, session)
        self.setTitle("Dreaminator")
        self.s = SKIN
        self.images = ImageLoader(budget=10 << 20)
        self.closed = False
        self.playing = False
        self.generation = 0
        self.zone = "cats"
        self.rail_open = False
        self.section = LAST_PLACE.get("section", "vod")
        self.last_section = self.section if self.section in SECTION_TYPE else "vod"
        self.rail_focus = RAIL_KEYS.index(self.section)
        self.rail_y = [y for _, _, y in T.RAIL_ITEMS]
        self.rail_shown = [True] * len(T.RAIL_ITEMS)
        self.sources = []
        self.source = None
        self.categories = []
        self.cat_model = ListModel(T.CATS["rows"])
        self.cat_active = 0
        self.cats_state = "loading"
        self.positions = {}
        self.refresh_pages = False
        self.key_trace = None
        self.last_beat = None
        self.pressed = False  # OK went down on this screen (its release may act)
        self.held = False  # OK is held down: its release must not open or play
        self.fav_busy = False
        self.fav_pending = None  # the favorite change on its way, if any
        self.channels_changed = False  # favorite channels changed: send them to Enigma2 on close
        self.keymap = False

        self["rail_panel"] = Label("")
        self["rail_shadow"] = Pixmap()
        self["rail_brand"] = Label("Dreaminator")
        self["rail_brand_sub"] = Label("каталог на телевизоре")
        for name in ("rail_active_s", "rail_active_w", "rail_focus_s", "rail_focus_w"):
            self[name] = Pixmap()
        for i, (key, label, y) in enumerate(T.RAIL_ITEMS):
            self["rail_icon_%d" % i] = Pixmap()
            self["rail_label_%d" % i] = Label(label)
        for name in ("context", "title", "meta", "hint_red", "cat_track", "cat_thumb", "msg", "msg2", "toast", "toast_text",
                     "toast_hint"):
            self[name] = Label("")
        self["hint_red_dot"] = Pixmap()
        self["cat_active"] = Pixmap()
        self["cat_focus"] = Pixmap()
        for i in range(T.CATS["rows"]):
            self["cat_%d" % i] = Label("")
        self.grid = PosterGrid(self, self.s, "", T.GRID, "ring_poster", self.images)
        self.live = ChannelList(self, self.s, "lv", T.LIVE, self.images)
        self.search = SearchPanel(self)
        self.menu = OverlayMenu(self, self.s)

        self.icons = {}
        self.image_timer = eTimer()
        self.image_timer.callback.append(self.request_images)
        self.settle_timer = eTimer()
        self.settle_timer.callback.append(self.settled)
        self.lag_timer = eTimer()
        self.lag_timer.callback.append(self.heartbeat)
        self.guides = OrderedDict()  # channel key -> (time, programme answer)
        self.guide_pending = None
        self.now_pending = False
        self.guide_timer = eTimer()
        self.guide_timer.callback.append(self.request_guide)
        self.minute_timer = eTimer()
        self.minute_timer.callback.append(self.minute)
        self["actions"] = self.action_map()
        self.onLayoutFinish.append(self.start)
        self.onClose.append(self.cleanup)

    # ------------------------------------------------------------ plumbing

    def action_map(self):
        handlers = dict((name, self.dispatcher(name)) for name in ACTIONS)
        self.keymap = plugin_module().load_keymap()
        if self.keymap:
            return ActionMap([KEYMAP_CONTEXT], handlers, -1)
        # Without our keymap the stock contexts cover the same keys; OK stays
        # on the press there, so no long OK (the card's button still works).
        fallback = dict(handlers)
        del fallback["okDown"], fallback["okLong"]
        fallback.update({"cancel": handlers["back"], "nextBouquet": handlers["pageNext"], "prevBouquet": handlers["pagePrev"],
                         "moveUp": handlers["pagePrev"], "moveDown": handlers["pageNext"]})
        return ActionMap(["OkCancelActions", "DirectionActions", "ColorActions", "ChannelSelectBaseActions", "MenuActions",
                          "NumberActions"], fallback, -1)

    def dispatcher(self, name):
        def run():
            if self.closed:
                return
            started = ui_trace.now()
            self.images.busy()
            if self.menu.is_open():
                self.menu_key(name)
            elif name.isdigit():
                self.key_digit(name)
            else:
                getattr(self, "key_" + name.lower())()
            if ui_trace.enabled() and name != "okDown":
                self.key_trace = (name, started, ui_trace.ms(started))
                self.settle_timer.start(0, True)
        return run

    def settled(self):
        if self.key_trace:
            name, started, handler_ms = self.key_trace
            ui_trace.log("key", name=name, zone=self.zone, handler=handler_ms, settle=ui_trace.ms(started), rss=ui_trace.rss_kb())
            self.key_trace = None

    def heartbeat(self):
        """Trace only: log main-loop stalls with what the pictures were doing."""
        now = ui_trace.now()
        if self.last_beat is not None and now - self.last_beat > 0.3:
            im = self.images
            ui_trace.log("lag", ms="%.0f" % ((now - self.last_beat) * 1000), decoding=int(bool(im.decoding)), held=int(im.held),
                         fetching=len(im.fetching), queued=len(im.queue), waiting=len(im.decode_queue))
        self.last_beat = now

    def api(self, path, ok, fail=None, body=None, timeout=35, stale=None, convert=False, guard=True, method=None):
        generation = self.generation

        def current():
            return not self.closed and (not guard or generation == self.generation)

        def done(result):
            if current():
                ok(result)
            elif stale:
                stale()

        def failed(failure):
            if current():
                if fail:
                    fail(ui_api.problem(failure))
            elif stale:
                stale()

        threads.deferToThread(ui_api.request_json, path, body, timeout, convert, method).addCallbacks(done, failed)

    def source_id(self):
        return self.source["id"] if self.source else ""

    def source_of(self, item):
        """The source a row plays from: library rows name their own."""
        sid = item.get("source") or self.source_id()
        for src in self.sources:
            if src.get("id") == sid:
                return src
        return self.source if sid == self.source_id() else {"id": sid, "name": ""}

    def library(self):
        return self.section in LIBRARY

    def library_category(self):
        return self.categories[self.cat_active].get("id") if self.categories else "itv"

    def icon(self, name, variant):
        key = (name, variant)
        if key not in self.icons:
            from Tools.LoadPixmap import LoadPixmap
            self.icons[key] = LoadPixmap(self.s.asset("icon_%s_%s" % (name, variant)), cached=True)
        return self.icons[key]

    def cleanup(self):
        if self.closed:
            return
        self.closed = True
        self.generation += 1
        for timer in (self.image_timer, self.settle_timer, self.lag_timer, self.search.timer, self.guide_timer, self.minute_timer):
            timer.stop()
        self.images.close()

        def send(_=None):
            # Off the GUI thread and after the screen is gone: nothing waits for it.
            threads.deferToThread(ui_library.close_catalog, self.channels_changed).addCallbacks(lambda _: None, lambda _: None)

        if self.fav_busy and self.fav_pending is not None:
            # A favorite change still on its way is sent first: Enigma2 gets the list with it.
            self.fav_pending.addBoth(send)
        else:
            send()

    # ------------------------------------------------------------ views

    def content(self):
        if self.section == "search":
            return self.search.content()
        if self.library():
            return self.live if self.library_category() == "itv" else self.grid
        return self.live if self.section == "live" else self.grid

    def schedule_images(self):
        self.image_timer.start(120, True)

    def request_images(self):
        if self.closed or not self.source:
            return
        sid = self.source_id()

        def path_for(item):
            return ui_api.catalog_path(item.get("source") or sid, "poster", key=item["key"])

        self.images.want(self.content().wanted(path_for))

    def show_message(self, text, hint=""):
        """In the empty content area, or as a panel when a list is on screen."""
        filled = self.content().count() > 0
        names = ("toast_text", "toast_hint") if filled else ("msg", "msg2")
        self[names[0]].setText(ui_api.text(text))
        self[names[1]].setText(ui_api.text(hint))
        for name in ("toast", "toast_text", "toast_hint", "msg", "msg2"):
            visible(self[name], (name.startswith("toast")) == filled)

    def hide_message(self):
        for name in ("msg", "msg2", "toast", "toast_text", "toast_hint"):
            self[name].hide()

    def show_categories(self, on):
        names = ["cat_%d" % i for i in range(T.CATS["rows"])] + ["cat_track"]
        for name in names:
            visible(self[name], on)
        if not on:
            for name in ("cat_focus", "cat_active", "cat_thumb"):
                self[name].hide()

    # ------------------------------------------------------------ start

    def start(self):
        ui_trace.log("open", rss=ui_trace.rss_kb(), **ui_trace.live_screens())
        if ui_trace.enabled():
            self.lag_timer.start(100, False)
        self.minute_timer.start(60000, False)
        for name in ("rail_panel", "rail_shadow", "rail_brand", "rail_brand_sub"):
            self[name].hide()
        self.menu.close()
        self.search.hide()
        self.live.clear()
        self.grid.clear()
        self.render_rail()
        self.update_subtitle()
        self.show_message("Загрузка…")
        self.api("/api/catalog/sources", self.sources_loaded, self.sources_failed)

    def sources_loaded(self, sources):
        self.sources = sources or []
        if not self.sources:
            # Nothing to browse yet: the familiar status/QR screen explains setup.
            self.session.openWithCallback(lambda *a: self.close(), plugin_module().DreaminatorScreen)
            return
        last = [src for src in self.sources if src.get("id") == LAST_PLACE.get("source")]
        self.use_source((last or self.sources)[0])

    def sources_failed(self, problem):
        self.cats_state = "error"
        self.show_message(problem.message, "Жёлтая — повторить · Back — назад")

    def use_source(self, source, browse=False):
        """browse: the viewer picked the source, so show its lists even when
        Favorites or Recents were on screen."""
        self.source = source
        LAST_PLACE["source"] = source.get("id")
        self["rail_label_%d" % RAIL_KEYS.index("portal")].setText(ui_api.text(source.get("name") or "Портал"))
        self.layout_rail()
        self.search.source_changed()
        section = self.section
        if section == "search" or browse and section in LIBRARY:
            section = self.last_section
        self.open_section(section)

    def has(self, section):
        """Whether the source has this section: playlists carry Live TV only."""
        sections = (self.source or {}).get("sections")
        return section not in SECTION_TYPE or sections is None or SECTION_TYPE[section] in sections

    def layout_rail(self):
        """Hide the sections the source lacks; the items below close up."""
        shown = [self.has(key) for key in RAIL_KEYS]
        if shown == self.rail_shown:
            return
        self.rail_shown = shown
        y = T.RAIL_ITEMS[0][2]
        for i, (key, label, top) in enumerate(T.RAIL_ITEMS):
            if key in ("settings", "about"):
                continue  # pinned to the bottom
            self.rail_y[i] = y
            if shown[i]:
                move(self, self.s, "rail_icon_%d" % i, 80, y + 16)
                move(self, self.s, "rail_label_%d" % i, 132, y)
                y += RAIL_PITCH
            else:
                self["rail_icon_%d" % i].hide()
                self["rail_label_%d" % i].hide()

    # ------------------------------------------------------------ sections and categories

    def leave_content(self):
        """Remember where the viewer was, so Back and section switches return there."""
        if (self.section in SECTION_TYPE or self.library()) and self.categories and self.content().count():
            self.positions[self.category_key()] = self.content().model.state()

    def open_section(self, section, refresh=False):
        if not self.has(section):
            section = "live"
        self.leave_content()
        self.hide_message()
        if self.section in SECTION_TYPE:
            self.last_section = self.section
        self.section = section
        if section in SECTION_TYPE or section in LIBRARY:
            LAST_PLACE["section"] = section
        self.generation += 1
        self.render_rail()
        if section == "search":
            self.grid.clear()
            self.live.clear()
            self.show_categories(False)
            if self.zone not in ("rail",):
                self.zone = "keys"
            self.search.open(self.last_section)
            self.update_subtitle()
            return
        self.search.hide()
        self.show_categories(True)
        (self.grid if section == "live" else self.live).clear()
        self.categories = []
        self.cat_model = ListModel(T.CATS["rows"])
        self.cat_active = 0
        self.cats_state = "loading"
        self.content().reset(self.source_id())
        self.render_cats()
        self.show_message("Загрузка…")
        self.update_subtitle()
        if section in LIBRARY:
            # Both views start empty: films may come first, and a previous
            # grid must not lend its position to the library.
            self.grid.reset(self.source_id())
            self.api(ui_library.HOME, self.home_loaded, self.categories_failed)
            return
        path = ui_api.catalog_path(self.source_id(), "categories", type=SECTION_TYPE[section], refresh=int(refresh))
        self.api(path, self.categories_loaded, self.categories_failed)

    def home_loaded(self, home):
        """Favorites or Recents: categories are "Каналы" and "Фильмы и сериалы"."""
        cats = ui_library.categories(home, self.section)
        if cats:
            self.categories_loaded(cats)
            return
        self.cats_state = "ready"
        self.zone = "cats" if self.zone != "rail" else "rail"
        self.render_cats()
        self.show_message(*ui_library.EMPTY[self.section])

    def categories_loaded(self, cats):
        self.categories = cats or []
        self.cats_state = "ready"
        if self.categories and self.zone not in ("rail",):
            self.zone = "content"
        if not self.categories:
            self.zone = "cats" if self.zone != "rail" else "rail"
            self.render_cats()
            if self.section == "series":
                self.show_message("На портале нет отдельного раздела сериалов",
                                  "Сериалы этого портала — в «Фильмах», с пометкой «Сериал»")
            elif self.section == "live" and not self.has("vod"):
                self.show_message("В плейлисте нет каналов", "Жёлтая — загрузить плейлист заново")
            elif self.section == "live":
                self.show_message("На портале нет телеканалов", "Каталог фильмов и сериалов доступен в других разделах")
            else:
                self.show_message("Портал не отдал категории", "Жёлтая — повторить")
            return
        self.load_category(0)

    def categories_failed(self, problem):
        self.cats_state = "error"
        self.render_cats()
        self.show_message(problem.message, "Жёлтая — повторить")

    def sort(self):
        return SORT_MEMORY.get(self.section, "added") if self.section in ("vod", "series") else ""

    def category_key(self, index=None):
        cat = self.categories[self.cat_active if index is None else index]
        return ("" if self.library() else self.source_id(), self.section, cat.get("id", ""), self.sort())

    def load_category(self, index, refresh=False):
        self.leave_content()
        self.generation += 1
        before = self.content()
        self.cat_active = index
        self.refresh_pages = refresh
        if self.content() is not before:
            before.clear()  # Favorites: channels are a list, films a grid
        self.content().reset(self.source_id(), self.positions.get(self.category_key()))
        self.render_cats()
        self.show_message("Загрузка…")
        self.update_subtitle()
        self.ensure_pages()

    # ------------------------------------------------------------ pages

    def ensure_pages(self):
        if self.section == "search":
            self.search.ensure_pages()
            return
        if not self.categories:
            return
        view = self.content()
        start, stop = view.window()
        for n in view.listing.missing(start, stop)[:2]:
            self.load_page(n)

    def load_page(self, n):
        cat = self.categories[self.cat_active]
        view = self.content()
        view.listing.loading.add(n)
        if self.library():
            path = ui_library.items_path(self.section, cat.get("id", ""), n)
        else:
            path = ui_api.catalog_path(self.source_id(), "items", type=SECTION_TYPE[self.section], category=cat.get("id", ""),
                                       page=n, refresh=int(self.refresh_pages and n == 1), sort=self.sort() or None)
        started = ui_trace.now()

        def ok(result):
            ui_trace.log("page", n=n, ms=ui_trace.ms(started), items=len(result.get("items") or []))
            self.page_loaded(view, n, result)

        def fail(problem):
            view.listing.fail(n, problem)
            if not view.count():
                hint = "Жёлтая — повторить"
                text = "Нет связи с порталом" if problem.kind == "network" else problem.message
                self.show_message(text, hint)

        self.api(path, ok, fail)

    def page_loaded(self, view, n, result):
        view.listing.store(n, result)
        count = view.count()
        if not count:
            self.show_message("В этой категории пока пусто")
            view.render(True, False)
            return
        self.hide_message()
        view.model.clamp(count)
        size = view.listing.page_size
        first, last = (n - 1) * size, n * size
        start, stop = view.window()
        if first < stop and last > start:
            started = ui_trace.now()
            view.render(True, self.zone == "content")
            ui_trace.log("render.grid", ms=ui_trace.ms(started), reason="page")
            self.schedule_images()
        self.update_subtitle()
        self.ensure_pages()

    # ------------------------------------------------------------ rendering

    def render_rail(self):
        open_ = self.rail_open
        focus = self.rail_focus if self.zone == "rail" else None
        active = RAIL_KEYS.index(self.section)
        for i, (key, label, y) in enumerate(T.RAIL_ITEMS):
            if not self.rail_shown[i]:
                continue
            variant = "dark" if i == focus else ("accent" if i == active else "muted")
            pixmap = self.icon(T.RAIL_ICON[key], variant)
            if pixmap:
                self["rail_icon_%d" % i].instance.setPixmap(pixmap)
                self["rail_icon_%d" % i].show()
            if open_:
                color(self, "rail_label_%d" % i, T.ON_ACCENT if i == focus else (T.ACCENT if i == active else T.TEXT_2))
            visible(self["rail_label_%d" % i], open_)
        y_active = self.rail_y[active]
        for name, on in (("rail_active_s", not open_ and active != focus), ("rail_active_w", open_ and active != focus)):
            move(self, self.s, name, 64, y_active)
            visible(self[name], on)
        for name, on in (("rail_focus_s", focus is not None and not open_), ("rail_focus_w", focus is not None and open_)):
            if focus is not None:
                move(self, self.s, name, 64, self.rail_y[focus])
            visible(self[name], on)

    def set_rail_open(self, open_):
        if self.rail_open == open_:
            return
        self.rail_open = open_
        for name in ("rail_panel", "rail_shadow", "rail_brand", "rail_brand_sub"):
            visible(self[name], open_)
        self.render_rail()

    def render_cats(self):
        if self.section == "search":
            return
        c = T.CATS
        m = self.cat_model
        count = len(self.categories)
        m.clamp(count)
        for i in range(c["rows"]):
            index = m.top + i
            name = "cat_%d" % i
            if index < count:
                self[name].setText(ui_api.text(T.clip(ui_api.unicode_text(self.categories[index].get("name", "")), 22)))
                focused = self.zone == "cats" and index == m.focus
                color(self, name, T.ON_ACCENT if focused else (T.ACCENT if index == self.cat_active else T.TEXT_2))
                font(self, self.s, name, 26, focused)
            else:
                self[name].setText("")
        if self.cats_state == "loading" and not count:
            self["cat_0"].setText("Загрузка…")
            color(self, "cat_0", T.MUTED)
        if count and self.zone == "cats" and m.top <= m.focus < m.top + c["rows"]:
            move(self, self.s, "cat_focus", c["x"], c["y"] + (m.focus - m.top) * c["row"])
            self["cat_focus"].show()
        else:
            self["cat_focus"].hide()
        on = count and m.top <= self.cat_active < m.top + c["rows"] and not (self.zone == "cats" and m.focus == self.cat_active)
        if on:
            move(self, self.s, "cat_active", c["x"], c["y"] + (self.cat_active - m.top) * c["row"])
        visible(self["cat_active"], on)
        if count > c["rows"]:
            track = c["rows"] * c["row"]
            h = max(40, track * c["rows"] // count)
            y = c["y"] + (track - h) * m.top // max(1, count - c["rows"])
            self["cat_thumb"].instance.resize(eSize(self.s.px(4), self.s.px(h)))
            move(self, self.s, "cat_thumb", c["x"] + c["w"] + 4, y)
            self["cat_thumb"].show()
            self["cat_track"].show()
        else:
            self["cat_thumb"].hide()
            self["cat_track"].hide()

    def update_subtitle(self):
        """Context line: portal · section · category · order · position."""
        parts = []
        if self.source and not self.library():
            parts.append(ui_api.text(self.source.get("name", "")))
        if self.section == "search":
            parts.append("Поиск в разделе «%s»" % SECTION_TITLE[self.search.scope])
        else:
            parts.append(SECTION_TITLE[self.section])
            if self.categories:
                parts.append(ui_api.text(T.clip(ui_api.unicode_text(self.categories[self.cat_active].get("name", "")), 24)))
            if self.sort():
                parts.append(SORT_CONTEXT[self.sort()])
            view = self.content()
            count = view.count()
            if count:
                m = view.model
                first = m.first_visible() + 1
                last = min(count, m.first_visible() + m.cols * m.rows)
                total = number(count) if view.listing.complete() else number(count) + "+"
                parts.append("%s–%s из %s" % (number(first), number(last), total))
        self["context"].setText(" · ".join(parts))
        sortable = self.section in ("vod", "series")
        self["hint_red"].setText("Сортировка" if sortable else "")
        visible(self["hint_red_dot"], sortable)
        self.update_focus_info()

    def update_focus_info(self):
        """Title and facts of the focused card, or the section's name."""
        item = self.content().item() if self.zone == "content" else None
        if item:
            self["title"].setText(ui_api.text(T.clip(ui_api.unicode_text(item.get("name") or ""), 40)))
            self["meta"].setText(focus_meta(item))
        else:
            self["title"].setText(SECTION_TITLE[self.section])
            self["meta"].setText("")
        if self.live_list() and self.live.item():
            self.guide_timer.start(GUIDE_DELAY_MS, True)

    # ------------------------------------------------------------ programmes

    def live_list(self):
        """The channel list with its programme panel: Live TV, or channels in
        Favorites and Recents."""
        return self.section == "live" or self.library() and self.content() is self.live

    def request_guide(self):
        """Now and next, with descriptions, for the channel in the panel."""
        if self.closed or not self.live_list():
            return
        item = self.live.item()
        if not item or item.get("missing"):
            return
        key = item["key"]
        cached = self.guides.get(key)
        if cached and time.time() - cached[0] < GUIDE_TTL:
            self.live.show_guide(key, cached[1])
            return
        if key == self.guide_pending:
            return
        self.guide_pending = key

        def settled():
            if self.guide_pending == key:
                self.guide_pending = None

        def ok(result):
            settled()
            self.guides.pop(key, None)
            self.guides[key] = (time.time(), result)
            while len(self.guides) > GUIDE_KEEP:
                self.guides.popitem(last=False)
            self.live.show_guide(key, result)
            # The row learns what is on, too (Xtream and Stalker rows only
            # know it once the provider was asked).
            now = result.get("now")
            self.live.update_now([key], {key: dict((k, now[k]) for k in ("title", "start", "stop") if k in now)} if now else {})

        def fail(problem):
            settled()
            if problem.kind != "expired":
                self.live.show_guide(key, None)

        path = ui_api.catalog_path(item.get("source") or self.source_id(), "epg", key=key, tz=ui_api.tz_minutes())
        self.api(path, ok, fail, timeout=20, stale=settled)

    def minute(self):
        """Progress bars move on; rows whose programme ended ask what is on."""
        if self.closed or self.playing:
            return
        view = self.content()
        if not isinstance(view, ChannelList) or not view.count():
            return
        t = time.time()
        ended = view.tick(t)
        if ended and not self.now_pending:
            # Favorites and Recents mix sources: one question per source.
            by_source = {}
            for item in view.visible_items():
                if item and not item.get("missing"):
                    by_source.setdefault(item.get("source") or self.source_id(), []).append(item["key"])
            self.now_pending = len(by_source)

            def asked(keys):
                def ok(result):
                    self.now_pending = max(0, self.now_pending - 1)
                    view.update_now(keys, result.get("now") or {})

                def done(*args):
                    self.now_pending = max(0, self.now_pending - 1)
                return ok, done

            for sid, keys in by_source.items():
                ok, done = asked(keys)
                path = ui_api.catalog_path(sid, "now", keys=",".join(keys))
                self.api(path, ok, done, timeout=20, stale=done)
        item = self.live.item() if view is self.live else None
        cached = self.guides.get(item["key"]) if item else None
        if cached and cached[1] and cached[1].get("now") and not on_air(cached[1]["now"], t):
            self.guides.pop(item["key"], None)
            self.request_guide()

    def set_zone(self, zone):
        self.zone = zone
        if self.section == "search":
            self.search.zone_changed()
        else:
            self.content().render(False, zone == "content")
            self.render_cats()
        self.render_rail()
        self.update_focus_info()

    # ------------------------------------------------------------ keys

    def key_up(self):
        self.move(0, -1)

    def key_down(self):
        self.move(0, 1)

    def key_left(self):
        self.move(-1, 0)

    def key_right(self):
        self.move(1, 0)

    def move(self, dx, dy):
        if self.zone == "rail":
            if dy:
                i = self.rail_focus + dy
                while 0 <= i < len(RAIL_KEYS) and not self.rail_shown[i]:
                    i += dy
                if 0 <= i < len(RAIL_KEYS):
                    self.rail_focus = i
                self.render_rail()
            elif dx > 0:
                self.leave_rail()
            return
        if self.zone == "keys":
            self.search.move_keys(dx, dy)
            return
        if self.zone == "scope":
            self.search.move_scope(dx, dy)
            return
        if self.zone == "cats":
            if dy:
                self.cat_model.move(0, dy, len(self.categories))
                self.render_cats()
            elif dx < 0:
                self.enter_rail()
            elif dx > 0 and self.categories:
                self.choose_category()
            return
        view = self.content()
        before = view.model.first_visible()
        if self.section == "live" or (self.section == "search" and self.search.scope == "live"):
            if dx < 0:
                self.content_left()
                return
            if dx > 0:
                return
            result = view.model.move(0, dy, view.count())
        else:
            result = view.model.move(dx, dy, view.count())
        if result == EDGE_LEFT:
            self.content_left()
        elif result == MOVED:
            self.after_move(view, before)

    def content_left(self):
        if self.section == "search":
            self.set_zone("keys")
            return
        self.zone = "cats"
        self.cat_model.focus = self.cat_active
        self.content().render(False, False)
        self.render_cats()
        self.update_focus_info()

    def after_move(self, view, before):
        started = ui_trace.now()
        scrolled = view.model.first_visible() != before
        view.render(scrolled, True)
        ui_trace.log("render.move", ms=ui_trace.ms(started), full=int(scrolled))
        if scrolled:
            self.update_subtitle()
        else:
            self.update_focus_info()
        self.schedule_images()
        self.ensure_pages()

    def key_pagenext(self):
        self.turn(1)

    def key_pageprev(self):
        self.turn(-1)

    def turn(self, delta):
        if self.zone == "cats":
            self.cat_model.page(delta, len(self.categories))
            self.render_cats()
            return
        if self.zone != "content":
            return
        view = self.content()
        before = view.model.first_visible()
        if view.model.page(delta, view.count()) == MOVED:
            self.after_move(view, before)

    def enter_rail(self):
        self.zone = "rail"
        self.rail_focus = RAIL_KEYS.index(self.section)
        self.set_rail_open(True)
        if self.section == "search":
            self.search.zone_changed()
        else:
            self.render_cats()
            self.content().render(False, False)

    def leave_rail(self):
        self.set_rail_open(False)
        if self.section == "search":
            self.set_zone("keys")
        elif self.content().count():
            self.set_zone("content")
        else:
            self.set_zone("cats")

    def choose_category(self):
        index = self.cat_model.focus
        self.zone = "content"
        if index != self.cat_active or not self.content().count():
            self.load_category(index)
        else:
            self.render_cats()
            self.content().render(False, True)
            self.update_focus_info()

    def key_okdown(self):
        self.pressed, self.held = True, False
        ui_library.OK_DOWN_SEEN[0] = True

    def key_oklong(self):
        if self.held or not self.pressed:
            return  # one change per press, however long it is held
        # Where there is nothing to mark, a long press is an ordinary OK.
        self.held = self.toggle_favorite()

    def key_ok(self):
        pressed, held = self.pressed, self.held
        self.pressed = self.held = False
        if self.keymap and ui_library.OK_DOWN_SEEN[0] and not pressed:
            return  # OK went down on another screen (settings, a message) that closed on it
        if held:
            return  # the release of a long press
        if self.zone == "rail":
            self.rail_action(RAIL_KEYS[self.rail_focus])
            return
        if self.zone in ("keys", "scope"):
            self.search.press()
            return
        if self.zone == "cats":
            if self.categories:
                self.choose_category()
            return
        item = self.content().item()
        if not item:
            return
        if item.get("missing"):
            self.show_toast(*ui_library.missing_message(item))
        elif item.get("kind") == "channel":
            self.play(item)
        elif item.get("kind") in ("movie", "series", "episode"):
            self.open_card(item)

    def toggle_favorite(self):
        """Long OK on a channel, film or series: into the favorites or out.
        Channels go to the channel list "Избранное", the rest to the library.
        False when the focus is on nothing that can be a favorite."""
        if self.zone != "content":
            return False
        item = self.content().item()
        if not item or item.get("kind") not in ("channel", "movie", "series") or item.get("missing") and not item.get("fav"):
            return False
        if self.fav_busy:
            return True  # one change at a time
        method, path, body = ui_library.favorite_request(item.get("source") or self.source_id(), item)
        self.fav_busy = True

        def ok(result):
            # Counts even after the catalog closed: the close waits for this answer.
            self.fav_busy = False
            item["fav"] = bool(result.get("fav"))
            if result.get("channel"):
                self.channels_changed = True
            if not self.closed:
                self.favorite_shown(item)
                self.show_toast(*ui_library.favorite_message(item, result))

        def fail(failure):
            self.fav_busy = False
            if not self.closed:
                self.show_toast(ui_api.problem(failure).message, "Не удалось изменить избранное")

        # Not tied to the list on screen: the change counts even if the viewer moved on.
        self.fav_pending = threads.deferToThread(ui_api.request_json, path, body, 35, True, method)
        self.fav_pending.addCallbacks(ok, fail)
        return True

    def favorite_shown(self, item):
        """Redraw the row or poster of item, wherever it is on screen now."""
        if not self.closed:
            self.content().redraw_item(item, self.zone == "content")
            self.update_focus_info()

    def rail_action(self, key):
        if key in SECTION_TYPE or key == "search" or key in LIBRARY:
            self.set_rail_open(False)
            self.zone = "keys" if key == "search" else "cats"
            if key != self.section:
                self.open_section(key)
            else:
                self.leave_rail()
        elif key == "portal":
            self.key_green()
        elif key == "settings":
            self.session.open(plugin_module().DreaminatorScreen)
        elif key == "about":
            try:
                from .ui_dialogs import AboutScreen
            except (ImportError, ValueError):
                from ui_dialogs import AboutScreen
            self.session.open(AboutScreen)

    def key_back(self):
        if self.zone == "content":
            self.content_left()
        elif self.zone == "scope":
            self.set_zone("keys")
        elif self.zone in ("cats", "keys"):
            self.enter_rail()
        else:
            self.close()

    def menu_key(self, name):
        """While a choice is open, keys belong to it."""
        if name in ("up", "down"):
            self.menu.move(-1 if name == "up" else 1)
        elif name == "ok":
            self.menu.close(choose=True)
        elif name in ("back", "red", "green"):
            self.menu.close()

    def key_red(self):
        """Order of films and series: new arrivals, popular, rating, name."""
        if self.section not in ("vod", "series") or not self.source:
            return
        options = [{"id": key, "name": label} for key, label in SORTS]
        self.menu.open("Сортировка", "Порядок фильмов и сериалов в категориях", options, self.sort(), self.sort_chosen)

    def sort_chosen(self, option=None):
        if not option or option.get("id") == self.sort() or self.closed:
            return
        self.leave_content()
        # Forget the on-screen list first, so its position is not saved again
        # under the new order (a new order starts at the top).
        self.content().reset(self.source_id())
        SORT_MEMORY[self.section] = option["id"]
        if self.categories:
            self.zone = "content"
            self.load_category(self.cat_active)
        else:
            self.update_subtitle()

    def key_yellow(self):
        """Refresh what is on screen (asks the portal again)."""
        if not self.source:
            self.start()
        elif self.library():
            self.open_section(self.section)
        elif self.section == "search":
            self.search.retry()
        elif self.cats_state == "error" or not self.categories:
            self.open_section(self.section, refresh=True)
        else:
            self.load_category(self.cat_active, refresh=True)

    def key_green(self):
        if len(self.sources) < 2:
            self.show_toast("Подключён один источник" if self.sources else "Источники не найдены",
                            "Порталы и плейлисты добавляются в веб-интерфейсе")
            return
        options = [dict(src, detail=SOURCE_KIND.get(src.get("type"), "")) for src in self.sources]
        self.menu.open("Источники", "Порталы и плейлисты из веб-интерфейса", options, self.source_id(), self.portal_chosen)

    def portal_chosen(self, source=None):
        if source and source.get("id") != self.source_id():
            self.leave_content()
            # Drop the old categories, so the list on screen is not saved again
            # under the new source's key.
            self.categories = []
            self.search.reset_results()
            self.use_source(source, browse=True)

    def show_toast(self, text, hint=""):
        self.show_message(text, hint)
        self.toast_timer = getattr(self, "toast_timer", None) or eTimer()
        try:
            self.toast_timer.callback.remove(self.end_toast)
        except ValueError:
            pass
        self.toast_timer.callback.append(self.end_toast)
        self.toast_timer.start(2500, True)

    def end_toast(self):
        if not self.closed and self.content().count():
            self.hide_message()

    def key_blue(self):
        if self.section == "search":
            self.open_section(self.last_section)
        else:
            self.set_rail_open(False)
            self.zone = "keys"
            self.open_section("search")

    def key_menu(self):
        self.session.open(plugin_module().DreaminatorScreen)

    def key_info(self):
        """EPG or Info: a film's card, or a channel's programme for the day."""
        if self.zone == "content":
            item = self.content().item()
            if item and item.get("kind") in ("movie", "series") and not item.get("missing"):
                self.open_card(item)
            elif item and item.get("kind") == "channel" and not item.get("missing"):
                self.open_guide(item)

    def key_erase(self):
        if self.section == "search":
            self.search.erase()

    def key_digit(self, digit):
        if self.section == "search":
            self.search.type_digit(digit)

    # ------------------------------------------------------------ cards and playback

    def open_card(self, item):
        try:
            from .ui_detail import DetailScreen
        except (ImportError, ValueError):
            from ui_detail import DetailScreen
        section = self.search.scope if self.section == "search" else self.section
        crumbs = [SECTION_TITLE.get(section, "")]
        if self.section == "search":
            crumbs.append("Поиск")
        elif self.categories:
            crumbs.append(ui_api.text(self.categories[self.cat_active].get("name", "")))
        source = dict(self.source_of(item), section=item.get("type") or SECTION_TYPE.get(section, "vod"))
        self.session.openWithCallback(self.card_closed, DetailScreen, source, item, " › ".join(crumbs), self.images,
                                      self.favorite_shown)

    def open_guide(self, item):
        try:
            from .ui_guide import GuideScreen
        except (ImportError, ValueError):
            from ui_guide import GuideScreen
        crumbs = [SECTION_TITLE[self.section if self.library() else "live"]]
        if self.section == "search":
            crumbs.append("Поиск")
        elif self.categories:
            crumbs.append(ui_api.text(self.categories[self.cat_active].get("name", "")))
        crumbs.append("Программа передач")

        def closed(*args):
            self.card_closed()
            if args and args[0] == "play" and not self.closed:
                self.play(item)

        self.session.openWithCallback(closed, GuideScreen, self.source_of(item), item, " › ".join(crumbs))

    def card_closed(self, *args):
        # Same card stays focused (its favorite mark may have changed in the
        # card); pictures resume where they were.
        if not self.closed:
            self.content().redraw_item(self.content().item(), self.zone == "content")
            self.update_focus_info()
            self.schedule_images()

    def play(self, item):
        if self.playing:
            return  # a second OK must not mint a second link
        self.playing = True
        self.show_message("Подготавливаю видео…")
        path = ui_api.catalog_path(item.get("source") or self.source_id(), "play")
        # By persistent id: it plays even after the list was refreshed meanwhile.
        self.api(path, self.open_player, self.play_failed, body=ui_library.entry(item), stale=self.play_dropped, convert=True)

    def play_dropped(self):
        # The viewer moved on while the link was minted; allow the next OK.
        self.playing = False

    def play_failed(self, problem):
        self.playing = False
        self.show_message(problem.message, "OK — попробовать снова")

    def open_player(self, result):
        self.hide_message()
        try:
            try:
                from .catalog_player import CatalogPlayer
            except (ImportError, ValueError):
                from catalog_player import CatalogPlayer
            # Playlist channels use the service type of the channel lists.
            ref = eServiceReference(service_type(result), 0, playback_url(result))
            ref.setName(ui_api.text(result.get("name") or "Dreaminator"))
            self.session.openWithCallback(self.player_closed, CatalogPlayer, ref)
        except Exception:
            self.playing = False
            self.show_message("Не удалось открыть проигрыватель", "OK — попробовать снова")

    def player_closed(self, *args):
        self.playing = False
        if not self.closed:
            self.hide_message()
            self.content().render(False, self.zone == "content")
            if args and args[0] == "failed":
                self.show_toast("Канал не отвечает", "Источник не отдал видео — попробуйте позже или другой канал")
