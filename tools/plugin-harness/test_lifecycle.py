# -*- coding: utf-8 -*-
"""Framework lifecycle regression checks; no receiver or network required."""
from __future__ import absolute_import

import os
import sys
import unittest

import run as harness

harness.install_fakes()
sys.path.insert(0, os.path.join(harness.ROOT, "plugin", "Dreaminator"))
import plugin


class ScreenLifecycleTest(unittest.TestCase):
    def test_session_can_show_and_close_screen_without_arguments(self):
        screen = plugin.DreaminatorScreen(session=None)
        screen.show()
        self.assertTrue(screen.shown)
        screen.close()
        self.assertTrue(screen.closed)
        self.assertIsNone(screen.timer.active)


if __name__ == "__main__":
    unittest.main()
