# -*- coding: utf-8 -*-
"""Card of a film or a series: backdrop, poster, facts, seasons and episodes.

Facts come from the portal and, when matched, from TMDB; playback always
uses the portal's own entry (a fresh link per OK). Seasons and episodes are
exactly what the portal lists: TMDB only adds names and descriptions to
episodes the portal has.
"""
from __future__ import absolute_import, division

from collections import OrderedDict

from enigma import eServiceReference, eTimer, getDesktop
from twisted.internet import threads
from Components.ActionMap import ActionMap
from Components.Label import Label
from Components.Pixmap import Pixmap
from Screens.Screen import Screen

try:
    from . import ui_api, ui_library, ui_theme as T, ui_trace
    from .ui_images import ImageLoader
    from .ui_models import ListModel, PagedList
    from .ui_widgets import RATING_LABEL, add_cell_components, clipped, color, duration, font, move, poster_cell_skin, visible
    from .catalog_state import playback_url
except (ImportError, ValueError):
    import ui_api
    import ui_library
    import ui_theme as T
    import ui_trace
    from ui_images import ImageLoader
    from ui_models import ListModel, PagedList
    from ui_widgets import RATING_LABEL, add_cell_components, clipped, color, duration, font, move, poster_cell_skin, visible
    from catalog_state import playback_url

D = T.DETAIL
CAST = T.CAST
INFO_X = 444
EPISODES = {"x": 96, "y": 690, "w": 1080, "row": 64, "pitch": 70, "rows": 4}
SEASON_CHIPS = 7
SEASON_Y = 610
BACKDROP_DELAY_MS = 350
ACTIONS = ("play", "about", "favorite")
ACTION_LABELS = {"play": "Смотреть", "about": "Описание", "favorite": "В избранное"}
ACTION_ICONS = {"play": "play", "about": "text", "favorite": "star"}


