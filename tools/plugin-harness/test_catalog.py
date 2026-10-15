# -*- coding: utf-8 -*-
from __future__ import absolute_import
import os
import sys
import unittest
import run as harness
harness.install_fakes()
sys.path.insert(0, os.path.join(harness.ROOT, "plugin", "Dreaminator"))
from catalog_state import playback_url

class PlaybackURLTest(unittest.TestCase):
    def test_headers_are_not_encoded_as_query_or_logged(self):
        u = playback_url({"url": "https://cdn.test/movie.mkv?token=123", "headers": {"User-Agent": "MAG test", "Cookie": "mac=aa; stb_lang=en"}})
        self.assertEqual(u, "https://cdn.test/movie.mkv?token=123#Cookie=mac=aa; stb_lang=en&User-Agent=MAG test")
        with self.assertRaises(ValueError):
            playback_url({"url": "file:///etc/passwd"})
        with self.assertRaises(ValueError):
            playback_url({"url": "http://test/movie", "headers": {"Cookie": "x\r\nAuthorization: leaked"}})

class Session(object):
    def __init__(self):
        self.opened = []

class CatalogPlayerTest(unittest.TestCase):
    def test_stop_restores_service_without_persisting_token(self):
        from catalog_player import CatalogPlayer
        from Components.config import config
        from enigma import eServiceReference
        class Nav(object):
            current = "previous-tv"
            def getCurrentlyPlayingServiceOrGroup(self):
                return self.current
            def playService(self, ref):
                self.current = ref
        session = Session()
        session.nav = Nav()
        config.usage.last_movie_played.value = "previous-movie"
        player = CatalogPlayer(session, eServiceReference(4097,0,"http://test/movie?secret=token"))
        player.leavePlayer()
        self.assertEqual(session.nav.current, "previous-tv")
        self.assertEqual(config.usage.last_movie_played.value, "previous-movie")
        self.assertFalse(player.ENABLE_RESUME_SUPPORT)

    def test_an_early_end_reports_failure_and_a_late_one_does_not(self):
        import catalog_player
        from enigma import eServiceReference
        class Nav(object):
            current = "previous-tv"
            def getCurrentlyPlayingServiceOrGroup(self):
                return self.current
            def playService(self, ref):
                self.current = ref
        session = Session()
        session.nav = Nav()
        player = catalog_player.CatalogPlayer(session, eServiceReference(4097, 0, "http://test/live.ts"))
        player.execing = True
        player.doEofInternal(True)
        self.assertEqual(player.closed_with, ("failed",))
        self.assertEqual(session.nav.current, "previous-tv")
        player = catalog_player.CatalogPlayer(session, eServiceReference(4097, 0, "http://test/movie.mp4"))
        player.execing = True
        player._opened -= catalog_player.EARLY_END_S + 1
        player.doEofInternal(True)
        self.assertEqual(player.closed_with, (None,))

if __name__ == "__main__":
    unittest.main()
