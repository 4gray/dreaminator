# -*- coding: utf-8 -*-
"""Exercise the Enigma2 plugin without a receiver.

Fake Enigma2 modules stand in for the GUI; the plugin talks to a real
Dreaminator (the virtual Dreambox from `make sim`). The script presses the
buttons, checks what the screen says and writes an HTML preview of the
screen laid out from the plugin's own skin.

    python3 tools/plugin-harness/run.py [preview.html]
    DREAMINATOR=http://127.0.0.1:8088 python2 tools/plugin-harness/run.py

Runs on Python 2.7 and 3, like the plugin.
"""
from __future__ import print_function

import base64
import io
import os
import sys
import time
import types
import xml.etree.ElementTree as ET

ROOT = os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
BASE = os.environ.get("DREAMINATOR", "http://127.0.0.1:8088")
WIDTH = int(os.environ.get("DESKTOP_WIDTH", "1920"))


# ---------------------------------------------------------------- fakes

def module(name):
    m = types.ModuleType(name)
    sys.modules[name] = m
    parent, _, child = name.rpartition(".")
    if parent:
        setattr(sys.modules[parent], child, m)
    return m


class Size(object):
    def __init__(self, w, h):
        self.w, self.h = w, h

    def width(self):
        return self.w

    def height(self):
        return self.h


class Instance(object):
    def __init__(self):
        self.sz = Size(0, 0)
        self.pixmap = None
        self.pos = None
        self.color = None
        self.font = None

    def size(self):
        return self.sz

    def setPixmap(self, p):
        self.pixmap = p

    def move(self, point):
        self.pos = (point.x, point.y)

    def resize(self, size):
        self.sz = Size(size.w, size.h)

    def setForegroundColor(self, color):
        self.color = color

    def setFont(self, font):
        self.font = font

    def setVAlign(self, align):
        self.valign = align

    def setValue(self, value):
        self.value = value


class Failure(object):
    def __init__(self, value):
        self.value = value


class Deferred(object):
    """Runs the job right away: good enough to drive the plugin's logic."""

    def __init__(self, fn, *args, **kwargs):
        try:
            self.result, self.ok = fn(*args, **kwargs), True
        except Exception as e:  # noqa: BLE001
            self.result, self.ok = e, False

    def addCallbacks(self, callback, errback):
        if self.ok:
            callback(self.result)
        else:
            errback(Failure(self.result))
        return self

    def addBoth(self, callback):
        callback(self.result)
        return self


