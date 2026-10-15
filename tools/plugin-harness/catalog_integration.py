# -*- coding: utf-8 -*-
"""Read catalog through the real simulator API; uses its built-in demo portal.

This creates a named demo source on localhost only, never selects bouquet entries.
"""
from __future__ import print_function
import json
import os
import sys
import time
try:
    from urllib import urlencode
except ImportError:
    from urllib.parse import urlencode
import run as harness
harness.install_fakes()
sys.path.insert(0, os.path.join(harness.ROOT, "plugin", "Dreaminator"))
import plugin
import ui_api

plugin.BASE = ui_api.BASE = harness.BASE
if plugin.BASE not in ("http://127.0.0.1:8088", "http://localhost:8088"):
    raise SystemExit("Catalog integration runs only against the local simulator")

api = ui_api.request_json
sources = api("/api/catalog/sources")
source = next((s for s in sources if s["name"] == "Catalog harness"), None)
if source is None:
    source = api("/api/sources", {"type":"stalker", "name":"Catalog harness", "server":"http://127.0.0.1/stalker_portal/c/", "mac":"00:1A:79:00:12:34"})
for _ in range(30):
    state = api("/api/state")
    saved = next(s for s in state["sources"] if s["id"] == source["id"])
    if not saved["refreshing"]:
        break
    time.sleep(0.2)
base = "/api/catalog/" + source["id"]

def items(section, parent="", page=1):
    return api(base + "/items?" + urlencode({"type":section,"category":"1","parent":parent,"page":page}))

def play(item):
    result = api(base + "/play", {"key":item["key"]})
    assert result["serviceType"] == 4097
    assert result["headers"]["User-Agent"]
    assert result["url"].startswith("https://download.blender.org/")
    return result

before = [(b["id"],len(b["channels"])) for b in state["bouquets"]]
movies = items("vod")["items"]
assert len(movies) == 3
film = movies[0]
assert "cmd" not in film and "password" not in film
# List rows are lean cards; the card screen asks for details.
assert "description" not in film, film
details = api(base + "/details?" + urlencode({"key": film["key"]}))
assert details["description"] and details["type"] == "vod" and "cmd" not in details
# Search runs on the portal and its results play like any other entry.
found = api(base + "/search?" + urlencode({"type": "vod", "q": "Тихая"}))
assert found["scope"] == "server" and found["items"], found
assert all("Тихая" in i["name"] for i in found["items"])
play(found["items"][0])
nothing = api(base + "/search?" + urlencode({"type": "vod", "q": "нет такого фильма"}))
assert nothing["scope"] == "server" and nothing["items"] == []
demo = api(base + "/items?" + urlencode({"type": "vod", "category": "2", "page": 1}))
assert demo["total"] >= 60 and demo["items"][0]["hasPoster"] and demo["items"][0]["rating"]["source"] == "imdb"
art = plugin.api(base + "/image?" + urlencode({"key": demo["items"][0]["key"], "kind": "poster"}))
assert art.startswith(b"\xff\xd8"), "JPEG artwork passes through unchanged"
first = play(film)
assert play(film)["url"] != first["url"]
poster = plugin.api(base + "/poster?" + urlencode({"key":film["key"]}))
assert poster.startswith(b"\x89PNG")
for section, root in [("vod",movies[1]), ("vod",movies[2]), ("series",items("series")["items"][0])]:
    seasons = items(section,root["key"])["items"]
    assert seasons and seasons[-1]["kind"] == "season"
    episodes = items(section,seasons[-1]["key"])["items"]
    assert len(episodes)>=2 and episodes[1]["kind"]=="episode"
    play(episodes[1])

