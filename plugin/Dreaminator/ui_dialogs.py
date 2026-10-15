# -*- coding: utf-8 -*-
"""About screen of the TV catalog."""
from __future__ import absolute_import, division

from enigma import getDesktop
from twisted.internet import threads
from Components.ActionMap import ActionMap
from Components.Label import Label
from Screens.Screen import Screen

try:
    from . import ui_api, ui_theme as T
except (ImportError, ValueError):
    import ui_api
    import ui_theme as T

def _plugin():
    try:
        from . import plugin
    except (ImportError, ValueError):
        import plugin
    return plugin


def _fonts():
    return _plugin().FONTS_READY


def _context():
    if _plugin().load_keymap():
        return ["DreaminatorActions"]
    return ["OkCancelActions", "ColorActions"]


TMDB_NOTICE = "This product uses the TMDB API but is not endorsed or certified by TMDB."
TMDB_NOTICE_RU = "Продукт использует TMDB API, но не одобрен и не сертифицирован TMDB."


def about_skin(width):
    s = T.Skin(width, _fonts())
    b = [s.box(0, 0, T.RAIL_W, 1080, T.RAIL),
         s.image(72, 48, 48, 48, "mark", z=1),
         s.text(72, 48, 48, 48, "D", 26, T.ON_ACCENT, semi=True, halign="center"),
         s.text(200, 44, 1200, 56, "О приложении", 44, T.TEXT, semi=True),
         s.label("version", 200, 104, 1200, 32, 24, T.MUTED),
         s.text(200, 180, 1500, 40, "Dreaminator — каталог IPTV и списки каналов для Enigma2", 30, T.TEXT),
         s.text(200, 230, 1500, 34, "Источники и списки настраиваются с планшета: адрес и QR-код — в «Настройках» боковой панели.", 24, T.TEXT_2),
         s.text(200, 320, 1500, 40, "Описания и постеры TMDB", 30, T.TEXT, semi=True),
         s.label("tmdb", 200, 370, 1500, 34, 24, T.TEXT_2),
         s.text(200, 420, 1500, 34, TMDB_NOTICE, 24, T.TEXT_2),
         s.text(200, 458, 1500, 34, TMDB_NOTICE_RU, 24, T.TEXT_2),
         s.text(200, 496, 1500, 34, "Данные TMDB только дополняют описание; видео и серии всегда берутся с портала.", 22, T.MUTED),
         s.text(200, 580, 1500, 40, "Шрифты и данные", 30, T.TEXT, semi=True),
         s.text(200, 630, 1500, 34, "Шрифт Geist — Vercel, SIL Open Font License 1.1.", 24, T.TEXT_2),
         s.text(200, 668, 1500, 34, "Каталог, рейтинги IMDb и КП, ссылки на видео — данные вашего портала или плейлиста.", 24, T.TEXT_2),
         s.text(200, 1000, 1500, 32, "OK или Back — назад", 22, T.MUTED)]
    return s, s.screen("DreaminatorAbout", b)


ABOUT_SKIN, ABOUT_XML = about_skin(getDesktop(0).size().width())


class AboutScreen(Screen):
    skin = ABOUT_XML

    def __init__(self, session):
        Screen.__init__(self, session)
        self.closed = False
        self["version"] = Label("")
        self["tmdb"] = Label("Проверяю настройку…")
        self["actions"] = ActionMap(_context(), {"ok": self.close, "back": self.close, "cancel": self.close, "red": self.close}, -1)
        self.onLayoutFinish.append(self.load)
        self.onClose.append(self.cleanup)

    def cleanup(self):
        self.closed = True

    def load(self):
        def summary(s):
            if not self.closed:
                self["version"].setText("Версия %s" % ui_api.text(s.get("version") or "—"))

        def tmdb(t):
            if self.closed:
                return
            if not t.get("keySet"):
                text = "Не подключено: ключ API добавляется в веб-интерфейсе (Настройки → TMDB)."
            elif not t.get("enabled"):
                text = "Выключено в веб-интерфейсе — используются данные портала."
            else:
                text = "Подключено, язык описаний: %s" % ui_api.text(t.get("language") or "ru-RU")
            self["tmdb"].setText(text)

        def no_tmdb(failure):
            if not self.closed:
                self["tmdb"].setText("Не подключено — используются данные портала.")

        threads.deferToThread(ui_api.request_json, "/api/summary", None, 10).addCallbacks(summary, lambda f: None)
        threads.deferToThread(ui_api.request_json, "/api/tmdb", None, 10).addCallbacks(tmdb, no_tmdb)
