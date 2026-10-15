# -*- coding: utf-8 -*-
"""Playback address for the native player, independent of Enigma2."""
from __future__ import absolute_import
try:
    from urllib.parse import urlsplit
except ImportError:
    from urlparse import urlsplit


# Players a playlist channel may use (the channel lists' setting).
SERVICE_TYPES = (4097, 5001, 5002)


def service_type(playback):
    kind = playback.get("serviceType")
    return kind if kind in SERVICE_TYPES else 4097


def playback_url(playback):
    url = playback.get("url", "")
    parsed = urlsplit(url)
    if parsed.scheme.lower() not in ("http", "https") or not parsed.netloc or any(c in url for c in "\r\n"):
        raise ValueError("Invalid playback address")
    # ServiceMP3 parses raw key=value pairs after '#'; it does not URL-decode.
    headers = []
    for key, value in sorted(playback.get("headers", {}).items()):
        if key not in ("User-Agent", "Cookie", "Authorization", "Referer"):
            continue
        if any(c in value for c in "\r\n&#"):
            raise ValueError("Unsupported playback header")
        headers.append(key + "=" + value)
    return url.split("#", 1)[0] + ("#" + "&".join(headers) if headers else "")