# Programme: channels outside the lists get the portal's short guide on
# request; rows carry "now" once it is known, without descriptions.
tz = ui_api.tz_minutes()
live = api(base + "/items?" + urlencode({"type": "itv", "category": "", "page": 1}))["items"]
mir = next(i for i in live if i["name"] == "Мир 24")
guide = api(base + "/epg?" + urlencode({"key": mir["key"], "tz": tz}))
assert guide["origin"] in ("provider", "lists") and guide["now"]["title"].startswith("Мир 24: передача") and guide["now"]["desc"], guide
assert guide["next"]["start"] == guide["now"]["stop"] and "programmes" not in guide, guide
today = api(base + "/epg?" + urlencode({"key": mir["key"], "day": 0, "tz": tz}))
assert today["programmes"] and all("desc" not in p for p in today["programmes"]) and 0 in today["days"], today
rows = api(base + "/items?" + urlencode({"type": "itv", "category": "", "page": 1}))["items"]
row = next(i for i in rows if i["name"] == "Мир 24")
assert row["now"]["title"] == guide["now"]["title"] and "desc" not in row["now"], row
now = api(base + "/now?" + urlencode({"keys": ",".join(i["key"] for i in rows)}))["now"]
assert now[mir["key"]]["title"] == guide["now"]["title"], now
silent = next(i for i in live if i["name"] == "Канал без сигнала")
nothing = api(base + "/epg?" + urlencode({"key": silent["key"], "day": 0, "tz": tz}))
assert "now" not in nothing and nothing["days"] == [] and not nothing.get("programmes"), nothing

# An uploaded M3U playlist browses as Live TV only and plays its own addresses.
M3U = u"""#EXTM3U
#EXTINF:-1 group-title="Новости",Harness News
http://stream.invalid/news.ts
#EXTINF:-1 group-title="Детские",Harness Kids
http://stream.invalid/kids.ts
"""

def upload_playlist():
    try:
        from urllib2 import Request, urlopen
    except ImportError:
        from urllib.request import Request, urlopen
    boundary = "harness-boundary"
    body = ("--%s\r\nContent-Disposition: form-data; name=\"name\"\r\n\r\nPlaylist harness\r\n"
            "--%s\r\nContent-Disposition: form-data; name=\"file\"; filename=\"harness.m3u\"\r\n"
            "Content-Type: audio/x-mpegurl\r\n\r\n" % (boundary, boundary)).encode("utf-8")
    body += M3U.encode("utf-8") + ("\r\n--%s--\r\n" % boundary).encode("utf-8")
    req = Request(plugin.BASE + "/api/sources/upload", body, {"Content-Type": "multipart/form-data; boundary=" + boundary})
    return json.loads(urlopen(req, timeout=30).read().decode("utf-8"))

playlist = next((s for s in api("/api/catalog/sources") if s["name"] == "Playlist harness"), None) or upload_playlist()
listed = next(s for s in api("/api/catalog/sources") if s["id"] == playlist["id"])
assert listed["type"] == "file" and listed["sections"] == ["itv"], listed
pbase = "/api/catalog/" + playlist["id"]
cats = api(pbase + "/categories?type=itv")
assert [ui_api.unicode_text(c["name"]) for c in cats] == [u"Все каналы", u"Детские", u"Новости"], cats
try:
    api(pbase + "/categories?type=vod")
    kind = None
except Exception as e:
    kind = ui_api.problem(harness.Failure(e)).kind
assert kind == "unsupported", "a playlist has no films: %r" % kind
news = api(pbase + "/items?" + urlencode({"type": "itv", "category": cats[2]["id"], "page": 1}))["items"]
assert [i["name"] for i in news] == ["Harness News"], news
played = api(pbase + "/play", {"key": news[0]["key"]})
assert played["url"] == "http://stream.invalid/news.ts" and played["serviceType"] in (4097, 5001, 5002) and not played["headers"], played
found = api(pbase + "/search?" + urlencode({"type": "itv", "q": "kids"}))
assert found["scope"] == "channels" and [i["name"] for i in found["items"]] == ["Harness Kids"], found
# A playlist without a guide has no programme, and says so.
none = api(pbase + "/epg?" + urlencode({"key": news[0]["key"], "tz": tz}))
assert "now" not in none and none["days"] == [] and "now" not in news[0], none

after = [(b["id"],len(b["channels"])) for b in api("/api/state")["bouquets"]]
assert before == after, "catalog mutated bouquet selections"

# Favorites and recents (v0.10). Rows carry a persistent id; a favorite film
# plays by it after its list was refreshed, and from the library list.
film_id = source["id"] + "/vod/film"
assert film["id"] == film_id, film
fav = api("/api/catalog/favorites", {"source": source["id"], "key": film["key"], "id": film_id})
assert fav["fav"] and not fav["channel"], fav
refreshed = api(base + "/items?" + urlencode({"type": "vod", "category": "1", "page": 1, "refresh": 1}))["items"]
assert any(r["id"] == film_id and r["fav"] for r in refreshed), refreshed

def home_rows(lst, category):
    return api("/api/catalog/home/items?" + urlencode({"list": lst, "category": category, "page": 1}))["items"]

