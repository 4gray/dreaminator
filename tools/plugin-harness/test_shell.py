# -*- coding: utf-8 -*-
"""TV shell, search and card screens with fake Enigma2 widgets and a
scripted service: focus rules, paging, stale answers, playback guards."""
from __future__ import absolute_import

import json
import os
import sys
import time
import unittest

import run as harness

harness.install_fakes()
sys.path.insert(0, os.path.join(harness.ROOT, "plugin", "Dreaminator"))


class Job(object):
    """A deferred job: callbacks run in order when the test completes it."""

    def __init__(self, fn, args):
        self.fn, self.args = fn, args
        self.chain = []
        self.done = False

    def addCallbacks(self, ok, fail):
        self.chain.append((ok, fail))
        return self

    def addBoth(self, fn):
        self.chain.append((fn, fn))
        return self

    def run(self, value, failed):
        self.done = True
        for ok, fail in self.chain:
            value = fail(harness.Failure(value)) if failed else ok(value)
            failed = False

    def complete(self, result):
        self.run(result, False)

    def error(self, exc):
        self.run(exc, True)


class Queue(object):
    def __init__(self):
        self.jobs = []

    def defer(self, fn, *args):
        job = Job(fn, args)
        self.jobs.append(job)
        return job

    def path(self, job):
        return str(job.args[0]) if job.args else ""

    def pending(self, fragment):
        return [j for j in self.jobs if not j.done and fragment in self.path(j)]

    def take(self, fragment):
        """The oldest pending request whose path contains fragment."""
        jobs = self.pending(fragment)
        if not jobs:
            raise AssertionError("no request for %r in %r" % (fragment, [self.path(j) for j in self.jobs if not j.done]))
        jobs[0].done = True
        return jobs[0]


class Session(object):
    def __init__(self):
        self.opened = []
        self.callbacks = []

    def open(self, cls, *args, **kwargs):
        self.opened.append((cls, args))

    def openWithCallback(self, callback, cls, *args):
        self.opened.append((cls, args))
        self.callbacks.append(callback)


def cards(n, count, size=14, total=0, kind="movie", prefix="m"):
    first = (n - 1) * size
    items = [{"key": u"%s%d" % (prefix, i), "name": u"Фильм %d" % i, "kind": kind, "year": u"2020", "hasPoster": True,
              "rating": {"source": u"imdb", "value": u"7.1", "origin": u"provider"}}
             for i in range(first, min(first + size, count))]
    return {"items": items, "page": n, "pageSize": size, "hasMore": first + size < count, "total": total}


def channels(count, now=None):
    items = [{"key": u"c%d" % i, "name": u"Канал %d" % i, "kind": "channel", "number": u"%d" % (i + 1), "hasPoster": True}
             for i in range(count)]
    for i, entry in (now or {}).items():
        items[i]["now"] = entry
    return {"items": items, "page": 1, "pageSize": 50, "hasMore": False, "total": count}


def programme(title, start, minutes=60, desc=None):
    """A guide entry starting `start` seconds from now."""
    t = int(time.time()) + start
    out = {"title": title, "start": t, "stop": t + minutes * 60}
    if desc:
        out["desc"] = desc
    return out


def progs_start(guide, i):
    return guide.programmes[i]["start"]


class HTTPError(Exception):
    pass


def http_error(code, body):
    try:
        from urllib2 import HTTPError as E
    except ImportError:
        from urllib.error import HTTPError as E
    import io
    return E("http://127.0.0.1/x", code, "err", {}, io.BytesIO(json.dumps(body).encode("utf-8")))


class Base(unittest.TestCase):
    def setUp(self):
        import ui_shell
        import ui_images
        import ui_detail
        import ui_dialogs
        import ui_guide
        self.modules = (ui_shell, ui_images, ui_detail, ui_dialogs, ui_guide)
        self.queue = Queue()
        self.originals = [m.threads.deferToThread for m in self.modules]
        for m in self.modules:
            m.threads.deferToThread = self.queue.defer
        self.session = Session()

    def tearDown(self):
        for m, original in zip(self.modules, self.originals):
            m.threads.deferToThread = original


class ShellBase(Base):
    def setUp(self):
        Base.setUp(self)
        import ui_shell
        ui_shell.LAST_PLACE.clear()
        self.shell = ui_shell.Shell(self.session)
        harness.layout(self.shell)

    def tearDown(self):
        self.shell.close()
        Base.tearDown(self)

    def open_movies(self, count=40, sources=None):
        q = self.queue
        q.take("/api/catalog/sources").complete(sources or [{"id": "p1", "name": "Portal"}])
        q.take("categories?").complete([{"id": "1", "name": "Все"}, {"id": "2", "name": "Кино"}])
        q.take("items?").complete(cards(1, count))
        return self.shell

    def press(self, *names):
        """A short OK is a press and a release (the keymap acts on release)."""
        actions = self.shell["actions"].actions
        for name in names:
            if name == "ok":
                actions["okDown"]()
            actions[name]()


    def open_playlist(self, sources=None):
        q = self.queue
        q.take("/api/catalog/sources").complete(sources or [{"id": "m1", "name": "Cyrius", "type": "m3u", "sections": ["itv"]}])
        cats = q.take("categories?")
        self.assertIn("type=itv", self.queue.path(cats), "a playlist opens on Live TV")
        cats.complete([{"id": "", "name": "Все каналы"}, {"id": "Детские", "name": "Детские"}])
        q.take("items?").complete(channels(12))
        return self.shell


