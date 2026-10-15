# -*- coding: utf-8 -*-
"""Focus and paging rules of the TV catalog (no Enigma2 needed)."""
from __future__ import absolute_import

import os
import sys
import unittest

ROOT = os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
sys.path.insert(0, os.path.join(ROOT, "plugin", "Dreaminator"))
import ui_models as m  # noqa: E402


def page(n, size, more, total=0):
    return {"items": [{"key": "%d-%d" % (n, i)} for i in range(size)], "hasMore": more, "pageSize": 14, "total": total}


class PagedListTest(unittest.TestCase):
    def test_unknown_length_grows_page_by_page(self):
        p = m.PagedList()
        self.assertEqual(p.missing(0, 10), [1])
        p.loading.add(1)
        self.assertEqual(p.missing(0, 10), [])
        p.store(1, page(1, 14, True))
        self.assertEqual(p.count(), 14)
        self.assertEqual(p.missing(0, 10), [])
        # The window reaches the next page: only page 2, nothing further.
        self.assertEqual(p.missing(10, 30), [2])
        p.store(2, page(2, 5, False))
        self.assertEqual(p.count(), 19)
        self.assertTrue(p.complete())
        self.assertEqual(p.get(18)["key"], "2-4")
        self.assertIsNone(p.get(19))
        self.assertEqual(p.missing(0, 40), [])

    def test_known_total_allows_jumps_and_stops_at_end(self):
        p = m.PagedList()
        p.store(1, page(1, 14, True, total=16644))
        self.assertEqual(p.count(), 16644)
        self.assertEqual(p.missing(140, 150), [11])
        self.assertEqual(p.missing(16640, 16660), [1189])

    def test_repeated_page_ends_the_list(self):
        p = m.PagedList()
        p.store(1, page(1, 14, True))
        p.store(2, {"items": [], "hasMore": False, "pageSize": 14})
        self.assertEqual(p.count(), 14)
        self.assertEqual(p.missing(0, 40), [])

    def test_failure_is_remembered_until_next_success(self):
        p = m.PagedList()
        p.loading.add(1)
        p.fail(1, "network")
        self.assertEqual(p.missing(0, 10), [])
        self.assertEqual(p.failed, "network")


class GridModelTest(unittest.TestCase):
    def test_arrows_move_focus_and_never_turn_pages(self):
        g = m.GridModel(5, 2)
        self.assertEqual(g.move(-1, 0, 30), m.EDGE_LEFT)
        self.assertEqual(g.focus, 0)
        for _ in range(4):
            self.assertEqual(g.move(1, 0, 30), m.MOVED)
        # Right at the last column stays put instead of flipping the page.
        self.assertEqual(g.move(1, 0, 30), m.EDGE_RIGHT)
        self.assertEqual((g.focus, g.top), (4, 0))

    def test_down_scrolls_one_row_and_up_scrolls_back(self):
        g = m.GridModel(5, 2)
        g.move(0, 1, 30)
        self.assertEqual((g.focus, g.top), (5, 0))
        g.move(0, 1, 30)
        self.assertEqual((g.focus, g.top), (10, 1))
        self.assertEqual(g.slot(), 5)
        g.move(0, -1, 30)
        g.move(0, -1, 30)
        self.assertEqual((g.focus, g.top), (0, 0))
        self.assertEqual(g.move(0, -1, 30), m.EDGE_UP)

    def test_down_into_short_last_row_lands_on_last_item(self):
        g = m.GridModel(5, 2)
        g.focus = 4
        self.assertEqual(g.move(0, 1, 7), m.MOVED)
        self.assertEqual(g.focus, 6)
        self.assertEqual(g.move(0, 1, 7), m.EDGE_DOWN)

    def test_page_keys_turn_a_screen_and_keep_the_slot(self):
        g = m.GridModel(5, 2)
        g.focus = 7
        self.assertEqual(g.page(1, 100), m.MOVED)
        self.assertEqual((g.focus, g.top), (17, 2))
        g.page(-1, 100)
        self.assertEqual((g.focus, g.top), (7, 0))
        self.assertEqual(g.page(-1, 100), m.MOVED)
        self.assertEqual(g.focus, 0)
        self.assertEqual(g.page(-1, 100), m.STAY)
        # Near the end the window stops at the last full screen.
        g.focus, g.top = 80, 16
        count = 23 * 5 - 2
        g.page(1, count)
        self.assertEqual((g.focus, g.top), (90, 18))
        g.page(1, count)
        g.page(1, count)
        self.assertEqual(g.top, 21)
        g.page(1, count)
        self.assertEqual(g.focus, count - 1)
        self.assertEqual(g.page(1, count), m.STAY)

    def test_state_restores_position(self):
        g = m.GridModel(5, 2)
        g.page(1, 100)
        saved = g.state()
        g.focus, g.top = 0, 0
        g.restore(saved)
        self.assertEqual(g.state(), saved)
        g.clamp(3)
        self.assertEqual((g.focus, g.top), (2, 0))


class ListModelTest(unittest.TestCase):
    def test_list_scrolls_window(self):
        l = m.ListModel(11)
        for _ in range(12):
            l.move(0, 1, 40)
        self.assertEqual((l.focus, l.top), (12, 2))
        self.assertEqual(l.move(-1, 0, 40), m.EDGE_LEFT)
        self.assertEqual(l.move(1, 0, 40), m.EDGE_RIGHT)


class PixmapCacheTest(unittest.TestCase):
    def test_budget_evicts_oldest_but_keeps_visible(self):
        c = m.PixmapCache(300)
        c.put("a", "A", 100)
        c.put("b", "B", 100)
        c.put("c", "C", 100)
        c.get("a")
        c.put("d", "D", 100, keep=("b",))
        self.assertIsNone(c.get("c"))
        self.assertEqual(c.get("a"), "A")
        self.assertEqual(c.get("b"), "B")
        self.assertEqual(c.used, 300)


class KeyboardTest(unittest.TestCase):
    def test_typing_and_special_keys(self):
        k = m.Keyboard()
        k.focus = m.LAYOUTS["ru"].index(u"з")
        self.assertTrue(k.press())
        self.assertEqual(k.text, u"з")
        # Down from the last letter row reaches the wide keys.
        for _ in range(10):
            if k.move(0, 1) != m.MOVED:
                break
        self.assertEqual(k.current()[0], "space")
        k.move(1, 0)
        self.assertEqual(k.current()[0], "erase")
        self.assertTrue(k.press())
        self.assertEqual(k.text, u"")
        self.assertFalse(k.press())
        k.move(1, 0)
        k.press()
        self.assertEqual(k.layout, "en")
        self.assertEqual(k.current()[0], "layout")
        self.assertEqual(k.move(1, 0), m.EDGE_RIGHT)
        self.assertEqual(k.move(0, -1), m.MOVED)
        self.assertEqual(k.current()[0], "char")

    def test_edges_hand_focus_to_neighbours(self):
        k = m.Keyboard()
        self.assertEqual(k.move(-1, 0), m.EDGE_LEFT)
        self.assertEqual(k.move(0, -1), m.EDGE_UP)
        k.focus = 6
        self.assertEqual(k.move(1, 0), m.EDGE_RIGHT)

    def test_text_is_bounded(self):
        k = m.Keyboard()
        for _ in range(60):
            k.type_digit(u"1")
        self.assertEqual(len(k.text), 40)


if __name__ == "__main__":
    unittest.main()
