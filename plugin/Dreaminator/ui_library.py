# -*- coding: utf-8 -*-
"""Favorites and recents of the TV catalog: requests and words, no widgets.

Favorite channels are the channel list "Избранное" of the service (the same
list the tablet edits); favorite films and series and the recents live in
the service's library. Rows of these lists come from every source and carry
their own source id, persistent id and navigation key.
"""
from __future__ import absolute_import

try:
    from . import ui_api, ui_i18n, ui_theme as T
except (ImportError, ValueError):
    import ui_api
    import ui_i18n
    import ui_theme as T

SECTIONS = ("favorites", "recent")
# Enigma2 reads keymap.xml once per run. Until a press arrives as "okDown",
# the loaded keymap may be an older one (OK on the press, no "okDown"): then
# a release must not be taken for a press that went to another screen.
OK_DOWN_SEEN = [False]
TITLES = {"favorites": "Избранное", "recent": "Недавние"}
HOME = "/api/catalog/home"
FAVORITES = "/api/catalog/favorites"
EMPTY = {
    "favorites": ("В избранном пока ничего нет", "Долгое OK на канале, фильме или сериале добавляет его сюда"),
    "recent": ("Вы ещё ничего не смотрели из каталога", "Здесь появятся 30 последних каналов, фильмов и сериалов"),
}


def categories(home, section):
    """Non-empty categories of one list ("Каналы", "Фильмы и сериалы")."""
    return [c for c in (home or {}).get(section) or [] if c.get("count")]


def items_path(section, category, page):
    return ui_api.catalog_path("home", "items", list=section, category=category, page=page)


def entry(item):
    """How the service finds a row: the persistent id (it survives list
    refreshes) and the navigation key as the fallback."""
    out = {"key": item.get("key")}
    if item.get("id"):
        out["id"] = item["id"]
    return out


def favorite_request(source, item):
    """(method, path, body) that flips the favorite state of a row."""
    body = dict(entry(item), source=source)
    return ("DELETE" if item.get("fav") else "POST"), FAVORITES, body


def name_of(item, limit=24):
    return ui_api.text(T.clip(ui_api.unicode_text(item.get("name") or ""), limit))


def empty_message(section):
    head, body = EMPTY[section]
    return ui_i18n.tr(head), ui_i18n.tr(body)


def favorite_message(item, result):
    """Toast after a change: where the row went and when the TV sees it."""
    return ui_i18n.favorite_message(item, result, name_of)


def missing_message(item):
    return ui_i18n.missing_message(item, name_of)


def close_catalog(apply_channels):
    """Run off the GUI thread when the catalog closes: the library reaches
    the flash now, and changed favorite channels reach Enigma2's channel
    list (POST /api/apply also sends the tablet's unsent changes)."""
    try:
        ui_api.request_json("/api/catalog/close", {}, 10)
    finally:
        if apply_channels:
            ui_api.request_json("/api/apply", {}, 60)