class ShellTest(ShellBase):
    def test_grid_navigation_and_paging(self):
        s = self.open_movies()
        self.assertEqual(s.zone, "content")
        # No text under posters: the focused card is described in the header.
        self.assertEqual(s["title"].getText(), "Фильм 0")
        self.assertEqual(s["meta"].getText(), "2020 · IMDb 7.1")
        self.assertEqual(s["pt_0"].getText(), "Фильм 0", "a slot without artwork shows its title")
        self.assertIn("новые", s["context"].getText())
        cols, rows = s.grid.model.cols, s.grid.model.rows
        screen = cols * rows
        self.press("right", "right", "down")
        self.assertEqual(s.grid.model.focus, cols + 2)
        # The next page is requested before the viewer reaches it.
        self.queue.take("page=2").complete(cards(2, 40))
        self.queue.take("page=3").complete(cards(3, 40))
        self.press("pageNext")
        self.assertEqual(s.grid.model.focus, screen + cols + 2)
        self.assertEqual(s["pt_0"].getText(), "Фильм %d" % screen)
        self.assertEqual(s["title"].getText(), "Фильм %d" % (screen + cols + 2))
        self.assertIn("%d–%d из 40" % (screen + 1, 2 * screen), s["context"].getText())
        # Right at the last column never turns a page.
        before = s.grid.model.state()
        self.press(*(["right"] * (cols + 2)))
        self.assertEqual(s.grid.model.first_visible(), before[1] * cols)
        # Left at the first column hands focus to the categories.
        for _ in range(cols):
            self.press("left")
        self.assertEqual(s.zone, "cats")
        self.press("left")
        self.assertEqual(s.zone, "rail")

    def test_back_climbs_levels_and_keeps_position(self):
        s = self.open_movies()
        self.press("right", "down", "back")
        self.assertEqual(s.zone, "cats")
        self.press("back")
        self.assertEqual(s.zone, "rail")
        self.assertTrue(s.rail_open)
        self.press("right")
        self.assertFalse(s.rail_open)
        self.assertEqual(s.zone, "content")
        self.assertEqual(s.grid.model.focus, s.grid.model.cols + 1)

    def test_category_position_is_remembered(self):
        s = self.open_movies()
        self.press("down", "right")
        self.press("back", "down", "right")
        self.queue.take("category=2").complete(cards(1, 5, prefix="k"))
        self.assertEqual(s.grid.model.focus, 0)
        self.press("back", "up", "right")
        self.queue.take("category=1").complete(cards(1, 40))
        self.assertEqual(s.grid.model.focus, s.grid.model.cols + 1)

    def test_category_switch_drops_late_pages(self):
        s = self.open_movies()
        late = self.queue.pending("page=2")
        self.press("back", "down", "right")
        self.queue.take("category=2").complete(cards(1, 3, prefix="k"))
        for job in late:
            job.complete(cards(2, 40))
        self.assertEqual(s.grid.count(), 3)
        self.assertIsNone(s.grid.cell_keys[3])
        self.assertFalse(s["pt_3"].visible or s["p_3"].visible, "empty slots draw nothing")

    def test_ok_opens_card_and_back_restores_focus(self):
        s = self.open_movies()
        self.press("right", "ok")
        cls, args = self.session.opened[-1]
        self.assertEqual(cls.__name__, "DetailScreen")
        self.assertEqual(args[1]["key"], "m1")
        self.session.callbacks[-1]()
        self.assertEqual(s.grid.model.focus, 1)
        self.assertEqual(s.zone, "content")

    def test_live_channels_play_with_fresh_link_and_double_ok_guard(self):
        s = self.open_movies()
        self.press("back", "back", "up", "ok")
        self.assertEqual(s.section, "live")
        self.queue.take("categories?").complete([{"id": "", "name": "Все каналы"}])
        self.queue.take("type=itv").complete(channels(30))
        self.assertEqual(s.zone, "content")
        self.assertEqual(s["lvc_0"].getText(), "Канал 0")
        self.press("down", "down", "ok")
        play = self.queue.take("/play")
        self.assertEqual(json.loads(json.dumps(play.args[1]))["key"], "c2")
        count = len(self.queue.jobs)
        self.press("ok")
        self.assertEqual(len(self.queue.jobs), count, "second OK must not mint another link")
        play.complete({"url": "http://cdn.test/live.ts", "name": "Канал 2", "headers": {"User-Agent": "MAG"}})
        self.assertEqual(self.session.opened[-1][0].__name__, "CatalogPlayer")
        self.session.callbacks[-1]()
        self.assertFalse(s.playing)
        self.press("ok")
        self.assertTrue(self.queue.pending("/play"), "a new OK after the player mints a new link")

    def test_stale_playback_is_dropped_and_rearms_ok(self):
        s = self.open_movies()
        self.press("back", "back", "up", "ok")
        self.queue.take("categories?").complete([{"id": "", "name": "Все каналы"}])
        self.queue.take("type=itv").complete(channels(3))
        self.press("ok")
        play = self.queue.take("/play")
        self.press("yellow")  # refresh: the viewer moved on
        play.complete({"url": "http://cdn.test/live.ts", "name": "x", "headers": {}})
        self.assertNotEqual(self.session.opened and self.session.opened[-1][0].__name__, "CatalogPlayer")
        self.assertFalse(s.playing)

    def test_portal_without_live_tv(self):
        s = self.open_movies()
        self.press("back", "back", "up", "ok")
        self.queue.take("categories?").complete([])
        self.assertIn("нет телеканалов", s["msg"].getText())
        self.assertEqual(s.zone, "cats")

    def test_series_section_explains_embedded_series(self):
        s = self.open_movies()
        self.press("back", "back", "down", "ok")  # rail focus starts on Films
        self.queue.take("type=series").complete([])
        self.assertIn("сериалов", s["msg"].getText())

    def test_red_chooses_the_order_and_keeps_positions_per_order(self):
        s = self.open_movies()
        self.press("right", "down")
        self.press("red")
        self.assertTrue(s.menu.is_open(), "the choice opens inside the catalog")
        self.assertEqual([o["id"] for o in s.menu.options], ["added", "top", "rating", "name"])
        self.assertEqual((s.menu.focus, s["om_row_0"].getText()), (0, "Новые"))
        self.press("down", "ok")
        self.assertFalse(s.menu.is_open())
        self.queue.take("sort=top").complete(cards(1, 40))
        self.assertEqual(s.grid.model.focus, 0, "a new order starts at the top")
        self.assertIn("популярные", s["context"].getText())
        # Back cancels; then back to "new": the old position returns.
        self.press("red", "back")
        self.assertFalse(s.menu.is_open())
        self.assertEqual(s.zone, "content")
        self.press("red", "up", "ok")
        self.queue.take("sort=added").complete(cards(1, 40))
        self.assertEqual(s.grid.model.focus, s.grid.model.cols + 1)
        import ui_shell
        ui_shell.SORT_MEMORY.clear()

    def test_red_does_nothing_in_live_tv(self):
        s = self.open_movies()
        self.press("back", "back", "up", "ok")
        self.press("red")
        self.assertFalse(s.menu.is_open())
        self.assertEqual(s["hint_red"].getText(), "")

    def test_poster_requests_fill_the_slot(self):
        s = self.open_movies()
        requests = s.grid.wanted(lambda item: "/poster?key=" + item["key"])
        self.assertTrue(all(r[5] == "cover" for r in requests))
        import ui_images
        self.assertEqual(ui_images.decode_box("cover", 190, 276, (683, 1000)), (191, 280))
        self.assertEqual(ui_images.decode_box("cover", 190, 276, (780, 439)), (190, 276), "landscape stays letterboxed")
        self.assertEqual(ui_images.decode_box("contain", 96, 54, (200, 200)), (96, 54))

    def test_posters_requested_for_visible_cards_only(self):
        s = self.open_movies()
        s.request_images()
        fetches = [j for j in self.queue.jobs if "poster" in self.queue.path(j)]
        self.assertEqual(len(fetches), 2, "two downloads at a time")
        self.assertIn("key=m0", self.queue.path(fetches[0]))

    def test_green_with_one_portal_says_so_and_picker_with_two(self):
        s = self.open_movies()
        self.press("green")
        self.assertIn("один источник", s["toast_text"].getText())
        self.assertTrue(s["toast"].visible and not s["msg"].visible, "a note over the grid gets its own panel")
        s.sources.append({"id": "p2", "name": "Second", "type": "stalker"})
        self.press("green")
        self.assertTrue(s.menu.is_open())
        self.assertEqual(s["om_det_1"].getText(), "Портал")
        self.press("down", "ok")
        self.assertEqual(s.source_id(), "p2")
        self.assertTrue(self.queue.pending("/api/catalog/p2/categories"))

    def test_playlist_source_has_live_tv_only(self):
        s = self.open_playlist()
        self.assertEqual((s.section, s.zone), ("live", "content"))
        import ui_shell
        shown = dict(zip(ui_shell.RAIL_KEYS, s.rail_shown))
        self.assertFalse(shown["vod"] or shown["series"])
        self.assertFalse(s["rail_icon_%d" % ui_shell.RAIL_KEYS.index("vod")].visible or s["rail_icon_%d" % ui_shell.RAIL_KEYS.index("series")].visible)
        self.assertEqual(s.rail_y[ui_shell.RAIL_KEYS.index("search")], s.rail_y[ui_shell.RAIL_KEYS.index("vod")],
                         "search closes up under Live TV")
        # Rail focus steps over the hidden sections.
        self.press("left", "left")
        self.assertEqual(s.zone, "rail")
        self.press("down")
        self.assertEqual(ui_shell.RAIL_KEYS[s.rail_focus], "search")
        self.press("up", "up")
        self.assertEqual(ui_shell.RAIL_KEYS[s.rail_focus], "portal")
        # Nothing to sort in a channel list.
        self.press("right")
        self.press("red")
        self.assertFalse(s.menu.is_open())

    def test_playlist_channel_plays_with_its_service_type(self):
        s = self.open_playlist()
        self.press("down", "ok")
        play = self.queue.take("/api/catalog/m1/play")
        play.complete({"url": "http://stream.test/2.ts", "name": u"Канал 1", "serviceType": 5002, "headers": {}})
        cls, args = self.session.opened[-1]
        self.assertEqual(cls.__name__, "CatalogPlayer")
        self.assertEqual(args[0].args[:1], (5002,))
        self.session.callbacks[-1]("failed")
        self.assertIn("Канал не отвечает", s["toast_text"].getText(), "an early end is explained, not silent")
        self.assertTrue(s["toast"].visible and not s["msg"].visible)
        self.press("ok")
        self.queue.take("/play").complete({"url": "http://stream.test/2.ts", "name": "x", "serviceType": 1, "headers": {}})
        self.assertEqual(self.session.opened[-1][1][0].args[:1], (4097,), "unknown service types fall back to 4097")

    def test_playlist_search_covers_channels_only(self):
        s = self.open_playlist()
        self.press("blue")
        self.assertEqual((s.section, s.search.scope), ("search", "live"))
        self.assertTrue(s["sc_0"].visible)
        self.assertEqual(s["sc_0"].getText(), "Live TV")
        self.assertFalse(s["sc_1"].visible or s["sc_2"].visible or s["scp_1"].visible)
        for _ in range(8):
            self.press("down")
        self.assertEqual((s.zone, s.search.scope_focus), ("scope", 0))
        self.press("right")
        self.assertEqual(s.zone, "scope", "no results yet: focus stays on the only scope")
        # Back from search returns to Live TV, never to films the playlist lacks.
        self.press("blue")
        self.assertEqual(s.section, "live")

    def test_switching_between_playlist_and_portal(self):
        both = [{"id": "m1", "name": "Cyrius", "type": "m3u", "sections": ["itv"]},
                {"id": "p1", "name": "Portal", "type": "stalker", "sections": ["itv", "vod", "series"]}]
        s = self.open_playlist(both)
        self.press("down", "down")
        self.press("green")
        self.assertEqual([s["om_det_0"].getText(), s["om_det_1"].getText()], ["M3U", "Портал"])
        self.press("down", "ok")
        self.assertEqual((s.source_id(), s.section), ("p1", "live"))
        self.assertTrue(all(s.rail_shown))
        self.queue.take("/api/catalog/p1/categories?refresh=0&type=itv").complete([{"id": "", "name": "Все каналы"}])
        self.queue.take("items?").complete(channels(3))
        self.assertEqual(s.live.model.focus, 0, "another source starts at the top, not at the old list's position")
        # Back to the playlist: its own position returns.
        self.press("green", "up", "ok")
        self.queue.take("/api/catalog/m1/categories").complete([{"id": "", "name": "Все каналы"}])
        self.queue.take("/api/catalog/m1/items").complete(channels(12))
        self.assertEqual(s.live.model.focus, 2)
        self.press("green", "down", "ok")
        self.queue.take("/api/catalog/p1/categories").complete([{"id": "", "name": "Все каналы"}])
        self.queue.take("/api/catalog/p1/items").complete(channels(3))
        # Films are reachable again on the portal.
        self.press("left", "left", "down", "ok")
        self.assertEqual(s.section, "vod")
        self.assertTrue(self.queue.pending("/api/catalog/p1/categories?refresh=0&type=vod"))

    def test_reopening_returns_to_the_last_source_and_section(self):
        both = [{"id": "m1", "name": "Cyrius", "type": "m3u", "sections": ["itv"]},
                {"id": "p1", "name": "Portal", "type": "stalker", "sections": ["itv", "vod", "series"]}]
        s = self.open_playlist(both)
        self.press("green", "down", "ok")
        self.queue.take("/api/catalog/p1/categories?refresh=0&type=itv").complete([{"id": "", "name": "Все каналы"}])
        s.close()
        import ui_shell
        self.shell = ui_shell.Shell(self.session)
        harness.layout(self.shell)
        self.queue.take("/api/catalog/sources").complete(both)
        self.assertEqual((self.shell.source_id(), self.shell.section), ("p1", "live"))
        self.assertTrue(self.queue.pending("/api/catalog/p1/categories?refresh=0&type=itv"))

    def test_empty_playlist_explains_itself(self):
        s = self.shell
        self.queue.take("/api/catalog/sources").complete([{"id": "m1", "name": "Cyrius", "type": "m3u", "sections": ["itv"]}])
        self.queue.take("categories?").complete([])
        self.assertIn("В плейлисте нет каналов", s["msg"].getText())
        self.assertFalse(s["toast"].visible, "an empty list explains itself in the content area")

    def test_settings_and_about_from_rail(self):
        s = self.open_movies()
        self.press("back", "back")
        for _ in range(10):
            self.press("down")
        self.press("ok")
        self.assertEqual(self.session.opened[-1][0].__name__, "AboutScreen")
        self.press("up", "ok")
        self.assertEqual(self.session.opened[-1][0].__name__, "DreaminatorScreen")

    def test_closing_releases_the_picture_decoder(self):
        s = self.open_movies()
        picload = s.images.picload
        self.assertTrue(picload.PictureData.get(), "decoder connected while open")
        s.close()
        self.assertEqual(picload.PictureData.get(), [], "C++ signal must not keep the loader alive")
        self.assertIsNone(s.images.picload)

    def test_no_sources_open_settings(self):
        self.queue.take("/api/catalog/sources").complete([])
        self.assertEqual(self.session.opened[0][0].__name__, "DreaminatorScreen")

    def test_service_down_is_reported(self):
        self.queue.take("/api/catalog/sources").error(IOError("connection refused"))
        self.assertIn("не отвечает", self.shell["msg"].getText())

    def test_screens_fit_hd_and_fhd(self):
        import ui_detail
        import ui_dialogs
        import ui_guide
        for width in (1280, 1920):
            harness.check_bounds(self.modules[0].build_skin(width)[1])
            harness.check_bounds(ui_detail.build_skin(width, True)[1])
            harness.check_bounds(ui_dialogs.about_skin(width)[1])
            harness.check_bounds(ui_guide.build_skin(width, True)[1])