def build_skin(width, fonts):
    s = T.Skin(width, fonts)
    b = [s.pixmap("backdrop", D["backdrop_x"], 0, D["backdrop_w"], D["backdrop_h"], z=0, alpha="off"),
         s.label("crumbs", 96, 48, 1100, 32, 24, T.MUTED, z=2)]
    b.extend(poster_cell_skin(s, "d", 0, D["poster_x"], D["poster_y"], D["poster_w"], D["poster_h"], corners="corner_big", r=14))
    b += [s.label("title", INFO_X, 104, 1300, 140, 56, T.TEXT, semi=True, valign="bottom", wrap=True, z=2, shadow=True),
          s.label("original", INFO_X, 250, 1300, 36, 26, T.TEXT_2, z=2, shadow=True),
          s.label("facts", INFO_X, 296, 1300, 36, 24, T.TEXT_2, z=2, shadow=True)]
    for i in range(3):
        x = INFO_X + i * 222
        b.append(s.pixmap("rp_%d" % i, x, 348, 200, 52, z=1, asset="chip"))
        b.append(s.label("r_%d" % i, x, 348, 200, 52, 24, T.TEXT, semi=True, halign="center", z=2))
    b += [s.label("description", INFO_X, 420, 1080, 136, 26, T.TEXT_2, valign="top", wrap=True, z=2, shadow=True),
          s.label("credits", INFO_X, 562, 1300, 32, 22, T.MUTED, z=2, shadow=True),
          s.label("cast_label", INFO_X, CAST["y"] - 36, 400, 30, 22, T.MUTED, z=2, shadow=True)]
    # Cast portraits (TMDB): flat placeholder, picture, tiny corner masks, name.
    for i in range(CAST["count"]):
        x = CAST["x"] + i * CAST["pitch"]
        b.extend(poster_cell_skin(s, "cast", i, x, CAST["y"], CAST["w"], CAST["h"]))
        b.append(s.label("castn_%d" % i, x - 10, CAST["y"] + CAST["h"] + 6, CAST["w"] + 20, 46, 18, T.TEXT, halign="center", valign="top",
                         wrap=True, z=2, shadow=True))
        b.append(s.label("castr_%d" % i, x - 10, CAST["y"] + CAST["h"] + 52, CAST["w"] + 20, 22, 16, T.MUTED, halign="center", z=2,
                         shadow=True))
    for i, action in enumerate(ACTIONS):
        x = INFO_X + i * 320
        b.append(s.pixmap("ab_%d" % i, x, D["button_y"], 300, 72, z=1, asset="button"))
        b.append(s.pixmap("abf_%d" % i, x, D["button_y"], 300, 72, z=2, asset="button_focus"))
        b.append(s.pixmap("ai_%d" % i, x + 40, D["button_y"] + 20, 32, 32, z=3))
        b.append(s.label("a_%d" % i, x + 84, D["button_y"], 200, 72, 28, T.TEXT, semi=True, z=3))
    # Series: seasons and episodes (only what the portal lists).
    b.append(s.label("seasons_label", 96, SEASON_Y, 140, 52, 24, T.MUTED, z=2))
    for i in range(SEASON_CHIPS):
        x = 240 + i * 214
        b.append(s.pixmap("sp_%d" % i, x, SEASON_Y, 200, 52, z=1, asset="chip"))
        b.append(s.pixmap("spf_%d" % i, x, SEASON_Y, 200, 52, z=2, asset="chip_focus"))
        b.append(s.label("s_%d" % i, x, SEASON_Y, 200, 52, 22, T.TEXT, halign="center", z=3))
    e = EPISODES
    b.append(s.pixmap("ep_pill", e["x"], e["y"], e["w"], e["row"], z=1, asset="pill_episode"))
    for i in range(e["rows"]):
        y = e["y"] + i * e["pitch"]
        b.append(s.label("en_%d" % i, e["x"] + 16, y, 64, e["row"], 24, T.MUTED, halign="right", z=2))
        b.append(s.label("et_%d" % i, e["x"] + 104, y, 760, e["row"], 26, T.TEXT, z=2))
        b.append(s.label("ed_%d" % i, e["x"] + 880, y, 180, e["row"], 22, T.MUTED, halign="right", z=2))
    st = T.STILL
    b.extend(poster_cell_skin(s, "still", 0, st["x"], st["y"], st["w"], st["h"]))
    b += [s.label("ep_title", st["x"], st["y"] + st["h"] + 10, 620, 34, 24, T.TEXT, semi=True, z=2),
          s.label("ep_info", st["x"], st["y"] + st["h"] + 46, 620, 54, 20, T.TEXT_2, valign="top", wrap=True, z=2),
          s.label("status", INFO_X, 960, 1300, 36, 24, T.TEXT_2, z=2),
          s.label("footer", 96, 1000, 1300, 32, 20, T.MUTED, z=2),
          s.label("hint", 1240, 1000, 600, 32, 22, T.MUTED, halign="right", z=2)]
    return s, s.screen("DreaminatorDetail", b)


def _skin():
    try:
        from . import plugin
    except (ImportError, ValueError):
        import plugin
    return build_skin(getDesktop(0).size().width(), plugin.FONTS_READY)


SKIN, SKIN_XML = _skin()


