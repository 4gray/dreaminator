# -*- coding: utf-8 -*-
"""Native MoviePlayer with catalog-specific exit and no URL history."""
from __future__ import absolute_import
import time

from enigma import eServiceReference
from Screens.InfoBar import MoviePlayer
from Components.config import config


# A stream that ends this soon after opening never really started: the
# provider refused it (channel down, subscription, connection limit).
EARLY_END_S = 8
FAILED = "failed"


class CatalogPlayer(MoviePlayer):
    ENABLE_RESUME_SUPPORT = False

    def __init__(self, session, service):
        self._opened = time.time()
        self._previous_movie = config.usage.last_movie_played.value
        MoviePlayer.__init__(self, session, service)
        self.skinName = "MoviePlayer"
        # OpenHDF saves cur_service on close. Restore its old value before that
        # handler so temporary provider credentials never enter last_movie_played.
        self.onClose.insert(0, self._forget_stream)

    def _forget_stream(self):
        self.cur_service = eServiceReference(self._previous_movie)

    def leavePlayer(self):
        self.close()

    def leavePlayerOnExit(self):
        self.close()

    def showMovies(self):
        # The inherited recordings browser writes a resume point for the URL.
        self.close()

    def doEofInternal(self, playing):
        if playing and self.execing:
            # The catalog explains an early end instead of silently returning.
            self.close(FAILED if time.time() - self._opened < EARLY_END_S else None)
