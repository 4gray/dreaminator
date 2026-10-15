# -*- coding: utf-8 -*-
"""Pictures for the TV catalog without blocking the GUI.

Downloads run in Twisted's thread pool, files go to /tmp (RAM), and
Enigma2's ePicLoad decodes and scales them on its own thread, one at a
time. Decoded pixmaps stay in a small byte-budgeted cache; requests for
pictures that scrolled away are dropped before they cost anything.
"""
from __future__ import absolute_import

import os
import tempfile
from collections import OrderedDict

from enigma import ePicLoad, eTimer
from twisted.internet import threads

try:
    from . import ui_api, ui_trace
    from .ui_models import PixmapCache
except (ImportError, ValueError):
    import ui_api
    import ui_trace
    from ui_models import PixmapCache

TMP_DIR = "/tmp"
QUIET_MS = 250


class Request(object):
    __slots__ = ("key", "path", "w", "h", "fit", "callbacks", "started")

    def __init__(self, key, path, w, h, fit="contain"):
        self.key, self.path, self.w, self.h, self.fit = key, path, w, h, fit
        self.callbacks = []
        self.started = 0


def decode_box(fit, w, h, size):
    """Size to ask ePicLoad for. "cover" fills the slot (the widget clips the
    overflow at its right/bottom edge) instead of leaving bars when a poster
    is not exactly 2:3; far-off shapes (a landscape still in a poster slot)
    stay letterboxed rather than show a sliver."""
    iw, ih = size
    if fit != "cover" or iw <= 0 or ih <= 0:
        return w, h
    scale = max(float(w) / iw, float(h) / ih)
    bw, bh = int(iw * scale + 0.999) + 1, int(ih * scale + 0.999) + 1
    if bw > w * 1.2 or bh > h * 1.2:
        return w, h
    return bw, bh


