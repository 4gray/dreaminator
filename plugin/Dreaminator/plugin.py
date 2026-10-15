# -*- coding: utf-8 -*-
# Dreaminator plugin for Enigma2: shows the tablet address (with a QR code)
# and the service status on the TV, plus a few actions.
#
# It runs inside Enigma2, so it is deliberately small: every request goes to
# the local Dreaminator service in a background thread, results come back on
# the reactor thread, and nothing here raises into Enigma2. Works on Python 2
# (OpenHDF 6.x, OE-Alliance 4.x) and Python 3 images.
from __future__ import absolute_import

import json
import os
import re
import sys

try:  # Python 2
    from urllib2 import HTTPError, Request, urlopen
except ImportError:  # Python 3
    from urllib.error import HTTPError
    from urllib.request import Request, urlopen

from enigma import eTimer, getDesktop
from twisted.internet import threads

from Components.ActionMap import ActionMap
from Components.Label import Label
from Components.Pixmap import Pixmap
from Plugins.Plugin import PluginDescriptor
from Screens.Screen import Screen
from Tools.LoadPixmap import LoadPixmap

BASE = "http://127.0.0.1:8088"
QR_PATH = "/tmp/dreaminator-qr.png"
PY2 = sys.version_info[0] == 2


def native(value):
    """Enigma2 on Python 2 wants UTF-8 byte strings; JSON gives unicode."""
    if PY2:
        if isinstance(value, dict):
            return dict((native(k), native(v)) for k, v in value.items())
        if isinstance(value, list):
            return [native(v) for v in value]
        if isinstance(value, unicode):  # noqa: F821 (Python 2 only)
            return value.encode("utf-8")
    return value


def api(path, post=False, timeout=10):
    req = Request(BASE + path, data=b"" if post else None)
    resp = urlopen(req, timeout=timeout)
    try:
        return resp.read()
    finally:
        resp.close()


def summary():
    return native(json.loads(api("/api/summary", timeout=5)))


def error_text(failure):
    err = getattr(failure, "value", failure)
    if isinstance(err, HTTPError):
        try:
            return native(json.loads(err.read())["error"])
        except Exception:
            return "ошибка %s" % err.code
    reason = getattr(err, "reason", None) or err
    return "%s" % reason


# Written for 1920x1080 skins and scaled for HD ones.
SKIN = """
<screen name="DreaminatorScreen" position="center,center" size="1400,780" title="Dreaminator" backgroundColor="#10141a">
	<eLabel position="40,40" size="500,500" backgroundColor="#ffffff" zPosition="1" />
	<widget name="qr" position="50,50" size="480,480" alphatest="on" zPosition="2" />
	<widget name="qrhint" position="40,555" size="500,90" font="Regular;28" halign="center" foregroundColor="#a8adb6" backgroundColor="#10141a" transparent="1" />
	<widget name="urllabel" position="590,45" size="770,40" font="Regular;30" foregroundColor="#a8adb6" backgroundColor="#10141a" transparent="1" />
	<widget name="url" position="590,90" size="770,70" font="Regular;50" foregroundColor="#ffffff" backgroundColor="#10141a" transparent="1" />
	<eLabel position="590,185" size="770,2" backgroundColor="#2c323d" />
	<widget name="status" position="590,210" size="770,440" font="Regular;31" foregroundColor="#e3e5e8" backgroundColor="#10141a" transparent="1" />
	<widget name="key_red" position="40,690" size="315,56" font="Regular;28" halign="center" valign="center" foregroundColor="#ffffff" backgroundColor="#9f1313" />
	<widget name="key_green" position="375,690" size="315,56" font="Regular;28" halign="center" valign="center" foregroundColor="#ffffff" backgroundColor="#1f771f" />
	<widget name="key_yellow" position="710,690" size="315,56" font="Regular;28" halign="center" valign="center" foregroundColor="#1a1a1a" backgroundColor="#c9a400" />
	<widget name="key_blue" position="1045,690" size="315,56" font="Regular;28" halign="center" valign="center" foregroundColor="#ffffff" backgroundColor="#18188b" />
</screen>"""


def scale_skin(skin, width):
    if width >= 1920:
        return skin
    f = width / 1920.0

    def dims(m):
        return '%s="%d,%d"' % (m.group(1), int(int(m.group(2)) * f), int(int(m.group(3)) * f))

    def font(m):
        return 'font="Regular;%d"' % max(12, int(int(m.group(1)) * f))

    skin = re.sub(r'(position|size)="(\d+),(\d+)"', dims, skin)
    return re.sub(r'font="Regular;(\d+)"', font, skin)


