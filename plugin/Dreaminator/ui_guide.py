# -*- coding: utf-8 -*-
"""Programme of one channel for a day.

Up and down move through the programmes. Days turn with the < > keys and
CH+/CH-: arrows only move focus, as everywhere in the catalog. OK watches
the channel, Back or EPG/Info return to the list. Only the programmes on
now and next carry descriptions: the service keeps the rest off its memory.

On a channel with an archive the past goes back as far as the provider
keeps recordings: OK on a recorded programme plays it from here, and the
player returns to the same row; red plays the one on air from its start.
"""
from __future__ import absolute_import, division

import datetime
import time

from enigma import eServiceReference, eTimer, getDesktop
from twisted.internet import threads
from Components.ActionMap import ActionMap
from Components.Label import Label
from Components.Pixmap import Pixmap
from Components.ProgressBar import ProgressBar
from Screens.Screen import Screen

try:
    from . import ui_api, ui_library, ui_theme as T
    from .catalog_state import playback_url, service_type
    from .ui_models import MOVED, ListModel
    from .ui_widgets import WidgetState, clipped, color, days_text, font, hhmm, length_text, move, on_air, progress, span_text, visible
except (ImportError, ValueError):
    import ui_api
    import ui_library
    import ui_theme as T
    from catalog_state import playback_url, service_type
    from ui_models import MOVED, ListModel
    from ui_widgets import WidgetState, clipped, color, days_text, font, hhmm, length_text, move, on_air, progress, span_text, visible

G = T.GUIDE
FIRST_DAY, LAST_DAY = -14, 14  # what the service answers (the past: archives)
# Written out: Python 2 cannot change the case of UTF-8 bytes.
WEEKDAYS = ("понедельник", "вторник", "среда", "четверг", "пятница", "суббота", "воскресенье")
WEEKDAYS_CAP = ("Понедельник", "Вторник", "Среда", "Четверг", "Пятница", "Суббота", "Воскресенье")
MONTHS = ("января", "февраля", "марта", "апреля", "мая", "июня", "июля", "августа", "сентября", "октября", "ноября", "декабря")
RELATIVE = {-1: "Вчера", 0: "Сегодня", 1: "Завтра"}


def day_label(offset, date):
    """'Сегодня, понедельник, 5 октября' or 'Среда, 7 октября'."""
    try:
        y, m, d = [int(x) for x in ui_api.text(date).split("-")]
        weekday = datetime.date(y, m, d).weekday()
    except (ValueError, TypeError, AttributeError):
        return RELATIVE.get(offset, "")
    if offset in RELATIVE:
        return "%s, %s, %d %s" % (RELATIVE[offset], WEEKDAYS[weekday], d, MONTHS[m - 1])
    return "%s, %d %s" % (WEEKDAYS_CAP[weekday], d, MONTHS[m - 1])


def day_of(t):
    """Days from today to the TV's day of moment t, within the guide's days."""
    try:
        days = (datetime.date.fromtimestamp(int(t)) - datetime.date.today()).days
    except (TypeError, ValueError, OverflowError, OSError):
        return 0
    return max(FIRST_DAY, min(LAST_DAY, days))


def build_skin(width, fonts):
    s = T.Skin(width, fonts)
    b = [s.box(0, 0, T.RAIL_W, 1080, T.RAIL),
         s.image(72, 48, 48, 48, "mark", z=1),
         s.label("crumbs", 200, 40, 1400, 30, 22, T.MUTED),
         s.label("channel", 200, 70, 1400, 52, 40, T.TEXT, semi=True),
         s.label("day", 200, 132, 1000, 40, 28, T.TEXT_2, semi=True),
         s.label("days_hint", 1240, 136, 600, 32, 22, T.MUTED, halign="right"),
         s.clock(1720, 40, 120, 44),
         s.pixmap("pill", G["x"], G["y"], G["w"], G["row"], z=1, asset="pill_guide")]
    for i in range(G["rows"]):
        y = G["y"] + i * G["pitch"]
        b.append(s.label("t_%d" % i, G["x"] + 24, y, 100, G["row"], 26, T.MUTED, z=2))
        b.append(s.label("n_%d" % i, G["x"] + 140, y, G["w"] - 164, G["row"], 26, T.TEXT, z=2))
    x, w, y = G["info_x"], G["info_w"], G["y"]
    b += [s.label("d_time", x, y + 10, w, 32, 24, T.MUTED),
          s.progress("d_bar", x, y + 52, w, 6),
          s.label("d_title", x, y + 76, w, 132, 34, T.TEXT, semi=True, valign="top", wrap=True),
          s.label("d_desc", x, y + 226, w, 520, 24, T.TEXT_2, valign="top", wrap=True),
          s.label("msg", G["x"], 420, G["w"], 50, 32, T.TEXT_2, halign="center", z=3),
          s.label("msg2", G["x"], 476, G["w"], 36, 24, T.MUTED, halign="center", z=3),
          s.label("hint", 200, 1000, 1640, 32, 22, T.MUTED)]
    return s, s.screen("DreaminatorGuide", b)