class ImageLoader(object):
    def __init__(self, budget=10 << 20, fetchers=2, background="#ff151a24"):
        self.cache = PixmapCache(budget)
        self.queue = OrderedDict()  # key -> Request, in priority order
        self.fetching = {}
        self.decode_queue = []
        self.decoding = None
        self.fetchers = fetchers
        self.background = background
        self.closed = False
        self.keep = ()
        self.picload = None
        self.conn = None
        self.quiet_until = 0
        self.held = False  # a decoded picture waits for the viewer to pause
        self.watchdog = eTimer()
        self.watchdog.callback.append(self._decode_stuck)
        self.resume_timer = eTimer()
        self.resume_timer.callback.append(self._resume)
        self._new_decoder()

    def busy(self, ms=QUIET_MS):
        """The viewer is pressing keys: keep the CPU for the interface.

        On a DM520 every decode and pixmap conversion competes with the GUI
        (and the stream player) for the CPU; pictures continue once the keys
        go quiet.
        """
        self.quiet_until = ui_trace.now() + ms / 1000.0
        self.resume_timer.start(ms + 20, True)

    def _quiet(self):
        return ui_trace.now() >= self.quiet_until

    def _resume(self):
        if self.closed:
            return
        if not self._quiet():
            self.resume_timer.start(int((self.quiet_until - ui_trace.now()) * 1000) + 20, True)
            return
        if self.held:
            self.held = False
            self._decoded()
        else:
            self._decode_next()

    def _new_decoder(self):
        self._drop_decoder()
        self.picload = ePicLoad()
        try:
            self.picload.PictureData.get().append(self._decoded)
        except AttributeError:  # newer images
            self.conn = self.picload.PictureData.connect(self._decoded)

    def _drop_decoder(self):
        """Disconnect ePicLoad's signal. It holds this loader from C++, where
        Python's garbage collector cannot see it: without this every closed
        screen kept its loader and decoder (measured on a DM520)."""
        if self.picload is None:
            return
        try:
            self.picload.PictureData.get().remove(self._decoded)
        except (AttributeError, ValueError):
            pass
        self.conn = None
        self.picload = None

    def get(self, key):
        return self.cache.get(key)

    def want(self, requests, keep=()):
        """Load these (key, path, w, h, callback) in order; drop the rest.

        Cached pictures are handed back at once and not re-requested.
        """
        if self.closed:
            return
        self.keep = tuple(keep) + tuple(r[0] for r in requests)
        fresh = OrderedDict()
        for request in requests:
            key, path, w, h, callback = request[:5]
            fit = request[5] if len(request) > 5 else "contain"
            pixmap = self.cache.get(key)
            if pixmap is not None:
                callback(key, pixmap)
                continue
            active = self.fetching.get(key) or self._decode_request(key)
            if active is not None:
                active.callbacks = [callback]
                continue
            req = self.queue.get(key) or fresh.get(key) or Request(key, path, w, h, fit)
            req.callbacks = [callback]
            fresh[key] = req
        self.queue = fresh
        self._pump()

    def _decode_request(self, key):
        if self.decoding and self.decoding[0].key == key:
            return self.decoding[0]
        for req, _ in self.decode_queue:
            if req.key == key:
                return req
        return None

    def _pump(self):
        while not self.closed and self.queue and len(self.fetching) < self.fetchers:
            key, req = self.queue.popitem(last=False)
            self.fetching[key] = req
            req.started = ui_trace.now()
            threads.deferToThread(_download, req.path).addCallbacks(
                lambda path, req=req: self._fetched(req, path),
                lambda failure, req=req: self._failed(req))

    def _fetched(self, req, result):
        path, size = result
        self.fetching.pop(req.key, None)
        if self.closed:
            _remove(path)
            return
        ui_trace.log("image.fetch", key=req.key[:12], ms=ui_trace.ms(req.started))
        req.w, req.h = decode_box(req.fit, req.w, req.h, size)
        self.decode_queue.append((req, path))
        self._decode_next()
        self._pump()

    def _failed(self, req):
        self.fetching.pop(req.key, None)
        self._pump()

    def _decode_next(self):
        if self.decoding or self.closed or self.held:
            return
        if not self._quiet():
            return  # _resume picks up after the keys go quiet
        # Pictures that left the screen meanwhile are not worth decoding.
        while self.decode_queue:
            req, path = self.decode_queue.pop(0)
            if req.key not in self.keep:
                _remove(path)
                continue
            self.picload.setPara((req.w, req.h, 1, 1, False, 1, self.background))
            self.decoding = (req, path, ui_trace.now())
            if self.picload.startDecode(path) != 0:
                self.decoding = None
                _remove(path)
                continue
            self.watchdog.start(4000, True)
            return

    def _decoded(self, info=None):
        self.watchdog.stop()
        if not self.decoding:
            return
        if not self._quiet() and not self.closed:
            # getData() converts on the GUI thread; do it when keys are quiet.
            self.held = True
            return
        req, path, started = self.decoding
        self.decoding = None
        _remove(path)
        if self.closed:
            return
        got = ui_trace.now()
        pixmap = self.picload.getData()
        if ui_trace.enabled():
            size = ""
            try:
                sz = pixmap.size()
                size = "%dx%d" % (sz.width(), sz.height())
            except Exception:
                pass
            ui_trace.log("image.decode", key=req.key[:12], ms="%.1f" % ((got - started) * 1000), getdata=ui_trace.ms(got), size=size)
        if pixmap is not None:
            self.cache.put(req.key, pixmap, req.w * req.h * 4, keep=self.keep)
            shown = ui_trace.now()
            for callback in req.callbacks:
                callback(req.key, pixmap)
            ui_trace.log("image.show", ms=ui_trace.ms(shown))
        self._decode_next()

    def _decode_stuck(self):
        # A broken file may never report back; start over with a new decoder.
        if self.decoding:
            _remove(self.decoding[1])
            self.decoding = None
            self._new_decoder()
            self._decode_next()

    def close(self):
        self.closed = True
        for timer, handler in ((self.watchdog, self._decode_stuck), (self.resume_timer, self._resume)):
            timer.stop()
            try:
                timer.callback.remove(handler)
            except ValueError:
                pass
        self._drop_decoder()
        for req, path in self.decode_queue:
            _remove(path)
        self.decode_queue = []
        if self.decoding:
            _remove(self.decoding[1])
            self.decoding = None
        self.queue.clear()
        self.cache.clear()


def _download(path):
    data, size = ui_api.fetch_sized(path, timeout=15)
    fd, name = tempfile.mkstemp(prefix="dreaminator-img-", suffix=".img", dir=TMP_DIR)
    try:
        os.write(fd, data)
    finally:
        os.close(fd)
    return name, size


def _remove(path):
    try:
        os.unlink(path)
    except OSError:
        pass