class DetailScreen(Screen):
    skin = SKIN_XML

    def __init__(self, session, source, item, crumbs, images=None, favorite_shown=None):
        Screen.__init__(self, session)
        self.setTitle("Dreaminator")
        self.s = SKIN
        self.source = source
        self.item = dict(item)
        self.row = item  # the list's own row: its favorite mark follows the card
        self.favorite_shown = favorite_shown  # redraws that row in the list behind
        self.series = item.get("kind") == "series"
        self.pressed = False
        self.held = False
        self.fav_busy = False
        self.details = {}
        self.closed = False
        self.playing = False
        self.generation = 0
        self.zone = "seasons" if self.series else "actions"
        self.action_focus = 0
        self.seasons = []
        self.season_model = ListModel(SEASON_CHIPS)
        self.season_active = 0
        self.episodes = PagedList()
        self.episode_model = ListModel(EPISODES["rows"])
        self.episode_names = {}
        self.episode_generation = 0
        self.shared_images = images
        self.images = ImageLoader(budget=6 << 20)
        self.wants = OrderedDict()
        self.icons = {}

        for name in ("crumbs", "title", "original", "facts", "description", "credits", "cast_label", "seasons_label", "ep_title",
                     "ep_info", "status", "footer", "hint"):
            self[name] = Label("")
        for i in range(CAST["count"]):
            add_cell_components(self, "cast", i)
            self["castn_%d" % i] = Label("")
            self["castr_%d" % i] = Label("")
        add_cell_components(self, "still", 0)
        self.still_timer = eTimer()
        self.still_timer.callback.append(self.load_still)
        self.still_key = None
        self["backdrop"] = Pixmap()
        add_cell_components(self, "d", 0)
        for i in range(3):
            self["rp_%d" % i] = Pixmap()
            self["r_%d" % i] = Label("")
        for i in range(len(ACTIONS)):
            self["ab_%d" % i] = Pixmap()
            self["abf_%d" % i] = Pixmap()
            self["ai_%d" % i] = Pixmap()
            self["a_%d" % i] = Label("")
        for i in range(SEASON_CHIPS):
            self["sp_%d" % i] = Pixmap()
            self["spf_%d" % i] = Pixmap()
            self["s_%d" % i] = Label("")
        self["ep_pill"] = Pixmap()
        for i in range(EPISODES["rows"]):
            for name in ("en_%d", "et_%d", "ed_%d"):
                self[name % i] = Label("")
        self.backdrop_timer = eTimer()
        self.backdrop_timer.callback.append(self.load_backdrop)
        self.season_timer = eTimer()
        self.season_timer.callback.append(self.open_season)
        try:
            from . import plugin
        except (ImportError, ValueError):
            import plugin
        keymap = self.keymap = plugin.load_keymap()
        context = ["DreaminatorActions"] if keymap else ["OkCancelActions", "DirectionActions", "ChannelSelectBaseActions", "ColorActions"]
        handlers = {"ok": self.key_ok, "back": self.close, "cancel": self.close, "red": self.close,
                    "up": lambda: self.move(0, -1), "down": lambda: self.move(0, 1),
                    "left": lambda: self.move(-1, 0), "right": lambda: self.move(1, 0),
                    "pageNext": lambda: self.turn(1), "pagePrev": lambda: self.turn(-1),
                    "nextBouquet": lambda: self.turn(1), "prevBouquet": lambda: self.turn(-1), "info": self.show_description}
        if keymap:
            # OK acts on release; holding it adds the film or series to the favorites.
            handlers.update({"okDown": self.key_okdown, "okLong": self.key_oklong})
        self["actions"] = ActionMap(context, dict((k, self.guard(v)) for k, v in handlers.items()), -1)
        self.crumbs = crumbs
        self.onLayoutFinish.append(self.start)
        self.onClose.append(self.cleanup)

    def guard(self, handler):
        def run():
            if not self.closed:
                self.images.busy()
                handler()
        return run

    def api(self, path, ok, fail=None, body=None, convert=False, stale=None, method=None):
        generation = self.generation

        def done(result):
            if not self.closed and generation == self.generation:
                ok(result)
            elif stale:
                stale()

        def failed(failure):
            if not self.closed and generation == self.generation:
                if fail:
                    fail(ui_api.problem(failure))
            elif stale:
                stale()

        threads.deferToThread(ui_api.request_json, path, body, 35, convert, method).addCallbacks(done, failed)

    def want(self, *requests, **kw):
        """The card asks for pictures at different moments (poster, backdrop,
        portraits, still); the loader keeps only its latest list, so merge.
        drop= removes requests that went stale (a still for an old focus)."""
        for key in kw.get("drop", ()):
            self.wants.pop(key, None)
        for r in requests:
            key, callback = r[0], r[4]

            def done(k, pixmap, callback=callback):
                self.wants.pop(k, None)
                callback(k, pixmap)

            self.wants[key] = r[:4] + (done,) + r[5:]
        self.images.want(list(self.wants.values()))

    def path(self, operation, **params):
        return ui_api.catalog_path(self.source["id"], operation, **params)

    def icon(self, name, variant):
        key = (name, variant)
        if key not in self.icons:
            from Tools.LoadPixmap import LoadPixmap
            self.icons[key] = LoadPixmap(self.s.asset("icon_%s_%s" % (name, variant)), cached=True)
        return self.icons[key]

    def cleanup(self):
        self.closed = True
        self.generation += 1
        for timer in (self.backdrop_timer, self.season_timer, self.still_timer):
            timer.stop()
        self.images.close()

    # ------------------------------------------------------------ content

    def start(self):
        self["crumbs"].setText(ui_api.text(self.crumbs))
        self["backdrop"].hide()
        self.render_facts()
        self.show_poster()
        for i in range(len(ACTIONS)):
            visible(self["ab_%d" % i], not self.series)
            visible(self["a_%d" % i], not self.series)
            visible(self["ai_%d" % i], not self.series)
        self["seasons_label"].setText("Сезоны" if self.series else "")
        self.render_cast([])
        self.show_still(None, None)
        self.render_seasons()
        self.render_episodes()
        self.render_actions()
        self.render_hint()
        self.api(self.path("details", key=self.item["key"]), self.details_loaded, self.details_failed)
        if self.series:
            self["status"].setText("Загружаю сезоны…")
            self.load_seasons(1)

    def details_loaded(self, details):
        tmdb = self.details.get("tmdb")
        self.details = details or {}
        if tmdb:
            self.details["tmdb"] = tmdb
        self.render_facts()
        self.load_tmdb()

    def details_failed(self, problem):
        self.render_facts()
        self.load_tmdb()

    def load_tmdb(self):
        """TMDB facts arrive separately, so the portal's data shows at once."""
        def ok(result):
            info = (result or {}).get("tmdb")
            if not info:
                return
            self.details["tmdb"] = info
            self.render_facts()
            if info.get("hasBackdrop"):
                # The card is usable at once; the backdrop follows a moment later.
                self.backdrop_timer.start(BACKDROP_DELAY_MS, True)
            if info.get("hasPoster") and not self.item.get("hasPoster"):
                self.item["hasPoster"] = True
                self.show_poster()

        self.api(self.path("tmdb", key=self.item["key"]), ok, None)

    def render_facts(self):
        d, item = self.details, self.item
        t = ui_api.text
        tm = d.get("tmdb") or {}
        # TMDB's title is localized; the portal's name (often the original,
        # sometimes with "s01") stays visible on the line below.
        portal_name = d.get("name") or item.get("name") or ""
        title = tm.get("title") or portal_name
        self["title"].setText(clipped(title, 64))
        original = tm.get("originalTitle") or d.get("originalName") or ""
        if tm.get("title") and ui_api.unicode_text(portal_name).lower() not in (
                ui_api.unicode_text(title).lower(), ui_api.unicode_text(original).lower()):
            original = portal_name
        year = tm.get("year") or d.get("year") or item.get("year") or ""
        self["original"].setText(" · ".join(x for x in (t(original), t(year)) if x))
        facts = []
        minutes = tm.get("runtime") or d.get("duration") or item.get("duration")
        if duration(minutes):
            facts.append(duration(minutes) if not self.series else "серия ≈ " + duration(minutes))
        if d.get("age"):
            facts.append(t(d["age"]))
        genres = ", ".join(t(g) for g in tm.get("genres") or []) or t(d.get("genres") or "")
        if genres:
            facts.append(genres)
        if d.get("country"):
            facts.append(t(d["country"]))
        self["facts"].setText(clipped(" · ".join(facts), 90))
        ratings = []
        if tm.get("rating"):
            ratings.append(("TMDB", t(tm["rating"])))
        for r in d.get("ratings") or ([item["rating"]] if item.get("rating") else []):
            if r.get("value"):
                ratings.append((RATING_LABEL.get(r.get("source")) or t(r.get("source")), t(r["value"])))
        for i in range(3):
            if i < len(ratings):
                self["r_%d" % i].setText("%s  %s" % ratings[i])
                self["rp_%d" % i].show()
            else:
                self["r_%d" % i].setText("")
                self["rp_%d" % i].hide()
        description = tm.get("overview") or d.get("description") or ""
        self.full_description = t(description)
        self["description"].setText(clipped(description, 300 if not self.series else 200))
        credits = []
        directors = ", ".join(t(x) for x in tm.get("directors") or [])
        if directors:
            credits.append(("Создатели: " if tm.get("type") == "tv" else "Режиссёр: ") + directors)
        elif d.get("director"):
            credits.append("Режиссёр: " + t(d["director"]))
        cast = tm.get("cast") or []
        if d.get("actors") and (self.series or not cast):
            # The portal's own cast list when TMDB has no portraits to show.
            credits.append("В ролях: " + t(d["actors"]))
        self["credits"].setText(clipped(" · ".join(credits), 100))
        self.render_cast([] if self.series else cast)
        origin = []
        if tm:
            origin.append("Описание и рейтинг TMDB" if tm.get("overview") else "Данные TMDB")
        if any(src != "TMDB" for src, _ in ratings):
            origin.append("рейтинги IMDb и КП — по данным портала")
        origin.append("видео — портал %s" % t(self.source.get("name", "")))
        footer = ui_api.unicode_text(" · ".join(origin))
        self["footer"].setText(clipped(footer[:1].upper() + footer[1:], 130))

    def render_cast(self, cast):
        """Up to eight TMDB portraits under the description (films)."""
        self["cast_label"].setText("В ролях" if cast else "")
        sid = self.source["id"]
        requests = []
        w, h = self.s.px(CAST["w"]), self.s.px(CAST["h"])
        for i in range(CAST["count"]):
            person = cast[i] if i < len(cast) else None
            on = person is not None
            for name in ("castpe_%d", "castpi_%d"):
                visible(self[name % i], on)
            show_cells = on
            for part in ("tl", "tr", "bl", "br"):
                visible(self["castc%s_%d" % (part, i)], show_cells)
            if not on:
                self["castp_%d" % i].hide()
                self["castn_%d" % i].setText("")
                self["castr_%d" % i].setText("")
                continue
            self["castn_%d" % i].setText(clipped(person.get("name"), 28))
            self["castr_%d" % i].setText(clipped(person.get("character"), 14))
            key = "%s:%s:cast:%d" % (sid, self.item.get("key"), i)
            pixmap = self.images.get(key)
            self.set_cast_photo(i, pixmap)
            if pixmap is None and person.get("hasPhoto"):
                requests.append((key, self.path("image", key=self.item["key"], kind="cast", n=i), w, h,
                                 lambda k, p, i=i: self.set_cast_photo(i, p), "cover"))
        if requests:
            self.want(*requests)

    def set_cast_photo(self, i, pixmap):
        if self.closed:
            return
        if pixmap is not None:
            self["castp_%d" % i].instance.setPixmap(pixmap)
            self["castp_%d" % i].show()
            self["castpe_%d" % i].hide()
            self["castpi_%d" % i].hide()
        else:
            self["castp_%d" % i].hide()

    def show_still(self, item, extra):
        """The focused episode's still and TMDB description, after a short pause."""
        self.still_timer.stop()
        on = bool(item) and bool(extra)
        self["ep_title"].setText(clipped(extra.get("name"), 40) if on else "")
        self["ep_info"].setText(clipped(extra.get("overview"), 120) if on else "")
        has = on and extra.get("hasStill")
        for name in ("stillpe_0", "stillpi_0"):
            visible(self[name], has)
        for part in ("tl", "tr", "bl", "br"):
            visible(self["stillc%s_0" % part], has)
        key = "%s:%s:still" % (self.source["id"], item.get("key")) if has else None
        if key != self.still_key:
            self["stillp_0"].hide()
            if self.still_key:
                self.want(drop=[self.still_key])
        self.still_key = key
        if key:
            pixmap = self.images.get(key)
            if pixmap is not None:
                self.set_still(key, pixmap)
            else:
                self.still_item = item
                self.still_timer.start(300, True)

    def load_still(self):
        key = self.still_key
        if not key or self.closed:
            return
        st = T.STILL
        self.want((key, self.path("image", key=self.still_item["key"], kind="still"), self.s.px(st["w"]), self.s.px(st["h"]),
                   self.set_still, "cover"))

    def set_still(self, key, pixmap):
        # A still that arrives after the focus moved on is dropped.
        if self.closed or key != self.still_key:
            return
        self["stillp_0"].instance.setPixmap(pixmap)
        self["stillp_0"].show()
        self["stillpe_0"].hide()
        self["stillpi_0"].hide()

    def show_poster(self):
        key = "%s:%s:poster" % (self.source["id"], self.item.get("key"))
        pixmap = self.images.get(key)
        if pixmap is None and self.shared_images is not None:
            # The grid's smaller poster shows at once; the large one follows.
            pixmap = self.shared_images.get(key)
        self.set_poster(pixmap)
        if self.item.get("hasPoster"):
            w, h = self.s.px(D["poster_w"]), self.s.px(D["poster_h"])
            self.want((key + ":big", self.path("image", key=self.item["key"], kind="poster"), w, h, lambda k, p: self.set_poster(p), "cover"))

    def set_poster(self, pixmap):
        if self.closed:
            return
        if pixmap is not None:
            self["dp_0"].instance.setPixmap(pixmap)
            self["dp_0"].show()
        else:
            self["dp_0"].hide()
        visible(self["dpe_0"], pixmap is None)
        visible(self["dpi_0"], pixmap is None)

    def load_backdrop(self):
        if self.closed:
            return
        generation = self.generation
        key = "%s:%s:backdrop" % (self.source["id"], self.item.get("key"))

        def ready(k, pixmap):
            # Ignore a backdrop that arrives after the card was closed.
            if not self.closed and generation == self.generation:
                self["backdrop"].instance.setPixmap(pixmap)
                self["backdrop"].show()

        w, h = self.s.px(D["backdrop_w"]), self.s.px(D["backdrop_h"])
        self.want((key, self.path("image", key=self.item["key"], kind="backdrop"), w, h, ready))

    # ------------------------------------------------------------ series

    def load_seasons(self, page):
        def ok(result):
            self.seasons.extend(result.get("items") or [])
            if result.get("hasMore") and page < 10:
                self.load_seasons(page + 1)
                return
            if not self.seasons:
                self["status"].setText("Портал не отдал сезоны этого сериала")
                return
            self["status"].setText("")
            self.render_seasons()
            self.open_season()

        def fail(problem):
            self["status"].setText(problem.message)

        self.api(self.path("items", type=self.item_type(), parent=self.item["key"], page=page), ok, fail)

    def item_type(self):
        return self.item.get("type") or self.details.get("type") or self.source.get("section") or "vod"

    def season_label(self, season, index):
        number = season.get("number")
        if number:
            return "Сезон %s" % ui_api.text(number)
        name = ui_api.text(season.get("name") or "")
        return name or "Сезон %d" % (index + 1)

    def render_seasons(self):
        if not self.series:
            for i in range(SEASON_CHIPS):
                for name in ("sp_%d", "spf_%d", "s_%d"):
                    self[name % i].hide()
            return
        m = self.season_model
        m.clamp(len(self.seasons))
        for i in range(SEASON_CHIPS):
            index = m.top + i
            if index < len(self.seasons):
                focused = self.zone == "seasons" and index == m.focus
                self["s_%d" % i].setText(clipped(self.season_label(self.seasons[index], index), 16))
                self["s_%d" % i].show()
                visible(self["spf_%d" % i], focused)
                visible(self["sp_%d" % i], not focused)
                color(self, "s_%d" % i, T.ON_ACCENT if focused else (T.ACCENT if index == self.season_active else T.TEXT))
                font(self, self.s, "s_%d" % i, 22, focused or index == self.season_active)
            else:
                for name in ("sp_%d", "spf_%d", "s_%d"):
                    self[name % i].hide()

    def open_season(self):
        """Load episodes of the focused season; older answers are dropped."""
        if not self.seasons:
            return
        self.season_active = self.season_model.focus
        self.render_seasons()
        self.episode_generation += 1
        self.episodes = PagedList()
        self.episode_model = ListModel(EPISODES["rows"])
        self.episode_names = {}
        self.render_episodes()
        self["status"].setText("Загружаю серии…")
        self.load_episodes(1)

    def load_episodes(self, page):
        season = self.seasons[self.season_active]
        generation = self.episode_generation
        self.episodes.loading.add(page)

        def ok(result):
            if generation != self.episode_generation:
                return
            self.episodes.store(page, result)
            self["status"].setText("" if self.episodes.count() else "В этом сезоне нет серий")
            self.render_episodes()
            if page == 1:
                self.load_episode_names(season, generation)

        def fail(problem):
            if generation == self.episode_generation:
                self.episodes.fail(page, problem)
                self["status"].setText(problem.message)

        self.api(self.path("items", type=self.item_type(), parent=season["key"], page=page), ok, fail)

    def load_episode_names(self, season, generation):
        """TMDB names for episodes the portal has (if the series is matched)."""
        def ok(result):
            if generation == self.episode_generation:
                for ep in result.get("episodes") or []:
                    self.episode_names[str(ep.get("number"))] = ep
                self.render_episodes()

        self.api(self.path("episodes", key=season["key"]), ok, None)

    def render_episodes(self):
        e = EPISODES
        m = self.episode_model
        count = self.episodes.count()
        m.clamp(count)
        for i in range(e["rows"]):
            index = m.top + i
            item = self.episodes.get(index) if index < count else None
            if item is None:
                for name in ("en_%d", "et_%d", "ed_%d"):
                    self[name % i].setText("")
                continue
            number = ui_api.text(item.get("number") or "")
            extra = self.episode_names.get(number) or {}
            name = extra.get("name") and "%s" % ui_api.text(extra["name"]) or ui_api.text(item.get("name") or "")
            self["en_%d" % i].setText(number)
            self["et_%d" % i].setText(clipped(name, 48))
            self["ed_%d" % i].setText(duration(extra.get("runtime") or item.get("duration")))
            focused = self.zone == "episodes" and index == m.focus
            for name_ in ("et_%d", "en_%d", "ed_%d"):
                color(self, name_ % i, T.ON_ACCENT if focused else (T.TEXT if name_ == "et_%d" else T.MUTED))
        focused_row = m.focus - m.top
        if self.zone == "episodes" and count:
            move(self, self.s, "ep_pill", e["x"], e["y"] + focused_row * e["pitch"])
            self["ep_pill"].show()
            item = self.episodes.get(m.focus) or {}
            self.show_still(item, self.episode_names.get(ui_api.text(item.get("number") or "")) or {})
        else:
            self["ep_pill"].hide()
            self.show_still(None, None)

    # ------------------------------------------------------------ focus

    def render_hint(self):
        held = "убрать из избранного" if self.item.get("fav") else "в избранное"
        self["hint"].setText("OK долго — %s · Back — назад" % held)

    def render_actions(self):
        for i, action in enumerate(ACTIONS):
            focused = self.zone == "actions" and i == self.action_focus and not self.series
            visible(self["abf_%d" % i], focused)
            label = ACTION_LABELS[action]
            if action == "favorite" and self.item.get("fav"):
                label = "В избранном"
            self["a_%d" % i].setText(label)
            color(self, "a_%d" % i, T.ON_ACCENT if focused else T.TEXT)
            pixmap = self.icon(ACTION_ICONS[action], "dark" if focused else "light")
            if pixmap and not self.series:
                self["ai_%d" % i].instance.setPixmap(pixmap)

    def set_zone(self, zone):
        self.zone = zone
        self.render_actions()
        self.render_seasons()
        self.render_episodes()

    def move(self, dx, dy):
        if self.zone == "actions":
            if dx:
                self.action_focus = max(0, min(len(ACTIONS) - 1, self.action_focus + dx))
                self.render_actions()
            return
        if self.zone == "seasons":
            if dx:
                before = self.season_model.focus
                self.season_model.move(0, dx, len(self.seasons))
                self.render_seasons()
                if self.season_model.focus != before:
                    # Wait until the viewer settles on a season.
                    self.season_timer.start(400, True)
            elif dy > 0 and self.episodes.count():
                self.set_zone("episodes")
            return
        if self.zone == "episodes":
            if dy:
                before = self.episode_model.focus
                result = self.episode_model.move(0, dy, self.episodes.count())
                if dy < 0 and before == self.episode_model.focus and self.episode_model.focus == 0:
                    self.set_zone("seasons")
                    return
                self.render_episodes()
                self.ensure_episodes()

    def turn(self, delta):
        if self.zone == "episodes":
            self.episode_model.page(delta, self.episodes.count())
            self.render_episodes()
            self.ensure_episodes()

    def ensure_episodes(self):
        m = self.episode_model
        for n in self.episodes.missing(m.top, m.top + 2 * EPISODES["rows"])[:1]:
            self.load_episodes(n)

    def key_okdown(self):
        self.pressed, self.held = True, False
        ui_library.OK_DOWN_SEEN[0] = True

    def key_oklong(self):
        if self.held or not self.pressed:
            return  # one change per press, however long it is held
        self.held = True
        self.toggle_favorite()

    def toggle_favorite(self):
        """The film or series of the card into the favorites, or out."""
        if self.fav_busy:
            return
        method, path, body = ui_library.favorite_request(self.source["id"], self.item)
        self.fav_busy = True

        def ok(result):
            # The list's row learns the answer even if the card closed meanwhile.
            self.fav_busy = False
            fav = bool(result.get("fav"))
            self.item["fav"] = self.row["fav"] = fav
            if self.favorite_shown:
                self.favorite_shown(self.row)
            if not self.closed:
                self.render_actions()
                self.render_hint()
                self["status"].setText(ui_library.favorite_message(self.item, result)[0])

        def fail(failure):
            self.fav_busy = False
            if not self.closed:
                self["status"].setText(ui_api.problem(failure).message)

        threads.deferToThread(ui_api.request_json, path, body, 35, True, method).addCallbacks(ok, fail)

    def key_ok(self):
        pressed, held = self.pressed, self.held
        self.pressed = self.held = False
        if self.keymap and ui_library.OK_DOWN_SEEN[0] and not pressed:
            return  # OK went down on another screen (the description) that closed on it
        if held:
            return  # the release of a long press
        if self.zone == "seasons":
            self.season_timer.stop()
            if self.season_model.focus != self.season_active or not self.episodes.count():
                self.open_season()
            elif self.episodes.count():
                self.set_zone("episodes")
            return
        if self.zone == "episodes":
            item = self.episodes.get(self.episode_model.focus)
            if item:
                self.play(item)
            return
        if ACTIONS[self.action_focus] == "about":
            self.show_description()
        elif ACTIONS[self.action_focus] == "favorite":
            self.toggle_favorite()
        else:
            self.play(self.item)

    def show_description(self):
        text = getattr(self, "full_description", "")
        if text:
            from Screens.MessageBox import MessageBox
            self.session.open(MessageBox, text, MessageBox.TYPE_INFO)

    # ------------------------------------------------------------ playback

    def play(self, item):
        if self.playing:
            return  # a second OK must not mint a second link
        self.playing = True
        self["status"].setText("Подготавливаю видео…")
        self.api(self.path("play"), self.open_player, self.play_failed, body=ui_library.entry(item), convert=True,
                 stale=self.play_dropped)

    def play_dropped(self):
        self.playing = False

    def play_failed(self, problem):
        self.playing = False
        self["status"].setText(problem.message + " · OK — ещё раз")

    def open_player(self, result):
        self["status"].setText("")
        try:
            try:
                from .catalog_player import CatalogPlayer
            except (ImportError, ValueError):
                from catalog_player import CatalogPlayer
            ref = eServiceReference(4097, 0, playback_url(result))
            ref.setName(ui_api.text(result.get("name") or "Dreaminator"))
            self.session.openWithCallback(self.player_closed, CatalogPlayer, ref)
        except Exception:
            self.playing = False
            self["status"].setText("Не удалось открыть проигрыватель · OK — ещё раз")

    def player_closed(self, *args):
        # Back on the card with the same focus; the next OK mints a new link.
        self.playing = False
        if not self.closed:
            self.set_zone(self.zone)
            if args and args[0] == "failed":
                self["status"].setText("Видео не запустилось: портал не отдал поток · OK — ещё раз")