def _plugin():
    try:
        from . import plugin
    except (ImportError, ValueError):
        import plugin
    return plugin


SKIN, SKIN_XML = build_skin(getDesktop(0).size().width(), _plugin().FONTS_READY)


class GuideScreen(Screen):
    skin = SKIN_XML

    def __init__(self, session, source, item, crumbs, start=None, archive_only=False):
        Screen.__init__(self, session)
        self.setTitle("Dreaminator")
        self.s = SKIN
        self.source = source
        self.item = dict(item)
        self.crumbs = crumbs
        self.archive_only = archive_only
        self.closed = False
        self.generation = 0
        # start (unix seconds): open on the programme on air then, on its day.
        self.focus_at = start
        self.day = day_of(start) if start else 0
        self.date = ""
        self.days = []
        self.programmes = []
        self.now_entry = self.next_entry = None
        self.archive = 0  # days the provider keeps recordings
        self.playing = False
        self.notice = False  # a message over the rows that the next key clears
        self.state = "loading"
        self.model = ListModel(G["rows"])
        self.value = None
        # A key press changes two rows; only changed values reach Enigma2.
        self.w = WidgetState(self)
        self.styles = {}
        for name in ("crumbs", "channel", "day", "days_hint", "d_time", "d_title", "d_desc", "msg", "msg2", "hint"):
            self[name] = Label("")
        self["pill"] = Pixmap()
        self["d_bar"] = ProgressBar()
        for i in range(G["rows"]):
            self["t_%d" % i] = Label("")
            self["n_%d" % i] = Label("")
        self.timer = eTimer()
        self.timer.callback.append(self.minute)
        context = ["DreaminatorActions"] if _plugin().load_keymap() else ["OkCancelActions", "DirectionActions", "ChannelSelectBaseActions",
                                                                         "ColorActions", "EPGSelectActions"]
        handlers = {"ok": self.key_ok, "back": self.close, "cancel": self.close, "info": self.close,
                    "up": lambda: self.move(-1), "down": lambda: self.move(1),
                    "pageNext": lambda: self.turn_day(1), "pagePrev": lambda: self.turn_day(-1),
                    "nextBouquet": lambda: self.turn_day(1), "prevBouquet": lambda: self.turn_day(-1),
                    "yellow": lambda: self.load(self.day), "red": self.key_red}
        self["actions"] = ActionMap(context, dict((k, self.guard(v)) for k, v in handlers.items()), -1)
        self.onLayoutFinish.append(self.start)
        self.onClose.append(self.cleanup)

    def guard(self, handler):
        def run():
            if not self.closed:
                handler()
        return run

    def cleanup(self):
        self.closed = True
        self.generation += 1
        self.timer.stop()

    def start(self):
        self["crumbs"].setText(ui_api.text(self.crumbs))
        self["channel"].setText(clipped(self.item.get("name"), 50))
        self.timer.start(60000, False)
        self.load(self.day)

    # ------------------------------------------------------------ data

    def load(self, day):
        self.notice = False
        self.generation += 1
        generation = self.generation
        self.day = day
        self.state = "loading"
        self.programmes = []
        self.model = ListModel(G["rows"])
        self.render()
        self.show_message("Загрузка…")
        path = ui_api.catalog_path(self.source["id"], "epg", key=self.item["key"], id=self.item.get("id"), day=day,
                                   tz=ui_api.tz_minutes())

        def done(result):
            if not self.closed and generation == self.generation:
                self.loaded(result)

        def failed(failure):
            if not self.closed and generation == self.generation:
                self.load_failed(ui_api.problem(failure))

        threads.deferToThread(ui_api.request_json, path, None, 25, False).addCallbacks(done, failed)

    def loaded(self, result):
        self.state = "ready"
        self.date = result.get("date") or ""
        self.days = []
        for d in result.get("days") or []:
            try:
                self.days.append(int(d))
            except (TypeError, ValueError):
                pass
        self.programmes = list(result.get("programmes") or [])
        self.now_entry, self.next_entry = result.get("now"), result.get("next")
        try:
            self.archive = int(result.get("archive") or 0)
        except (TypeError, ValueError):
            self.archive = 0
        # Today opens on the programme on air (or the next one); a guide
        # opened for a moment, on the programme on air then.
        focus = 0
        t, self.focus_at = self.focus_at, None
        if t is None and self.day == 0:
            t = time.time()
        if t is not None:
            for i, p in enumerate(self.programmes):
                if on_air(p, t) or int(p.get("start") or 0) > t:
                    focus = i
                    break
        self.model.focus = focus
        self.model.top = max(0, min(focus - 2, len(self.programmes) - G["rows"]))
        self.model.clamp(len(self.programmes))
        self.hide_message()
        self.render()
        if not self.programmes:
            if not self.days:
                self.show_message("Нет программы", "Источник не знает, что идёт на этом канале")
            else:
                self.show_message("На этот день программы нет", "‹ › или CH± — другие дни")

    def load_failed(self, problem):
        self.state = "error"
        self.render()
        if problem.kind == "expired":
            self.show_message("Список каналов обновился", "Back — вернуться к списку")
        else:
            self.show_message("Программа не загрузилась", "Жёлтая — повторить")

    def recorded(self, p, t):
        """The service's rule (rec), worked out again for now: a guide left
        open while a programme begins, or while a recording plays, must not
        go by the moment it was loaded."""
        try:
            start = int(p["start"])
        except (KeyError, TypeError, ValueError):
            return False
        return self.archive > 0 and t - self.archive * 86400 <= start <= t

    def describe(self, p):
        """Descriptions come with now and next only."""
        for entry in (self.now_entry, self.next_entry):
            if entry and entry.get("start") == p.get("start") and entry.get("desc"):
                return entry.get("desc")
        return ""

    # ------------------------------------------------------------ drawing

    def show_message(self, text, hint=""):
        self["msg"].setText(ui_api.text(text))
        self["msg2"].setText(ui_api.text(hint))
        self["msg"].show()
        self["msg2"].show()

    def hide_message(self):
        self.notice = False
        self["msg"].hide()
        self["msg2"].hide()

    def show_notice(self, text, hint=""):
        """A message over the rows until the next key."""
        self.show_message(text, hint)
        self.notice = True

    def render(self):
        t = time.time()
        self["day"].setText(day_label(self.day, self.date) if self.date else RELATIVE.get(self.day, ""))
        others = [d for d in self.days if d != self.day]
        self["days_hint"].setText("‹ › или CH± — другие дни" if others else "")
        m = self.model
        count = len(self.programmes)
        slot = m.focus - m.top if count else None
        for i in range(G["rows"]):
            index = m.top + i
            p = self.programmes[index] if index < count else None
            t_name, n_name = "t_%d" % i, "n_%d" % i
            if p is None:
                self.w.text(t_name, "")
                self.w.text(n_name, "")
                continue
            self.w.text(t_name, hhmm(p.get("start")))
            self.w.text(n_name, clipped(p.get("title"), 60))
            air = on_air(p, t)
            if i == slot:
                title, when = T.ON_ACCENT, T.ON_ACCENT
            elif air:
                title, when = T.ACCENT, T.ACCENT
            elif int(p.get("stop") or 0) <= t:
                # A past programme the provider keeps can still be watched.
                title, when = (T.TEXT_2, T.MUTED) if self.recorded(p, t) else (T.MUTED, T.MUTED)
            else:
                title, when = T.TEXT, T.MUTED
            self.style(n_name, title, air)
            self.style(t_name, when, None)
        if slot is not None:
            move(self, self.s, "pill", G["x"], G["y"] + slot * G["pitch"])
            self["pill"].show()
        else:
            self["pill"].hide()
        focused = self.programmes[m.focus] if count else None
        self.render_details(focused, t)
        self.render_hint(focused, t)

    def style(self, name, colour, semi):
        if self.styles.get(name) != (colour, semi):
            self.styles[name] = (colour, semi)
            color(self, name, colour)
            if semi is not None:
                font(self, self.s, name, 26, semi)

    def render_hint(self, p, t):
        days = "‹ › или CH± — дни"
        if p is not None and self.recorded(p, t) and int(p.get("stop") or 0) <= t:
            text = "OK — смотреть запись · " + days + " · Back — назад"
        elif p is not None and self.recorded(p, t) and on_air(p, t):
            action = "OK — с начала" if self.archive_only else "OK — смотреть канал · Красная — с начала"
            text = action + " · " + days + " · Back — назад"
        elif self.archive_only:
            text = "↑↓ — передачи · " + days + " · Back — назад"
        else:
            text = "↑↓ — передачи · " + days + " · OK — смотреть канал · Back — назад"
        self.w.text("hint", text)

    def render_details(self, p, t):
        if p is None:
            for name in ("d_time", "d_title", "d_desc"):
                self[name].setText("")
            self["d_bar"].hide()
            return
        air = on_air(p, t)
        when = span_text(p)
        length = length_text(p)
        if length:
            when += " · " + length
        if air:
            when = "Сейчас · " + when
        elif int(p.get("stop") or 0) <= t and self.archive:
            when = ("Запись · " if self.recorded(p, t) else "Нет в архиве · ") + when
        self["d_time"].setText(when)
        color(self, "d_time", T.ACCENT if air else T.MUTED)
        self["d_title"].setText(clipped(p.get("title"), 90))
        self["d_desc"].setText(clipped(self.describe(p), 700))
        if air:
            value = progress(p, t)
            if value != self.value:
                self.value = value
                self["d_bar"].setValue(value)
        visible(self["d_bar"], air)

    def minute(self):
        if not self.closed and self.state == "ready":
            self.render()

    # ------------------------------------------------------------ keys

    def move(self, dy):
        if self.notice:
            self.hide_message()
        if self.model.move(0, dy, len(self.programmes)) == MOVED:
            self.render()

    def turn_day(self, delta):
        target = self.day + delta
        if not FIRST_DAY <= target <= LAST_DAY:
            return
        if self.state == "ready" and self.days:
            first, last = min(self.days), max(self.days)
            # Opened on a day the source does not know (a programme from the
            # receiver's own week-long EPG): the next key steps to the
            # nearest day it knows, when that is in the key's direction.
            if not first <= self.day <= last:
                nearest = max(first, min(last, target))
                if (nearest - self.day) * delta > 0:
                    target = nearest
        # Past the last day the source knows nothing: stay, and say so.
        if self.state == "ready" and (not self.days or not min(self.days) <= target <= max(self.days)):
            self["days_hint"].setText("Дальше программы нет" if delta > 0 else "Раньше программы нет")
            return
        self.load(target)

    def focused(self):
        if self.state != "ready" or not self.programmes:
            return None
        return self.programmes[self.model.focus]

    def key_ok(self):
        p, t = self.focused(), time.time()
        if self.archive_only and p is not None and int(p.get("start") or 0) > t:
            self.show_notice("Эта передача ещё не началась")
            return
        if p is not None and (self.archive_only or int(p.get("stop") or 0) <= t) and self.archive:
            if self.recorded(p, t):
                self.play_recording(p)
            else:
                self.show_notice("Этой передачи нет в архиве", "Провайдер хранит записи %s" % days_text(self.archive))
            return
        self.close("play")

    def key_red(self):
        p = self.focused()
        t = time.time()
        if p is not None and self.recorded(p, t) and on_air(p, t):
            self.play_recording(p)

    # ------------------------------------------------------------ recordings

    def play_recording(self, p):
        """The service builds the recording's address: the TV names the
        channel and the programme's start."""
        if self.playing:
            return  # a second OK must not mint a second link
        self.playing = True
        self.show_message("Подготавливаю запись…")
        generation = self.generation
        body = dict(ui_library.entry(self.item), start=int(p.get("start") or 0))
        path = ui_api.catalog_path(self.source["id"], "play")

        def done(result):
            if self.closed or generation != self.generation:
                self.playing = False
            else:
                self.open_player(result, p)

        def failed(failure):
            self.playing = False
            if not self.closed and generation == self.generation:
                problem = ui_api.problem(failure)
                self.show_notice(problem.message, "Жёлтая — обновить программу" if problem.kind == "archive" else "")

        threads.deferToThread(ui_api.request_json, path, body, 35, True).addCallbacks(done, failed)

    def open_player(self, result, p):
        self.hide_message()
        try:
            try:
                from .catalog_player import CatalogPlayer
            except (ImportError, ValueError):
                from catalog_player import CatalogPlayer
            ref = eServiceReference(service_type(result), 0, playback_url(result))
            name = "%s · %s" % (clipped(p.get("title"), 60), ui_api.text(result.get("name") or ""))
            ref.setName(name)
            self.session.openWithCallback(self.player_closed, CatalogPlayer, ref)
        except Exception:
            self.playing = False
            self.show_notice("Не удалось открыть проигрыватель", "OK — попробовать снова")

    def player_closed(self, *args):
        self.playing = False
        if self.closed:
            return
        self.render()
        if args and args[0] == "failed":
            self.show_notice("Запись не отвечает", "Провайдер не отдал запись — попробуйте позже")