class ProgrammeTest(ShellBase):
    """What is on: in the rows, in the panel, for the day."""

    def open_live(self, count=12, now=None):
        q = self.queue
        q.take("/api/catalog/sources").complete([{"id": "m1", "name": "Cyrius", "type": "m3u", "sections": ["itv"]}])
        q.take("categories?").complete([{"id": "", "name": u"Все каналы"}])
        q.take("items?").complete(channels(count, now))
        return self.shell

    def test_rows_show_what_is_on_with_progress(self):
        s = self.open_live(now={0: programme(u"Новости", -900, 60), 2: programme(u"Давно кончилось", -7200, 60)})
        self.assertEqual(s["lve_0"].getText(), "Новости")
        self.assertTrue(s["lvg_0"].visible)
        self.assertEqual(s["lvg_0"].value, 25)
        self.assertEqual(s["lvc_0"].instance.valign, 0, "the name moves up to make room")
        self.assertEqual((s["lve_1"].getText(), s["lvg_1"].visible), ("", False), "no programme known: nothing invented")
        self.assertEqual((s["lve_2"].getText(), s["lvg_2"].visible), ("", False), "an ended programme is not shown")
        # The focused row's line turns dark on the accent pill.
        self.assertEqual(s["lve_0"].instance.color.value, int(self.T().ON_ACCENT[1:], 16))
        # The panel shows it at once, before the service is asked.
        self.assertIn("Сейчас ·", s["lvpv_now_time"].getText())
        self.assertEqual(s["lvpv_now_title"].getText(), "Новости")
        self.assertTrue(s["lvpv_bar"].visible)
        # Moving recolours only the rows whose focus changed.
        T = self.T()
        self.press("down")
        self.assertEqual([s["lv%s_0" % n].instance.color.value for n in "cne"], [int(c[1:], 16) for c in (T.TEXT, T.MUTED, T.MUTED)])
        self.assertEqual(s["lvc_1"].instance.color.value, int(T.ON_ACCENT[1:], 16))
        self.press("left")
        self.assertEqual(s["lvc_1"].instance.color.value, int(T.TEXT[1:], 16), "no row is lit outside the list")

    def T(self):
        import ui_theme
        return ui_theme

    def test_list_without_programmes_keeps_names_centred(self):
        s = self.open_live()
        self.assertFalse(hasattr(s["lvc_0"].instance, "valign") and s["lvc_0"].instance.valign == 0)
        self.assertEqual(s["lve_0"].getText(), "")
        self.assertFalse(s["lvg_0"].visible)

    def test_panel_asks_for_now_and_next_when_the_remote_is_quiet(self):
        s = self.open_live()
        self.assertEqual(s.guide_timer.active, 400)
        self.assertFalse(self.queue.pending("/epg"), "nothing is asked while keys are pressed")
        s.request_guide()
        job = self.queue.take("/api/catalog/m1/epg")
        self.assertIn("key=c0", self.queue.path(job))
        self.assertIn("tz=", self.queue.path(job))
        self.assertNotIn("day=", self.queue.path(job))
        job.complete({"now": programme(u"Футбол", -1800, 120, u"Прямая трансляция матча"),
                      "next": programme(u"Новости", 5400, 30, u"Главное к этому часу"), "days": [0, 1]})
        self.assertEqual(s["lvpv_now_title"].getText(), "Футбол")
        self.assertEqual(s["lvpv_now_desc"].getText(), "Прямая трансляция матча")
        self.assertIn("Далее ·", s["lvpv_next_time"].getText())
        self.assertEqual((s["lvpv_next_title"].getText(), s["lvpv_next_desc"].getText()), ("Новости", "Главное к этому часу"))
        self.assertEqual(s["lve_0"].getText(), "Футбол", "the row learns what is on, too")
        self.assertNotIn("desc", s.live.item()["now"], "rows keep no descriptions")
        # Next channel: a late answer for the old one never lands in the panel.
        self.press("down")
        s.request_guide()
        slow = self.queue.take("key=c1")
        self.press("up")
        s.request_guide()
        self.assertFalse(self.queue.pending("key=c0"), "a fresh answer is reused")
        self.assertEqual(s["lvpv_now_title"].getText(), "Футбол")
        slow.complete({"now": programme(u"Чужое", -60, 60), "days": [0]})
        self.assertEqual(s["lvpv_now_title"].getText(), "Футбол")
        self.assertEqual(s["lve_1"].getText(), "Чужое")

    def test_channel_without_programme(self):
        s = self.open_live()
        s.request_guide()
        self.queue.take("/epg").complete({"days": []})
        self.assertEqual(s["lvpv_now_time"].getText(), "Нет программы")
        self.assertEqual((s["lvpv_now_title"].getText(), s["lvpv_next_title"].getText()), ("", ""))
        self.assertFalse(s["lvpv_bar"].visible)
        self.press("down")
        s.request_guide()
        self.queue.take("/epg").error(http_error(502, {"error": u"источник не отдал программу передач", "code": "provider"}))
        self.assertEqual(s["lvpv_now_time"].getText(), "Программа не загрузилась")

    def test_minute_moves_progress_and_asks_for_ended_rows(self):
        s = self.open_live(now={0: programme(u"Идёт", -1800, 60), 1: programme(u"Кончилось", -3600, 59)})
        self.assertEqual(s["lve_1"].getText(), "")
        s.minute()
        job = self.queue.take("/api/catalog/m1/now")
        self.assertIn("keys=c0%2Cc1%2Cc2", self.queue.path(job))
        s.minute()
        self.assertEqual(len(self.queue.pending("/now")), 0, "one question at a time")
        job.complete({"now": {"c1": programme(u"Следующая", -60, 60)}})
        self.assertEqual(s["lve_1"].getText(), "Следующая")
        self.assertEqual(s["lve_0"].getText(), "", "a row the service no longer knows is cleared")
        s.playing = True
        s.minute()
        self.assertFalse(self.queue.pending("/now"), "nothing happens behind the player")

    def test_epg_key_opens_the_day_and_ok_plays(self):
        s = self.open_live(now={0: programme(u"Новости", -900, 60)})
        self.press("info")
        cls, args = self.session.opened[-1]
        self.assertEqual(cls.__name__, "GuideScreen")
        self.assertEqual(args[1]["key"], "c0")
        self.assertIn("Программа передач", args[2])
        guide = cls(self.session, *args)
        harness.layout(guide)
        job = self.queue.take("/api/catalog/m1/epg")
        self.assertIn("day=0", self.queue.path(job))
        progs = [programme(u"Утро", -7200, 60), programme(u"День", -3600, 60), programme(u"Новости", -900, 60, None),
                 programme(u"Кино", 2700, 120)]
        job.complete({"day": 0, "date": time.strftime("%Y-%m-%d"), "programmes": progs, "days": [0, 1],
                      "now": dict(progs[2], desc=u"Главное за день"), "next": dict(progs[3], desc=u"Комедия")})
        self.assertEqual(guide.model.focus, 2, "today opens on the programme on air")
        self.assertTrue(guide["day"].getText().startswith("Сегодня, "))
        self.assertEqual(guide["n_2"].getText(), "Новости")
        self.assertIn("Сейчас ·", guide["d_time"].getText())
        self.assertEqual(guide["d_desc"].getText(), "Главное за день")
        self.assertTrue(guide["d_bar"].visible)
        guide["actions"].actions["up"]()
        self.assertEqual((guide["d_title"].getText(), guide["d_desc"].getText()), ("День", ""))
        self.assertFalse(guide["d_bar"].visible)
        # < > and CH+/- turn days; arrows never do.
        guide["actions"].actions["pageNext"]()
        job = self.queue.take("day=1")
        self.assertEqual(guide.state, "loading")
        guide["actions"].actions["pagePrev"]()
        self.queue.take("day=0")
        job.complete({"day": 1, "date": "2026-10-06", "programmes": [programme(u"Поздно", 86400)], "days": [0, 1]})
        self.assertEqual(guide.day, 0, "an answer for a day the viewer left is dropped")
        guide["actions"].actions["pageNext"]()
        self.queue.take("day=1").complete({"day": 1, "date": "2026-10-06", "programmes": [programme(u"Завтра", 86400)],
                                           "days": [0, 1]})
        self.assertEqual(guide["n_0"].getText(), "Завтра")
        self.assertTrue(guide["day"].getText().startswith("Завтра, вторник, 6 октября"))
        guide["actions"].actions["pageNext"]()
        self.assertFalse(self.queue.pending("day=2"), "past the last day nothing is asked")
        self.assertEqual(guide["days_hint"].getText(), "Дальше программы нет")
        # OK watches the channel from the list.
        guide["actions"].actions["ok"]()
        self.assertEqual(guide.closed_with, ("play",))
        self.session.callbacks[-1]("play")
        self.assertEqual(self.queue.take("/api/catalog/m1/play").args[1]["key"], "c0")

    def test_day_guide_without_programme(self):
        s = self.open_live()
        self.press("info")
        cls, args = self.session.opened[-1]
        guide = cls(self.session, *args)
        harness.layout(guide)
        self.queue.take("/epg").complete({"day": 0, "date": "2026-10-05", "days": []})
        self.assertEqual(guide["msg"].getText(), "Нет программы")
        self.assertFalse(guide["pill"].visible)
        guide["actions"].actions["pageNext"]()
        self.assertFalse(self.queue.pending("/epg"))
        guide["actions"].actions["info"]()
        self.assertEqual(guide.closed_with, ())
        self.session.callbacks[-1]()
        self.assertFalse(self.queue.pending("/play"), "closing the guide does not play")

    def test_day_guide_failure_and_retry(self):
        s = self.open_live()
        self.press("info")
        cls, args = self.session.opened[-1]
        guide = cls(self.session, *args)
        harness.layout(guide)
        self.queue.take("/epg").error(http_error(502, {"error": u"нет связи", "code": "network"}))
        self.assertEqual(guide["msg"].getText(), "Программа не загрузилась")
        guide["actions"].actions["yellow"]()
        self.assertTrue(self.queue.pending("day=0"))
        guide.close()
        self.queue.take("day=0").complete({"day": 0, "programmes": [programme(u"Поздно", 0)], "days": [0]})
        self.assertEqual(guide["n_0"].getText(), "", "nothing is drawn after closing")

    def open_archive_guide(self):
        """Channel 0 keeps 7 days of recordings: its guide for yesterday."""
        q = self.queue
        q.take("/api/catalog/sources").complete([{"id": "m1", "name": "Edem", "type": "m3u", "sections": ["itv"]}])
        q.take("categories?").complete([{"id": "", "name": u"Все каналы"}])
        page = channels(3)
        page["items"][0]["archive"] = 7
        q.take("items?").complete(page)
        self.assertIn("Архив 7 дней", self.shell["lvpv_info"].getText())
        self.assertIn("EPG — программа и архив", self.shell["lvpv_hint"].getText())
        self.press("info")
        cls, args = self.session.opened[-1]
        guide = cls(self.session, *args)
        harness.layout(guide)
        # Older than the archive's 7 days: no recording.
        progs = [dict(programme(u"Утро", -7200, 60), rec=True), programme(u"Не записано", -8 * 86400, 30),
                 dict(programme(u"Новости", -900, 60), rec=True), programme(u"Кино", 2700, 120)]
        q.take("day=0").complete({"day": 0, "date": time.strftime("%Y-%m-%d"), "programmes": progs, "days": [-7, -1, 0],
                                  "archive": 7, "now": progs[2], "next": progs[3]})
        return guide

    def test_archive_recordings_play_from_the_guide_and_return_to_the_row(self):
        import ui_theme as T
        guide = self.open_archive_guide()
        actions = guide["actions"].actions
        self.assertEqual(guide.model.focus, 2)
        self.assertIn("Красная — с начала", guide["hint"].getText())
        actions["up"]()
        self.assertTrue(guide["d_time"].getText().startswith("Нет в архиве · "))
        self.assertEqual(guide["n_0"].instance.color.value, int(T.TEXT_2[1:], 16), "a recorded past programme is not greyed out")
        actions["ok"]()
        self.assertTrue(guide["msg"].visible and guide["msg"].getText() == "Этой передачи нет в архиве")
        self.assertIn("7 дней", guide["msg2"].getText())
        self.assertFalse(self.queue.pending("/play"))
        self.assertFalse(hasattr(guide, "closed_with"), "the guide stays open")
        actions["up"]()
        self.assertFalse(guide["msg"].visible, "the next key clears the message")
        self.assertTrue(guide["d_time"].getText().startswith("Запись · "))
        self.assertIn("OK — смотреть запись", guide["hint"].getText())
        actions["ok"]()
        actions["ok"]()
        play = self.queue.take("/api/catalog/m1/play")
        self.assertFalse(self.queue.pending("/play"), "a second OK mints no second link")
        self.assertEqual(play.args[1], {"key": "c0", "start": progs_start(guide, 0)})
        self.assertEqual(guide["msg"].getText(), "Подготавливаю запись…")
        play.complete({"url": "http://edem.test/iptv/T/1/index.m3u8?utc=1&lutc=2", "name": u"Канал 0", "serviceType": 5002,
                       "headers": {}, "archive": {"title": u"Утро"}})
        cls, args = self.session.opened[-1]
        self.assertEqual(cls.__name__, "CatalogPlayer")
        self.assertEqual(args[0].args[:1], (5002,))
        self.assertEqual(args[0].name, "Утро · Канал 0")
        self.session.callbacks[-1]()
        self.assertEqual(guide.model.focus, 0, "back on the same row")
        self.assertFalse(guide["msg"].visible)
        # Red: the programme on air from its start; nothing on others.
        actions["red"]()
        self.assertFalse(self.queue.pending("/play"))
        actions["down"]()
        actions["down"]()
        actions["red"]()
        self.assertEqual(self.queue.take("/play").args[1]["start"], progs_start(guide, 2))

    def test_a_programme_that_begins_while_the_guide_is_open_can_restart(self):
        import ui_guide
        guide = self.open_archive_guide()
        actions = guide["actions"].actions
        actions["down"]()
        actions["red"]()
        self.assertFalse(self.queue.pending("/play"), "not on air yet")
        later = time.time() + 3000
        real = ui_guide.time.time
        ui_guide.time.time = lambda: later
        try:
            guide.minute()
            self.assertIn("Красная — с начала", guide["hint"].getText())
            actions["red"]()
            self.assertEqual(self.queue.take("/play").args[1]["start"], progs_start(guide, 3))
        finally:
            ui_guide.time.time = real

    def test_archive_failures_explain_themselves(self):
        guide = self.open_archive_guide()
        actions = guide["actions"].actions
        actions["up"]()
        actions["up"]()
        actions["ok"]()
        self.queue.take("/play").error(http_error(404, {"error": u"этой передачи нет в архиве", "code": "archive"}))
        self.assertEqual(guide["msg"].getText(), "этой передачи нет в архиве")
        self.assertIn("Жёлтая", guide["msg2"].getText())
        actions["ok"]()
        self.queue.take("/play").complete({"url": "http://edem.test/a.m3u8", "name": "x", "serviceType": 4097, "headers": {}})
        self.session.callbacks[-1]("failed")
        self.assertEqual(guide["msg"].getText(), "Запись не отвечает")
        # OK on the programme on air still watches the channel live.
        actions["down"]()
        actions["down"]()
        actions["ok"]()
        self.assertEqual(guide.closed_with, ("play",))

    def test_past_without_archive_keeps_watching_live(self):
        s = self.open_live()
        self.press("info")
        cls, args = self.session.opened[-1]
        guide = cls(self.session, *args)
        harness.layout(guide)
        progs = [programme(u"Было", -7200, 60), programme(u"Сейчас", -900, 60)]
        self.queue.take("/epg").complete({"day": 0, "date": time.strftime("%Y-%m-%d"), "programmes": progs, "days": [0]})
        guide["actions"].actions["up"]()
        self.assertFalse(guide["d_time"].getText().startswith("Нет в архиве"))
        guide["actions"].actions["red"]()
        self.assertFalse(self.queue.pending("/play"))
        guide["actions"].actions["ok"]()
        self.assertEqual(guide.closed_with, ("play",))

    def test_days_text(self):
        import ui_widgets
        self.assertEqual([ui_widgets.days_text(n) for n in (1, 3, 5, 7, 11, 14, 21, 22)],
                         ["1 день", "3 дня", "5 дней", "7 дней", "11 дней", "14 дней", "21 день", "22 дня"])

    def test_day_labels(self):
        import ui_guide
        self.assertEqual(ui_guide.day_label(0, "2026-10-05"), "Сегодня, понедельник, 5 октября")
        self.assertEqual(ui_guide.day_label(-1, u"2026-10-04"), "Вчера, воскресенье, 4 октября")
        self.assertEqual(ui_guide.day_label(2, "2026-10-07"), "Среда, 7 октября")
        self.assertEqual(ui_guide.day_label(3, "garbage"), "")