row = next(r for r in home_rows("favorites", "video") if r["id"] == film_id)
assert row["source"] == source["id"] and row["type"] == "vod" and row["hasPoster"], row
by_id = api(base + "/play", {"key": row["key"], "id": row["id"]})
assert by_id["url"].startswith("https://download.blender.org/"), by_id
assert home_rows("recent", "video")[0]["id"] == film_id, "the started film is the newest recent"
# A favorite channel goes to the channel list "Избранное", like on the tablet.
channel = api("/api/catalog/favorites", {"id": news[0]["id"]})
assert channel["fav"] and channel["channel"] and channel["list"], channel
lists = [b for b in api("/api/state")["bouquets"] if b["name"] == channel["list"]]
assert lists and any(c["source"] == playlist["id"] for c in lists[0]["channels"]), lists
assert any(r["id"] == news[0]["id"] for r in home_rows("favorites", "itv"))
home = api("/api/catalog/home")
assert [c["id"] for c in home["favorites"]] == ["itv", "video"] and home["favorites"][1]["count"] >= 1, home
for entry in (film_id, news[0]["id"]):
    gone = ui_api.request_json("/api/catalog/favorites?" + urlencode({"id": entry}), None, 35, True, "DELETE")
    assert not gone["fav"], gone
api("/api/catalog/close", {})

# Archive (v0.11): the simulator's demo playlist has tvg-rec="3" (Edem) and
# catchup="shift" catchup-days="1" (VipLime); its guide holds yesterday and
# today only, the nightly pass keeps the past.
arch = next((s for s in api("/api/catalog/sources") if s["name"] == "Archive harness"), None)
if arch is None:
    arch = api("/api/sources", {"type": "m3u", "name": "Archive harness", "url": "http://127.0.0.1/archive/playlist.m3u"})
for _ in range(100):
    if not next(s for s in api("/api/state")["sources"] if s["id"] == arch["id"])["refreshing"]:
        break
    time.sleep(0.2)
built = api("/api/state")["epg"].get("updatedAt")
api("/api/epg/refresh", {})
for _ in range(600):
    epg = api("/api/state")["epg"]
    if epg.get("updatedAt") != built and not epg.get("running"):
        break
    time.sleep(0.2)
else:
    raise AssertionError("the guide was not rebuilt")
abase = "/api/catalog/" + arch["id"]
rows = dict((r["name"], r) for r in api(abase + "/items?" + urlencode({"type": "itv", "category": "", "page": 1}))["items"])
edem, vip = rows["Архив Edem"], rows["Архив VipLime"]
assert edem["archive"] == 3 and vip["archive"] == 1 and "archive" not in rows["Без архива"], rows
yesterday = api(abase + "/epg?" + urlencode({"key": edem["key"], "day": -1, "tz": tz}))
assert yesterday["archive"] == 3 and -1 in yesterday["days"] and yesterday["programmes"], yesterday
last = yesterday["programmes"][-1]
assert last["rec"] and "desc" not in last, last
rec = api(abase + "/play", {"key": edem["key"], "id": edem["id"], "start": last["start"]})
assert rec["url"].startswith("http://127.0.0.1/archive/edem/index.m3u8?utc=%d&lutc=" % last["start"]), rec
assert int(rec["url"].rsplit("=", 1)[1]) >= time.time() - 60, "lutc is the moment of the request: %s" % rec["url"]
assert rec["archive"]["title"] == last["title"] and rec["name"] == "Архив Edem", rec
today = api(abase + "/epg?" + urlencode({"key": vip["key"], "tz": tz}))
assert today["now"]["rec"] and not today["next"].get("rec"), today
start_over = api(abase + "/play", {"key": vip["key"], "start": today["now"]["start"]})
assert start_over["url"].startswith("http://127.0.0.1/archive/vip/index.m3u8?utc=%d&lutc=" % today["now"]["start"]), start_over
try:
    api(abase + "/play", {"key": vip["key"], "start": today["next"]["start"]})
    kind = None
except Exception as e:
    kind = ui_api.problem(harness.Failure(e)).kind
assert kind == "archive", "the future is not in the archive: %r" % kind
assert api("/api/debug")["guide"]["archiveChannels"] >= 2
print("Catalog API: film, details, search, three series modes, fresh links, artwork, programme, playlist Live TV, "
      "bouquet separation, favorites and recents, archive OK (Python %d.%d)" % sys.version_info[:2])
