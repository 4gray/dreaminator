# -*- coding: utf-8 -*-
"""The EPG key on a channel of the receiver's channel lists.

Enigma2 has no archive for IPTV services: a list line plays the stream on
air. With the EPG key given to Dreaminator (Hotkey setup, or "Dreaminator —
программа и архив" in the event info menu; the entry is registered by the
DreaminatorEPG plugin folder), a playlist channel with an
archive opens the catalog's programme screen, where past programmes play
from the provider's archive and Back returns to the channel on air. Every
other service keeps the receiver's own EPG.
"""
from __future__ import absolute_import

from twisted.internet import threads

try:
    from . import ui_api
except (ImportError, ValueError):
    import ui_api

NAME = "Dreaminator — программа и архив"
# What the EPG key did before it was given to Dreaminator (the Hotkey
# function id); OpenHDF's default is the single service EPG.
NATIVE_DEFAULT = "Infobar/openSingleServiceEPG"
CRUMB = "Программа и архив"
TIMEOUT = 5
BUSY = [False]  # one lookup at a time: a second press while it runs is dropped


def register_config():
    """config.plugins.Dreaminator.epgnative: the receiver's own EPG for
    services without an archive. Kept in /etc/enigma2/settings."""
    try:
        from Components.config import ConfigSubsection, ConfigText, config
        if not hasattr(config.plugins, "Dreaminator"):
            config.plugins.Dreaminator = ConfigSubsection()
        if not hasattr(config.plugins.Dreaminator, "epgnative"):
            config.plugins.Dreaminator.epgnative = ConfigText(default=NATIVE_DEFAULT, fixed_size=False)
    except Exception:
        pass


def native_function():
    try:
        from Components.config import config
        value = config.plugins.Dreaminator.epgnative.value
    except Exception:
        value = ""
    # Only InfoBar methods: a plugin here could be Dreaminator itself.
    if not value.startswith("Infobar/") or "Dreaminator" in value:
        value = NATIVE_DEFAULT
    return value.split("/", 1)[1]


def infobar():
    try:
        from Screens.InfoBar import InfoBar
        return InfoBar.instance
    except Exception:
        return None


def open_native(screen):
    """The receiver's EPG, on the screen the key was pressed on (the
    infobar, or a player with its own EPG) or else the infobar."""
    method = native_function()
    for target in (screen, infobar()):
        fn = getattr(target, method, None)
        if callable(fn):
            fn()
            return True
    return False


def played_ref(session, service):
    """The reference string of the service: the one the event info menu
    names, or else the one playing."""
    ref = None
    if service is not None:
        ref = getattr(service, "ref", service)  # ServiceReference or eServiceReference
    if ref is None:
        try:
            ref = session.nav.getCurrentlyPlayingServiceReference()
        except Exception:
            ref = None
    try:
        return ref.toString() if ref is not None else ""
    except Exception:
        return ""


def is_stream(ref):
    """An IPTV line: a stream address in the path field (Enigma2 prints
    ':' in it as %3a). Satellite services need no request."""
    parts = ref.split(":")
    if len(parts) < 11:
        return False
    path = parts[10].lower()
    return "%3a//" in path or "://" in path


def open_guide(session, found, ref, start=None):
    try:
        from .ui_guide import GuideScreen
    except (ImportError, ValueError):
        from ui_guide import GuideScreen
    source, item = found.get("source") or {}, found.get("item") or {}
    crumbs = " › ".join(ui_api.text(x) for x in (source.get("name") or "", CRUMB) if x)

    def closed(*args):
        # OK on the programme on air: watch the channel. From the EPG key it
        # is on already; the event info menu may name another one.
        if args and args[0] == "play" and played_ref(session, None) != ref:
            try:
                from enigma import eServiceReference
                session.nav.playService(eServiceReference(ref))
            except Exception:
                pass

    session.openWithCallback(closed, GuideScreen, source, item, crumbs, start)


def show_message(session, text):
    try:
        from Screens.MessageBox import MessageBox
        session.open(MessageBox, text, MessageBox.TYPE_INFO, timeout=5)
    except Exception:
        pass


def epg(session, service=None, event=None, **kwargs):
    """WHERE_EVENTINFO entry. From the EPG key (Hotkey, the EPG plugin list)
    Enigma2 passes servicelist; from the event info menu service and event.

    No "servicelist" or "selectedevent" in the signature: with them OpenHDF
    would leave the plugin out of the event info menu or the Hotkey list."""
    from_menu = service is not None
    screen = getattr(session, "current_dialog", None)
    ref = played_ref(session, service)
    start = None
    if event is not None:
        try:
            start = int(event.getBeginTime())
        except Exception:
            start = None

    def fallback():
        if from_menu:
            show_message(session, "Этого канала нет в плейлистах Dreaminator")
        else:
            open_native(screen)

    if not ref or not is_stream(ref):
        fallback()
        return

    def moved():
        """The viewer went elsewhere while the service answered: another
        screen, or (from the EPG key) another channel."""
        if getattr(session, "current_dialog", None) is not screen:
            return True
        return not from_menu and played_ref(session, None) != ref
    if BUSY[0]:
        return
    BUSY[0] = True

    def done(found):
        BUSY[0] = False
        item = (found or {}).get("item") or {}
        try:
            archive = int(item.get("archive") or 0)
        except (TypeError, ValueError):
            archive = 0
        if moved():
            return
        if not item.get("key") or (archive <= 0 and not from_menu):
            fallback()
            return
        open_guide(session, found, ref, start)

    def failed(failure):
        BUSY[0] = False
        if not moved():
            fallback()

    path = ui_api.query_path("/api/catalog/service", ref=ref)
    threads.deferToThread(ui_api.request_json, path, None, TIMEOUT, True).addCallbacks(done, failed)