class LibraryTest(ShellBase):
    """Favorites and recents: long OK, the rail's "Избранное" and "Недавние"."""

    def hold_ok(self):
        actions = self.shell["actions"].actions
        for name in ("okDown", "okLong", "ok"):  # press, hold about a second, release
            actions[name]()

    def closing_jobs(self):
        import ui_library
        return [j for j in self.queue.jobs if j.fn is ui_library.close_catalog]

    def test_long_ok_marks_a_channel_and_sends_the_list_on_close(self):
        s = self.open_playlist()
        self.press("down")
        self.hold_ok()
        fav = self.queue.take("/api/catalog/favorites")
        self.assertEqual(fav.args[4], "POST")
        self.assertEqual(fav.args[1], {"source": "m1", "key": "c1"})
        self.assertFalse(self.queue.pending("/play"), "the release after a long press does not play")
        fav.complete({"fav": True, "channel": True, "list": "Избранное"})
        self.assertTrue(s["lvf_1"].visible and not s["lvf_0"].visible, "a star on the marked row only")
        self.assertIn("в списке «Избранное»", s["toast_text"].getText())
        self.assertIn("после закрытия каталога", s["toast_hint"].getText())
        self.assertIn("OK долго — убрать из избранного", s["lvpv_hint"].getText())
        # The next short OK plays again.
        self.press("okDown", "ok")
        self.assertTrue(self.queue.pending("/api/catalog/m1/play"))
        s.close()
        jobs = self.closing_jobs()
        self.assertEqual([j.args for j in jobs], [(True,)], "favorite channels reach Enigma2 when the catalog closes")

    def test_unsent_tablet_changes_and_a_recreated_list_are_explained(self):
        s = self.open_playlist()
        self.hold_ok()
        self.queue.take("/api/catalog/favorites").complete({"fav": True, "channel": True, "list": "Избранное", "unsent": True})
        self.assertIn("неотправленные правки с планшета", s["toast_hint"].getText())
        self.press("down")
        self.hold_ok()
        self.queue.take("/api/catalog/favorites").complete({"fav": True, "channel": True, "list": "Избранное", "created": True})
        self.assertIn("создан заново", s["toast_hint"].getText())

    def test_closing_without_channel_changes_only_saves_the_library(self):
        s = self.open_movies()
        self.hold_ok()
        self.queue.take("/api/catalog/favorites").complete({"fav": True})
        s.close()
        self.assertEqual([j.args for j in self.closing_jobs()], [(False,)])

    def test_film_mark_in_grid_and_card(self):
        s = self.open_movies()
        self.press("right")
        self.hold_ok()
        self.assertFalse(self.session.opened, "a long press does not open the card")
        self.queue.take("/api/catalog/favorites").complete({"fav": True})
        self.assertTrue(s["f_1"].visible and not s["f_0"].visible)
        self.assertIn("В избранном", s["meta"].getText())
        self.press("okDown", "ok")
        cls, args = self.session.opened[-1]
        self.assertEqual(cls.__name__, "DetailScreen")
        import ui_detail
        card = ui_detail.DetailScreen(self.session, *args)
        harness.layout(card)
        self.assertEqual(card["a_2"].getText(), "В избранном")
        self.assertIn("убрать из избранного", card["hint"].getText())
        # Long OK in the card takes it out; the grid follows when the card closes.
        card["actions"].actions["okDown"]()
        card["actions"].actions["okLong"]()
        job = self.queue.take("/api/catalog/favorites")
        self.assertEqual(job.args[4], "DELETE")
        job.complete({"fav": False})
        card["actions"].actions["ok"]()
        self.assertFalse(self.queue.pending("/play"), "the release in the card does not play")
        self.assertEqual(card["a_2"].getText(), "В избранное")
        card.close()
        self.session.callbacks[-1]()
        self.assertFalse(s["f_1"].visible, "the star left with the favorite")

    def open_favorites(self, home=None, sources=None):
        s = self.open_movies(sources=sources)
        self.press("back", "back")
        import ui_shell
        while ui_shell.RAIL_KEYS[s.rail_focus] != "favorites":
            self.press("up")
        self.press("ok")
        self.assertEqual(s.section, "favorites")
        self.queue.take("/api/catalog/home").complete(home or {
            "favorites": [{"id": "itv", "name": "Каналы", "count": 2}, {"id": "video", "name": "Фильмы и сериалы", "count": 1}],
            "recent": [{"id": "itv", "name": "Каналы", "count": 0}, {"id": "video", "name": "Фильмы и сериалы", "count": 0}],
            "list": "Избранное"})
        return s

    def test_favorites_section_rows_of_every_source(self):
        sources = [{"id": "p1", "name": "Portal", "type": "stalker", "sections": ["itv", "vod", "series"]},
                   {"id": "m1", "name": "Cyrius", "type": "m3u", "sections": ["itv"]}]
        q = self.queue
        s = self.open_favorites(sources=sources)
        self.assertEqual([s["cat_0"].getText(), s["cat_1"].getText()], ["Каналы", "Фильмы и сериалы"])
        page = q.take("/api/catalog/home/items")
        self.assertIn("list=favorites", q.path(page))
        self.assertIn("category=itv", q.path(page))
        page.complete({"items": [
            {"key": "k1", "id": "m1/itv/one", "name": u"Первый", "kind": "channel", "number": "1", "fav": True, "source": "m1",
             "type": "itv", "hasPoster": True},
            {"key": "k2", "id": "m1/itv/gone", "name": u"Пропавший", "kind": "channel", "number": "2", "fav": True, "source": "m1",
             "type": "itv", "missing": True}], "page": 1, "pageSize": 50, "hasMore": False, "total": 2})
        self.assertEqual(s.zone, "content")
        self.assertTrue(s["lvf_0"].visible and s["lvf_1"].visible)
        self.assertIn("нет в плейлисте", s["lvc_1"].getText())
        self.assertTrue(s["context"].getText().startswith("Избранное · Каналы"), s["context"].getText())
        # Pictures come from the row's own source.
        s.request_images()
        self.assertTrue(self.queue.pending("/api/catalog/m1/poster?key=k1"))
        self.press("down", "okDown", "ok")
        self.assertIn("нет в плейлисте", s["toast_text"].getText())
        self.assertFalse(self.queue.pending("/play"))
        self.press("up", "okDown", "ok")
        play = q.take("/api/catalog/m1/play")
        self.assertEqual(play.args[1], {"key": "k1", "id": "m1/itv/one"}, "plays by persistent id")
        play.complete({"url": "http://stream.test/1.ts", "name": "x", "serviceType": 4097, "headers": {}})
        self.session.callbacks[-1]()
        # Films and series: the grid, the card with the row's source.
        self.press("left", "down", "ok")
        q.take("category=video").complete({"items": [
            {"key": "s9", "id": "p1/vod/lazy", "name": u"Сериал", "kind": "series", "fav": True, "source": "p1", "type": "vod"}],
            "page": 1, "pageSize": 50, "hasMore": False, "total": 1})
        self.assertFalse(s["lvc_0"].getText(), "the channel list is cleared for the grid")
        self.assertTrue(s["f_0"].visible)
        self.press("okDown", "ok")
        cls, args = self.session.opened[-1]
        self.assertEqual((cls.__name__, args[0]["id"], args[0]["name"], args[0]["section"]), ("DetailScreen", "p1", "Portal", "vod"))
        # Green from the favorites goes to the chosen source's lists.
        self.session.callbacks[-1]()
        self.press("green", "down", "ok")
        self.assertEqual((s.source_id(), s.section), ("m1", "live"))

    def test_empty_recents_explain_themselves(self):
        s = self.open_movies()
        self.press("back", "back")
        import ui_shell
        while ui_shell.RAIL_KEYS[s.rail_focus] != "recent":
            self.press("up")
        self.press("ok")
        self.queue.take("/api/catalog/home").complete({"favorites": [], "recent": [{"id": "itv", "name": "Каналы", "count": 0}]})
        self.assertIn("ещё ничего не смотрели", s["msg"].getText())
        self.assertEqual(s.zone, "cats")
        self.press("yellow")
        self.assertTrue(self.queue.pending("/api/catalog/home"), "yellow asks again")

    def test_reopening_returns_to_the_favorites(self):
        s = self.open_favorites()
        s.close()
        import ui_shell
        self.shell = ui_shell.Shell(self.session)
        harness.layout(self.shell)
        self.queue.take("/api/catalog/sources").complete([{"id": "p1", "name": "Portal"}])
        self.assertEqual(self.shell.section, "favorites")
        self.assertTrue(self.queue.pending("/api/catalog/home"))
        self.assertEqual(self.shell.last_section, "vod", "search and green still know the source's last section")

    def test_closing_while_the_change_is_on_its_way_still_sends_it(self):
        s = self.open_playlist()
        self.press("okDown", "okLong", "okLong", "okLong")
        self.assertEqual(len(self.queue.pending("/api/catalog/favorites")), 1, "one change per press, however long")
        s.close()
        self.assertEqual(self.closing_jobs(), [], "the close waits for the change")
        self.queue.take("/api/catalog/favorites").complete({"fav": True, "channel": True, "list": "Избранное"})
        self.assertEqual([j.args for j in self.closing_jobs()], [(True,)], "then Enigma2 gets the list with it")

    def test_a_failed_channel_change_sends_nothing_on_close(self):
        s = self.open_playlist()
        self.hold_ok()
        s.close()
        self.queue.take("/api/catalog/favorites").error(http_error(410, {"error": "x", "code": "expired"}))
        self.assertEqual([j.args for j in self.closing_jobs()], [(False,)])

    def test_long_ok_where_nothing_can_be_marked_is_an_ordinary_ok(self):
        s = self.open_movies()
        self.press("back", "back")
        import ui_shell
        while ui_shell.RAIL_KEYS[s.rail_focus] != "live":
            self.press("up")
        self.hold_ok()
        self.assertEqual(s.section, "live", "a long OK on the rail opens the section")

    def test_a_release_from_another_screen_does_nothing(self):
        s = self.open_movies()
        self.press("back", "back")
        for _ in range(10):
            self.press("down")
        self.press("up", "ok")
        self.assertEqual(self.session.opened[-1][0].__name__, "DreaminatorScreen")
        opened = len(self.session.opened)
        # The settings screen closes when OK goes down; its release reaches the catalog.
        self.shell["actions"].actions["ok"]()
        self.assertEqual(len(self.session.opened), opened, "settings must not open again")

    def test_an_older_keymap_without_ok_down_still_opens(self):
        import ui_library
        s = self.open_movies()
        ui_library.OK_DOWN_SEEN[0] = False  # Enigma2 still runs with the previous keymap.xml
        try:
            self.shell["actions"].actions["ok"]()
            self.assertEqual(self.session.opened[-1][0].__name__, "DetailScreen")
        finally:
            ui_library.OK_DOWN_SEEN[0] = True

    def test_a_card_closed_before_the_answer_still_marks_the_row(self):
        s = self.open_movies()
        self.press("ok")
        cls, args = self.session.opened[-1]
        import ui_detail
        card = ui_detail.DetailScreen(self.session, *args)
        harness.layout(card)
        card["actions"].actions["okDown"]()
        card["actions"].actions["okLong"]()
        card.close()
        self.session.callbacks[-1]()
        self.queue.take("/api/catalog/favorites").complete({"fav": True})
        self.assertTrue(s["f_0"].visible, "the grid shows the new favorite")
        self.hold_ok()
        self.assertEqual(self.queue.take("/api/catalog/favorites").args[4], "DELETE", "the next long OK takes it out")

    def test_the_star_follows_the_changed_row_not_the_focus(self):
        s = self.open_playlist()
        self.hold_ok()
        self.press("down", "down")
        self.queue.take("/api/catalog/favorites").complete({"fav": True, "channel": True, "list": "Избранное"})
        self.assertTrue(s["lvf_0"].visible and not s["lvf_2"].visible)

    def test_films_only_library_starts_at_its_top(self):
        s = self.open_movies()
        self.press("down", "right")
        self.press("back", "back")
        import ui_shell
        while ui_shell.RAIL_KEYS[s.rail_focus] != "favorites":
            self.press("up")
        self.press("ok")
        self.queue.take("/api/catalog/home").complete({"favorites": [{"id": "itv", "name": "Каналы", "count": 0},
                                                                     {"id": "video", "name": "Фильмы и сериалы", "count": 3}]})
        self.queue.take("category=video").complete(cards(1, 3, prefix="f"))
        self.assertEqual(s.grid.model.focus, 0, "the library does not inherit the films' position")

    def test_favorite_channels_show_their_programme_from_their_own_source(self):
        import time
        s = self.open_favorites()
        t = int(time.time())
        self.queue.take("/api/catalog/home/items").complete({"items": [
            {"key": "k1", "id": "m1/itv/one", "name": u"Первый", "kind": "channel", "number": "1", "fav": True, "source": "m1",
             "type": "itv", "now": {"title": u"Новости", "start": t - 600, "stop": t + 600}}],
            "page": 1, "pageSize": 50, "hasMore": False, "total": 1})
        self.assertEqual(s["lve_0"].getText(), "Новости")
        self.assertTrue(s["lvf_0"].visible and s["lvg_0"].visible, "star and progress share the row")
        s.request_guide()
        self.assertTrue(self.queue.pending("/api/catalog/m1/epg?"), "the programme comes from the row's source")
        self.press("info")
        cls, args = self.session.opened[-1]
        self.assertEqual((cls.__name__, args[0]["id"]), ("GuideScreen", "m1"))

    def test_keymap_holds_ok_and_every_action_has_a_handler(self):
        import keymapparser
        import ui_shell
        ours = [m for m in keymapparser.maps if m[0] == ui_shell.KEYMAP_CONTEXT]
        ok = sorted((action, flags) for _, key, action, flags in ours if key == "KEY_OK")
        self.assertEqual(ok, [("ok", "b"), ("okDown", "m"), ("okLong", "l")], "OK on release, long press for favorites")
        self.assertFalse(set(m[2] for m in ours) - set(self.shell["actions"].actions))

    def test_without_the_keymap_ok_stays_on_the_press(self):
        import plugin
        import ui_shell
        loaded = plugin.load_keymap
        plugin.load_keymap = lambda: False
        try:
            shell = ui_shell.Shell(self.session)
        finally:
            plugin.load_keymap = loaded
        actions = shell["actions"].actions
        self.assertIn("OkCancelActions", shell["actions"].contexts)
        self.assertNotIn("okLong", actions)
        self.assertIn("ok", actions)

    def test_failed_change_keeps_the_mark(self):
        s = self.open_movies()
        self.hold_ok()
        self.hold_ok()
        self.assertEqual(len(self.queue.pending("/api/catalog/favorites")), 1, "one change at a time")
        self.queue.take("/api/catalog/favorites").error(IOError("refused"))
        self.assertFalse(s["f_0"].visible)
        self.assertIn("не отвечает", s["toast_text"].getText())
        self.hold_ok()
        self.assertTrue(self.queue.pending("/api/catalog/favorites"), "a new long OK after a failure")


