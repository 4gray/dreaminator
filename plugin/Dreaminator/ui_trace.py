# -*- coding: utf-8 -*-
"""Optional timing log for measuring the interface on a receiver.

Enabled by creating /tmp/dreaminator-ui-trace; lines go to
/tmp/dreaminator-ui-trace.log (RAM, never the flash). Off by default.
"""
from __future__ import absolute_import

import os
import time

FLAG = "/tmp/dreaminator-ui-trace"
LOG = "/tmp/dreaminator-ui-trace.log"


def enabled():
    return os.path.exists(FLAG)


def rss_kb():
    try:
        with open("/proc/self/status") as f:
            for line in f:
                if line.startswith("VmRSS:"):
                    return int(line.split()[1])
    except (IOError, OSError, ValueError):
        pass
    return 0


def log(event, **values):
    if not enabled():
        return
    parts = ["%.3f" % time.time(), event] + ["%s=%s" % (k, values[k]) for k in sorted(values)]
    try:
        with open(LOG, "a") as f:
            f.write(" ".join(parts) + "\n")
    except (IOError, OSError):
        pass


def now():
    return time.time()


def ms(start):
    return "%.1f" % ((time.time() - start) * 1000)


def live_screens():
    """Trace only: how many catalog screens are still alive (leak check)."""
    if not enabled():
        return {}
    import gc
    gc.collect()
    names = ("Shell", "DetailScreen", "GuideScreen", "ImageLoader")
    counts = dict((n, 0) for n in names)
    for obj in gc.get_objects():
        name = type(obj).__name__
        if name in counts:
            counts[name] += 1
    return dict(("live_" + k, v) for k, v in counts.items())
