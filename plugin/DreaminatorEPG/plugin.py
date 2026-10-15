# -*- coding: utf-8 -*-
# The EPG key entry of Dreaminator (Dreaminator/ui_epgkey.py). It lives in its
# own plugin folder: OpenHDF's Hotkey numbers plugin entries per folder
# (Plugins/Extensions/<folder>/<n>, event info entries first), so a second
# entry in Dreaminator's folder would take the number that buttons already
# given to the catalog use.
from __future__ import absolute_import

from Plugins.Plugin import PluginDescriptor


def _dreaminator():
    try:
        from Plugins.Extensions.Dreaminator import plugin
    except ImportError:  # the test harness puts Dreaminator on sys.path
        import plugin
    return plugin


def epg(session, service=None, event=None, **kwargs):
    """No "servicelist" or "selectedevent" among the names here: with them
    OpenHDF leaves the entry out of the event info menu or the Hotkey list."""
    _dreaminator().epg(session, service=service, event=event, **kwargs)


def Plugins(**kwargs):
    try:
        from Plugins.Extensions.Dreaminator import ui_epgkey
    except ImportError:
        import ui_epgkey
    ui_epgkey.register_config()
    return [PluginDescriptor(name=ui_epgkey.NAME, description="Прошедшие передачи из архива провайдера",
                             where=PluginDescriptor.WHERE_EVENTINFO, fnc=epg)]
