# -*- coding: utf-8 -*-
"""Native and legacy bouquet entry points, with asynchronous receiver UI."""
from __future__ import absolute_import

import sys
import unittest

from test_shell import Queue, Session, harness, programme, http_error
import plugin
from Plugins.Plugin import PluginDescriptor
from enigma import eServiceReference

REF = "4097:0:1:ABCD:1234:1DE:0:0:0:0:http%3a//provider.test/live.ts:Selected"
FOUND = {"source": {"id": "s1", "name": "Playlist"},
         "item": {"key": "channel", "id": "s1/itv/1", "name": "Selected", "kind": "channel", "archive": 3}}


class BouquetTest(unittest.TestCase):
    def test_native_menu_uses_selected_service_and_keeps_catalog_entries(self):
        PluginDescriptor.WHERE_CHANNEL_CONTEXT_MENU = "channelmenu"
        try:
            entries = plugin.Plugins()
            matches = [p for p in entries if p.where == "channelmenu"]
            self.assertEqual(len(matches), 1)
            self.assertEqual(matches[0].name, "Архив Dreaminator")
            session = Session()
            matches[0].fnc(session, service=eServiceReference(REF))
            cls, args = session.opened[-1]
            self.assertEqual((cls.__name__, args), ("BouquetArchiveScreen", (REF,)))
            self.assertEqual([p.name for p in entries if p.where == "pluginmenu"], ["Dreaminator"])
        finally:
            del PluginDescriptor.WHERE_CHANNEL_CONTEXT_MENU

    def test_legacy_menu_is_installed_once_and_preserves_existing_actions(self):
        import ui_bouquet
        from Screens.Screen import Screen
        class ChoiceList(object):
            def __init__(self):
                self.list = [[("Existing action", lambda: None)]]
                self.instance = None
            def setList(self, values):
                self.list = values
        class Context(Screen):
            calls = 0
            def __init__(self, session, csel):
                Screen.__init__(self, session)
                self.csel = csel
                Context.calls += 1
                self["menu"] = ChoiceList()
        class Selection(object):
            bouquet_mark_edit = 0
            movemode = False
            def getCurrentSelection(self):
                return eServiceReference(REF)
        module = harness.module("Screens.ChannelSelection")
        module.ChannelContextMenu = Context
        choices = harness.module("Components.ChoiceList")
        choices.ChoiceEntryComponent = lambda text: [text]
        ui_bouquet.install_legacy_menu(0)
        ui_bouquet.install_legacy_menu(0)
        session = Session()
        menu = Context(session, Selection())
        self.assertEqual(Context.calls, 1)
        self.assertEqual([e[0][0] for e in menu["menu"].list], ["Архив Dreaminator", "Existing action"])
        menu["menu"].list[0][0][1]()
        self.assertEqual(session.opened[-1][1], (REF,))
        self.assertEqual(menu.closed_with, ())
        Selection.movemode = True
        editing = Context(Session(), Selection())
        self.assertEqual(len(editing["menu"].list), 1)
        Selection.movemode = False
        Selection.getCurrentSelection = lambda self: eServiceReference(REF.replace(":0:1:", ":40:1:", 1))
        self.assertEqual(len(Context(Session(), Selection())["menu"].list), 1, "markers keep the original menu")

    def test_parental_protection_is_checked_before_opening(self):
        import ui_bouquet
        from Components.ParentalControl import parentalControl
        original = parentalControl.isServicePlayable
        pending = []
        def check(ref, callback):
            pending.append((ref, callback))
            return False
        parentalControl.isServicePlayable = check
        try:
            session = Session()
            selected = eServiceReference(REF)
            ui_bouquet.open_archive(session, service=selected)
            self.assertEqual(session.opened, [])
            self.assertIs(pending[0][0], selected)
            pending[0][1](ref=selected)  # the receiver accepted the PIN
            self.assertEqual(session.opened[-1][1], (REF,))
        finally:
            parentalControl.isServicePlayable = original

    def test_legacy_registration_preserves_the_existing_hotkey_entry_numbers(self):
        entries = plugin.Plugins()
        self.assertEqual([p.where for p in entries], ["pluginmenu", "extensionsmenu", "sessionstart"])
        self.assertIs(entries[-1].fnc, plugin.archive_session)

    def test_closing_loader_drops_a_late_answer(self):
        import ui_bouquet
        q = Queue()
        original = ui_bouquet.threads.deferToThread
        ui_bouquet.threads.deferToThread = q.defer
        try:
            session = Session()
            screen = ui_bouquet.BouquetArchiveScreen(session, REF)
            harness.layout(screen)
            screen.close()
            q.take("/api/catalog/service").complete(FOUND)
            self.assertEqual(session.opened, [])
        finally:
            ui_bouquet.threads.deferToThread = original

    def test_resolver_opens_archive_guide_and_back_closes_loader(self):
        import ui_bouquet
        q = Queue()
        original = ui_bouquet.threads.deferToThread
        ui_bouquet.threads.deferToThread = q.defer
        try:
            session = Session()
            screen = ui_bouquet.BouquetArchiveScreen(session, REF)
            harness.layout(screen)
            q.take("/api/catalog/service").complete(FOUND)
            cls, args = session.opened[-1]
            self.assertEqual(cls.__name__, "GuideScreen")
            self.assertEqual(args[:2], (FOUND["source"], FOUND["item"]))
            self.assertTrue(args[-1], "bouquet archive mode does not tune live on OK")
            session.callbacks[-1]()
            self.assertTrue(screen.closed)
        finally:
            ui_bouquet.threads.deferToThread = original

    def test_no_archive_explains_instead_of_opening_a_guide(self):
        import ui_bouquet
        screen = ui_bouquet.BouquetArchiveScreen(Session(), REF)
        screen.resolved(dict(FOUND, item=dict(FOUND["item"], archive=0)))
        self.assertEqual(screen.session.opened, [])
        self.assertIn("нет архива", screen["status"].getText())

    def test_failure_is_retryable_and_repeat_presses_share_one_lookup(self):
        import ui_bouquet
        q = Queue()
        original = ui_bouquet.threads.deferToThread
        ui_bouquet.threads.deferToThread = q.defer
        try:
            screen = ui_bouquet.BouquetArchiveScreen(Session(), REF)
            harness.layout(screen)
            screen.load()
            self.assertEqual(len(q.pending("/service")), 1)
            q.take("/service").error(http_error(404, {"error": "missing", "code": "missing"}))
            self.assertIn("нет в плейлистах", screen["status"].getText())
            screen["actions"].actions["yellow"]()
            self.assertEqual(len(q.pending("/service")), 1)
        finally:
            ui_bouquet.threads.deferToThread = original

    def test_archive_ok_restarts_current_and_does_not_tune_future(self):
        import ui_guide
        q = Queue()
        original = ui_guide.threads.deferToThread
        ui_guide.threads.deferToThread = q.defer
        try:
            screen = ui_guide.GuideScreen(Session(), FOUND["source"], FOUND["item"], "Archive", None, True)
            harness.layout(screen)
            current, future = programme("Current", -1200), programme("Future", 2400)
            q.take("/epg?").complete({"archive": 3, "days": [0], "programmes": [current, future]})
            screen.key_ok()
            self.assertEqual(q.take("/play").args[1]["start"], current["start"])
            screen.playing = False
            screen.model.focus = 1
            screen.key_ok()
            self.assertIn("ещё не началась", screen["msg"].getText())
            self.assertFalse(q.pending("/play"))
            self.assertFalse(hasattr(screen, "closed_with"))
        finally:
            ui_guide.threads.deferToThread = original


if __name__ == "__main__":
    unittest.main()