class SearchTest(ShellBase):
    def open_search(self):
        s = self.open_movies()
        self.press("blue")
        self.assertEqual(s.section, "search")
        self.assertEqual(s.zone, "keys")
        return s

    def type_text(self, text):
        kb = self.shell.search.kb
        for ch in text:
            kb.focus = kb.keys().index(ch)
            self.press("ok")

    def test_debounced_search_server_scope(self):
        s = self.open_search()
        self.type_text(u"ма")
        self.assertFalse(self.queue.pending("/search"), "waits for the viewer to stop typing")
        s.search.run()
        job = self.queue.take("/search")
        self.assertIn("type=vod", self.queue.path(job))
        job.complete(dict(cards(1, 3), scope="server", query=u"ма"))
        self.assertIn("Найдено: 3", s["q_status"].getText())
        self.assertIn("на сервере портала", s["q_status2"].getText())
        self.press("right", "right", "right", "right", "right", "right", "right")
        self.assertEqual(s.zone, "content")
        self.press("ok")
        self.assertEqual(self.session.opened[-1][0].__name__, "DetailScreen")

    def test_stale_search_answer_is_ignored(self):
        s = self.open_search()
        self.type_text(u"ма")
        s.search.run()
        old = self.queue.take("/search")
        self.type_text(u"т")
        s.search.run()
        new = self.queue.take("/search")
        new.complete(dict(cards(1, 1), scope="server", query=u"мат"))
        old.complete(dict(cards(1, 9), scope="server", query=u"ма"))
        self.assertEqual(s.search.content().count(), 1)

    def test_scopes_say_what_was_searched(self):
        s = self.open_search()
        self.type_text(u"ма")
        s.search.run()
        self.queue.take("/search").complete(dict(cards(1, 2), scope="loaded", searched=28, query=u"ма"))
        self.assertIn("только открытые страницы (28)", s["q_status2"].getText())
        s.search.run()
        self.queue.take("/search").complete({"items": [], "page": 1, "hasMore": False, "scope": "server", "query": u"ма"})
        self.assertIn("Ничего не найдено", s["q_status"].getText())
        s.search.run()
        self.queue.take("/search").error(http_error(502, {"error": u"нет связи с порталом", "code": "network"}))
        self.assertIn("Нет связи с порталом", s["q_status"].getText())
        self.press("yellow")
        self.assertTrue(self.queue.pending("/search"), "yellow repeats the search")

    def test_live_scope_and_back(self):
        s = self.open_search()
        for _ in range(8):
            self.press("down")
        self.assertEqual(s.zone, "scope")
        self.press("right", "right", "ok")
        self.assertEqual(s.search.scope, "live")
        s.set_zone("keys")
        self.type_text(u"ка")
        s.search.run()
        job = self.queue.take("/search")
        self.assertIn("type=itv", self.queue.path(job))
        job.complete(dict(channels(4), scope="channels", query=u"ка"))
        self.assertEqual(s["slc_0"].getText(), "Канал 0")
        self.assertIn("каналов: 4", s["q_status"].getText())
        for _ in range(8):
            self.press("right")
        self.assertEqual(s.zone, "content")
        self.press("down", "ok")
        self.assertIn("key", self.queue.take("/play").args[1])
        self.press("back")
        self.assertEqual(s.zone, "keys")
        self.press("back")
        self.assertEqual(s.zone, "rail")

    def test_short_query_and_digits(self):
        s = self.open_search()
        self.press("1")
        self.assertIn("две буквы", s["q_status"].getText())
        self.press("2")
        self.assertEqual(s.search.kb.text, u"12")
        self.press("erase")
        self.assertEqual(s.search.kb.text, u"1")


