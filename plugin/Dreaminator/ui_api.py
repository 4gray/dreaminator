# -*- coding: utf-8 -*-
"""Requests to the local Dreaminator service, run off the GUI thread."""
from __future__ import absolute_import

import calendar
import json
import os
import sys
import time

try:  # Python 2
    from urllib import quote, urlencode
    from urllib2 import HTTPError, Request, URLError, urlopen
except ImportError:  # Python 3
    from urllib.error import HTTPError, URLError
    from urllib.parse import quote, urlencode
    from urllib.request import Request, urlopen

PY2 = sys.version_info[0] == 2
BASE = "http://127.0.0.1:8088"


def native(value):
    """Enigma2 on Python 2 wants UTF-8 byte strings; JSON gives unicode."""
    if PY2:
        if isinstance(value, dict):
            return dict((native(k), native(v)) for k, v in value.items())
        if isinstance(value, list):
            return [native(v) for v in value]
        if isinstance(value, unicode):  # noqa: F821 (Python 2 only)
            return value.encode("utf-8")
    return value


def text(value):
    """Label text for this Python: UTF-8 bytes on 2, str on 3."""
    if value is None:
        return ""
    if PY2 and isinstance(value, unicode):  # noqa: F821
        return value.encode("utf-8")
    if not PY2 and isinstance(value, bytes):
        return value.decode("utf-8", "replace")
    return value


def unicode_text(value):
    if isinstance(value, bytes):
        return value.decode("utf-8", "replace")
    return value


def request_json(path, body=None, timeout=35, convert=True, method=None):
    """GET (or POST body) a JSON answer; method overrides the verb (DELETE).

    convert=False keeps Python 2 unicode as is: converting every string of
    a catalog page costs the GUI thread more than the few labels it shows.
    """
    data = None if body is None else json.dumps(body).encode("utf-8")
    req = Request(BASE + path, data=data)
    if method:
        req.get_method = lambda: method  # Python 2's Request has no method argument
    if data is not None:
        req.add_header("Content-Type", "application/json")
    started = time.time()
    resp = urlopen(req, timeout=timeout)
    try:
        raw = resp.read()
    finally:
        resp.close()
    got = time.time()
    value = json.loads(raw.decode("utf-8"))
    if convert:
        value = native(value)
    if os.path.exists("/tmp/dreaminator-ui-trace"):
        try:
            with open("/tmp/dreaminator-ui-trace.log", "a") as f:
                f.write("%.3f api path=%s bytes=%d net=%.1f parse=%.1f\n" % (
                    time.time(), path.split("?")[0][-20:], len(raw), (got - started) * 1000, (time.time() - got) * 1000))
        except (IOError, OSError):
            pass
    return value


def fetch(path, timeout=15):
    return fetch_sized(path, timeout)[0]


def fetch_sized(path, timeout=15):
    """Picture bytes and their size from the service's X-Image-* headers."""
    resp = urlopen(Request(BASE + path), timeout=timeout)
    try:
        info = resp.info()
        try:
            size = (int(info.get("X-Image-Width") or 0), int(info.get("X-Image-Height") or 0))
        except (TypeError, ValueError):
            size = (0, 0)
        return resp.read(), size
    finally:
        resp.close()


def tz_minutes(t=None):
    """The receiver's offset from UTC now, in minutes: the service counts
    the programme's days in the TV's zone, whatever zone data Go has."""
    t = time.time() if t is None else t
    return int(round((calendar.timegm(time.localtime(t)) - int(t)) / 60.0))


def query_path(path, **params):
    """path?params, without the ones that are None."""
    query = {}
    for k, v in params.items():
        if v is None:
            continue
        if PY2 and isinstance(v, unicode):  # noqa: F821
            v = v.encode("utf-8")
        query[k] = v
    return path + "?" + urlencode(sorted(query.items())) if query else path


def catalog_path(source, operation, **params):
    if PY2 and isinstance(source, unicode):  # noqa: F821
        source = source.encode("utf-8")
    return query_path("/api/catalog/%s/%s" % (quote(source, safe=""), operation), **params)


class Problem(object):
    """What went wrong, in words for the viewer, without URLs or secrets."""

    def __init__(self, kind, message):
        self.kind = kind
        self.message = message


def problem(failure):
    err = getattr(failure, "value", failure)
    if isinstance(err, HTTPError):
        message, kind = "", ""
        try:
            body = json.loads(err.read().decode("utf-8"))
            message, kind = body.get("error", ""), body.get("code", "")
        except Exception:
            pass
        if err.code == 410:
            kind = "expired"
        elif err.code == 404:
            kind = kind or "missing"
        elif err.code == 400:
            kind = kind or "input"
        else:
            kind = kind or "provider"
        return Problem(kind, text(native(message)) or "Ошибка %d" % err.code)
    if isinstance(err, (URLError, IOError, OSError)):
        return Problem("service", "Сервис Dreaminator не отвечает")
    # Never show exception text: it may carry addresses with tokens.
    return Problem("internal", "Не удалось обработать ответ сервиса")
