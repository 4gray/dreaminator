# -*- coding: utf-8 -*-
"""Archive entry for a selected native bouquet service, on old and new images."""
from __future__ import absolute_import

from twisted.internet import threads
from Components.ActionMap import ActionMap
from Components.Label import Label
from Screens.Screen import Screen

try:
    from . import ui_api, ui_epgkey
except (ImportError, ValueError):
    import ui_api
    import ui_epgkey

NAME = "Архив Dreaminator"


def open_archive(session, service=None, serviceref=None, **kwargs):
    selected = service if service is not None else serviceref
    ref = ui_epgkey.played_ref(session, selected) if selected is not None else ""

    def allowed(**unused):
        session.open(BouquetArchiveScreen, ref)

    if selected is not None and ui_epgkey.is_stream(ref):
        from Components.ParentalControl import parentalControl
        if not parentalControl.isServicePlayable(getattr(selected, "ref", selected), callback=allowed):
            return
    allowed()


class BouquetArchiveScreen(Screen):
    # Kept small while the local service resolves the selected channel.
    # GuideScreen and CatalogPlayer provide the actual archive interface.
    def __init__(self, session, ref):
        try:
            from . import plugin
        except (ImportError, ValueError):
            import plugin
        from enigma import getDesktop
        self.skin = plugin.scale_skin('''<screen name="DreaminatorBouquetArchive" position="center,center" size="1100,240" title="Архив Dreaminator" backgroundColor="#10141a">
            <widget name="status" position="40,30" size="1020,110" font="Regular;32" foregroundColor="#ffffff" backgroundColor="#10141a" />
            <widget name="hint" position="40,165" size="1020,40" font="Regular;25" foregroundColor="#a8adb6" backgroundColor="#10141a" />
        </screen>''', getDesktop(0).size().width())
        Screen.__init__(self, session)
        self.setTitle(NAME)
        self.ref = ref
        self.closed = False
        self.loading = False
        self["status"] = Label("Открываю архив канала…")
        self["hint"] = Label("Back — вернуться в список каналов")
        self["actions"] = ActionMap(["OkCancelActions", "ColorActions"], {
            "cancel": self.close, "red": self.close, "yellow": self.load,
        }, -1)
        self.onLayoutFinish.append(self.load)
        self.onClose.append(self.cleanup)

    def cleanup(self):
        self.closed = True

    def load(self):
        if self.closed or self.loading:
            return
        if not ui_epgkey.is_stream(self.ref):
            self["status"].setText("Выберите IPTV-канал из плейлистов Dreaminator")
            return
        self.loading = True
        self["status"].setText("Открываю архив канала…")
        path = ui_api.query_path("/api/catalog/service", ref=self.ref)
        threads.deferToThread(ui_api.request_json, path, None, 5, True).addCallbacks(self.resolved, self.failed)

    def resolved(self, found):
        self.loading = False
        if self.closed:
            return
        source, item = (found or {}).get("source") or {}, (found or {}).get("item") or {}
        if not source.get("id") or not item.get("key"):
            self["status"].setText("Этого канала нет в плейлистах Dreaminator")
            return
        if not item.get("archive"):
            self["status"].setText("У этого канала нет архива в плейлисте провайдера")
            return
        try:
            from .ui_guide import GuideScreen
        except (ImportError, ValueError):
            from ui_guide import GuideScreen
        self.session.openWithCallback(self.close, GuideScreen, source, item, NAME, None, True)

    def failed(self, failure):
        self.loading = False
        if self.closed:
            return
        problem = ui_api.problem(failure)
        self["status"].setText("Этого канала нет в плейлистах Dreaminator" if problem.kind == "missing" else problem.message)
        self["hint"].setText("Жёлтая — повторить · Back — вернуться в список")


def install_legacy_menu(reason, **kwargs):
    """OpenHDF 6.x has no WHERE_CHANNEL_CONTEXT_MENU. Extend its menu once,
    leaving the original constructor and all existing actions intact."""
    if reason != 0:
        return
    from Screens.ChannelSelection import ChannelContextMenu
    from Components.ChoiceList import ChoiceEntryComponent
    original = ChannelContextMenu.__init__
    if getattr(original, "dreaminator_archive", False):
        return

    def init(context, session, csel, *args, **kw):
        original(context, session, csel, *args, **kw)
        if getattr(csel, "bouquet_mark_edit", 0) or getattr(csel, "movemode", False):
            return
        service = csel.getCurrentSelection()
        ref = ui_epgkey.played_ref(session, service) if service is not None else ""
        parts = ref.split(":")
        if len(parts) < 11 or parts[1] != "0" or not ui_epgkey.is_stream(ref):
            return

        def launch():
            # Match the native hook's ordering: open the child, close this menu.
            try:
                from . import plugin
            except (ImportError, ValueError):
                import plugin
            plugin.archive(session, service=service)
            context.close()

        menu = context["menu"]
        menu.setList([ChoiceEntryComponent(text=(NAME, launch))] + list(menu.list))

    init.dreaminator_archive = True
    ChannelContextMenu.__init__ = init