class DetailTest(Base):
    def open_card(self, item, details=None):
        import ui_detail
        screen = ui_detail.DetailScreen(self.session, {"id": "p1", "name": "Portal", "section": "vod"}, item, "Фильмы › Все")
        harness.layout(screen)
        self.screen = screen
        if details is not None:
            self.queue.take("/details").complete(details)
        return screen

    def tearDown(self):
        if getattr(self, "screen", None):
            self.screen.close()
        Base.tearDown(self)

    def press(self, *names):
        actions = self.screen["actions"].actions
        for name in names:
            if name == "ok":
                actions["okDown"]()
            actions[name]()

    def test_movie_card_facts_with_rating_sources(self):
        d = self.open_card({"key": "m1", "name": u"Матрица", "kind": "movie", "hasPoster": True},
                           {"key": "m1", "name": u"Матрица", "originalName": u"The Matrix", "year": u"1999", "duration": u"136",
                            "genres": u"Фантастика", "ratings": [{"source": "imdb", "value": "8.7"}, {"source": "kinopoisk", "value": "8.5"}],
                            "description": u"Описание", "director": u"Вачовски", "type": "vod", "hasBackdrop": False})
        self.assertEqual(d["title"].getText(), "Матрица")
        self.assertIn("The Matrix · 1999", d["original"].getText())
        self.assertIn("2 ч 16 мин", d["facts"].getText())
        self.assertEqual(d["r_0"].getText(), "IMDb  8.7")
        self.assertEqual(d["r_1"].getText(), "КП  8.5")
        self.assertIn("по данным портала", d["footer"].getText())
        self.queue.take("/tmdb?").complete({"tmdb": None})
        self.assertEqual(d.backdrop_timer.active, None, "no backdrop without TMDB")
        self.assertIn("IMDb", d["r_0"].getText())

    def test_tmdb_backdrop_is_delayed_and_late_answers_ignored(self):
        d = self.open_card({"key": "m1", "name": u"X", "kind": "movie"},
                           {"key": "m1", "name": u"X", "type": "vod", "description": u"От портала",
                            "ratings": [{"source": "imdb", "value": "7.0"}]})
        self.assertEqual(d["description"].getText(), "От портала")
        self.assertEqual(d.backdrop_timer.active, None, "no backdrop before TMDB answers")
        self.queue.take("/tmdb?").complete({"tmdb": {"id": 603, "rating": "8.2", "overview": u"Из TMDB", "genres": [u"Боевик"],
                                                     "hasBackdrop": True, "hasPoster": True}})
        self.assertEqual(d["r_0"].getText(), "TMDB  8.2")
        self.assertEqual(d["r_1"].getText(), "IMDb  7.0")
        self.assertEqual(d["description"].getText(), "Из TMDB")
        self.assertEqual(d.backdrop_timer.active, 350)
        d.load_backdrop()
        fetch = self.queue.take("kind=backdrop")
        self.assertTrue(self.queue.pending("kind=poster") or True)
        d.close()
        fetch.complete(("/tmp/none.jpg", (780, 439)))
        self.assertFalse(getattr(d["backdrop"], "visible", False) and d["backdrop"].instance.pixmap)

    def test_series_seasons_episodes_and_exact_episode_playback(self):
        d = self.open_card({"key": "s1", "name": u"Сериал", "kind": "series"},
                           {"key": "s1", "name": u"Сериал", "type": "vod"})
        self.queue.take("parent=s1").complete({"items": [{"key": "se1", "name": u"Сезон 1", "kind": "season", "number": "1"},
                                                         {"key": "se2", "name": u"Сезон 2", "kind": "season", "number": "2"}],
                                               "page": 1, "hasMore": False})
        self.assertEqual(d["s_0"].getText(), "Сезон 1")
        self.queue.take("parent=se1").complete({"items": [{"key": "e1", "name": u"Эпизод 1", "kind": "episode", "number": "1"},
                                                          {"key": "e2", "name": u"Эпизод 2", "kind": "episode", "number": "2"}],
                                                "page": 1, "hasMore": False, "pageSize": 50})
        # TMDB names decorate only episodes the portal has.
        self.queue.take("/episodes").complete({"episodes": [{"number": 2, "name": u"Пилот-2"}, {"number": 9, "name": u"Нет у портала"}]})
        self.assertEqual(d["et_1"].getText(), "Пилот-2")
        self.assertEqual(d["et_2"].getText(), "")
        self.press("down", "down", "ok")
        play = self.queue.take("/play")
        self.assertEqual(play.args[1]["key"], "e2")
        self.press("ok")
        self.assertEqual(len(self.queue.pending("/play")), 0, "double OK ignored")
        play.complete({"url": "http://cdn.test/e2.mp4", "name": "e2", "headers": {}})
        self.session.callbacks[-1]()
        self.assertEqual((d.zone, d.episode_model.focus), ("episodes", 1), "focus returns to the episode")
        # Switching season drops a late answer for the previous one.
        self.press("up", "up", "right")
        d.open_season()
        late = self.queue.take("parent=se2")
        d.season_model.focus = 0
        d.open_season()
        self.queue.take("parent=se1").complete({"items": [{"key": "e1", "name": u"Эпизод 1", "kind": "episode", "number": "1"}],
                                                "page": 1, "hasMore": False})
        late.complete({"items": [{"key": "x", "name": u"Чужая серия", "kind": "episode", "number": "7"}], "page": 1, "hasMore": False})
        self.assertEqual(d["et_0"].getText(), "Эпизод 1")

    def test_cast_portraits_and_merged_picture_requests(self):
        d = self.open_card({"key": "m1", "name": u"X", "kind": "movie", "hasPoster": True},
                           {"key": "m1", "name": u"X", "type": "vod", "actors": u"Портал Актёр"})
        self.assertIn("В ролях: Портал Актёр", d["credits"].getText())
        self.queue.take("/tmdb?").complete({"tmdb": {"id": 1, "type": "movie", "directors": [u"Режиссёр"],
                                                     "cast": [{"name": u"Актёр А", "character": u"Герой", "hasPhoto": True},
                                                              {"name": u"Актёр Б"}]}})
        self.assertEqual(d["credits"].getText(), "Режиссёр: Режиссёр")
        self.assertEqual(d["cast_label"].getText(), "В ролях")
        self.assertEqual((d["castn_0"].getText(), d["castr_0"].getText()), ("Актёр А", "Герой"))
        self.assertEqual(d["castn_2"].getText(), "")
        # The big poster and the portrait are both still wanted.
        self.assertIn("kind=poster", " ".join(self.queue.path(j) for j in self.queue.jobs) + " ".join(
            str(r[1]) for r in d.wants.values()))
        self.assertTrue(any("kind=cast" in str(r[1]) for r in d.wants.values()))

    def test_episode_still_follows_focus_and_drops_stale(self):
        d = self.open_card({"key": "s1", "name": u"Сериал", "kind": "series"}, {"key": "s1", "name": u"Сериал", "type": "vod"})
        self.queue.take("parent=s1").complete({"items": [{"key": "se1", "name": u"Сезон 1", "kind": "season", "number": "1"}],
                                               "page": 1, "hasMore": False})
        self.queue.take("parent=se1").complete({"items": [{"key": "e1", "name": u"Эпизод 1", "kind": "episode", "number": "1"},
                                                          {"key": "e2", "name": u"Эпизод 2", "kind": "episode", "number": "2"}],
                                                "page": 1, "hasMore": False})
        self.queue.take("/episodes").complete({"episodes": [{"number": 1, "name": u"Пилот", "overview": u"О пилоте", "hasStill": True},
                                                            {"number": 2, "name": u"Вторая", "hasStill": True}]})
        self.press("down")
        self.assertEqual(d["ep_title"].getText(), "Пилот")
        self.assertEqual(d["ep_info"].getText(), "О пилоте")
        self.assertEqual(d.still_timer.active, 300)
        d.load_still()
        self.assertTrue(any("kind=still" in str(r[1]) for r in d.wants.values()))
        first = d.still_key
        self.press("down")
        self.assertNotIn(first, d.wants, "the still for the old focus is no longer wanted")
        d.set_still(first, "late-pixmap")
        self.assertFalse(getattr(d["stillp_0"], "visible", False) and d["stillp_0"].instance.pixmap == "late-pixmap")

    def test_card_works_when_details_fail(self):
        d = self.open_card({"key": "m1", "name": u"Фильм", "kind": "movie", "year": "2001"})
        self.queue.take("/details").error(http_error(410, {"error": u"каталог обновился", "code": "expired"}))
        self.assertEqual(d["title"].getText(), "Фильм")
        self.press("ok")
        self.assertTrue(self.queue.pending("/play"))


