# -*- coding: utf-8 -*-
"""Favorites and recents of the TV catalog: requests and words, no widgets.

Favorite channels are the channel list "Избранное" of the service (the same
list the tablet edits); favorite films and series and the recents live in
the service's library. Rows of these lists come from every source and carry
their own source id, persistent id and navigation key.
"""
from __future__ import absolute_import

try:
    from . import ui_api, ui_theme as T
except (ImportError, ValueError):
    import ui_api
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


def favorite_message(item, result):
    """Toast after a change: where the row went and when the TV sees it."""
    added = bool(result.get("fav"))
    if not result.get("channel"):
        if added:
            return "«%s» — в избранном" % name_of(item), "Раздел «Избранное» — вверху панели слева"
        return "«%s» убран из избранного" % name_of(item), ""
    lst = name_of({"name": result.get("list") or "Избранное"}, 20)
    if added:
        text = "«%s» — в списке «%s»" % (name_of(item, 20), lst)
        hint = "В списке каналов ТВ — после закрытия каталога"
    else:
        text = "«%s» убран из списка «%s»" % (name_of(item, 18), lst)
        hint = "Список каналов ТВ обновится после закрытия каталога"
    if result.get("created"):
        hint = "Список «%s» создан заново · на ТВ — после закрытия каталога" % lst
    elif result.get("unsent"):
        # Sending the list also sends what the tablet has not sent yet.
        hint = "При закрытии на ТВ уйдут и неотправленные правки с планшета"
    return text, hint


def missing_message(item):
    return "«%s» нет в плейлисте" % name_of(item), "Провайдер убрал канал · долгое OK — убрать из списка"


def close_catalog(apply_channels):
    """Run off the GUI thread when the catalog closes: the library reaches
    the flash now, and changed favorite channels reach Enigma2's channel
    list (POST /api/apply also sends the tablet's unsent changes)."""
    try:
        ui_api.request_json("/api/catalog/close", {}, 10)
    finally:
        if apply_channels:
            ui_api.request_json("/api/apply", {}, 60)