def describe(s, busy):
    lines = []
    if busy:
        lines.append(busy)
    elif s.get("refreshing"):
        lines.append("Обновляю плейлисты…")
    if not s.get("sources"):
        lines.append("Плейлистов пока нет — добавьте их с планшета.")
    else:
        when = s.get("lastRefreshText") or "ещё не загружались"
        lines.append("Плейлистов: %d, обновлены %s" % (s["sources"], when))
        lines.append("Каналов в ваших списках: %d" % s.get("channels", 0))
    if s.get("dirty"):
        lines.append("Есть изменения, не отправленные на ТВ — зелёная кнопка.")
    elif s.get("lastApplyText"):
        lines.append("Списки отправлены на ТВ %s" % s["lastApplyText"])
    lines.append("Программа передач: %s" % s.get("epgText", ""))
    problems = s.get("problems") or []
    if problems:
        lines.append("")
        lines.append("Внимание:")
        for p in problems[:3]:
            lines.append("• " + p)
    return "\n".join(lines)


class DreaminatorScreen(Screen):
    skin = scale_skin(SKIN, getDesktop(0).size().width())

    def __init__(self, session):
        Screen.__init__(self, session)
        self.setTitle("Dreaminator")
        self["qr"] = Pixmap()
        self["qrhint"] = Label("Наведите камеру планшета,\nчтобы открыть Dreaminator")
        self["urllabel"] = Label("Адрес для планшета и компьютера:")
        self["url"] = Label("…")
        self["status"] = Label("Подключаюсь к Dreaminator…")
        self["key_red"] = Label("Закрыть")
        self["key_green"] = Label("Отправить на ТВ")
        self["key_yellow"] = Label("Обновить плейлисты")
        self["key_blue"] = Label("Перезапустить")
        self["actions"] = ActionMap(["OkCancelActions", "ColorActions"], {
            "ok": self.close,
            "cancel": self.close,
            "red": self.close,
            "green": self.apply,
            "yellow": self.refresh,
            "blue": self.restart,
        }, -1)
        self.busy = None
        self.closed = False
        self.qr_url = None
        self.timer = eTimer()
        self.timer.callback.append(self.poll)
        self.onLayoutFinish.append(self.poll)
        self.onClose.append(self.cleanup)

    def cleanup(self):
        self.closed = True
        self.timer.stop()

    def later(self, ms):
        if not self.closed:
            self.timer.start(ms, True)

    # ---- status ----

    def poll(self):
        self.timer.stop()
        threads.deferToThread(summary).addCallbacks(self.show, self.unreachable)

    def show(self, s):
        if self.closed:
            return
        url = s.get("url") or ""
        self["url"].setText(url or "нет адреса — проверьте сеть")
        if url and url != self.qr_url:
            self.load_qr(url)
        self["status"].setText(describe(s, self.busy))
        self.later(3000 if (s.get("busy") or self.busy) else 10000)

    def unreachable(self, failure):
        if self.closed:
            return
        self["status"].setText("Сервис Dreaminator не отвечает.\n"
                               "Нажмите синюю кнопку, чтобы перезапустить его.\n\n"
                               "Подробности для сына: %s" % error_text(failure))
        self.later(5000)

    def load_qr(self, url):
        size = self["qr"].instance.size().width()

        def fetch():
            data = api("/api/qr.png?size=%d" % size)
            with open(QR_PATH, "wb") as f:
                f.write(data)

        threads.deferToThread(fetch).addCallbacks(lambda _: self.show_qr(url), lambda _: None)

    def show_qr(self, url):
        if self.closed:
            return
        pixmap = LoadPixmap(QR_PATH, cached=False)
        if pixmap:
            self["qr"].instance.setPixmap(pixmap)
            self.qr_url = url

    # ---- actions ----

    def run(self, busy, path, timeout, done_text):
        if self.busy:
            return
        self.busy = busy
        self["status"].setText(busy)

        def ok(_):
            self.busy = None
            if not self.closed:
                self["status"].setText(done_text)
                self.later(1500)

        def fail(failure):
            self.busy = None
            if not self.closed:
                self["status"].setText("Не получилось: %s" % error_text(failure))
                self.later(8000)

        threads.deferToThread(api, path, True, timeout).addCallbacks(ok, fail)

    def apply(self):
        self.run("Отправляю списки на ТВ…", "/api/apply", 90, "Готово! Списки обновлены на ТВ.")

    def refresh(self):
        self.run("Обновляю плейлисты…", "/api/refresh-all", 10, "Обновляю плейлисты…")

    def restart(self):
        if self.busy:
            return
        self.busy = "Перезапускаю сервис Dreaminator…"
        self["status"].setText(self.busy)

        def restarted(_):
            self.busy = None
            self.later(3000)

        threads.deferToThread(os.system, "/etc/init.d/dreaminator restart >/dev/null 2>&1").addBoth(restarted)


def main(session, **kwargs):
    session.open(DreaminatorScreen)


def Plugins(**kwargs):
    description = "Плейлисты IPTV с планшета: адрес и состояние"
    return [
        PluginDescriptor(name="Dreaminator", description=description,
                         where=PluginDescriptor.WHERE_PLUGINMENU, icon="plugin.png", fnc=main),
        PluginDescriptor(name="Dreaminator", description=description,
                         where=PluginDescriptor.WHERE_EXTENSIONSMENU, fnc=main),
    ]