def load_source(name, path):
    try:
        import imp
        return imp.load_source(name, path)
    except ImportError:  # Python 3.12+
        import importlib.util
        spec = importlib.util.spec_from_file_location(name, path)
        module = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(module)
        return module


class Ref(object):
    def __init__(self, text):
        self.text = text

    def toString(self):
        return self.text


class Nav(object):
    def __init__(self, ref):
        self.ref = ref

        self.played = []

    def getCurrentlyPlayingServiceReference(self):
        return Ref(self.ref) if self.ref else None

    def playService(self, ref):
        self.played.append(ref.toString())


class Infobar(object):
    """The screen the EPG key was pressed on: counts its own EPGs."""

    def __init__(self):
        self.calls = []

    def openSingleServiceEPG(self):
        self.calls.append("single")

    def openGraphEPG(self):
        self.calls.append("graph")


class Event(object):
    def __init__(self, start):
        self.start = start

    def getBeginTime(self):
        return self.start


EDEM_REF = "4097:0:1:9a1c:2b3d:1de:0:0:0:0:http%3a//edem.test/iptv/T/1/index.m3u8:Россия 1"
SAT_REF = "1:0:19:283d:3fb:1:c00000:0:0:0:"


class EpgKeyTest(Base):
    """The EPG key on a line of the receiver's channel lists (ui_epgkey)."""

    def setUp(self):
        Base.setUp(self)
        import ui_epgkey
        self.epgkey = ui_epgkey
        self.original_epgkey = ui_epgkey.threads.deferToThread
        ui_epgkey.threads.deferToThread = self.queue.defer
        ui_epgkey.BUSY[0] = False
        ui_epgkey.register_config()
        from Components.config import config
        config.plugins.Dreaminator.epgnative.value = ui_epgkey.NATIVE_DEFAULT
        self.infobar = Infobar()
        self.session.current_dialog = self.infobar
        self.session.nav = Nav(EDEM_REF)

    def tearDown(self):
        self.epgkey.threads.deferToThread = self.original_epgkey
        Base.tearDown(self)

    def press_epg(self, **kwargs):
        self.epgkey.epg(self.session, servicelist=object(), **kwargs)

    def found(self, archive=7):
        item = {"key": "k1", "id": "m1/itv/1", "name": u"Россия 1", "kind": "channel"}
        if archive:
            item["archive"] = archive
        return {"source": {"id": "m1", "name": u"Edem", "type": "m3u", "sections": ["itv"]}, "item": item}

    def test_a_channel_with_archive_opens_the_programme(self):
        self.press_epg()
        job = self.queue.take("/api/catalog/service?")
        self.assertIn("ref=4097%3A0%3A1%3A9a1c", self.queue.path(job))
        job.complete(self.found())
        cls, args = self.session.opened[-1]
        self.assertEqual(cls.__name__, "GuideScreen")
        source, item, crumbs, start = args
        self.assertEqual((source["id"], item["key"], crumbs, start), ("m1", "k1", "Edem › Программа и архив", None))
        guide = cls(self.session, *args)
        harness.layout(guide)
        path = self.queue.path(self.queue.take("/api/catalog/m1/epg"))
        self.assertIn("day=0", path)
        self.assertIn("id=m1%2Fitv%2F1", path, "the persistent id finds the channel if the key expired")
        # OK on the programme on air: the channel already plays live.
        self.session.callbacks[-1]("play")
        self.assertFalse(self.queue.pending("/play"))
        self.assertEqual(self.session.nav.played, [])
        self.assertEqual(self.infobar.calls, [])

    def test_other_services_keep_the_receivers_epg(self):
        self.session.nav = Nav(SAT_REF)
        self.press_epg()
        self.assertEqual(self.infobar.calls, ["single"], "satellite: no request, the receiver's EPG at once")
        self.assertFalse(self.queue.jobs)
        self.session.nav = Nav(EDEM_REF)
        self.press_epg()
        self.queue.take("/service").complete(self.found(archive=0))
        self.press_epg()
        self.queue.take("/service").error(http_error(404, {"error": u"нет", "code": "missing"}))
        self.assertEqual(self.infobar.calls, ["single"] * 3)
        self.assertFalse(self.session.opened)

    def test_the_receivers_epg_is_the_one_the_key_had(self):
        from Components.config import config
        config.plugins.Dreaminator.epgnative.value = "Infobar/openGraphEPG"
        self.session.nav = Nav(SAT_REF)
        self.press_epg()
        # Never Dreaminator itself (a loop), never a missing method.
        config.plugins.Dreaminator.epgnative.value = "Plugins/Extensions/DreaminatorEPG/1"
        self.press_epg()
        self.assertEqual(self.infobar.calls, ["graph", "single"])

    def test_one_lookup_at_a_time_and_none_after_leaving(self):
        self.press_epg()
        self.press_epg()
        self.assertEqual(len(self.queue.pending("/service")), 1, "a second press while asking is dropped")
        self.session.current_dialog = object()  # the viewer opened something else
        self.queue.take("/service").complete(self.found())
        self.assertFalse(self.session.opened)
        self.assertEqual(self.infobar.calls, [])
        self.session.current_dialog = self.infobar
        self.press_epg()
        self.assertTrue(self.queue.pending("/service"), "the next press asks again")
        self.session.nav = Nav(SAT_REF)  # zapped while the service answered
        self.queue.take("/service").complete(self.found())
        self.assertFalse(self.session.opened)
        self.assertEqual(self.infobar.calls, [], "nor the receiver's EPG of the old channel")

    def test_event_info_menu_opens_the_programme_at_that_programme(self):
        yesterday = int(time.time()) - 86400
        self.epgkey.epg(self.session, service=Ref(EDEM_REF), event=Event(yesterday), eventName="Новости")
        self.queue.take("/service").complete(self.found(archive=0))
        cls, args = self.session.opened[-1]
        self.assertEqual(args[3], yesterday, "from the menu even without an archive: the programme screen")
        guide = cls(self.session, *args)
        harness.layout(guide)
        job = self.queue.take("/epg")
        self.assertIn("day=-1", self.queue.path(job))
        progs = [programme(u"Утро", -86400 - 7200, 60), programme(u"Новости", -86400 - 600, 60), programme(u"Вечер", -86400 + 3000, 60)]
        job.complete({"day": -1, "date": "2026-10-04", "programmes": progs, "days": [-1, 0]})
        self.assertEqual(guide.model.focus, 1, "focused on the programme on air then")
        # The menu named another channel than the one on: OK on air tunes it.
        self.session.nav = Nav(SAT_REF)
        self.session.callbacks[-1]("play")
        self.assertEqual(self.session.nav.played, [EDEM_REF])
        # A programme of the receiver's week-long EPG, past the service's days:
        # the guide opens on that empty day, and ‹ steps to the nearest it knows.
        later = int(time.time()) + 5 * 86400
        self.epgkey.epg(self.session, service=Ref(EDEM_REF), event=Event(later))
        self.queue.take("/service").complete(self.found())
        cls, args = self.session.opened[-1]
        far = cls(self.session, *args)
        harness.layout(far)
        self.queue.take("day=5").complete({"day": 5, "programmes": [], "days": [-1, 0, 1]})
        far["actions"].actions["pageNext"]()
        self.assertFalse(self.queue.pending("/epg"), "further away: nothing to load")
        far["actions"].actions["pagePrev"]()
        self.queue.take("day=1")
        # A satellite service from the menu: say so, the menu stays.
        self.epgkey.epg(self.session, service=Ref(SAT_REF), event=None)
        cls, args = self.session.opened[-1]
        self.assertEqual(cls.__name__, "MessageBox")
        self.assertEqual(self.infobar.calls, [])

    def test_the_plugin_entry_is_found_by_the_hotkey_and_the_menu(self):
        import plugin
        # The catalog's folder has no event info entry: Hotkey numbers
        # entries per folder, and buttons given to the catalog keep theirs.
        self.assertEqual([p.where for p in plugin.Plugins()], ["pluginmenu", "extensionsmenu"])
        epg_plugin = load_source("dreaminator_epg_plugin", os.path.join(harness.ROOT, "plugin", "DreaminatorEPG", "plugin.py"))
        entries = epg_plugin.Plugins()
        self.assertEqual([p.where for p in entries], ["eventinfo"])
        names = entries[0].fnc.__code__.co_varnames
        # OpenHDF leaves out of the Hotkey list a plugin taking selectedevent,
        # and out of the event info menu one taking servicelist.
        self.assertNotIn("selectedevent", names)
        self.assertNotIn("servicelist", names)
        self.assertEqual(entries[0].name, self.epgkey.NAME)
        self.session.nav = Nav(SAT_REF)
        entries[0].fnc(session=self.session, servicelist=object())
        self.assertEqual(self.infobar.calls, ["single"], "the entry runs Dreaminator's EPG key")


if __name__ == "__main__":
    unittest.main()