def install_fakes():
    enigma = module("enigma")

    class eTimer(object):
        def __init__(self):
            self.callback = []
            self.active = None

        def start(self, ms, single=False):
            self.active = ms

        def stop(self):
            self.active = None

    class Desktop(object):
        def size(self):
            return Size(WIDTH, WIDTH * 9 // 16)

    class eServiceReference(object):
        def __init__(self, *args):
            self.args = args
        def setName(self, name):
            self.name = name
        def toString(self):
            return str(self.args[-1])
    enigma.eServiceReference = eServiceReference
    enigma.eTimer = eTimer
    enigma.getDesktop = lambda n: Desktop()

    class ePoint(object):
        def __init__(self, x, y):
            self.x, self.y = x, y

    class eSize(object):
        def __init__(self, w, h):
            self.w, self.h = w, h

    class gFont(object):
        def __init__(self, family, size):
            self.family, self.size = family, size

    class gRGB(object):
        def __init__(self, value):
            self.value = value

    class Signal(object):
        def __init__(self):
            self.handlers = []

        def get(self):
            return self.handlers

    class ePicLoad(object):
        """Decodes nothing: hands back the file name as the 'pixmap'."""
        def __init__(self):
            self.PictureData = Signal()
            self.para = None
            self.data = None

        def setPara(self, para):
            self.para = para

        def startDecode(self, path):
            self.data = path
            for f in self.PictureData.get():
                f("")
            return 0

        def getData(self):
            return self.data

    class eLabel(object):
        alignLeft = alignTop = 0
        alignCenter = 1
        alignRight = alignBottom = 2

    enigma.ePoint, enigma.eSize, enigma.gFont, enigma.gRGB, enigma.ePicLoad = ePoint, eSize, gFont, gRGB, ePicLoad
    enigma.eLabel = eLabel

    module("twisted")
    module("twisted.internet")
    module("twisted.internet.threads").deferToThread = lambda fn, *a, **k: Deferred(fn, *a, **k)

    module("Components")

    class ActionMap(object):
        def __init__(self, contexts, actions, prio=0):
            self.contexts, self.actions = contexts, actions

    module("Components.ActionMap").ActionMap = ActionMap

    class ParentalControl(object):
        def isServicePlayable(self, ref, callback, session=None):
            return True
    module("Components.ParentalControl").parentalControl = ParentalControl()

    class Label(object):
        def __init__(self, text=""):
            self.text = text
            self.instance = Instance()
            self.visible = True

        def setText(self, text):
            self.text = text

        def getText(self):
            return self.text

    Label.show = lambda self: setattr(self, "visible", True)
    Label.hide = lambda self: setattr(self, "visible", False)
    module("Components.Label").Label = Label

    class Pixmap(object):
        def __init__(self):
            self.instance = Instance()
            self.visible = True

    Pixmap.show = lambda self: setattr(self, "visible", True)
    Pixmap.hide = lambda self: setattr(self, "visible", False)
    module("Components.Pixmap").Pixmap = Pixmap

    class ProgressBar(object):
        """eSlider: a value from 0 to 100."""
        def __init__(self):
            self.instance = Instance()
            self.visible = True
            self.value = 0

        def setValue(self, value):
            self.value = int(value)
            self.instance.setValue(self.value)

        def getText(self):
            return ""

    ProgressBar.show = lambda self: setattr(self, "visible", True)
    ProgressBar.hide = lambda self: setattr(self, "visible", False)
    module("Components.ProgressBar").ProgressBar = ProgressBar

    class MenuList(object):
        def __init__(self, items):
            self.items, self.index = items, 0
            self.onSelectionChanged = []
            self.instance = Instance()
        def setList(self, items):
            self.items, self.index = items, 0
        def getSelectionIndex(self):
            return self.index
        def moveToIndex(self, index):
            self.index = max(0, min(index, len(self.items)-1))
            for fn in self.onSelectionChanged:
                fn()
        def up(self):
            self.moveToIndex(self.index-1)
        def down(self):
            self.moveToIndex(self.index+1)
        def getText(self):
            return "\n".join(self.items)
    module("Components.MenuList").MenuList = MenuList

    module("Plugins")

    class PluginDescriptor(object):
        WHERE_PLUGINMENU = "pluginmenu"
        WHERE_EXTENSIONSMENU = "extensionsmenu"
        WHERE_SESSIONSTART = "sessionstart"
        WHERE_EVENTINFO = "eventinfo"

        def __init__(self, **kwargs):
            self.__dict__.update(kwargs)

    module("Plugins.Plugin").PluginDescriptor = PluginDescriptor

    module("Screens")

    class Screen(dict):
        def __init__(self, session):
            dict.__init__(self)
            self.session = session
            self.onLayoutFinish = []
            self.onClose = []
            self.title = ""

        def setTitle(self, title):
            self.title = title

        def show(self):
            self.shown = True

        def close(self, *retval):
            self.closed_with = retval
            for f in self.onClose:
                f()
            # GUISkin.deleteGUIScreen walks the components like this; an
            # attribute shadowing dict.items/values crashes Enigma2 on close.
            for name, val in self.items():
                assert hasattr(val, "instance") or name == "actions" or val is None, name
            list(self.values())

    module("Screens.Screen").Screen = Screen

    class ConfigValue(object):
        value = "previous-movie"
        def save(self):
            self.saved = self.value
    class ConfigSubsection(object):
        """Like Enigma2's: unknown names raise AttributeError."""

    class ConfigText(object):
        def __init__(self, default="", fixed_size=True):
            self.value = default

        def save(self):
            pass

    config = types.ModuleType("config")
    config.usage = types.ModuleType("usage")
    config.usage.last_movie_played = ConfigValue()
    config.plugins = ConfigSubsection()
    components_config = module("Components.config")
    components_config.config = config
    components_config.ConfigSubsection = ConfigSubsection
    components_config.ConfigText = ConfigText

    class MessageBox(Screen):
        TYPE_INFO = 1

        def __init__(self, session, text, kind=0, timeout=0):
            Screen.__init__(self, session)
            self.text = text
    module("Screens.MessageBox").MessageBox = MessageBox

    class MoviePlayer(Screen):
        def __init__(self, session, service):
            Screen.__init__(self, session)
            self.cur_service = service
            self.lastservice = session.nav.getCurrentlyPlayingServiceOrGroup()
            session.nav.playService(service)
            self.onClose.append(self.restore)
        def restore(self):
            self.session.nav.playService(self.lastservice)
            config.usage.last_movie_played.value = self.cur_service.toString()
            config.usage.last_movie_played.save()
    class InfoBar(Screen):
        instance = None
    infobar = module("Screens.InfoBar")
    infobar.MoviePlayer = MoviePlayer
    infobar.InfoBar = InfoBar

    module("Tools")
    module("Tools.LoadPixmap").LoadPixmap = lambda path, desktop=None, cached=None: path if os.path.exists(path) else None

    # Enigma2's keymap reader: parses the plugin's keymap.xml like the real
    # one, so a broken file fails here too. maps holds (key, action, flags).
    keymapparser = module("keymapparser")
    keymapparser.maps = []

    def readKeymap(path):
        for m in ET.parse(path).getroot().iter("map"):
            for key in m.iter("key"):
                keymapparser.maps.append((m.get("context"), key.get("id"), key.get("mapto"), key.get("flags")))
    keymapparser.readKeymap = readKeymap


# ---------------------------------------------------------------- layout

def parse_skin(skin):
    screen = ET.fromstring(skin.strip())
    return screen, list(screen)


def xy(value):
    a, b = value.split(",")
    return int(a), int(b)


def layout(screen_obj):
    """Give widgets their skin sizes, then fire onLayoutFinish like Enigma2."""
    root, elements = parse_skin(screen_obj.skin)
    for el in elements:
        name = el.get("name")
        if el.tag == "widget" and el.get("source"):
            continue  # global sources (the clock) need no component
        if el.tag == "widget":
            assert name in screen_obj, "skin widget %r has no component" % name
            screen_obj[name].instance.sz = Size(*xy(el.get("size")))
    for name in screen_obj:
        if name != "actions":
            assert any(el.get("name") == name for el in elements), "component %r missing from skin" % name
    for f in screen_obj.onLayoutFinish:
        f()


def check_bounds(skin):
    root, elements = parse_skin(skin)
    sw, sh = xy(root.get("size"))
    for el in elements:
        x, y = xy(el.get("position"))
        w, h = xy(el.get("size"))
        assert x >= 0 and y >= 0 and x + w <= sw and y + h <= sh, "%s %s is outside the screen" % (el.tag, el.get("name"))
    return sw, sh


# ---------------------------------------------------------------- preview

def html_escape(s):
    s = to_text(s)
    return s.replace("&", "&amp;").replace("<", "&lt;").replace(">", "&gt;").replace("\n", "<br>")


def to_text(s):
    """Unicode on both Pythons (the plugin hands Python 2 UTF-8 bytes)."""
    return s.decode("utf-8") if isinstance(s, bytes) else s


def preview(screen_obj, path, qr_file):
    root, elements = parse_skin(screen_obj.skin)
    sw, sh = xy(root.get("size"))
    dw, dh = WIDTH, WIDTH * 9 // 16
    left, top = (dw - sw) // 2, (dh - sh) // 2
    parts = []
    for el in elements:
        x, y = xy(el.get("position"))
        w, h = xy(el.get("size"))
        style = "left:%dpx;top:%dpx;width:%dpx;height:%dpx;" % (x, y, w, h)
        if el.get("backgroundColor") and el.get("transparent") != "1":
            style += "background:%s;" % el.get("backgroundColor")
        if el.get("foregroundColor"):
            style += "color:%s;" % el.get("foregroundColor")
        if el.get("font"):
            style += "font-size:%spx;" % el.get("font").split(";")[1]
        if el.get("halign") == "center":
            style += "text-align:center;justify-content:center;"
        if el.get("valign") == "center":
            style += "align-items:center;"
        style += "z-index:%s;" % el.get("zPosition", "0")
        body = ""
        name = el.get("name")
        if name == "qr" and screen_obj["qr"].instance.pixmap:
            with open(qr_file, "rb") as f:
                data = base64.b64encode(f.read()).decode("ascii")
            body = '<img src="data:image/png;base64,%s" style="image-rendering:pixelated">' % data
        elif name:
            body = html_escape(screen_obj[name].getText())
        parts.append('<div class="w" style="%s">%s</div>' % (style, body))
    page = u"""<!doctype html><meta charset="utf-8"><title>Dreaminator plugin preview</title>
<style>
body{margin:0;background:#000;font-family:"Segoe UI",Roboto,Arial,sans-serif}
.tv{position:relative;width:%dpx;height:%dpx;background:linear-gradient(135deg,#1c2b3a,#0b0f14);overflow:hidden}
.title{position:absolute;left:%dpx;top:%dpx;width:%dpx;height:54px;background:#1e2a3a;color:#fff;font-size:30px;display:flex;align-items:center;padding-left:24px;box-sizing:border-box}
.scr{position:absolute;left:%dpx;top:%dpx;width:%dpx;height:%dpx;background:%s}
.w{position:absolute;display:flex;white-space:normal;line-height:1.25;overflow:hidden;box-sizing:border-box}
.w img{width:100%%;height:100%%;object-fit:contain}
</style>
<div class="tv"><div class="title">%s</div><div class="scr">%s</div></div>""" % (
        dw, dh, left, top - 54, sw, left, top, sw, sh, root.get("backgroundColor"),
        html_escape(screen_obj.title), u"".join(to_text(p) for p in parts))
    with io.open(path, "w", encoding="utf-8") as f:
        f.write(page)


# ---------------------------------------------------------------- run

def main():
    install_fakes()
    sys.path.insert(0, os.path.join(ROOT, "plugin", "Dreaminator"))
    import plugin  # noqa: E402

    plugin.BASE = BASE
    for width in (1920, 1280):
        check_bounds(plugin.scale_skin(plugin.SKIN, width))

    descriptors = plugin.Plugins()
    assert [d.where for d in descriptors] == ["pluginmenu", "extensionsmenu", "sessionstart"]
    assert descriptors[-1].fnc is plugin.archive_session

    screen = plugin.DreaminatorScreen(session=None)
    screen.show()  # Enigma2 Session.execBegin calls Screen.show without arguments.
    layout(screen)
    status = screen["status"].getText()
    url = screen["url"].getText()
    print("url:", to_text(url))
    print("status:\n" + to_text(status))
    assert to_text(url).startswith("http://"), url
    assert u"Программа передач" in to_text(status), status
    assert screen["qr"].instance.pixmap, "QR code was not loaded"
    assert screen.timer.active in (3000, 10000), screen.timer.active

    screen["actions"].actions["green"]()
    print("green:", to_text(screen["status"].getText()))
    assert u"Готово" in to_text(screen["status"].getText()), screen["status"].getText()

    screen["actions"].actions["yellow"]()
    time.sleep(0.5)
    screen.poll()
    print("yellow:", to_text(screen["status"].getText()).split("\n")[0])
    for _ in range(60):
        if screen.timer.active == 10000:
            break
        time.sleep(1)
        screen.poll()
    assert screen.timer.active == 10000, "refresh did not finish"

    if len(sys.argv) > 1:
        preview(screen, sys.argv[1], plugin.QR_PATH)
        print("preview:", sys.argv[1])

    plugin.BASE = "http://127.0.0.1:1"
    screen.poll()
    print("offline:", to_text(screen["status"].getText()).split("\n")[0])
    assert u"не отвечает" in to_text(screen["status"].getText())

    screen.close()
    assert screen.closed and screen.timer.active is None
    print("OK (Python %d.%d)" % sys.version_info[:2])


if __name__ == "__main__":
    main()
