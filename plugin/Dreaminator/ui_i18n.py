# -*- coding: utf-8 -*-
"""Multi-language support for the Enigma2 TV interface (Python 2.7 & 3)."""
from __future__ import absolute_import

import datetime
import json
import os
import re

try:
    from . import ui_api
except (ImportError, ValueError):
    import ui_api

SUPPORTED = ("ru", "pt", "fr", "he", "ar", "it", "de", "es", "pl", "en", "ja", "zh", "th", "tr", "cs")
STATE_PATH = "/etc/dreaminator/state.json"

_OVERRIDE_LANG = None
_SUMMARY_LANG = None
_STATE_CACHE = (None, None)  # (mtime, lang)


def normalize_lang(code):
    if not code:
        return ""
    s = ui_api.unicode_text(code).strip().lower().replace("_", "-")
    if not s or s == "auto":
        return ""
    primary = s.split("-")[0]
    if primary == "iw":
        return "he"
    if primary == "cz":
        return "cs"
    if primary == "jp":
        return "ja"
    return primary if primary in SUPPORTED else ""


def set_language(code):
    """Explicit override (mainly for tests); pass None/'' to clear."""
    global _OVERRIDE_LANG
    if not code or code == "auto":
        _OVERRIDE_LANG = None
    else:
        _OVERRIDE_LANG = normalize_lang(code) or None


def set_summary_language(code):
    global _SUMMARY_LANG
    _SUMMARY_LANG = normalize_lang(code) or None


def _state_file_language():
    global _STATE_CACHE
    try:
        st = os.stat(STATE_PATH)
        mtime = st.st_mtime
    except OSError:
        _STATE_CACHE = (None, None)
        return ""
    if _STATE_CACHE[0] == mtime:
        return _STATE_CACHE[1] or ""
    lang = ""
    try:
        with open(STATE_PATH, "rb") as f:
            data = json.loads(f.read().decode("utf-8"))
        lang = normalize_lang(((data or {}).get("settings") or {}).get("language"))
    except Exception:
        lang = ""
    _STATE_CACHE = (mtime, lang)
    return lang


def _enigma_osd_language():
    try:
        from Components.Language import language
        code = normalize_lang(language.getLanguage())
        if code:
            return code
    except Exception:
        pass
    try:
        from Components.config import config
        code = normalize_lang(config.osd.language.value)
        if code:
            return code
    except Exception:
        pass
    return ""


def get_language():
    if _OVERRIDE_LANG:
        return _OVERRIDE_LANG
    env = normalize_lang(os.environ.get("DREAMINATOR_LANG", ""))
    if env:
        return env
    file_lang = _state_file_language()
    if file_lang:
        return file_lang
    if _SUMMARY_LANG:
        return _SUMMARY_LANG
    osd = _enigma_osd_language()
    if osd:
        return osd
    return "ru"


# ---------------------------------------------------------------- translations
# Keyed by the original Russian string (unicode).
# Every entry translates into: pt, fr, he, ar, it, de, es, pl, en, ja, zh, th, tr, cs.

TRANSLATIONS = {
    u"ошибка %s": {
        "en": u"error %s", "fr": u"erreur %s", "pt": u"erro %s", "es": u"error %s",
        "it": u"errore %s", "de": u"Fehler %s", "pl": u"błąd %s", "cs": u"chyba %s",
        "tr": u"hata %s", "he": u"שגיאה %s", "ar": u"خطأ %s", "ja": u"エラー %s",
        "zh": u"错误 %s", "th": u"ข้อผิดพลาด %s",
    },
    u"Обновляю плейлисты…": {
        "en": u"Updating playlists…", "fr": u"Mise à jour des playlists…", "pt": u"A atualizar playlists…",
        "es": u"Actualizando listas…", "it": u"Aggiornamento playlist…", "de": u"Playlists werden aktualisiert…",
        "pl": u"Aktualizuję playlisty…", "cs": u"Aktualizuji playlisty…", "tr": u"Listeler güncelleniyor…",
        "he": u"מעדכן פלייליסטים…", "ar": u"جارٍ تحديث القوائم…", "ja": u"プレイリストを更新中…",
        "zh": u"正在更新播放列表…", "th": u"กำลังอัปเดตเพลย์ลิสต์…",
    },
    u"Плейлистов пока нет — добавьте их с планшета.": {
        "en": u"No playlists yet — add them from your tablet or browser.",
        "fr": u"Aucune playlist — ajoutez-en depuis votre tablette ou navigateur.",
        "pt": u"Sem playlists — adicione-as no tablet ou navegador.",
        "es": u"Aún no hay listas — añádelas desde la tablet o navegador.",
        "it": u"Nessuna playlist — aggiungile dal tablet o browser.",
        "de": u"Noch keine Playlists — fügen Sie sie über den Browser hinzu.",
        "pl": u"Brak playlist — dodaj je z tabletu lub przeglądarki.",
        "cs": u"Zatím žádné playlisty — přidejte je z tabletu nebo prohlížeče.",
        "tr": u"Henüz liste yok — tablet veya tarayıcıdan ekleyin.",
        "he": u"אין פלייליסטים עדיין — הוסף מהטאבלט או הדפדפן.",
        "ar": u"لا توجد قوائم بعد — أضفها من الجهاز اللوحي أو المتصفح.",
        "ja": u"プレイリストがありません — タブレットやブラウザから追加してください。",
        "zh": u"暂无播放列表 — 请通过平板或浏览器添加。",
        "th": u"ยังไม่มีเพลย์ลิสต์ — เพิ่มจากแท็บเล็ตหรือเบราว์เซอร์",
    },
    u"ещё не загружались": {
        "en": u"not loaded yet", "fr": u"pas encore chargées", "pt": u"ainda não carregadas",
        "es": u"aún no cargadas", "it": u"non ancora caricate", "de": u"noch nicht geladen",
        "pl": u"jeszcze nie pobrane", "cs": u"ještě nenačteno", "tr": u"henüz yüklenmedi",
        "he": u"טרם נטענו", "ar": u"لم تُحمّل بعد", "ja": u"未読み込み",
        "zh": u"尚未加载", "th": u"ยังไม่ได้โหลด",
    },
    u"Плейлистов: %d, обновлены %s": {
        "en": u"Playlists: %d, updated %s", "fr": u"Playlists : %d, mises à jour %s", "pt": u"Playlists: %d, atualizadas %s",
        "es": u"Listas: %d, actualizadas %s", "it": u"Playlist: %d, aggiornate %s", "de": u"Playlists: %d, aktualisiert %s",
        "pl": u"Playlisty: %d, zaktualizowano %s", "cs": u"Playlisty: %d, aktualizováno %s", "tr": u"Liste: %d, güncelleme %s",
        "he": u"פלייליסטים: %d, עודכנו %s", "ar": u"القوائم: %d، حُدثت %s", "ja": u"プレイリスト: %d、更新 %s",
        "zh": u"播放列表：%d，更新于 %s", "th": u"เพลย์ลิสต์: %d, อัปเดต %s",
    },
    u"Каналов в ваших списках: %d": {
        "en": u"Channels in your lists: %d", "fr": u"Chaînes dans vos listes : %d", "pt": u"Canais nas suas listas: %d",
        "es": u"Canales en tus listas: %d", "it": u"Canali nelle tue liste: %d", "de": u"Kanäle in Ihren Listen: %d",
        "pl": u"Kanały na listach: %d", "cs": u"Kanály ve vašich seznamech: %d", "tr": u"Listelerinizdeki kanallar: %d",
        "he": u"ערוצים ברשימות שלך: %d", "ar": u"القنوات في قوائمك: %d", "ja": u"リスト内のチャンネル数: %d",
        "zh": u"列表中的频道：%d", "th": u"ช่องในรายการของคุณ: %d",
    },
    u"Подписка: %s": {
        "en": u"Subscription: %s", "fr": u"Abonnement : %s", "pt": u"Subscrição: %s", "es": u"Suscripción: %s",
        "it": u"Abbonamento: %s", "de": u"Abo: %s", "pl": u"Subskrypcja: %s", "cs": u"Předplatné: %s",
        "tr": u"Abonelik: %s", "he": u"מנוי: %s", "ar": u"الاشتراك: %s", "ja": u"契約: %s",
        "zh": u"订阅：%s", "th": u"สมาชิก: %s",
    },
    u"Есть изменения, не отправленные на ТВ — зелёная кнопка.": {
        "en": u"Unsent changes — press the Green button to send to TV.",
        "fr": u"Modifications non envoyées — bouton Vert pour envoyer à la TV.",
        "pt": u"Alterações por enviar — botão Verde para enviar à TV.",
        "es": u"Cambios sin enviar — pulsa el botón Verde.",
        "it": u"Modifiche non inviate — tasto Verde per inviare alla TV.",
        "de": u"Ungesendete Änderungen — Grüne Taste drücken.",
        "pl": u"Niewysłane zmiany — naciśnij Zielony przycisk.",
        "cs": u"Neodeslané změny — stiskněte Zelené tlačítko.",
        "tr": u"Gönderilmemiş değişiklikler var — Yeşil düğmeye basın.",
        "he": u"יש שינויים שלא נשלחו לטלוויזיה — כפתור ירוק.",
        "ar": u"توجد تغييرات غير مرسلة — اضغط الزر الأخضر.",
        "ja": u"未送信の変更があります — 緑ボタンでTVへ送信。",
        "zh": u"有未发送到电视的更改 — 请按绿色键。",
        "th": u"มีการเปลี่ยนแปลงที่ยังไม่ส่ง — กดปุ่มสีเขียว",
    },
    u"Списки отправлены на ТВ %s": {
        "en": u"Lists sent to TV %s", "fr": u"Listes envoyées à la TV %s", "pt": u"Listas enviadas para a TV %s",
        "es": u"Listas enviadas a la TV %s", "it": u"Liste inviate alla TV %s", "de": u"Listen an TV gesendet %s",
        "pl": u"Listy wysłane na TV %s", "cs": u"Seznamy odeslány do TV %s", "tr": u"Listeler TV'ye gönderildi %s",
        "he": u"הרשימות נשלחו לטלוויזיה %s", "ar": u"أُرسلت القوائم للتلفاز %s", "ja": u"TVへ送信済み %s",
        "zh": u"列表已发送到电视 %s", "th": u"ส่งรายการไปยังทีวีแล้ว %s",
    },
    u"Программа передач: %s": {
        "en": u"Program guide: %s", "fr": u"Guide TV : %s", "pt": u"Guia TV: %s", "es": u"Guía TV: %s",
        "it": u"Guida TV: %s", "de": u"EPG: %s", "pl": u"EPG: %s", "cs": u"EPG: %s",
        "tr": u"Program rehberi: %s", "he": u"לוח שידורים: %s", "ar": u"دليل البرامج: %s", "ja": u"番組表: %s",
        "zh": u"节目单：%s", "th": u"ผังรายการ: %s",
    },
    u"Внимание:": {
        "en": u"Warning:", "fr": u"Attention :", "pt": u"Atenção:", "es": u"Atención:",
        "it": u"Attenzione:", "de": u"Achtung:", "pl": u"Uwaga:", "cs": u"Upozornění:",
        "tr": u"Uyarı:", "he": u"שים לב:", "ar": u"تنبيه:", "ja": u"注意:",
        "zh": u"注意：", "th": u"คำเตือน:",
    },
    u"Наведите камеру планшета,\nчтобы открыть Dreaminator": {
        "en": u"Point your phone or tablet camera\nto open Dreaminator",
        "fr": u"Scannez avec votre mobile ou tablette\npour ouvrir Dreaminator",
        "pt": u"Aponte a câmara do telemóvel\npara abrir o Dreaminator",
        "es": u"Apunta la cámara de tu móvil\npara abrir Dreaminator",
        "it": u"Inquadra il QR con lo smartphone\nper aprire Dreaminator",
        "de": u"QR-Code scannen,\num Dreaminator zu öffnen",
        "pl": u"Zeskanuj telefonem,\naby otworzyć Dreaminator",
        "cs": u"Naskenujte mobilem\npro otevření Dreaminator",
        "tr": u"Dreaminator’ı açmak için\nkamerayı doğrultun",
        "he": u"כוון את מצלמת הטלפון\nלפתיחת Dreaminator",
        "ar": u"وجّه كاميرا الهاتف\nلفتح Dreaminator",
        "ja": u"スマホやタブレットのカメラで\nDreaminator を開く",
        "zh": u"用手机或平板扫码\n打开 Dreaminator",
        "th": u"สแกนด้วยกล้องมือถือ\nเพื่อเปิด Dreaminator",
    },
    u"Адрес для планшета и компьютера:": {
        "en": u"Address for tablet and computer:", "fr": u"Adresse pour tablette et ordinateur :",
        "pt": u"Endereço para tablet e computador:", "es": u"Dirección para tablet y ordenador:",
        "it": u"Indirizzo per tablet e computer:", "de": u"Adresse für Tablet und Computer:",
        "pl": u"Adres dla tabletu i komputera:", "cs": u"Adresa pro tablet a počítač:",
        "tr": u"Tablet ve bilgisayar adresi:", "he": u"כתובת לטאבלט ולמחשב:",
        "ar": u"عنوان الجهاز اللوحي والكمبيوتر:", "ja": u"タブレット・PC用アドレス:",
        "zh": u"平板与电脑访问地址：", "th": u"ที่อยู่สำหรับแท็บเล็ตและคอมพิวเตอร์:",
    },
    u"Подключаюсь к Dreaminator…": {
        "en": u"Connecting to Dreaminator…", "fr": u"Connexion à Dreaminator…", "pt": u"A ligar ao Dreaminator…",
        "es": u"Conectando a Dreaminator…", "it": u"Connessione a Dreaminator…", "de": u"Verbinde mit Dreaminator…",
        "pl": u"Łączenie z Dreaminator…", "cs": u"Připojování k Dreaminator…", "tr": u"Dreaminator’a bağlanıyor…",
        "he": u"מתחבר ל-Dreaminator…", "ar": u"جارٍ الاتصال بـ Dreaminator…", "ja": u"Dreaminator に接続中…",
        "zh": u"正在连接 Dreaminator…", "th": u"กำลังเชื่อมต่อ Dreaminator…",
    },
    u"Закрыть": {
        "en": u"Close", "fr": u"Fermer", "pt": u"Fechar", "es": u"Cerrar", "it": u"Chiudi",
        "de": u"Schließen", "pl": u"Zamknij", "cs": u"Zavřít", "tr": u"Kapat",
        "he": u"סגור", "ar": u"إغلاق", "ja": u"閉じる", "zh": u"关闭", "th": u"ปิด",
    },
    u"Отправить на ТВ": {
        "en": u"Send to TV", "fr": u"Envoyer à la TV", "pt": u"Enviar para a TV", "es": u"Enviar a la TV",
        "it": u"Invia alla TV", "de": u"An TV senden", "pl": u"Wyślij na TV", "cs": u"Odeslat do TV",
        "tr": u"TV'ye Gönder", "he": u"שלח לטלוויזיה", "ar": u"إرسال للتلفاز", "ja": u"TVへ送信",
        "zh": u"发送到电视", "th": u"ส่งไปยังทีวี",
    },
    u"Обновить плейлисты": {
        "en": u"Refresh playlists", "fr": u"Actualiser playlists", "pt": u"Atualizar playlists", "es": u"Actualizar listas",
        "it": u"Aggiorna playlist", "de": u"Playlists laden", "pl": u"Odśwież playlisty", "cs": u"Obnovit playlisty",
        "tr": u"Listeleri yenile", "he": u"רענן פלייליסטים", "ar": u"تحديث القوائم", "ja": u"リストを更新",
        "zh": u"更新播放列表", "th": u"รีเฟรชเพลย์ลิสต์",
    },
    u"Перезапустить": {
        "en": u"Restart", "fr": u"Redémarrer", "pt": u"Reiniciar", "es": u"Reiniciar", "it": u"Riavvia",
        "de": u"Neustarten", "pl": u"Uruchom ponownie", "cs": u"Restartovat", "tr": u"Yeniden başlat",
        "he": u"הפעל מחדש", "ar": u"إعادة تشغيل", "ja": u"再起動", "zh": u"重启服务", "th": u"รีสตาร์ท",
    },
    u"нет адреса — проверьте сеть": {
        "en": u"no address — check network", "fr": u"aucune adresse — vérifiez le réseau",
        "pt": u"sem endereço — verifique a rede", "es": u"sin dirección — revisa la red",
        "it": u"nessun indirizzo — controlla la rete", "de": u"keine Adresse — Netzwerk prüfen",
        "pl": u"brak adresu — sprawdź sieć", "cs": u"bez adresy — zkontrolujte síť",
        "tr": u"adres yok — ağı kontrol edin", "he": u"אין כתובת — בדוק את הרשת",
        "ar": u"لا يوجد عنوان — تحقق من الشبكة", "ja": u"アドレスなし — ネットワークを確認",
        "zh": u"无地址 — 请检查网络", "th": u"ไม่มีที่อยู่ — ตรวจสอบเครือข่าย",
    },
    u"Сервис Dreaminator не отвечает.\nНажмите синюю кнопку, чтобы перезапустить его.\n\nПодробности для сына: %s": {
        "en": u"Dreaminator service is not responding.\nPress the Blue button to restart it.\n\nDetails: %s",
        "fr": u"Le service Dreaminator ne répond pas.\nAppuyez sur le bouton Bleu pour le redémarrer.\n\nDétails : %s",
        "pt": u"O serviço Dreaminator não responde.\nPrima o botão Azul para reiniciar.\n\nDetalhes: %s",
        "es": u"El servicio Dreaminator no responde.\nPulsa el botón Azul para reiniciar.\n\nDetalles: %s",
        "it": u"Il servizio Dreaminator non risponde.\nPremi il tasto Blu per riavviarlo.\n\nDettagli: %s",
        "de": u"Der Dreaminator-Dienst antwortet nicht.\nDrücken Sie die Blaue Taste für einen Neustart.\n\nDetails: %s",
        "pl": u"Usługa Dreaminator nie odpowiada.\nNaciśnij Niebieski przycisk, aby zrestartować.\n\nSzczegóły: %s",
        "cs": u"Služba Dreaminator neodpovídá.\nStiskněte Modré tlačítko pro restart.\n\nPodrobnosti: %s",
        "tr": u"Dreaminator hizmeti yanıt vermiyor.\nYeniden başlatmak için Mavi düğmeye basın.\n\nAyrıntılar: %s",
        "he": u"שירות Dreaminator אינו מגיב.\nלחץ על הכפתור הכחול להפעלה מחדש.\n\nפרטים: %s",
        "ar": u"خدمة Dreaminator لا تستجيب.\nاضغط الزر الأزرق لإعادة تشغيلها.\n\nالتفاصيل: %s",
        "ja": u"Dreaminator サービスが応答しません。\n青ボタンで再起動してください。\n\n詳細: %s",
        "zh": u"Dreaminator 服务无响应。\n请按蓝色键重启服务。\n\n详情：%s",
        "th": u"บริการ Dreaminator ไม่ตอบสนอง\nกดปุ่มสีน้ำเงินเพื่อรีสตาร์ท\n\nรายละเอียด: %s",
    },
    u"Не получилось: %s": {
        "en": u"Failed: %s", "fr": u"Échec : %s", "pt": u"Falhou: %s", "es": u"Error: %s",
        "it": u"Errore: %s", "de": u"Fehlgeschlagen: %s", "pl": u"Niepowodzenie: %s", "cs": u"Nezdařilo se: %s",
        "tr": u"Başarısız: %s", "he": u"נכשל: %s", "ar": u"فشل: %s", "ja": u"失敗: %s",
        "zh": u"失败：%s", "th": u"ไม่สำเร็จ: %s",
    },
    u"Отправляю списки на ТВ…": {
        "en": u"Sending lists to TV…", "fr": u"Envoi des listes à la TV…", "pt": u"A enviar listas para a TV…",
        "es": u"Enviando listas a la TV…", "it": u"Invio liste alla TV…", "de": u"Sende Listen an TV…",
        "pl": u"Wysyłam listy na TV…", "cs": u"Odesílám seznamy do TV…", "tr": u"Listeler TV'ye gönderiliyor…",
        "he": u"שולח רשימות לטלוויזיה…", "ar": u"جارٍ إرسال القوائم للتلفاز…", "ja": u"TVへリストを送信中…",
        "zh": u"正在发送列表到电视…", "th": u"กำลังส่งรายการไปยังทีวี…",
    },
    u"Готово! Списки обновлены на ТВ.": {
        "en": u"Done! Lists updated on TV.", "fr": u"Terminé ! Listes mises à jour sur la TV.",
        "pt": u"Concluído! Listas atualizadas na TV.", "es": u"¡Listo! Listas actualizadas en la TV.",
        "it": u"Fatto! Liste aggiornate sulla TV.", "de": u"Fertig! Listen auf TV aktualisiert.",
        "pl": u"Gotowe! Listy zaktualizowane na TV.", "cs": u"Hotovo! Seznamy aktualizovány v TV.",
        "tr": u"Tamam! Listeler TV'de güncellendi.", "he": u"בוצע! הרשימות עודכנו בטלוויזיה.",
        "ar": u"تم! حُدثت القوائم على التلفاز.", "ja": u"完了！TVのリストを更新しました。",
        "zh": u"完成！电视列表已更新。", "th": u"เสร็จสิ้น! อัปเดตรายการบนทีวีแล้ว",
    },
    u"Перезапускаю сервис Dreaminator…": {
        "en": u"Restarting Dreaminator service…", "fr": u"Redémarrage de Dreaminator…",
        "pt": u"A reiniciar o Dreaminator…", "es": u"Reiniciando Dreaminator…",
        "it": u"Riavvio di Dreaminator…", "de": u"Starte Dreaminator neu…",
        "pl": u"Restartuję usługę Dreaminator…", "cs": u"Restartuji službu Dreaminator…",
        "tr": u"Dreaminator yeniden başlatılıyor…", "he": u"מפעיל מחדש את Dreaminator…",
        "ar": u"جارٍ إعادة تشغيل Dreaminator…", "ja": u"Dreaminator を再起動中…",
        "zh": u"正在重启 Dreaminator 服务…", "th": u"กำลังรีสตาร์ทบริการ Dreaminator…",
    },
    u"Каталог Stalker и управление IPTV с планшета": {
        "en": u"Stalker catalog and IPTV management from tablet",
        "fr": u"Catalogue Stalker et gestion IPTV depuis tablette",
        "pt": u"Catálogo Stalker e gestão de IPTV via tablet",
        "es": u"Catálogo Stalker y gestión IPTV desde tablet",
        "it": u"Catalogo Stalker e gestione IPTV da tablet",
        "de": u"Stalker-Katalog und IPTV-Verwaltung per Tablet",
        "pl": u"Katalog Stalker i zarządzanie IPTV z tabletu",
        "cs": u"Katalog Stalker a správa IPTV z tabletu",
        "tr": u"Stalker kataloğu ve tabletten IPTV yönetimi",
        "he": u"קטלוג Stalker וניהול IPTV מהטאבלט",
        "ar": u"كتالوج Stalker وإدارة IPTV من الجهاز اللوحي",
        "ja": u"Stalker カタログとタブレットからの IPTV 管理",
        "zh": u"Stalker 目录与平板端 IPTV 管理",
        "th": u"แคตตาล็อก Stalker และจัดการ IPTV จากแท็บเล็ต",
    },
    u"Архив Dreaminator": {
        "en": u"Dreaminator Archive", "fr": u"Archives Dreaminator", "pt": u"Arquivo Dreaminator",
        "es": u"Archivo Dreaminator", "it": u"Archivio Dreaminator", "de": u"Dreaminator-Archiv",
        "pl": u"Archiwum Dreaminator", "cs": u"Archiv Dreaminator", "tr": u"Dreaminator Arşivi",
        "he": u"ארכיון Dreaminator", "ar": u"أرشيف Dreaminator", "ja": u"Dreaminator アーカイブ",
        "zh": u"Dreaminator 回看", "th": u"รายการย้อนหลัง Dreaminator",
    },
    u"Записи выбранного канала": {
        "en": u"Catch-up recordings of selected channel",
        "fr": u"Rediffusions de la chaîne sélectionnée",
        "pt": u"Gravações do canal selecionado",
        "es": u"Grabaciones del canal seleccionado",
        "it": u"Registrazioni del canale selezionato",
        "de": u"Aufnahmen des gewählten Kanals",
        "pl": u"Nagrania wybranego kanału",
        "cs": u"Záznamy vybraného kanálu",
        "tr": u"Seçili kanalın kayıtları",
        "he": u"הקלטות הערוץ הנבחר",
        "ar": u"تسجيلات القناة المحددة",
        "ja": u"選択チャンネルの録画",
        "zh": u"所选频道的回看录像",
        "th": u"รายการย้อนหลังของช่องที่เลือก",
    },
    u"Избранное": {
        "en": u"Favorites", "fr": u"Favoris", "pt": u"Favoritos", "es": u"Favoritos",
        "it": u"Preferiti", "de": u"Favoriten", "pl": u"Ulubione", "cs": u"Oblíbené",
        "tr": u"Favoriler", "he": u"מועדפים", "ar": u"المفضلة", "ja": u"お気に入り",
        "zh": u"收藏夹", "th": u"รายการโปรด",
    },
    u"Недавние": {
        "en": u"Recent", "fr": u"Récents", "pt": u"Recentes", "es": u"Recientes",
        "it": u"Recenti", "de": u"Zuletzt", "pl": u"Ostatnie", "cs": u"Nedávné",
        "tr": u"Son", "he": u"אחרונים", "ar": u"الأخيرة", "ja": u"最近",
        "zh": u"最近观看", "th": u"ล่าสุด",
    },
    u"Портал": {
        "en": u"Portal", "fr": u"Portail", "pt": u"Portal", "es": u"Portal",
        "it": u"Portale", "de": u"Portal", "pl": u"Portal", "cs": u"Portál",
        "tr": u"Portal", "he": u"פורטל", "ar": u"البوابة", "ja": u"ポータル",
        "zh": u"门户", "th": u"พอร์ทัล",
    },
    u"Live TV": {
        "en": u"Live TV", "fr": u"TV en direct", "pt": u"TV ao vivo", "es": u"TV en vivo",
        "it": u"Live TV", "de": u"Live TV", "pl": u"Live TV", "cs": u"Live TV",
        "tr": u"Canlı TV", "he": u"שידור חי", "ar": u"البث المباشر", "ja": u"ライブTV",
        "zh": u"电视直播", "th": u"ทีวีสด",
    },
    u"Фильмы": {
        "en": u"Movies", "fr": u"Films", "pt": u"Filmes", "es": u"Películas",
        "it": u"Film", "de": u"Filme", "pl": u"Filmy", "cs": u"Filmy",
        "tr": u"Filmler", "he": u"סרטים", "ar": u"أفلام", "ja": u"映画",
        "zh": u"电影", "th": u"ภาพยนตร์",
    },
    u"Сериалы": {
        "en": u"Series", "fr": u"Séries", "pt": u"Séries", "es": u"Series",
        "it": u"Serie TV", "de": u"Serien", "pl": u"Seriale", "cs": u"Seriály",
        "tr": u"Diziler", "he": u"סדרות", "ar": u"مسلسلات", "ja": u"シリーズ",
        "zh": u"剧集", "th": u"ซีรีส์",
    },
    u"Поиск": {
        "en": u"Search", "fr": u"Recherche", "pt": u"Pesquisa", "es": u"Buscar",
        "it": u"Cerca", "de": u"Suche", "pl": u"Szukaj", "cs": u"Hledat",
        "tr": u"Ara", "he": u"חיפוש", "ar": u"بحث", "ja": u"検索",
        "zh": u"搜索", "th": u"ค้นหา",
    },
    u"Настройки": {
        "en": u"Settings", "fr": u"Paramètres", "pt": u"Definições", "es": u"Ajustes",
        "it": u"Impostazioni", "de": u"Einstellungen", "pl": u"Ustawienia", "cs": u"Nastavení",
        "tr": u"Ayarlar", "he": u"הגדרות", "ar": u"الإعدادات", "ja": u"設定",
        "zh": u"设置", "th": u"การตั้งค่า",
    },
    u"О приложении": {
        "en": u"About", "fr": u"À propos", "pt": u"Sobre", "es": u"Acerca de",
        "it": u"Info", "de": u"Über", "pl": u"O aplikacji", "cs": u"O aplikaci",
        "tr": u"Hakkında", "he": u"אודות", "ar": u"حول", "ja": u"アプリについて",
        "zh": u"关于", "th": u"เกี่ยวกับ",
    },
    u"Новые": {
        "en": u"Newest", "fr": u"Nouveautés", "pt": u"Recentes", "es": u"Recientes",
        "it": u"Più recenti", "de": u"Neueste", "pl": u"Najnowsze", "cs": u"Nejnovější",
        "tr": u"En yeni", "he": u"חדשים", "ar": u"الأحدث", "ja": u"新着",
        "zh": u"最新", "th": u"ใหม่ล่าสุด",
    },
    u"Популярные": {
        "en": u"Popular", "fr": u"Populaires", "pt": u"Populares", "es": u"Populares",
        "it": u"Popolari", "de": u"Beliebt", "pl": u"Popularne", "cs": u"Populární",
        "tr": u"Popüler", "he": u"פופולרי", "ar": u"الأكثر شعبية", "ja": u"人気",
        "zh": u"热门", "th": u"ยอดนิยม",
    },
    u"По рейтингу": {
        "en": u"By rating", "fr": u"Par note", "pt": u"Por avaliação", "es": u"Por puntuación",
        "it": u"Per voto", "de": u"Bewertung", "pl": u"Wg oceny", "cs": u"Podle hodnocení",
        "tr": u"Puana göre", "he": u"לפי דירוג", "ar": u"حسب التقييم", "ja": u"評価順",
        "zh": u"按评分", "th": u"ตามคะแนน",
    },
    u"По названию": {
        "en": u"By title", "fr": u"Par titre", "pt": u"Por título", "es": u"Por título",
        "it": u"Per titolo", "de": u"Nach Titel", "pl": u"Wg tytułu", "cs": u"Podle názvu",
        "tr": u"Ada göre", "he": u"לפי שם", "ar": u"حسب الاسم", "ja": u"タイトル順",
        "zh": u"按名称", "th": u"ตามชื่อ",
    },
    u"новые": {
        "en": u"newest", "fr": u"nouveautés", "pt": u"recentes", "es": u"recientes",
        "it": u"più recenti", "de": u"neueste", "pl": u"najnowsze", "cs": u"nejnovější",
        "tr": u"en yeni", "he": u"חדשים", "ar": u"الأحدث", "ja": u"新着",
        "zh": u"最新", "th": u"ใหม่ล่าสุด",
    },
    u"популярные": {
        "en": u"popular", "fr": u"populaires", "pt": u"populares", "es": u"populares",
        "it": u"popolari", "de": u"beliebt", "pl": u"popularne", "cs": u"populární",
        "tr": u"popüler", "he": u"פופולרי", "ar": u"الأكثر شعبية", "ja": u"人気",
        "zh": u"热门", "th": u"ยอดนิยม",
    },
    u"по рейтингу": {
        "en": u"by rating", "fr": u"par note", "pt": u"por avaliação", "es": u"por puntuación",
        "it": u"per voto", "de": u"Bewertung", "pl": u"wg oceny", "cs": u"podle hodnocení",
        "tr": u"puana göre", "he": u"לפי דירוג", "ar": u"حسب التقييم", "ja": u"評価順",
        "zh": u"按评分", "th": u"ตามคะแนน",
    },
    u"по названию": {
        "en": u"by title", "fr": u"par titre", "pt": u"por título", "es": u"por título",
        "it": u"per titolo", "de": u"nach Titel", "pl": u"wg tytułu", "cs": u"podle názvu",
        "tr": u"ada göre", "he": u"לפי שם", "ar": u"حسب الاسم", "ja": u"タイトル順",
        "zh": u"按名称", "th": u"ตามชื่อ",
    },
    u"Страницы": {
        "en": u"Pages", "fr": u"Pages", "pt": u"Páginas", "es": u"Páginas",
        "it": u"Pagine", "de": u"Seiten", "pl": u"Strony", "cs": u"Stránky",
        "tr": u"Sayfalar", "he": u"דפים", "ar": u"الصفحات", "ja": u"ページ",
        "zh": u"翻页", "th": u"หน้า",
    },
    u"Обновить": {
        "en": u"Refresh", "fr": u"Actualiser", "pt": u"Atualizar", "es": u"Actualizar",
        "it": u"Aggiorna", "de": u"Aktual.", "pl": u"Odśwież", "cs": u"Obnovit",
        "tr": u"Yenile", "he": u"רענן", "ar": u"تحديث", "ja": u"更新",
        "zh": u"刷新", "th": u"รีเฟรช",
    },
    u"Источники": {
        "en": u"Sources", "fr": u"Sources", "pt": u"Fontes", "es": u"Fuentes",
        "it": u"Fonti", "de": u"Quellen", "pl": u"Źródła", "cs": u"Zdroje",
        "tr": u"Kaynaklar", "he": u"מקורות", "ar": u"المصادر", "ja": u"ソース",
        "zh": u"源", "th": u"แหล่งที่มา",
    },
    u"Сортировка": {
        "en": u"Sort", "fr": u"Trier", "pt": u"Ordenar", "es": u"Ordenar",
        "it": u"Ordina", "de": u"Sortieren", "pl": u"Sortuj", "cs": u"Seřadit",
        "tr": u"Sırala", "he": u"מיון", "ar": u"ترتيب", "ja": u"並び替え",
        "zh": u"排序", "th": u"เรียง",
    },
    u"каталог на телевизоре": {
        "en": u"TV catalog", "fr": u"catalogue TV", "pt": u"catálogo na TV", "es": u"catálogo en TV",
        "it": u"catalogo TV", "de": u"TV-Katalog", "pl": u"katalog na TV", "cs": u"katalog v TV",
        "tr": u"TV kataloğu", "he": u"קטלוג בטלוויזיה", "ar": u"دليل التلفاز", "ja": u"TVカタログ",
        "zh": u"电视目录", "th": u"แคตตาล็อกทีวี",
    },
    u"Загрузка…": {
        "en": u"Loading…", "fr": u"Chargement…", "pt": u"A carregar…", "es": u"Cargando…",
        "it": u"Caricamento…", "de": u"Laden…", "pl": u"Ładowanie…", "cs": u"Načítání…",
        "tr": u"Yükleniyor…", "he": u"טוען…", "ar": u"جارٍ التحميل…", "ja": u"読み込み中…",
        "zh": u"加载中…", "th": u"กำลังโหลด…",
    },
    u"Жёлтая — повторить · Back — назад": {
        "en": u"Yellow — retry · Back — return", "fr": u"Jaune — réessayer · Back — retour",
        "pt": u"Amarelo — repetir · Back — voltar", "es": u"Amarillo — reintentar · Back — volver",
        "it": u"Giallo — riprova · Back — indietro", "de": u"Gelb — wiederholen · Back — zurück",
        "pl": u"Żółty — ponów · Back — powrót", "cs": u"Žluté — opakovat · Back — zpět",
        "tr": u"Sarı — tekrar dene · Back — geri", "he": u"צהוב — נסה שוב · Back — חזרה",
        "ar": u"الأصفر — إعادة · Back — رجوع", "ja": u"黄 — 再試行 · Back — 戻る",
        "zh": u"黄键 — 重试 · 返回键 — 返回", "th": u"สีเหลือง — ลองใหม่ · Back — กลับ",
    },
    u"На портале нет отдельного раздела сериалов": {
        "en": u"This portal has no separate Series section", "fr": u"Pas de section Séries séparée sur ce portail",
        "pt": u"Este portal não tem secção separada de séries", "es": u"Este portal no tiene sección de series aparte",
        "it": u"Il portale non ha una sezione serie separata", "de": u"Kein eigener Serienbereich auf diesem Portal",
        "pl": u"Portal nie ma osobnej sekcji seriali", "cs": u"Portál nemá samostatnou sekci seriálů",
        "tr": u"Portalda ayrı dizi bölümü yok", "he": u"בפורטל אין מדור סדרות נפרד",
        "ar": u"لا يوجد قسم منفصل للمسلسلات في البوابة", "ja": u"ポータルに独立したシリーズ欄がありません",
        "zh": u"该门户没有单独的剧集栏目", "th": u"พอร์ทัลไม่มีหมวดซีรีส์แยกต่างหาก",
    },
    u"Сериалы этого портала — в «Фильмах», с пометкой «Сериал»": {
        "en": u"Series on this portal are in “Movies”, marked “Series”",
        "fr": u"Les séries sont dans « Films », marquées « Série »",
        "pt": u"As séries estão em «Filmes», marcadas «Série»",
        "es": u"Las series están en «Películas», marcadas «Serie»",
        "it": u"Le serie sono in «Film», contrassegnate «Serie»",
        "de": u"Serien finden Sie unter „Filme“ mit dem Hinweis „Serie“",
        "pl": u"Seriale są w „Filmach” z oznaczeniem „Serial”",
        "cs": u"Seriály jsou ve „Filmech“ s označením „Seriál“",
        "tr": u"Bu portalın dizileri «Filmler» içinde «Dizi» etiketiyle yer alır",
        "he": u"הסדרות נמצאות ב«סרטים» עם התווית «סדרה»",
        "ar": u"المسلسلات موجودة في «أفلام» بعلامة «مسلسل»",
        "ja": u"このポータルのシリーズは「映画」内に「シリーズ」表示であります",
        "zh": u"该门户的剧集位于“电影”中，带有“剧集”标签",
        "th": u"ซีรีส์ของพอร์ทัลนี้อยู่ใน “ภาพยนตร์” โดยมีป้ายกำกับ “ซีรีส์”",
    },
    u"В плейлисте нет каналов": {
        "en": u"No channels in playlist", "fr": u"Aucune chaîne dans la playlist",
        "pt": u"Sem canais na playlist", "es": u"No hay canales en la lista",
        "it": u"Nessun canale nella playlist", "de": u"Keine Kanäle in der Playlist",
        "pl": u"Brak kanałów na playliście", "cs": u"V playlistu nejsou žádné kanály",
        "tr": u"Oynatma listesinde kanal yok", "he": u"אין ערוצים בפלייליסט",
        "ar": u"لا توجد قنوات في القائمة", "ja": u"プレイリストにチャンネルがありません",
        "zh": u"播放列表中没有频道", "th": u"ไม่มีช่องในเพลย์ลิสต์",
    },
    u"Жёлтая — загрузить плейлист заново": {
        "en": u"Yellow — reload playlist", "fr": u"Jaune — recharger la playlist",
        "pt": u"Amarelo — recarregar playlist", "es": u"Amarillo — recargar lista",
        "it": u"Giallo — ricarica playlist", "de": u"Gelb — Playlist neu laden",
        "pl": u"Żółty — pobierz ponownie", "cs": u"Žluté — znovu načíst playlist",
        "tr": u"Sarı — listeyi yeniden yükle", "he": u"צהוב — טען פלייליסט מחדש",
        "ar": u"الأصفر — إعادة تحميل القائمة", "ja": u"黄 — プレイリストを再読込",
        "zh": u"黄键 — 重新加载播放列表", "th": u"สีเหลือง — โหลดเพลย์ลิสต์ใหม่",
    },
    u"На портале нет телеканалов": {
        "en": u"No Live TV channels on portal", "fr": u"Aucune chaîne TV sur ce portail",
        "pt": u"Sem canais de TV no portal", "es": u"No hay canales de TV en el portal",
        "it": u"Nessun canale TV sul portale", "de": u"Keine TV-Kanäle auf dem Portal",
        "pl": u"Brak kanałów TV na portalu", "cs": u"Na portálu nejsou televizní kanály",
        "tr": u"Portalda TV kanalı yok", "he": u"אין ערוצי טלוויזיה בפורטל",
        "ar": u"لا توجد قنوات تلفاز في البوابة", "ja": u"ポータルにTVチャンネルがありません",
        "zh": u"门户上没有电视频道", "th": u"ไม่มีช่องทีวีบนพอร์ทัล",
    },
    u"Каталог фильмов и сериалов доступен в других разделах": {
        "en": u"Movies and series catalog is in other sections",
        "fr": u"Films et séries disponibles dans les autres sections",
        "pt": u"Filmes e séries disponíveis noutras secções",
        "es": u"Películas y series en otras secciones",
        "it": u"Film e serie disponibili nelle altre sezioni",
        "de": u"Filme und Serien sind in anderen Bereichen verfügbar",
        "pl": u"Katalog filmów i seriali jest w innych sekcjach",
        "cs": u"Katalog filmů a seriálů je v dalších sekcích",
        "tr": u"Film ve dizi kataloğu diğer bölümlerde",
        "he": u"סרטים וסדרות זמינים במדורים האחרים",
        "ar": u"الأفلام والمسلسلات متاحة في الأقسام الأخرى",
        "ja": u"映画とシリーズは他のセクションにあります",
        "zh": u"电影和剧集目录位于其他栏目中",
        "th": u"แคตตาล็อกภาพยนตร์และซีรีส์อยู่ในหมวดอื่น",
    },
    u"Портал не отдал категории": {
        "en": u"Portal returned no categories", "fr": u"Le portail n’a renvoyé aucune catégorie",
        "pt": u"O portal não devolveu categorias", "es": u"El portal no devolvió categorías",
        "it": u"Il portale non ha restituito categorie", "de": u"Portal lieferte keine Kategorien",
        "pl": u"Portal nie zwrócił kategorii", "cs": u"Portál nevrátil žádné kategorie",
        "tr": u"Portal kategori döndürmedi", "he": u"הפורטל לא החזיר קטגוריות",
        "ar": u"لم ترجع البوابة أي فئات", "ja": u"ポータルからカテゴリを取得できません",
        "zh": u"门户未返回分类", "th": u"พอร์ทัลไม่ส่งคืนหมวดหมู่",
    },
    u"Жёлтая — повторить": {
        "en": u"Yellow — retry", "fr": u"Jaune — réessayer", "pt": u"Amarelo — repetir", "es": u"Amarillo — reintentar",
        "it": u"Giallo — riprova", "de": u"Gelb — wiederholen", "pl": u"Żółty — ponów", "cs": u"Žluté — opakovat",
        "tr": u"Sarı — tekrar dene", "he": u"צהוב — נסה שוב", "ar": u"الأصفر — إعادة", "ja": u"黄 — 再試行",
        "zh": u"黄键 — 重试", "th": u"สีเหลือง — ลองใหม่",
    },
    u"Нет связи с порталом": {
        "en": u"Cannot reach portal", "fr": u"Portail injoignable", "pt": u"Sem ligação ao portal",
        "es": u"Sin conexión con el portal", "it": u"Portàle non raggiungibile", "de": u"Portal nicht erreichbar",
        "pl": u"Brak połączenia z portalem", "cs": u"Nelze se spojit s portálem", "tr": u"Portala ulaşılamıyor",
        "he": u"אין חיבור לפורטל", "ar": u"تعذر الاتصال بالبوابة", "ja": u"ポータルに接続できません",
        "zh": u"无法连接到门户", "th": u"ไม่สามารถเชื่อมต่อพอร์ทัล",
    },
    u"В этой категории пока пусто": {
        "en": u"This category is empty", "fr": u"Cette catégorie est vide", "pt": u"Esta categoria está vazia",
        "es": u"Esta categoría está vacía", "it": u"Questa categoria è vuota", "de": u"Diese Kategorie ist leer",
        "pl": u"Ta kategoria jest pusta", "cs": u"Tato kategorie je prázdná", "tr": u"Bu kategori boş",
        "he": u"קטגוריה זו ריקה", "ar": u"هذه الفئة فارغة", "ja": u"このカテゴリは空です",
        "zh": u"此分类暂无内容", "th": u"หมวดหมู่นี้ยังว่างอยู่",
    },
    u"Поиск в разделе «%s»": {
        "en": u"Search in “%s”", "fr": u"Recherche dans « %s »", "pt": u"Pesquisar em «%s»", "es": u"Buscar en «%s»",
        "it": u"Cerca in «%s»", "de": u"Suche in „%s“", "pl": u"Szukaj w „%s”", "cs": u"Hledat v „%s“",
        "tr": u"«%s» içinde ara", "he": u"חיפוש ב-«%s»", "ar": u"بحث في «%s»", "ja": u"「%s」内を検索",
        "zh": u"在“%s”中搜索", "th": u"ค้นหาใน “%s”",
    },
    u"%s–%s из %s": {
        "en": u"%s–%s of %s", "fr": u"%s–%s sur %s", "pt": u"%s–%s de %s", "es": u"%s–%s de %s",
        "it": u"%s–%s di %s", "de": u"%s–%s von %s", "pl": u"%s–%s z %s", "cs": u"%s–%s z %s",
        "tr": u"%s–%s / %s", "he": u"%s–%s מתוך %s", "ar": u"%s–%s من %s", "ja": u"%s–%s / %s",
        "zh": u"%s–%s / %s", "th": u"%s–%s จาก %s",
    },
    u"Не удалось изменить избранное": {
        "en": u"Failed to update favorites", "fr": u"Impossible de modifier les favoris",
        "pt": u"Não foi possível atualizar favoritos", "es": u"No se pudieron actualizar los favoritos",
        "it": u"Impossibile aggiornare i preferiti", "de": u"Favoriten konnten nicht geändert werden",
        "pl": u"Nie udało się zmienić ulubionych", "cs": u"Nepodařilo se upravit oblíbené",
        "tr": u"Favoriler güncellenemedi", "he": u"עדכון המועדפים נכשל",
        "ar": u"تعذر تعديل المفضلة", "ja": u"お気に入りの更新に失敗",
        "zh": u"无法更新收藏夹", "th": u"อัปเดตรายการโปรดไม่สำเร็จ",
    },
    u"Порядок фильмов и сериалов в категориях": {
        "en": u"Order of movies and series in categories", "fr": u"Ordre des films et séries",
        "pt": u"Ordem de filmes e séries", "es": u"Orden de películas y series",
        "it": u"Ordine di film e serie", "de": u"Sortierung der Filme und Serien",
        "pl": u"Kolejność filmów i seriali", "cs": u"Pořadí filmů a seriálů",
        "tr": u"Film ve dizilerin sıralaması", "he": u"סדר סרטים וסדרות",
        "ar": u"ترتيب الأفلام والمسلسلات", "ja": u"映画とシリーズの並び順",
        "zh": u"分类中电影与剧集的排序", "th": u"ลำดับภาพยนตร์และซีรีส์",
    },
    u"Подключён один источник": {
        "en": u"One source connected", "fr": u"Une seule source connectée", "pt": u"Uma fonte ligada",
        "es": u"Una fuente conectada", "it": u"Una sola fonte collegata", "de": u"Eine Quelle verbunden",
        "pl": u"Podłączono jedno źródło", "cs": u"Připojen jeden zdroj", "tr": u"Bir kaynak bağlı",
        "he": u"מקור אחד מחובר", "ar": u"مصدر واحد متصل", "ja": u"ソースが1つ接続されています",
        "zh": u"已连接 1 个源", "th": u"เชื่อมต่ออยู่ 1 แหล่ง",
    },
    u"Источники не найдены": {
        "en": u"No sources found", "fr": u"Aucune source trouvée", "pt": u"Nenhuma fonte encontrada",
        "es": u"No se encontraron fuentes", "it": u"Nessuna fonte trovata", "de": u"Keine Quellen gefunden",
        "pl": u"Nie znaleziono źródeł", "cs": u"Nenalezeny žádné zdroje", "tr": u"Kaynak bulunamadı",
        "he": u"לא נמצאו מקורות", "ar": u"لم يُعثر على مصادر", "ja": u"ソースが見つかりません",
        "zh": u"未找到源", "th": u"ไม่พบแหล่งที่มา",
    },
    u"Порталы и плейлисты добавляются в веб-интерфейсе": {
        "en": u"Add portals and playlists in the web interface",
        "fr": u"Ajoutez portails et playlists dans l’interface web",
        "pt": u"Adicione portais e playlists na interface web",
        "es": u"Añade portales y listas en la interfaz web",
        "it": u"Aggiungi portali e playlist nell’interfaccia web",
        "de": u"Portale und Playlists über das Webinterface hinzufügen",
        "pl": u"Portale i playlisty dodaje się w interfejsie web",
        "cs": u"Portály a playlisty se přidávají ve webovém rozhraní",
        "tr": u"Portallar ve listeler web arayüzünden eklenir",
        "he": u"פורטלים ופלייליסטים נוספים בממשק הווב",
        "ar": u"تُضاف البوابات والقوائم من واجهة الويب",
        "ja": u"ポータルやプレイリストはWeb画面から追加します",
        "zh": u"请在网页端添加门户和播放列表",
        "th": u"เพิ่มพอร์ทัลและเพลย์ลิสต์ได้ในหน้าเว็บ",
    },
    u"Порталы и плейлисты из веб-интерфейса": {
        "en": u"Portals and playlists from web interface",
        "fr": u"Portails et playlists de l’interface web",
        "pt": u"Portais e playlists da interface web",
        "es": u"Portales y listas de la interfaz web",
        "it": u"Portali e playlist dall’interfaccia web",
        "de": u"Portale und Playlists aus dem Webinterface",
        "pl": u"Portale i playlisty z interfejsu web",
        "cs": u"Portály a playlisty z webového rozhraní",
        "tr": u"Web arayüzünden portallar ve listeler",
        "he": u"פורטלים ופלייליסטים מממשק הווב",
        "ar": u"البوابات والقوائم من واجهة الويب",
        "ja": u"Web画面のポータルとプレイリスト",
        "zh": u"来自网页端的门户和播放列表",
        "th": u"พอร์ทัลและเพลย์ลิสต์จากหน้าเว็บ",
    },
    u"Программа передач": {
        "en": u"Program guide", "fr": u"Guide TV", "pt": u"Guia TV", "es": u"Guía TV",
        "it": u"Guida TV", "de": u"Programmführer", "pl": u"Program TV", "cs": u"Program",
        "tr": u"Program rehberi", "he": u"לוח שידורים", "ar": u"دليل البرامج", "ja": u"番組表",
        "zh": u"节目单", "th": u"ผังรายการ",
    },
    u"Подготавливаю видео…": {
        "en": u"Preparing video…", "fr": u"Préparation de la vidéo…", "pt": u"A preparar vídeo…",
        "es": u"Preparando vídeo…", "it": u"Preparazione video…", "de": u"Bereite Video vor…",
        "pl": u"Przygotowuję wideo…", "cs": u"Připravuji video…", "tr": u"Video hazırlanıyor…",
        "he": u"מכין וידאו…", "ar": u"جارٍ تحضير الفيديو…", "ja": u"動画を準備中…",
        "zh": u"正在准备视频…", "th": u"กำลังเตรียมวิดีโอ…",
    },
    u"OK — попробовать снова": {
        "en": u"OK — try again", "fr": u"OK — réessayer", "pt": u"OK — tentar novamente", "es": u"OK — reintentar",
        "it": u"OK — riprova", "de": u"OK — erneut versuchen", "pl": u"OK — spróbuj ponownie", "cs": u"OK — zkusit znovu",
        "tr": u"OK — tekrar dene", "he": u"OK — נסה שוב", "ar": u"OK — إعادة المحاولة", "ja": u"OK — 再試行",
        "zh": u"OK — 重试", "th": u"OK — ลองอีกครั้ง",
    },
    u"Не удалось открыть проигрыватель": {
        "en": u"Could not open player", "fr": u"Impossible d’ouvrir le lecteur", "pt": u"Não foi possível abrir o leitor",
        "es": u"No se pudo abrir el reproductor", "it": u"Impossibile aprire il lettore", "de": u"Player konnte nicht geöffnet werden",
        "pl": u"Nie udało się otworzyć odtwarzacza", "cs": u"Nepodařilo se otevřít přehrávač",
        "tr": u"Oynatıcı açılamadı", "he": u"פתיחת הנגן נכשלה", "ar": u"تعذر فتح المشغل",
        "ja": u"プレーヤーを開けませんでした", "zh": u"无法打开播放器", "th": u"ไม่สามารถเปิดเครื่องเล่น",
    },
    u"Канал не отвечает": {
        "en": u"Channel not responding", "fr": u"La chaîne ne répond pas", "pt": u"Canal sem resposta",
        "es": u"El canal no responde", "it": u"Il canale non risponde", "de": u"Kanal antwortet nicht",
        "pl": u"Kanał nie odpowiada", "cs": u"Kanál neodpovídá", "tr": u"Kanal yanıt vermiyor",
        "he": u"הערוץ אינו מגיב", "ar": u"القناة لا تستجيب", "ja": u"チャンネルが応答しません",
        "zh": u"频道无响应", "th": u"ช่องไม่ตอบสนอง",
    },
    u"Источник не отдал видео — попробуйте позже или другой канал": {
        "en": u"Source returned no video — try later or another channel",
        "fr": u"La source n’a pas fourni de flux — réessayez plus tard",
        "pt": u"A fonte não forneceu vídeo — tente mais tarde",
        "es": u"La fuente no entregó vídeo — prueba más tarde",
        "it": u"La fonte non ha restituito il video — riprova più tardi",
        "de": u"Quelle lieferte kein Video — später oder anderen Kanal versuchen",
        "pl": u"Źródło nie zwróciło wideo — spróbuj później",
        "cs": u"Zdroj neposkytl video — zkuste později nebo jiný kanál",
        "tr": u"Kaynak video vermedi — daha sonra veya başka kanal deneyin",
        "he": u"המקור לא החזיר וידאו — נסה מאוחר יותר",
        "ar": u"لم يرجع المصدر فيديو — جرب لاحقاً",
        "ja": u"ソースから映像を取得できませんでした — 後でやり直すか別チャンネルをお試しください",
        "zh": u"源未返回视频 — 请稍后再试或更换频道",
        "th": u"แหล่งที่มาไม่ส่งวิดีโอ — ลองใหม่ภายหลังหรือเลือกช่องอื่น",
    },
    u"Сериал": {
        "en": u"Series", "fr": u"Série", "pt": u"Série", "es": u"Serie",
        "it": u"Serie", "de": u"Serie", "pl": u"Serial", "cs": u"Seriál",
        "tr": u"Dizi", "he": u"סדרה", "ar": u"مسلسل", "ja": u"シリーズ",
        "zh": u"剧集", "th": u"ซีรีส์",
    },
    u"Канал %s": {
        "en": u"Channel %s", "fr": u"Chaîne %s", "pt": u"Canal %s", "es": u"Canal %s",
        "it": u"Canale %s", "de": u"Kanal %s", "pl": u"Kanał %s", "cs": u"Kanál %s",
        "tr": u"Kanal %s", "he": u"ערוץ %s", "ar": u"قناة %s", "ja": u"チャンネル %s",
        "zh": u"频道 %s", "th": u"ช่อง %s",
    },
    u"Нет в плейлисте": {
        "en": u"Not in playlist", "fr": u"Absente de la playlist", "pt": u"Fora da playlist",
        "es": u"No en la lista", "it": u"Non in playlist", "de": u"Nicht in Playlist",
        "pl": u"Brak na playliście", "cs": u"Není v playlistu", "tr": u"Listede yok",
        "he": u"לא בפלייליסט", "ar": u"غير موجود بالقائمة", "ja": u"リスト外",
        "zh": u"不在播放列表中", "th": u"ไม่อยู่ในเพลย์ลิสต์",
    },
    u"В избранном": {
        "en": u"In favorites", "fr": u"Dans les favoris", "pt": u"Nos favoritos", "es": u"En favoritos",
        "it": u"Nei preferiti", "de": u"In Favoriten", "pl": u"W ulubionych", "cs": u"V oblíbených",
        "tr": u"Favorilerde", "he": u"במועדפים", "ar": u"في المفضلة", "ja": u"お気に入り済み",
        "zh": u"已收藏", "th": u"อยู่ในรายการโปรด",
    },
    u"\n(нет в плейлисте)": {
        "en": u"\n(not in playlist)", "fr": u"\n(hors playlist)", "pt": u"\n(fora da playlist)",
        "es": u"\n(fuera de lista)", "it": u"\n(non in playlist)", "de": u"\n(nicht in Playlist)",
        "pl": u"\n(brak na liście)", "cs": u"\n(není v playlistu)", "tr": u"\n(listede yok)",
        "he": u"\n(לא בפלייליסט)", "ar": u"\n(ليس بالقائمة)", "ja": u"\n(リスト外)",
        "zh": u"\n(不在列表中)", "th": u"\n(ไม่อยู่ในเพลย์ลิสต์)",
    },
    u" · нет в плейлисте": {
        "en": u" · not in playlist", "fr": u" · hors playlist", "pt": u" · fora da playlist",
        "es": u" · fuera de lista", "it": u" · non in playlist", "de": u" · nicht in Playlist",
        "pl": u" · brak na liście", "cs": u" · není v playlistu", "tr": u" · listede yok",
        "he": u" · לא בפלייליסט", "ar": u" · ليس بالقائمة", "ja": u" · リスト外",
        "zh": u" · 不在列表中", "th": u" · ไม่อยู่ในเพลย์ลิสต์",
    },
    u"Архив ": {
        "en": u"Archive ", "fr": u"Archives ", "pt": u"Arquivo ", "es": u"Archivo ",
        "it": u"Archivio ", "de": u"Archiv ", "pl": u"Archiwum ", "cs": u"Archiv ",
        "tr": u"Arşiv ", "he": u"ארכיון ", "ar": u"أرشيف ", "ja": u"アーカイブ ",
        "zh": u"回看 ", "th": u"ย้อนหลัง ",
    },
    u"OK долго — убрать из избранного": {
        "en": u"Long OK — remove favorite", "fr": u"OK long — retirer favori",
        "pt": u"OK longo — remover favorito", "es": u"OK largo — quitar favorito",
        "it": u"OK lungo — rimuovi preferito", "de": u"Lang OK — aus Favoriten",
        "pl": u"Długie OK — usuń z ulub.", "cs": u"Dlouhé OK — odebrat z obl.",
        "tr": u"Uzun OK — favoriden çıkar", "he": u"OK ארוך — הסר ממועדפים",
        "ar": u"OK مطول — إزالة من المفضلة", "ja": u"OK長押し — お気に入り解除",
        "zh": u"长按 OK — 取消收藏", "th": u"กด OK ค้าง — ลบจากโปรด",
    },
    u"OK долго — в избранное": {
        "en": u"Long OK — add favorite", "fr": u"OK long — ajouter favori",
        "pt": u"OK longo — adicionar favorito", "es": u"OK largo — añadir a favoritos",
        "it": u"OK lungo — aggiungi preferito", "de": u"Lang OK — zu Favoriten",
        "pl": u"Długie OK — do ulubionych", "cs": u"Dlouhé OK — do oblíbených",
        "tr": u"Uzun OK — favoriye ekle", "he": u"OK ארוך — הוסף למועדפים",
        "ar": u"OK مطول — إضافة للمفضلة", "ja": u"OK長押し — お気に入り追加",
        "zh": u"长按 OK — 收藏", "th": u"กด OK ค้าง — เพิ่มในโปรด",
    },
    u"EPG — программа и архив": {
        "en": u"EPG — guide & archive", "fr": u"EPG — guide et archives", "pt": u"EPG — guia e arquivo",
        "es": u"EPG — guía y archivo", "it": u"EPG — guida e archivio", "de": u"EPG — Guide & Archiv",
        "pl": u"EPG — program i archiwum", "cs": u"EPG — program a archiv", "tr": u"EPG — rehber ve arşiv",
        "he": u"EPG — מדריך וארכיון", "ar": u"EPG — الدليل والأرشيف", "ja": u"EPG — 番組表・アーカイブ",
        "zh": u"EPG — 节目单与回看", "th": u"EPG — ผังรายการและย้อนหลัง",
    },
    u"EPG — программа": {
        "en": u"EPG — guide", "fr": u"EPG — guide TV", "pt": u"EPG — guia", "es": u"EPG — guía",
        "it": u"EPG — guida", "de": u"EPG — Programm", "pl": u"EPG — program", "cs": u"EPG — program",
        "tr": u"EPG — rehber", "he": u"EPG — מדריך", "ar": u"EPG — الدليل", "ja": u"EPG — 番組表",
        "zh": u"EPG — 节目单", "th": u"EPG — ผังรายการ",
    },
    u"OK — смотреть · ": {
        "en": u"OK — watch · ", "fr": u"OK — regarder · ", "pt": u"OK — ver · ", "es": u"OK — ver · ",
        "it": u"OK — guarda · ", "de": u"OK — ansehen · ", "pl": u"OK — oglądaj · ", "cs": u"OK — sledovat · ",
        "tr": u"OK — izle · ", "he": u"OK — צפה · ", "ar": u"OK — مشاهدة · ", "ja": u"OK — 視聴 · ",
        "zh": u"OK — 观看 · ", "th": u"OK — ดู · ",
    },
    u"Сейчас · ": {
        "en": u"Now · ", "fr": u"En cours · ", "pt": u"Agora · ", "es": u"Ahora · ",
        "it": u"Ora · ", "de": u"Jetzt · ", "pl": u"Teraz · ", "cs": u"Nyní · ",
        "tr": u"Şimdi · ", "he": u"עכשיו · ", "ar": u"الآن · ", "ja": u"放送中 · ",
        "zh": u"正在播出 · ", "th": u"กำลังออกอากาศ · ",
    },
    u"Программа не загрузилась": {
        "en": u"Guide failed to load", "fr": u"Échec du chargement du guide", "pt": u"Falha ao carregar guia",
        "es": u"No se pudo cargar la guía", "it": u"Guida non caricata", "de": u"Programm nicht geladen",
        "pl": u"Nie pobrano programu", "cs": u"Program se nenačetl", "tr": u"Rehber yüklenemedi",
        "he": u"המדריך לא נטען", "ar": u"تعذر تحميل الدليل", "ja": u"番組表の読込に失敗",
        "zh": u"节目单加载失败", "th": u"โหลดผังรายการไม่สำเร็จ",
    },
    u"Нет программы": {
        "en": u"No guide", "fr": u"Aucun guide", "pt": u"Sem guia", "es": u"Sin guía",
        "it": u"Nessuna guida", "de": u"Kein Programm", "pl": u"Brak programu", "cs": u"Bez programu",
        "tr": u"Rehber yok", "he": u"אין מדריך", "ar": u"لا يوجد دليل", "ja": u"番組情報なし",
        "zh": u"暂无节目单", "th": u"ไม่มีผังรายการ",
    },
    u"Далее · ": {
        "en": u"Next · ", "fr": u"Ensuite · ", "pt": u"A seguir · ", "es": u"Siguiente · ",
        "it": u"Poi · ", "de": u"Danach · ", "pl": u"Następnie · ", "cs": u"Následuje · ",
        "tr": u"Sonra · ", "he": u"הבא · ", "ar": u"التالي · ", "ja": u"次 · ",
        "zh": u"稍后 · ", "th": u"ถัดไป · ",
    },
    u"OK — выбрать · Back — отмена": {
        "en": u"OK — select · Back — cancel", "fr": u"OK — choisir · Back — annuler",
        "pt": u"OK — escolher · Back — cancelar", "es": u"OK — elegir · Back — cancelar",
        "it": u"OK — scegli · Back — annulla", "de": u"OK — wählen · Back — abbrechen",
        "pl": u"OK — wybierz · Back — anuluj", "cs": u"OK — vybrat · Back — zrušit",
        "tr": u"OK — seç · Back — iptal", "he": u"OK — בחר · Back — ביטול",
        "ar": u"OK — اختيار · Back — إلغاء", "ja": u"OK — 選択 · Back — キャンセル",
        "zh": u"OK — 选择 · 返回键 — 取消", "th": u"OK — เลือก · Back — ยกเลิก",
    },
    u"Смотреть": {
        "en": u"Watch", "fr": u"Regarder", "pt": u"Ver", "es": u"Ver",
        "it": u"Guarda", "de": u"Ansehen", "pl": u"Oglądaj", "cs": u"Přehrát",
        "tr": u"İzle", "he": u"צפה", "ar": u"مشاهدة", "ja": u"再生",
        "zh": u"观看", "th": u"รับชม",
    },
    u"Описание": {
        "en": u"Overview", "fr": u"Résumé", "pt": u"Sinopse", "es": u"Sinopsis",
        "it": u"Trama", "de": u"Info", "pl": u"Opis", "cs": u"Popis",
        "tr": u"Özet", "he": u"תקציר", "ar": u"الوصف", "ja": u"概要",
        "zh": u"简介", "th": u"รายละเอียด",
    },
    u"В избранное": {
        "en": u"Favorite", "fr": u"Favori", "pt": u"Favorito", "es": u"Favorito",
        "it": u"Preferito", "de": u"Favorit", "pl": u"Ulubione", "cs": u"Oblíbené",
        "tr": u"Favori", "he": u"למועדפים", "ar": u"مفضلة", "ja": u"お気に入り",
        "zh": u"收藏", "th": u"เพิ่มโปรด",
    },
    u"Сезоны": {
        "en": u"Seasons", "fr": u"Saisons", "pt": u"Temporadas", "es": u"Temporadas",
        "it": u"Stagioni", "de": u"Staffeln", "pl": u"Sezony", "cs": u"Série",
        "tr": u"Sezonlar", "he": u"עונות", "ar": u"المواسم", "ja": u"シーズン",
        "zh": u"季", "th": u"ซีซัน",
    },
    u"Загружаю сезоны…": {
        "en": u"Loading seasons…", "fr": u"Chargement des saisons…", "pt": u"A carregar temporadas…",
        "es": u"Cargando temporadas…", "it": u"Caricamento stagioni…", "de": u"Lade Staffeln…",
        "pl": u"Ładowanie sezonów…", "cs": u"Načítám série…", "tr": u"Sezonlar yükleniyor…",
        "he": u"טוען עונות…", "ar": u"جارٍ تحميل المواسم…", "ja": u"シーズンを読み込み中…",
        "zh": u"正在加载季列表…", "th": u"กำลังโหลดซีซัน…",
    },
    u"серия ≈ ": {
        "en": u"episode ≈ ", "fr": u"épisode ≈ ", "pt": u"episódio ≈ ", "es": u"episodio ≈ ",
        "it": u"episodio ≈ ", "de": u"Folge ≈ ", "pl": u"odcinek ≈ ", "cs": u"epizoda ≈ ",
        "tr": u"bölüm ≈ ", "he": u"פרק ≈ ", "ar": u"حلقة ≈ ", "ja": u"各話 ≈ ",
        "zh": u"单集 ≈ ", "th": u"ตอนละ ≈ ",
    },
    u"Создатели: ": {
        "en": u"Creators: ", "fr": u"Créateurs : ", "pt": u"Criadores: ", "es": u"Creadores: ",
        "it": u"Creatori: ", "de": u"Schöpfer: ", "pl": u"Twórcy: ", "cs": u"Tvůrci: ",
        "tr": u"Yaratıcılar: ", "he": u"יוצרים: ", "ar": u"المبدعون: ", "ja": u"制作: ",
        "zh": u"主创：", "th": u"ผู้สร้าง: ",
    },
    u"Режиссёр: ": {
        "en": u"Director: ", "fr": u"Réalisation : ", "pt": u"Realização: ", "es": u"Director: ",
        "it": u"Regia: ", "de": u"Regie: ", "pl": u"Reżyseria: ", "cs": u"Režie: ",
        "tr": u"Yönetmen: ", "he": u"במאי: ", "ar": u"المخرج: ", "ja": u"監督: ",
        "zh": u"导演：", "th": u"ผู้กำกับ: ",
    },
    u"В ролях: ": {
        "en": u"Cast: ", "fr": u"Distribution : ", "pt": u"Elenco: ", "es": u"Reparto: ",
        "it": u"Cast: ", "de": u"Besetzung: ", "pl": u"Obsada: ", "cs": u"Hrají: ",
        "tr": u"Oyuncular: ", "he": u"שחקנים: ", "ar": u"الطاقم: ", "ja": u"出演: ",
        "zh": u"主演：", "th": u"นักแสดง: ",
    },
    u"В ролях": {
        "en": u"Cast", "fr": u"Distribution", "pt": u"Elenco", "es": u"Reparto",
        "it": u"Cast", "de": u"Besetzung", "pl": u"Obsada", "cs": u"Obsazení",
        "tr": u"Oyuncular", "he": u"שחקנים", "ar": u"الطاقم", "ja": u"出演",
        "zh": u"演职员", "th": u"นักแสดง",
    },
    u"Описание и рейтинг TMDB": {
        "en": u"TMDB overview & rating", "fr": u"Résumé et note TMDB", "pt": u"Sinopse e nota TMDB",
        "es": u"Sinopsis y nota de TMDB", "it": u"Trama e voto TMDB", "de": u"TMDB-Info & Bewertung",
        "pl": u"Opis i ocena TMDB", "cs": u"Popis a hodnocení TMDB", "tr": u"TMDB özeti ve puanı",
        "he": u"תיאור ודירוג TMDB", "ar": u"وصف وتقييم TMDB", "ja": u"TMDB 概要・評価",
        "zh": u"TMDB 简介与评分", "th": u"เรื่องย่อและคะแนน TMDB",
    },
    u"Данные TMDB": {
        "en": u"TMDB data", "fr": u"Données TMDB", "pt": u"Dados TMDB", "es": u"Datos de TMDB",
        "it": u"Dati TMDB", "de": u"TMDB-Daten", "pl": u"Dane TMDB", "cs": u"Data TMDB",
        "tr": u"TMDB verileri", "he": u"נתוני TMDB", "ar": u"بيانات TMDB", "ja": u"TMDB データ",
        "zh": u"TMDB 数据", "th": u"ข้อมูล TMDB",
    },
    u"рейтинги IMDb и КП — по данным портала": {
        "en": u"IMDb ratings from portal", "fr": u"notes IMDb du portail", "pt": u"notas IMDb do portal",
        "es": u"notas IMDb del portal", "it": u"voti IMDb dal portale", "de": u"IMDb-Wertungen vom Portal",
        "pl": u"oceny IMDb z portalu", "cs": u"hodnocení IMDb z portálu", "tr": u"IMDb puanları portaldan",
        "he": u"דירוגי IMDb מהפורטל", "ar": u"تقييمات IMDb من البوابة", "ja": u"IMDb評価はポータル提供",
        "zh": u"IMDb 评分来自门户", "th": u"คะแนน IMDb จากพอร์ทัล",
    },
    u"видео — портал %s": {
        "en": u"video — portal %s", "fr": u"vidéo — portail %s", "pt": u"vídeo — portal %s",
        "es": u"vídeo — portal %s", "it": u"video — portale %s", "de": u"Video — Portal %s",
        "pl": u"wideo — portal %s", "cs": u"video — portál %s", "tr": u"video — portal %s",
        "he": u"וידאו — פורטל %s", "ar": u"فيديو — بوابة %s", "ja": u"映像 — ポータル %s",
        "zh": u"视频源 — 门户 %s", "th": u"วิดีโอ — พอร์ทัล %s",
    },
    u"Портал не отдал сезоны этого сериала": {
        "en": u"Portal returned no seasons for this series", "fr": u"Le portail n’a renvoyé aucune saison",
        "pt": u"O portal não devolveu temporadas", "es": u"El portal no devolvió temporadas",
        "it": u"Il portale non ha restituito stagioni", "de": u"Portal lieferte keine Staffeln",
        "pl": u"Portal nie zwrócił sezonów tego serialu", "cs": u"Portál nevrátil série tohoto seriálu",
        "tr": u"Portal bu dizi için sezon döndürmedi", "he": u"הפורטל לא החזיר עונות לסדרה זו",
        "ar": u"لم ترجع البوابة مواسم لهذا المسلسل", "ja": u"ポータルからシーズンを取得できませんでした",
        "zh": u"门户未返回该剧集的季列表", "th": u"พอร์ทัลไม่ส่งคืนข้อมูลซีซัน",
    },
    u"Сезон %s": {
        "en": u"Season %s", "fr": u"Saison %s", "pt": u"Temporada %s", "es": u"Temporada %s",
        "it": u"Stagione %s", "de": u"Staffel %s", "pl": u"Sezon %s", "cs": u"Série %s",
        "tr": u"Sezon %s", "he": u"עונה %s", "ar": u"الموسم %s", "ja": u"シーズン %s",
        "zh": u"第 %s 季", "th": u"ซีซัน %s",
    },
    u"Сезон %d": {
        "en": u"Season %d", "fr": u"Saison %d", "pt": u"Temporada %d", "es": u"Temporada %d",
        "it": u"Stagione %d", "de": u"Staffel %d", "pl": u"Sezon %d", "cs": u"Série %d",
        "tr": u"Sezon %d", "he": u"עונה %d", "ar": u"الموسم %d", "ja": u"シーズン %d",
        "zh": u"第 %d 季", "th": u"ซีซัน %d",
    },
    u"Загружаю серии…": {
        "en": u"Loading episodes…", "fr": u"Chargement des épisodes…", "pt": u"A carregar episódios…",
        "es": u"Cargando episodios…", "it": u"Caricamento episodi…", "de": u"Lade Folgen…",
        "pl": u"Ładowanie odcinków…", "cs": u"Načítám epizody…", "tr": u"Bölümler yükleniyor…",
        "he": u"טוען פרקים…", "ar": u"جارٍ تحميل الحلقات…", "ja": u"エピソードを読み込み中…",
        "zh": u"正在加载剧集…", "th": u"กำลังโหลดตอน…",
    },
    u"В этом сезоне нет серий": {
        "en": u"No episodes in this season", "fr": u"Aucun épisode dans cette saison",
        "pt": u"Sem episódios nesta temporada", "es": u"No hay episodios en esta temporada",
        "it": u"Nessun episodio in questa stagione", "de": u"Keine Folgen in dieser Staffel",
        "pl": u"Brak odcinków w tym sezonie", "cs": u"V této sérii nejsou epizody",
        "tr": u"Bu sezonda bölüm yok", "he": u"אין פרקים בעונה זו",
        "ar": u"لا توجد حلقات في هذا الموسم", "ja": u"このシーズンにはエピソードがありません",
        "zh": u"本季暂无剧集", "th": u"ไม่มีตอนในซีซันนี้",
    },
    u"убрать из избранного": {
        "en": u"remove favorite", "fr": u"retirer des favoris", "pt": u"remover dos favoritos",
        "es": u"quitar de favoritos", "it": u"rimuovi dai preferiti", "de": u"aus Favoriten entfernen",
        "pl": u"usuń z ulubionych", "cs": u"odebrat z oblíbených", "tr": u"favorilerden çıkar",
        "he": u"הסר ממועדפים", "ar": u"إزالة من المفضلة", "ja": u"お気に入り解除",
        "zh": u"取消收藏", "th": u"ลบจากรายการโปรด",
    },
    u"в избранное": {
        "en": u"add to favorites", "fr": u"ajouter aux favoris", "pt": u"adicionar aos favoritos",
        "es": u"añadir a favoritos", "it": u"aggiungi ai preferiti", "de": u"zu Favoriten hinzufügen",
        "pl": u"dodaj do ulubionych", "cs": u"přidat do oblíbených", "tr": u"favorilere ekle",
        "he": u"הוסף למועדפים", "ar": u"إضافة للمفضلة", "ja": u"お気に入り追加",
        "zh": u"加入收藏", "th": u"เพิ่มในรายการโปรด",
    },
    u"OK долго — %s · Back — назад": {
        "en": u"Long OK — %s · Back — return", "fr": u"OK long — %s · Back — retour",
        "pt": u"OK longo — %s · Back — voltar", "es": u"OK largo — %s · Back — volver",
        "it": u"OK lungo — %s · Back — indietro", "de": u"Lang OK — %s · Back — zurück",
        "pl": u"Długie OK — %s · Back — powrót", "cs": u"Dlouhé OK — %s · Back — zpět",
        "tr": u"Uzun OK — %s · Back — geri", "he": u"OK ארוך — %s · Back — חזרה",
        "ar": u"OK مطول — %s · Back — رجوع", "ja": u"OK長押し — %s · Back — 戻る",
        "zh": u"长按 OK — %s · 返回键 — 返回", "th": u"กด OK ค้าง — %s · Back — กลับ",
    },
    u"%s · OK — ещё раз": {
        "en": u"%s · OK — try again", "fr": u"%s · OK — réessayer", "pt": u"%s · OK — tentar de novo",
        "es": u"%s · OK — reintentar", "it": u"%s · OK — riprova", "de": u"%s · OK — erneut",
        "pl": u"%s · OK — spróbuj ponownie", "cs": u"%s · OK — znovu", "tr": u"%s · OK — tekrar",
        "he": u"%s · OK — נסה שוב", "ar": u"%s · OK — إعادة", "ja": u"%s · OK — 再試行",
        "zh": u"%s · OK — 重试", "th": u"%s · OK — ลองใหม่",
    },
    u"Не удалось открыть проигрыватель · OK — ещё раз": {
        "en": u"Could not open player · OK — try again", "fr": u"Impossible d’ouvrir le lecteur · OK — réessayer",
        "pt": u"Erro ao abrir leitor · OK — tentar de novo", "es": u"No se pudo abrir reproductor · OK — reintentar",
        "it": u"Errore lettore · OK — riprova", "de": u"Player-Fehler · OK — erneut",
        "pl": u"Błąd odtwarzacza · OK — spróbuj ponownie", "cs": u"Chyba přehrávače · OK — znovu",
        "tr": u"Oynatıcı açılamadı · OK — tekrar", "he": u"שגיאת נגן · OK — נסה שוב",
        "ar": u"تعذر فتح المشغل · OK — إعادة", "ja": u"プレーヤーを開けません · OK — 再試行",
        "zh": u"无法打开播放器 · OK — 重试", "th": u"เปิดเครื่องเล่นไม่ได้ · OK — ลองใหม่",
    },
    u"Видео не запустилось: портал не отдал поток · OK — ещё раз": {
        "en": u"Video did not start: no stream from portal · OK — try again",
        "fr": u"La vidéo n’a pas démarré : aucun flux · OK — réessayer",
        "pt": u"O vídeo não iniciou: sem fluxo do portal · OK — tentar de novo",
        "es": u"El vídeo no inició: sin flujo del portal · OK — reintentar",
        "it": u"Video non avviato: nessun flusso dal portale · OK — riprova",
        "de": u"Video nicht gestartet: kein Stream vom Portal · OK — erneut",
        "pl": u"Wideo nie ruszyło: brak strumienia · OK — ponownie",
        "cs": u"Video se nespustilo: portál nedal stream · OK — znovu",
        "tr": u"Video başlamadı: akış alınamadı · OK — tekrar",
        "he": u"הווידאו לא התחיל: אין שידור מהפורטל · OK — נסה שוב",
        "ar": u"لم يبدأ الفيديو: لا يوجد بث · OK — إعادة",
        "ja": u"再生できませんでした: ストリームなし · OK — 再試行",
        "zh": u"视频未能播放：门户未返回流 · OK — 重试",
        "th": u"วิดีโอไม่เริ่มเล่น: ไม่มีสตรีมจากพอร์ทัล · OK — ลองใหม่",
    },
    u"Пробел": {
        "en": u"Space", "fr": u"Espace", "pt": u"Espaço", "es": u"Espacio",
        "it": u"Spazio", "de": u"Leer", "pl": u"Spacja", "cs": u"Mezera",
        "tr": u"Boşluk", "he": u"רווח", "ar": u"مسافة", "ja": u"空白",
        "zh": u"空格", "th": u"เว้นวรรค",
    },
    u"Стереть": {
        "en": u"Delete", "fr": u"Effacer", "pt": u"Apagar", "es": u"Borrar",
        "it": u"Canc.", "de": u"Löschen", "pl": u"Usuń", "cs": u"Smazat",
        "tr": u"Sil", "he": u"מחק", "ar": u"حذف", "ja": u"削除",
        "zh": u"退格", "th": u"ลบ",
    },
    u"Название…": {
        "en": u"Title…", "fr": u"Titre…", "pt": u"Título…", "es": u"Título…",
        "it": u"Titolo…", "de": u"Titel…", "pl": u"Tytuł…", "cs": u"Název…",
        "tr": u"Başlık…", "he": u"שם…", "ar": u"العنوان…", "ja": u"タイトル…",
        "zh": u"输入名称…", "th": u"ชื่อเรื่อง…",
    },
    u"Наберите название": {
        "en": u"Enter a title", "fr": u"Saisissez un titre", "pt": u"Digite um título",
        "es": u"Escribe un título", "it": u"Inserisci un titolo", "de": u"Titel eingeben",
        "pl": u"Wpisz tytuł", "cs": u"Zadejte název", "tr": u"Bir başlık yazın",
        "he": u"הקלד שם", "ar": u"أدخل عنواناً", "ja": u"タイトルを入力",
        "zh": u"请输入名称", "th": u"พิมพ์ชื่อเพื่อค้นหา",
    },
    u"Ищем на выбранном портале, в разделе ниже": {
        "en": u"Searching selected portal in the section below",
        "fr": u"Recherche sur le portail choisi dans la section ci-dessous",
        "pt": u"Pesquisa no portal selecionado na secção abaixo",
        "es": u"Busca en el portal elegido en la sección de abajo",
        "it": u"Ricerca sul portale nella sezione in basso",
        "de": u"Suche im gewählten Portal und Bereich unten",
        "pl": u"Wyszukiwanie w wybranym portalu i sekcji poniżej",
        "cs": u"Hledá se na vybraném portálu v sekci níže",
        "tr": u"Seçili portalda aşağıdaki bölümde aranır",
        "he": u"מחפש בפורטל הנבחר במדור למטה",
        "ar": u"البحث في البوابة المحددة بالقسم أدناه",
        "ja": u"下の区分で選択中のポータルを検索します",
        "zh": u"将在所选门户的下方栏目中搜索",
        "th": u"ค้นหาในพอร์ทัลที่เลือกในหมวดด้านล่าง",
    },
    u"Введите хотя бы две буквы": {
        "en": u"Enter at least two characters", "fr": u"Saisissez au moins deux lettres",
        "pt": u"Digite pelo menos duas letras", "es": u"Escribe al menos dos letras",
        "it": u"Inserisci almeno due lettere", "de": u"Mindestens zwei Zeichen eingeben",
        "pl": u"Wpisz co najmniej dwa znaki", "cs": u"Zadejte alespoň dva znaky",
        "tr": u"En az iki harf girin", "he": u"הזן לפחות שני תווים",
        "ar": u"أدخل حرفين على الأقل", "ja": u"2文字以上入力してください",
        "zh": u"请至少输入两个字符", "th": u"ป้อนอย่างน้อย 2 ตัวอักษร",
    },
    u"Ищу…": {
        "en": u"Searching…", "fr": u"Recherche…", "pt": u"A procurar…", "es": u"Buscando…",
        "it": u"Ricerca…", "de": u"Suche…", "pl": u"Szukam…", "cs": u"Hledám…",
        "tr": u"Aranıyor…", "he": u"מחפש…", "ar": u"جارٍ البحث…", "ja": u"検索中…",
        "zh": u"搜索中…", "th": u"กำลังค้นหา…",
    },
    u"Ничего не найдено по «%s»": {
        "en": u"Nothing found for “%s”", "fr": u"Aucun résultat pour « %s »", "pt": u"Nada encontrado para «%s»",
        "es": u"Nada encontrado para «%s»", "it": u"Nessun risultato per «%s»", "de": u"Nichts gefunden für „%s“",
        "pl": u"Nic nie znaleziono dla „%s”", "cs": u"Nic nenalezeno pro „%s“", "tr": u"«%s» için sonuç yok",
        "he": u"לא נמצאו תוצאות עבור «%s»", "ar": u"لا توجد نتائج لـ «%s»", "ja": u"「%s」の検索結果なし",
        "zh": u"未找到与“%s”相关的内容", "th": u"ไม่พบผลลัพธ์สำหรับ “%s”",
    },
    u"Найдено каналов: %d%s": {
        "en": u"Channels found: %d%s", "fr": u"Chaînes trouvées : %d%s", "pt": u"Canais encontrados: %d%s",
        "es": u"Canales encontrados: %d%s", "it": u"Canali trovati: %d%s", "de": u"Gefundene Kanäle: %d%s",
        "pl": u"Znaleziono kanałów: %d%s", "cs": u"Nalezeno kanálů: %d%s", "tr": u"Bulunan kanal: %d%s",
        "he": u"ערוצים שנמצאו: %d%s", "ar": u"القنوات الموجودة: %d%s", "ja": u"見つかったチャンネル: %d%s",
        "zh": u"找到频道：%d%s", "th": u"พบช่อง: %d%s",
    },
    u"Найдено: %d%s": {
        "en": u"Found: %d%s", "fr": u"Trouvés : %d%s", "pt": u"Encontrados: %d%s", "es": u"Encontrados: %d%s",
        "it": u"Trovati: %d%s", "de": u"Gefunden: %d%s", "pl": u"Znaleziono: %d%s", "cs": u"Nalezeno: %d%s",
        "tr": u"Bulunan: %d%s", "he": u"נמצאו: %d%s", "ar": u"النتائج: %d%s", "ja": u"検索結果: %d%s",
        "zh": u"找到：%d%s", "th": u"พบ: %d%s",
    },
    u"Поиск на сервере портала": {
        "en": u"Server-side portal search", "fr": u"Recherche sur le serveur du portail",
        "pt": u"Pesquisa no servidor do portal", "es": u"Búsqueda en el servidor del portal",
        "it": u"Ricerca sul server del portale", "de": u"Suche auf dem Portal-Server",
        "pl": u"Wyszukiwanie na serwerze portalu", "cs": u"Vyhledávání na serveru portálu",
        "tr": u"Portal sunucusunda arama", "he": u"חיפוש בשרת הפורטל",
        "ar": u"بحث على خادم البوابة", "ja": u"ポータルサーバー検索",
        "zh": u"门户服务器端搜索", "th": u"ค้นหาบนเซิร์ฟเวอร์พอร์ทัล",
    },
    u"Поиск по всем телеканалам портала": {
        "en": u"Search across all live channels", "fr": u"Recherche sur toutes les chaînes TV",
        "pt": u"Pesquisa em todos os canais de TV", "es": u"Búsqueda en todos los canales de TV",
        "it": u"Ricerca su tutti i canali TV", "de": u"Suche über alle TV-Kanäle",
        "pl": u"Wyszukiwanie wśród wszystkich kanałów", "cs": u"Hledání ve všech TV kanálech",
        "tr": u"Tüm TV kanallarında arama", "he": u"חיפוש בכל ערוצי הטלוויזיה",
        "ar": u"بحث في جميع القنوات", "ja": u"全テレビチャンネルを検索",
        "zh": u"搜索全部电视频道", "th": u"ค้นหาในช่องทีวีทั้งหมด",
    },
    u"Портал не умеет искать — проверены только открытые страницы (%d)": {
        "en": u"Portal search unsupported — checked opened pages only (%d)",
        "fr": u"Recherche portail indisponible — pages déjà ouvertes vérifiées (%d)",
        "pt": u"Portal sem pesquisa — verificadas apenas páginas abertas (%d)",
        "es": u"El portal no busca — revisadas solo páginas abiertas (%d)",
        "it": u"Ricerca portale non supportata — controllate solo pagine aperte (%d)",
        "de": u"Portal unterstützt keine Suche — nur geladene Seiten geprüft (%d)",
        "pl": u"Portal nie wspiera szukania — sprawdzono otwarte strony (%d)",
        "cs": u"Portál nepodporuje hledání — ověřeny jen otevřené stránky (%d)",
        "tr": u"Portal arama desteklemiyor — yalnızca açılan sayfalar tarandı (%d)",
        "he": u"הפורטל לא תומך בחיפוש — נבדקו רק דפים פתוחים (%d)",
        "ar": u"البوابة لا تدعم البحث — تم فحص الصفحات المفتوحة فقط (%d)",
        "ja": u"ポータル検索非対応 — 読込済みページのみ検索 (%d)",
        "zh": u"门户不支持搜索 — 仅检查已打开页面 (%d)",
        "th": u"พอร์ทัลไม่รองรับการค้นหา — ตรวจเฉพาะหน้าที่เปิดแล้ว (%d)",
    },
    u"Источник не знает, что идёт на этом канале": {
        "en": u"No programme information from source", "fr": u"Aucune info programme de la source",
        "pt": u"A fonte não tem programação para este canal", "es": u"La fuente no tiene guía para este canal",
        "it": u"Nessuna informazione guida dalla fonte", "de": u"Quelle hat keine Programmdaten",
        "pl": u"Źródło nie ma programu dla tego kanału", "cs": u"Zdroj nemá program pro tento kanál",
        "tr": u"Kaynakta bu kanal için rehber yok", "he": u"אין נתוני שידורים לערוץ זה",
        "ar": u"لا تتوفر معلومات برامج لهذه القناة", "ja": u"このチャンネルの番組情報はありません",
        "zh": u"源没有该频道的节目单数据", "th": u"แหล่งที่มาไม่มีข้อมูลรายการของช่องนี้",
    },
    u"На этот день программы нет": {
        "en": u"No guide for this day", "fr": u"Aucun programme pour ce jour",
        "pt": u"Sem programação para este dia", "es": u"Sin programación para este día",
        "it": u"Nessun programma per questo giorno", "de": u"Kein Programm für diesen Tag",
        "pl": u"Brak programu na ten dzień", "cs": u"Pro tento den není program",
        "tr": u"Bu gün için program yok", "he": u"אין תוכניות ליום זה",
        "ar": u"لا توجد برامج لهذا اليوم", "ja": u"この日の番組情報はありません",
        "zh": u"这一天没有节目单", "th": u"ไม่มีรายการสำหรับวันนี้",
    },
    u"‹ › или CH± — другие дни": {
        "en": u"‹ › or CH± — other days", "fr": u"‹ › ou CH± — autres jours",
        "pt": u"‹ › ou CH± — outros dias", "es": u"‹ › o CH± — otros días",
        "it": u"‹ › o CH± — altri giorni", "de": u"‹ › oder CH± — andere Tage",
        "pl": u"‹ › lub CH± — inne dni", "cs": u"‹ › nebo CH± — jiné dny",
        "tr": u"‹ › veya CH± — diğer günler", "he": u"‹ › או CH± — ימים אחרים",
        "ar": u"‹ › أو CH± — أيام أخرى", "ja": u"‹ › / CH± — 他の日",
        "zh": u"‹ › 或 CH± — 切换日期", "th": u"‹ › หรือ CH± — วันอื่น",
    },
    u"Список каналов обновился": {
        "en": u"Channel list refreshed", "fr": u"Liste des chaînes mise à jour",
        "pt": u"Lista de canais atualizada", "es": u"Lista de canales actualizada",
        "it": u"Lista canali aggiornata", "de": u"Kanalliste aktualisiert",
        "pl": u"Lista kanałów została odświeżona", "cs": u"Seznam kanálů byl aktualizován",
        "tr": u"Kanal listesi yenilendi", "he": u"רשימת הערוצים התעדכנה",
        "ar": u"تم تحديث قائمة القنوات", "ja": u"チャンネルリストが更新されました",
        "zh": u"频道列表已更新", "th": u"รายการช่องอัปเดตแล้ว",
    },
    u"Back — вернуться к списку": {
        "en": u"Back — return to list", "fr": u"Back — retour à la liste",
        "pt": u"Back — voltar à lista", "es": u"Back — volver a la lista",
        "it": u"Back — torna alla lista", "de": u"Back — zurück zur Liste",
        "pl": u"Back — powrót do listy", "cs": u"Back — zpět na seznam",
        "tr": u"Back — listeye dön", "he": u"Back — חזרה לרשימה",
        "ar": u"Back — العودة للقائمة", "ja": u"Back — リストへ戻る",
        "zh": u"返回键 — 返回列表", "th": u"Back — กลับไปที่รายการ",
    },
    u"‹ › или CH± — дни": {
        "en": u"‹ › or CH± — days", "fr": u"‹ › ou CH± — jours",
        "pt": u"‹ › ou CH± — dias", "es": u"‹ › o CH± — días",
        "it": u"‹ › o CH± — giorni", "de": u"‹ › oder CH± — Tage",
        "pl": u"‹ › lub CH± — dni", "cs": u"‹ › nebo CH± — dny",
        "tr": u"‹ › veya CH± — günler", "he": u"‹ › או CH± — ימים",
        "ar": u"‹ › أو CH± — الأيام", "ja": u"‹ › / CH± — 日付",
        "zh": u"‹ › 或 CH± — 日期", "th": u"‹ › หรือ CH± — วัน",
    },
    u"OK — смотреть запись · ": {
        "en": u"OK — watch recording · ", "fr": u"OK — voir l’enregistrement · ",
        "pt": u"OK — ver gravação · ", "es": u"OK — ver grabación · ",
        "it": u"OK — guarda registrazione · ", "de": u"OK — Aufnahme ansehen · ",
        "pl": u"OK — oglądaj nagranie · ", "cs": u"OK — přehrát záznam · ",
        "tr": u"OK — kaydı izle · ", "he": u"OK — צפה בהקלטה · ",
        "ar": u"OK — مشاهدة التسجيل · ", "ja": u"OK — 録画再生 · ",
        "zh": u"OK — 播放回看 · ", "th": u"OK — ดูรายการย้อนหลัง · ",
    },
    u" · Back — назад": {
        "en": u" · Back — return", "fr": u" · Back — retour",
        "pt": u" · Back — voltar", "es": u" · Back — volver",
        "it": u" · Back — indietro", "de": u" · Back — zurück",
        "pl": u" · Back — powrót", "cs": u" · Back — zpět",
        "tr": u" · Back — geri", "he": u" · Back — חזרה",
        "ar": u" · Back — رجوع", "ja": u" · Back — 戻る",
        "zh": u" · 返回键 — 返回", "th": u" · Back — กลับ",
    },
    u"OK — с начала": {
        "en": u"OK — from start", "fr": u"OK — depuis le début", "pt": u"OK — do início",
        "es": u"OK — desde el inicio", "it": u"OK — dall’inizio", "de": u"OK — von Anfang",
        "pl": u"OK — od początku", "cs": u"OK — od začátku", "tr": u"OK — baştan izle",
        "he": u"OK — מההתחלה", "ar": u"OK — من البداية", "ja": u"OK — 最初から",
        "zh": u"OK — 从头观看", "th": u"OK — ดูตั้งแต่ต้น",
    },
    u"OK — смотреть канал · Красная — с начала": {
        "en": u"OK — watch live · Red — from start",
        "fr": u"OK — regarder · Rouge — depuis le début",
        "pt": u"OK — ver canal · Vermelho — do início",
        "es": u"OK — ver canal · Rojo — desde el inicio",
        "it": u"OK — guarda canale · Rosso — dall’inizio",
        "de": u"OK — Kanal · Rot — von Anfang",
        "pl": u"OK — oglądaj · Czerwony — od początku",
        "cs": u"OK — sledovat · Červené — od začátku",
        "tr": u"OK — izle · Kırmızı — baştan izle",
        "he": u"OK — צפה · אדום — מההתחלה",
        "ar": u"OK — بث مباشر · الأحمر — من البداية",
        "ja": u"OK — ライブ視聴 · 赤 — 最初から",
        "zh": u"OK — 观看直播 · 红键 — 从头回看",
        "th": u"OK — ดูสด · สีแดง — ตั้งแต่ต้น",
    },
    u"↑↓ — передачи · ": {
        "en": u"↑↓ — programmes · ", "fr": u"↑↓ — programmes · ", "pt": u"↑↓ — programas · ",
        "es": u"↑↓ — programas · ", "it": u"↑↓ — programmi · ", "de": u"↑↓ — Sendungen · ",
        "pl": u"↑↓ — audycje · ", "cs": u"↑↓ — pořady · ", "tr": u"↑↓ — programlar · ",
        "he": u"↑↓ — תוכניות · ", "ar": u"↑↓ — البرامج · ", "ja": u"↑↓ — 番組 · ",
        "zh": u"↑↓ — 节目 · ", "th": u"↑↓ — รายการ · ",
    },
    u" · OK — смотреть канал · Back — назад": {
        "en": u" · OK — watch channel · Back — return", "fr": u" · OK — regarder · Back — retour",
        "pt": u" · OK — ver canal · Back — voltar", "es": u" · OK — ver canal · Back — volver",
        "it": u" · OK — guarda canale · Back — indietro", "de": u" · OK — Kanal · Back — zurück",
        "pl": u" · OK — oglądaj · Back — powrót", "cs": u" · OK — sledovat · Back — zpět",
        "tr": u" · OK — izle · Back — geri", "he": u" · OK — צפה · Back — חזרה",
        "ar": u" · OK — مشاهدة · Back — رجوع", "ja": u" · OK — 視聴 · Back — 戻る",
        "zh": u" · OK — 观看频道 · 返回键 — 返回", "th": u" · OK — ดูช่อง · Back — กลับ",
    },
    u"Запись · ": {
        "en": u"Recorded · ", "fr": u"Enregistré · ", "pt": u"Gravado · ", "es": u"Grabado · ",
        "it": u"Registrato · ", "de": u"Aufnahme · ", "pl": u"Nagranie · ", "cs": u"Záznam · ",
        "tr": u"Kayıt · ", "he": u"מוקלט · ", "ar": u"مسجل · ", "ja": u"録画あり · ",
        "zh": u"可回看 · ", "th": u"บันทึกแล้ว · ",
    },
    u"Нет в архиве · ": {
        "en": u"Not in archive · ", "fr": u"Hors archives · ", "pt": u"Fora do arquivo · ", "es": u"Sin archivo · ",
        "it": u"Non in archivio · ", "de": u"Nicht im Archiv · ", "pl": u"Brak w archiwum · ", "cs": u"Není v archivu · ",
        "tr": u"Arşivde yok · ", "he": u"לא בארכיון · ", "ar": u"غير متاح بالأرشيف · ", "ja": u"アーカイブ外 · ",
        "zh": u"无回看 · ", "th": u"ไม่มีในย้อนหลัง · ",
    },
    u"Дальше программы нет": {
        "en": u"No later guide data", "fr": u"Pas de programme après", "pt": u"Sem programação depois",
        "es": u"No hay más guía después", "it": u"Nessun programma successivo", "de": u"Keine späteren Daten",
        "pl": u"Brak dalszego programu", "cs": u"Další program není", "tr": u"Sonrası için rehber yok",
        "he": u"אין נתונים מאוחרים יותר", "ar": u"لا توجد برامج لاحقة", "ja": u"これ以降の番組表はありません",
        "zh": u"之后没有节目单", "th": u"ไม่มีข้อมูลวันถัดไป",
    },
    u"Раньше программы нет": {
        "en": u"No earlier guide data", "fr": u"Pas de programme avant", "pt": u"Sem programação antes",
        "es": u"No hay guía anterior", "it": u"Nessun programma precedente", "de": u"Keine früheren Daten",
        "pl": u"Brak wcześniejszego programu", "cs": u"Dřívější program není", "tr": u"Öncesi için rehber yok",
        "he": u"אין נתונים מוקדמים יותר", "ar": u"لا توجد برامج سابقة", "ja": u"これ以前の番組表はありません",
        "zh": u"之前没有节目单", "th": u"ไม่มีข้อมูลวันก่อนหน้า",
    },
    u"Эта передача ещё не началась": {
        "en": u"This programme has not started yet", "fr": u"Ce programme n’a pas encore commencé",
        "pt": u"Este programa ainda não começou", "es": u"Este programa aún no ha empezado",
        "it": u"Questo programma non è ancora iniziato", "de": u"Diese Sendung hat noch nicht begonnen",
        "pl": u"Ta audycja jeszcze się nie zaczęła", "cs": u"Tento pořad ještě nezačal",
        "tr": u"Bu program henüz başlamadı", "he": u"התוכנית טרם התחילה",
        "ar": u"لم يبدأ هذا البرنامج بعد", "ja": u"この番組はまだ始まっていません",
        "zh": u"该节目尚未开始", "th": u"รายการนี้ยังไม่เริ่ม",
    },
    u"Этой передачи нет в архиве": {
        "en": u"This programme is not in the archive", "fr": u"Ce programme n’est pas dans les archives",
        "pt": u"Este programa não está no arquivo", "es": u"Este programa no está en el archivo",
        "it": u"Programma non presente in archivio", "de": u"Sendung nicht im Archiv",
        "pl": u"Tej audycji nie ma w archiwum", "cs": u"Tento pořad není v archivu",
        "tr": u"Bu program arşivde yok", "he": u"תוכנית זו אינה בארכיון",
        "ar": u"هذا البرنامج غير موجود في الأرشيف", "ja": u"この番組はアーカイブにありません",
        "zh": u"该节目不在回看中", "th": u"รายการนี้ไม่อยู่ในระบบย้อนหลัง",
    },
    u"Провайдер хранит записи %s": {
        "en": u"Provider keeps recordings for %s", "fr": u"Le fournisseur garde les rediffusions %s",
        "pt": u"O fornecedor guarda gravações por %s", "es": u"El proveedor guarda grabaciones %s",
        "it": u"Il provider conserva le registrazioni per %s", "de": u"Anbieter speichert Aufnahmen für %s",
        "pl": u"Dostawca przechowuje nagrania przez %s", "cs": u"Poskytovatel uchovává záznamy %s",
        "tr": u"Sağlayıcı kayıtları %s saklar", "he": u"הספק שומר הקלטות למשך %s",
        "ar": u"يحتفظ المزود بالتسجيلات لمدة %s", "ja": u"プロバイダの録画保持期間: %s",
        "zh": u"提供商保留录像 %s", "th": u"ผู้ให้บริการเก็บย้อนหลัง %s",
    },
    u"Подготавливаю запись…": {
        "en": u"Preparing recording…", "fr": u"Préparation de l’enregistrement…",
        "pt": u"A preparar gravação…", "es": u"Preparando grabación…",
        "it": u"Preparazione registrazione…", "de": u"Bereite Aufnahme vor…",
        "pl": u"Przygotowuję nagranie…", "cs": u"Připravuji záznam…",
        "tr": u"Kayıt hazırlanıyor…", "he": u"מכין הקלטה…",
        "ar": u"جارٍ تحضير التسجيل…", "ja": u"録画を準備中…",
        "zh": u"正在准备回看录像…", "th": u"กำลังเตรียมรายการย้อนหลัง…",
    },
    u"Жёлтая — обновить программу": {
        "en": u"Yellow — refresh guide", "fr": u"Jaune — actualiser le guide",
        "pt": u"Amarelo — atualizar guia", "es": u"Amarillo — actualizar guía",
        "it": u"Giallo — aggiorna guida", "de": u"Gelb — Guide aktualisieren",
        "pl": u"Żółty — odśwież program", "cs": u"Žluté — obnovit program",
        "tr": u"Sarı — rehberi yenile", "he": u"צהוב — רענן מדריך",
        "ar": u"الأصفر — تحديث الدليل", "ja": u"黄 — 番組表を更新",
        "zh": u"黄键 — 刷新节目单", "th": u"สีเหลือง — รีเฟรชผังรายการ",
    },
    u"Запись не отвечает": {
        "en": u"Recording not responding", "fr": u"L’enregistrement ne répond pas",
        "pt": u"Gravação sem resposta", "es": u"La grabación no responde",
        "it": u"La registrazione non risponde", "de": u"Aufnahme antwortet nicht",
        "pl": u"Nagranie nie odpowiada", "cs": u"Záznam neodpovídá",
        "tr": u"Kayıt yanıt vermiyor", "he": u"ההקלטה אינה מגיבה",
        "ar": u"التسجيل لا يستجيب", "ja": u"録画が応答しません",
        "zh": u"回看录像无响应", "th": u"รายการย้อนหลังไม่ตอบสนอง",
    },
    u"Провайдер не отдал запись — попробуйте позже": {
        "en": u"Provider returned no recording — try again later",
        "fr": u"Le fournisseur n’a pas renvoyé la vidéo — réessayez plus tard",
        "pt": u"O fornecedor não enviou a gravação — tente mais tarde",
        "es": u"El proveedor no entregó la grabación — prueba más tarde",
        "it": u"Il provider non ha restituito la registrazione — riprova più tardi",
        "de": u"Anbieter lieferte keine Aufnahme — später erneut versuchen",
        "pl": u"Dostawca nie zwrócił nagrania — spróbuj później",
        "cs": u"Poskytovatel neposkytl záznam — zkuste později",
        "tr": u"Sağlayıcı kaydı vermedi — daha sonra deneyin",
        "he": u"הספק לא החזיר הקלטה — נסה מאוחר יותר",
        "ar": u"لم يرجع المزود التسجيل — جرب لاحقاً",
        "ja": u"プロバイダが録画を返しませんでした — 後でもう一度お試しください",
        "zh": u"提供商未返回录像 — 请稍后再试",
        "th": u"ผู้ให้บริการไม่ส่งรายการย้อนหลัง — ลองใหม่ภายหลัง",
    },
    u"Продукт использует TMDB API, но не одобрен и не сертифицирован TMDB.": {
        "en": u"",
        "fr": u"Ce produit utilise l’API TMDB mais n’est ni approuvé ni certifié par TMDB.",
        "pt": u"Este produto usa a API TMDB mas não é endossado nem certificado pela TMDB.",
        "es": u"Este producto usa la API de TMDB pero no está respaldado ni certificado por TMDB.",
        "it": u"Questo prodotto utilizza l’API TMDB ma non è approvato né certificato da TMDB.",
        "de": u"Dieses Produkt nutzt die TMDB-API, ist aber nicht von TMDB unterstützt oder zertifiziert.",
        "pl": u"Ten produkt używa API TMDB, ale nie jest zatwierdzony ani certyfikowany przez TMDB.",
        "cs": u"Tento produkt používá API TMDB, ale není schválen ani certifikován TMDB.",
        "tr": u"Bu ürün TMDB API kullanır ancak TMDB tarafından onaylanmamış veya sertifikalanmamıştır.",
        "he": u"מוצר זה משתמש ב-API של TMDB אך אינו מאושר או מוסמך על ידי TMDB.",
        "ar": u"يستخدم هذا المنتج واجهة TMDB ولكنه غير معتمد أو مصدق من قبل TMDB.",
        "ja": u"本製品はTMDB APIを使用していますが、TMDBの承認または認定を受けたものではありません。",
        "zh": u"本产品使用 TMDB API，但未获得 TMDB 认可或认证。",
        "th": u"ผลิตภัณฑ์นี้ใช้ TMDB API แต่ไม่ได้รับการรับรองจาก TMDB",
    },
    u"Dreaminator — каталог IPTV и списки каналов для Enigma2": {
        "en": u"Dreaminator — IPTV catalog and channel lists for Enigma2",
        "fr": u"Dreaminator — catalogue IPTV et listes de chaînes pour Enigma2",
        "pt": u"Dreaminator — catálogo IPTV e listas de canais para Enigma2",
        "es": u"Dreaminator — catálogo IPTV y listas de canales para Enigma2",
        "it": u"Dreaminator — catalogo IPTV e liste canali per Enigma2",
        "de": u"Dreaminator — IPTV-Katalog und Kanallisten für Enigma2",
        "pl": u"Dreaminator — katalog IPTV i listy kanałów dla Enigma2",
        "cs": u"Dreaminator — katalog IPTV a seznamy kanálů pro Enigma2",
        "tr": u"Dreaminator — Enigma2 için IPTV kataloğu ve kanal listeleri",
        "he": u"Dreaminator — קטלוג IPTV ורשימות ערוצים ל-Enigma2",
        "ar": u"Dreaminator — كتالوج IPTV وقوائم القنوات لـ Enigma2",
        "ja": u"Dreaminator — Enigma2 用 IPTV カタログ・チャンネルリスト",
        "zh": u"Dreaminator — 适用于 Enigma2 的 IPTV 目录与频道列表",
        "th": u"Dreaminator — แคตตาล็อก IPTV และรายการช่องสำหรับ Enigma2",
    },
    u"Источники и списки настраиваются с планшета: адрес и QR-код — в «Настройках» боковой панели.": {
        "en": u"Configure sources and lists from your tablet or browser: URL and QR code are in Settings.",
        "fr": u"Configurez sources et listes depuis le navigateur : URL et QR code sont dans Paramètres.",
        "pt": u"Configure fontes e listas no navegador: o endereço e o QR estão em Definições.",
        "es": u"Configura fuentes y listas desde el navegador: la dirección y el QR están en Ajustes.",
        "it": u"Configura fonti e liste dal browser: indirizzo e QR sono in Impostazioni.",
        "de": u"Quellen und Listen werden im Browser eingerichtet: Adresse und QR-Code unter Einstellungen.",
        "pl": u"Źródła i listy konfiguruje się w przeglądarce: adres i kod QR są w Ustawieniach.",
        "cs": u"Zdroje a seznamy nastavíte v prohlížeči: adresa a QR kód jsou v Nastavení.",
        "tr": u"Kaynakları ve listeleri tarayıcıdan ayarlayın: adres ve QR kod Ayarlar’da.",
        "he": u"הגדר מקורות ורשימות מהדפדפן: הכתובת וקוד ה-QR נמצאים בהגדרות.",
        "ar": u"يتم إعداد المصادر والقوائم من المتصفح: العنوان ورمز QR في الإعدادات.",
        "ja": u"ソースやリストはブラウザから設定します：URLとQRコードは「設定」にあります。",
        "zh": u"请在浏览器中配置源和列表：网址和二维码在“设置”中。",
        "th": u"ตั้งค่าแหล่งที่มาและรายการผ่านเบราว์เซอร์: ที่อยู่และ QR โค้ดอยู่ในเมนูการตั้งค่า",
    },
    u"Описания и постеры TMDB": {
        "en": u"TMDB overviews & posters", "fr": u"Descriptions et affiches TMDB",
        "pt": u"Descrições e pósteres TMDB", "es": u"Descripciones y pósteres TMDB",
        "it": u"Descrizioni e poster TMDB", "de": u"TMDB-Infos & Poster",
        "pl": u"Opisy i plakaty TMDB", "cs": u"Popisky a plakáty TMDB",
        "tr": u"TMDB açıklamaları ve posterler", "he": u"תיאורים ופוסטרים TMDB",
        "ar": u"أوصاف وملصقات TMDB", "ja": u"TMDB 概要とポスター",
        "zh": u"TMDB 简介与海报", "th": u"คำอธิบายและโปสเตอร์ TMDB",
    },
    u"Данные TMDB только дополняют описание; видео и серии всегда берутся с портала.": {
        "en": u"TMDB only enriches metadata; video streams and episodes always come from your portal.",
        "fr": u"TMDB enrichit uniquement les fiches ; vidéos et épisodes viennent toujours du portail.",
        "pt": u"O TMDB apenas complementa metadados; vídeos e episódios vêm sempre do portal.",
        "es": u"TMDB solo completa la ficha; los vídeos y episodios siempre vienen del portal.",
        "it": u"TMDB integra solo le schede; video ed episodi provengono sempre dal portale.",
        "de": u"TMDB ergänzt nur Metadaten; Videos und Folgen kommen immer vom Portal.",
        "pl": u"Dane TMDB uzupełniają tylko opisy; wideo i odcinki zawsze pochodzą z portalu.",
        "cs": u"Data TMDB jen doplňují popisky; videa a epizody jsou vždy z portálu.",
        "tr": u"TMDB yalnızca açıklamaları tamamlar; videolar her zaman portaldan gelir.",
        "he": u"נתוני TMDB משלימים תיאורים בלבד; הווידאו והפרקים מגיעים תמיד מהפורטל.",
        "ar": u"بيانات TMDB تكمل المعلومات فقط؛ الفيديو والحلقات تأتي دائماً من البوابة.",
        "ja": u"TMDBは情報補完のみに使用され、動画とエピソードは常にポータルから取得されます。",
        "zh": u"TMDB 仅用于补充简介和海报；视频与剧集始终来自您的门户。",
        "th": u"ข้อมูล TMDB ใช้เสริมรายละเอียดเท่านั้น วิดีโอและตอนต่างๆ มาจากพอร์ทัลเสมอ",
    },
    u"Шрифты и данные": {
        "en": u"Fonts & data", "fr": u"Polices et données", "pt": u"Tipos de letra e dados",
        "es": u"Fuentes y datos", "it": u"Caratteri e dati", "de": u"Schriften & Daten",
        "pl": u"Czcionki i dane", "cs": u"Písma a data", "tr": u"Yazı tipleri ve veriler",
        "he": u"גופנים ונתונים", "ar": u"الخطوط والبيانات", "ja": u"フォントとデータ",
        "zh": u"字体与数据", "th": u"แบบอักษรและข้อมูล",
    },
    u"Шрифт Geist — Vercel, SIL Open Font License 1.1.": {
        "en": u"Geist font — Vercel, SIL Open Font License 1.1.",
        "fr": u"Police Geist — Vercel, licence SIL Open Font 1.1.",
        "pt": u"Fonte Geist — Vercel, licença SIL Open Font 1.1.",
        "es": u"Fuente Geist — Vercel, licencia SIL Open Font 1.1.",
        "it": u"Font Geist — Vercel, licenza SIL Open Font 1.1.",
        "de": u"Geist-Schrift — Vercel, SIL Open Font License 1.1.",
        "pl": u"Czcionka Geist — Vercel, licencja SIL Open Font 1.1.",
        "cs": u"Písmo Geist — Vercel, licence SIL Open Font 1.1.",
        "tr": u"Geist yazı tipi — Vercel, SIL Open Font License 1.1.",
        "he": u"גופן Geist — Vercel, רישיון SIL Open Font 1.1.",
        "ar": u"خط Geist — Vercel، رخصة SIL Open Font 1.1.",
        "ja": u"Geist フォント — Vercel, SIL Open Font License 1.1.",
        "zh": u"Geist 字体 — Vercel，SIL Open Font License 1.1。",
        "th": u"แบบอักษร Geist — Vercel, SIL Open Font License 1.1",
    },
    u"Каталог, рейтинги IMDb и КП, ссылки на видео — данные вашего портала или плейлиста.": {
        "en": u"Catalog, ratings, and stream links come from your portal or playlist.",
        "fr": u"Catalogue, notes et liens proviennent de votre portail ou playlist.",
        "pt": u"Catálogo, avaliações e links vêm do seu portal ou playlist.",
        "es": u"Catálogo, valoraciones y enlaces provienen de tu portal o lista.",
        "it": u"Catalogo, voti e link provengono dal tuo portale o playlist.",
        "de": u"Katalog, Wertungen und Videolinks stammen von Ihrem Portal oder Ihrer Playlist.",
        "pl": u"Katalog, oceny i linki pochodzą z Twojego portalu lub playlisty.",
        "cs": u"Katalog, hodnocení a odkazy pocházejí z vašeho portálu nebo playlistu.",
        "tr": u"Katalog, puanlar ve akış bağlantıları portalınızdan veya listenizden gelir.",
        "he": u"הקטלוג, הדירוגים והקישורים מגיעים מהפורטל או הפלייליסט שלך.",
        "ar": u"الدليل والتقييمات والروابط تأتي من بوابتك أو قائمة التشغيل.",
        "ja": u"カタログ、評価、映像リンクはお使いのポータルまたはプレイリストのデータです。",
        "zh": u"目录、评分及视频链接均来自您的门户或播放列表。",
        "th": u"แคตตาล็อก คะแนน และลิงก์วิดีโอมาจากพอร์ทัลหรือเพลย์ลิสต์ของคุณ",
    },
    u"OK или Back — назад": {
        "en": u"OK or Back — return", "fr": u"OK ou Back — retour", "pt": u"OK ou Back — voltar",
        "es": u"OK o Back — volver", "it": u"OK o Back — indietro", "de": u"OK oder Back — zurück",
        "pl": u"OK lub Back — powrót", "cs": u"OK nebo Back — zpět", "tr": u"OK veya Back — geri",
        "he": u"OK או Back — חזרה", "ar": u"OK أو Back — رجوع", "ja": u"OK または Back — 戻る",
        "zh": u"OK 或 返回键 — 返回", "th": u"OK หรือ Back — กลับ",
    },
    u"Проверяю настройку…": {
        "en": u"Checking settings…", "fr": u"Vérification des paramètres…", "pt": u"A verificar definições…",
        "es": u"Comprobando ajustes…", "it": u"Verifica impostazioni…", "de": u"Prüfe Einstellungen…",
        "pl": u"Sprawdzam ustawienia…", "cs": u"Ověřuji nastavení…", "tr": u"Ayarlar kontrol ediliyor…",
        "he": u"בודק הגדרות…", "ar": u"جارٍ فحص الإعدادات…", "ja": u"設定を確認中…",
        "zh": u"正在检查设置…", "th": u"กำลังตรวจสอบการตั้งค่า…",
    },
    u"Версия %s": {
        "en": u"Version %s", "fr": u"Version %s", "pt": u"Versão %s", "es": u"Versión %s",
        "it": u"Versione %s", "de": u"Version %s", "pl": u"Wersja %s", "cs": u"Verze %s",
        "tr": u"Sürüm %s", "he": u"גרסה %s", "ar": u"الإصدار %s", "ja": u"バージョン %s",
        "zh": u"版本 %s", "th": u"เวอร์ชัน %s",
    },
    u"Не подключено: ключ API добавляется в веб-интерфейсе (Настройки → TMDB).": {
        "en": u"Not connected: add your API key in the web UI (Settings → TMDB).",
        "fr": u"Non connecté : ajoutez la clé API dans l’interface web (Paramètres → TMDB).",
        "pt": u"Desligado: adicione a chave API na interface web (Definições → TMDB).",
        "es": u"No conectado: añade la clave API en la interfaz web (Ajustes → TMDB).",
        "it": u"Non connesso: aggiungi la chiave API nell’interfaccia web (Impostazioni → TMDB).",
        "de": u"Nicht verbunden: API-Schlüssel im Webinterface eintragen (Einstellungen → TMDB).",
        "pl": u"Niepołączono: dodaj klucz API w interfejsie web (Ustawienia → TMDB).",
        "cs": u"Nepřipojeno: přidejte API klíč ve webovém rozhraní (Nastavení → TMDB).",
        "tr": u"Bağlı değil: API anahtarını web arayüzünden ekleyin (Ayarlar → TMDB).",
        "he": u"לא מחובר: הוסף מפתח API בממשק הווב (הגדרות → TMDB).",
        "ar": u"غير متصل: أضف مفتاح API في واجهة الويب (الإعدادات ← TMDB).",
        "ja": u"未接続: Web画面の「設定 → TMDB」からAPIキーを追加してください。",
        "zh": u"未连接：请在网页端（设置 → TMDB）添加 API 密钥。",
        "th": u"ไม่ได้เชื่อมต่อ: เพิ่ม API Key ในหน้าเว็บ (การตั้งค่า → TMDB)",
    },
    u"Выключено в веб-интерфейсе — используются данные портала.": {
        "en": u"Disabled in web UI — using portal metadata.",
        "fr": u"Désactivé dans l’interface web — données du portail utilisées.",
        "pt": u"Desativado na interface web — a usar dados do portal.",
        "es": u"Desactivado en la web — usando datos del portal.",
        "it": u"Disattivato nell’interfaccia web — usati i dati del portale.",
        "de": u"Im Webinterface deaktiviert — Portaldaten werden genutzt.",
        "pl": u"Wyłączone w interfejsie web — używane są dane portalu.",
        "cs": u"Vypnuto ve webovém rozhraní — používají se data portálu.",
        "tr": u"Web arayüzünde kapalı — portal verileri kullanılıyor.",
        "he": u"מכובה בממשק הווב — מוצגים נתוני הפורטל.",
        "ar": u"معطل في واجهة الويب — تُستخدم بيانات البوابة.",
        "ja": u"Web画面で無効化されています — ポータルデータを使用中。",
        "zh": u"已在网页端禁用 — 正使用门户自带数据。",
        "th": u"ปิดใช้งานในหน้าเว็บ — กำลังใช้ข้อมูลจากพอร์ทัล",
    },
    u"Подключено, язык описаний: %s": {
        "en": u"Connected, description language: %s", "fr": u"Connecté, langue : %s",
        "pt": u"Ligado, idioma das descrições: %s", "es": u"Conectado, idioma: %s",
        "it": u"Connesso, lingua: %s", "de": u"Verbunden, Sprache: %s",
        "pl": u"Połączono, język opisów: %s", "cs": u"Připojeno, jazyk popisů: %s",
        "tr": u"Bağlı, açıklama dili: %s", "he": u"מחובר, שפת תיאורים: %s",
        "ar": u"متصل، لغة الأوصاف: %s", "ja": u"接続済み、説明の言語: %s",
        "zh": u"已连接，简介语言：%s", "th": u"เชื่อมต่อแล้ว ภาษาคำอธิบาย: %s",
    },
    u"Не подключено — используются данные портала.": {
        "en": u"Not connected — using portal metadata.", "fr": u"Non connecté — données du portail utilisées.",
        "pt": u"Desligado — a usar dados do portal.", "es": u"No conectado — usando datos del portal.",
        "it": u"Non connesso — usati i dati del portale.", "de": u"Nicht verbunden — Portaldaten aktiv.",
        "pl": u"Niepołączono — używane są dane portalu.", "cs": u"Nepřipojeno — používají se data portálu.",
        "tr": u"Bağlı değil — portal verileri kullanılıyor.", "he": u"לא מחובר — משתמש בנתוני הפורטל.",
        "ar": u"غير متصل — تُستخدم بيانات البوابة.", "ja": u"未接続 — ポータルデータを使用中。",
        "zh": u"未连接 — 使用门户数据。", "th": u"ไม่ได้เชื่อมต่อ — ใช้ข้อมูลจากพอร์ทัล",
    },
    u"В избранном пока ничего нет": {
        "en": u"No favorites yet", "fr": u"Aucun favori pour l’instant", "pt": u"Sem favoritos ainda",
        "es": u"Aún no hay favoritos", "it": u"Ancora nessun preferito", "de": u"Noch keine Favoriten",
        "pl": u"Brak ulubionych", "cs": u"Zatím žádné oblíbené", "tr": u"Henüz favori yok",
        "he": u"אין מועדפים עדיין", "ar": u"لا توجد مفضلة بعد", "ja": u"お気に入りはまだありません",
        "zh": u"收藏夹暂无内容", "th": u"ยังไม่มีรายการโปรด",
    },
    u"Долгое OK на канале, фильме или сериале добавляет его сюда": {
        "en": u"Hold OK on a channel, movie or series to add it here",
        "fr": u"Appui long sur OK sur une chaîne, un film ou une série pour l’ajouter",
        "pt": u"Prima OK longo num canal, filme ou série para adicionar aqui",
        "es": u"Mantén OK en un canal, película o serie para añadirlo aquí",
        "it": u"Tieni premuto OK su un canale, film o serie per aggiungerlo qui",
        "de": u"OK gedrückt halten fügt Kanäle, Filme oder Serien hier hinzu",
        "pl": u"Przytrzymaj OK na kanale, filmie lub serialu, aby dodać tutaj",
        "cs": u"Dlouhým stiskem OK na kanálu, filmu či seriálu jej přidáte sem",
        "tr": u"Eklemek için kanal, film veya dizide OK tuşuna basılı tutun",
        "he": u"לחיצה ארוכה על OK בערוץ, סרט או סדרה תוסיף לכאן",
        "ar": u"اضغط OK مطولاً على قناة أو فيلم أو مسلسل لإضافته هنا",
        "ja": u"チャンネル・映画・シリーズで OK 長押しするとここに追加されます",
        "zh": u"在频道、电影或剧集上长按 OK 即可添加到此处",
        "th": u"กด OK ค้างที่ช่อง ภาพยนตร์ หรือซีรีส์เพื่อเพิ่มที่นี่",
    },
    u"Вы ещё ничего не смотрели из каталога": {
        "en": u"Nothing watched from catalog yet", "fr": u"Rien regardé depuis le catalogue",
        "pt": u"Ainda nada visto no catálogo", "es": u"Aún no has visto nada del catálogo",
        "it": u"Nulla visto dal catalogo", "de": u"Noch nichts aus dem Katalog angesehen",
        "pl": u"Nic jeszcze nie oglądano z katalogu", "cs": u"Z katalogu jste zatím nic nesledovali",
        "tr": u"Katalogdan henüz bir şey izlenmedi", "he": u"טרם צפית במשהו מהקטלוג",
        "ar": u"لم تشاهد شيئاً من الدليل بعد", "ja": u"カタログの視聴履歴はありません",
        "zh": u"尚未在目录中观看过任何内容", "th": u"ยังไม่มีประวัติการรับชมจากแคตตาล็อก",
    },
    u"Здесь появятся 30 последних каналов, фильмов и сериалов": {
        "en": u"Your 30 most recent channels, movies and series appear here",
        "fr": u"Vos 30 dernières chaînes, films et séries apparaîtront ici",
        "pt": u"Os últimos 30 canais, filmes e séries aparecem aqui",
        "es": u"Aquí aparecerán los últimos 30 canales, películas y series",
        "it": u"Qui appariranno gli ultimi 30 canali, film e serie",
        "de": u"Die letzten 30 Kanäle, Filme und Serien erscheinen hier",
        "pl": u"Pojawi się tu 30 ostatnich kanałów, filmów i seriali",
        "cs": u"Zde se zobrazí 30 posledních kanálů, filmů a seriálů",
        "tr": u"Son 30 kanal, film ve dizi burada görünür",
        "he": u"כאן יופיעו 30 הערוצים, הסרטים והסדרות האחרונים",
        "ar": u"ستظهر هنا آخر 30 قناة وفيلم ومسلسل",
        "ja": u"最近再生したチャンネル・映画・シリーズ30件が表示されます",
        "zh": u"最近观看的 30 个频道、电影和剧集将显示在这里",
        "th": u"ช่อง ภาพยนตร์ และซีรีส์ 30 รายการล่าสุดจะแสดงที่นี่",
    },
    u"Открываю архив канала…": {
        "en": u"Opening channel archive…", "fr": u"Ouverture des archives de la chaîne…",
        "pt": u"A abrir arquivo do canal…", "es": u"Abriendo archivo del canal…",
        "it": u"Apertura archivio canale…", "de": u"Öffne Kanal-Archiv…",
        "pl": u"Otwieram archiwum kanału…", "cs": u"Otevírám archiv kanálu…",
        "tr": u"Kanal arşivi açılıyor…", "he": u"פותח ארכיון ערוץ…",
        "ar": u"جارٍ فتح أرشيف القناة…", "ja": u"チャンネルアーカイブを開いています…",
        "zh": u"正在打开频道回看…", "th": u"กำลังเปิดรายการย้อนหลัง…",
    },
    u"Back — вернуться в список каналов": {
        "en": u"Back — return to channel list", "fr": u"Back — retour à la liste",
        "pt": u"Back — voltar à lista de canais", "es": u"Back — volver a canales",
        "it": u"Back — torna alla lista canali", "de": u"Back — zur Kanalliste",
        "pl": u"Back — powrót do listy kanałów", "cs": u"Back — zpět na seznam kanálů",
        "tr": u"Back — kanal listesine dön", "he": u"Back — חזרה לרשימת הערוצים",
        "ar": u"Back — العودة لقائمة القنوات", "ja": u"Back — チャンネル一覧へ戻る",
        "zh": u"返回键 — 返回频道列表", "th": u"Back — กลับไปรายการช่อง",
    },
    u"Выберите IPTV-канал из плейлистов Dreaminator": {
        "en": u"Select an IPTV channel from Dreaminator playlists",
        "fr": u"Choisissez une chaîne IPTV des playlists Dreaminator",
        "pt": u"Selecione um canal IPTV das playlists Dreaminator",
        "es": u"Elige un canal IPTV de las listas de Dreaminator",
        "it": u"Seleziona un canale IPTV dalle playlist Dreaminator",
        "de": u"Wählen Sie einen IPTV-Kanal aus Dreaminator-Playlists",
        "pl": u"Wybierz kanał IPTV z playlist Dreaminator",
        "cs": u"Vyberte IPTV kanál z playlistů Dreaminator",
        "tr": u"Dreaminator listelerinden bir IPTV kanalı seçin",
        "he": u"בחר ערוץ IPTV מפלייליסטים של Dreaminator",
        "ar": u"اختر قناة IPTV من قوائم Dreaminator",
        "ja": u"Dreaminator のプレイリストから IPTV チャンネルを選択してください",
        "zh": u"请从 Dreaminator 播放列表中选择 IPTV 频道",
        "th": u"เลือกช่อง IPTV จากเพลย์ลิสต์ของ Dreaminator",
    },
    u"Этого канала нет в плейлистах Dreaminator": {
        "en": u"This channel is not in Dreaminator playlists",
        "fr": u"Cette chaîne n’est pas dans les playlists Dreaminator",
        "pt": u"Este canal não está nas playlists Dreaminator",
        "es": u"Este canal no está en las listas de Dreaminator",
        "it": u"Questo canale non è nelle playlist Dreaminator",
        "de": u"Dieser Kanal ist nicht in Dreaminator-Playlists",
        "pl": u"Tego kanału nie ma na playlistach Dreaminator",
        "cs": u"Tento kanál není v playlistech Dreaminator",
        "tr": u"Bu kanal Dreaminator listelerinde yok",
        "he": u"ערוץ זה אינו בפלייליסטים של Dreaminator",
        "ar": u"هذه القناة غير موجودة في قوائم Dreaminator",
        "ja": u"このチャンネルは Dreaminator のプレイリストにありません",
        "zh": u"此频道不在 Dreaminator 播放列表中",
        "th": u"ช่องนี้ไม่อยู่ในเพลย์ลิสต์ของ Dreaminator",
    },
    u"У этого канала нет архива в плейлисте провайдера": {
        "en": u"This channel has no catch-up archive in provider playlist",
        "fr": u"Cette chaîne n’a pas d’archive dans la playlist",
        "pt": u"Este canal não tem arquivo na playlist do fornecedor",
        "es": u"Este canal no tiene archivo en la lista del proveedor",
        "it": u"Questo canale non ha archivio nella playlist",
        "de": u"Dieser Kanal hat kein Archiv in der Playlist",
        "pl": u"Ten kanał nie ma archiwum u dostawcy",
        "cs": u"Tento kanál nemá archiv v playlistu poskytovatele",
        "tr": u"Bu kanalın sağlayıcı listesinde arşivi yok",
        "he": u"לערוץ זה אין ארכיון בפלייליסט של הספק",
        "ar": u"لا يوجد أرشيف لهذه القناة لدى المزود",
        "ja": u"このチャンネルにはアーカイブがありません",
        "zh": u"提供商播放列表中该频道无回看",
        "th": u"ช่องนี้ไม่มีรายการย้อนหลังในเพลย์ลิสต์ของผู้ให้บริการ",
    },
    u"Доступ к каналу защищён PIN-кодом": {
        "en": u"Channel access is PIN protected", "fr": u"Accès protégé par code PIN",
        "pt": u"Acesso protegido por código PIN", "es": u"Acceso protegido por PIN",
        "it": u"Accesso protetto da PIN", "de": u"Kanalzugriff per PIN geschützt",
        "pl": u"Dostęp do kanału chroniony kodem PIN", "cs": u"Přístup ke kanálu je chráněn PIN kódem",
        "tr": u"Kanal erişimi PIN ile korunuyor", "he": u"הגישה לערוץ מוגנת בקוד PIN",
        "ar": u"الوصول للقناة محمي برمز PIN", "ja": u"このチャンネルは PIN で保護されています",
        "zh": u"频道受 PIN 码保护", "th": u"ช่องนี้ถูกล็อกด้วยรหัส PIN",
    },
    u"Жёлтая — повторить · Back — вернуться в список": {
        "en": u"Yellow — retry · Back — return to list",
        "fr": u"Jaune — réessayer · Back — retour à la liste",
        "pt": u"Amarelo — repetir · Back — voltar à lista",
        "es": u"Amarillo — reintentar · Back — volver a la lista",
        "it": u"Giallo — riprova · Back — torna alla lista",
        "de": u"Gelb — wiederholen · Back — zurück zur Liste",
        "pl": u"Żółty — ponów · Back — powrót do listy",
        "cs": u"Žluté — opakovat · Back — zpět na seznam",
        "tr": u"Sarı — tekrar dene · Back — listeye dön",
        "he": u"צהוב — נסה שוב · Back — חזרה לרשימה",
        "ar": u"الأصفر — إعادة · Back — العودة للقائمة",
        "ja": u"黄 — 再試行 · Back — リストへ戻る",
        "zh": u"黄键 — 重试 · 返回键 — 返回列表",
        "th": u"สีเหลือง — ลองใหม่ · Back — กลับไปที่รายการ",
    },
    u"Dreaminator — программа и архив": {
        "en": u"Dreaminator — Guide & Archive", "fr": u"Dreaminator — Guide et Archives",
        "pt": u"Dreaminator — Guia e Arquivo", "es": u"Dreaminator — Guía y Archivo",
        "it": u"Dreaminator — Guida e Archivio", "de": u"Dreaminator — Guide & Archiv",
        "pl": u"Dreaminator — Program i Archiwum", "cs": u"Dreaminator — Program a Archiv",
        "tr": u"Dreaminator — Rehber ve Arşiv", "he": u"Dreaminator — מדריך וארכיון",
        "ar": u"Dreaminator — الدليل والأرشيف", "ja": u"Dreaminator — 番組表とアーカイブ",
        "zh": u"Dreaminator — 节目单与回看", "th": u"Dreaminator — ผังรายการและย้อนหลัง",
    },
    u"Программа и архив": {
        "en": u"Guide & Archive", "fr": u"Guide et Archives", "pt": u"Guia e Arquivo",
        "es": u"Guía y Archivo", "it": u"Guida e Archivio", "de": u"Guide & Archiv",
        "pl": u"Program i Archiwum", "cs": u"Program a Archiv", "tr": u"Rehber ve Arşiv",
        "he": u"מדריך וארכיון", "ar": u"الدليل والأرشيف", "ja": u"番組表とアーカイブ",
        "zh": u"节目单与回看", "th": u"ผังรายการและย้อนหลัง",
    },
    u"Прошедшие передачи из архива провайдера": {
        "en": u"Past programmes from provider archive", "fr": u"Programmes passés depuis les archives",
        "pt": u"Programas passados do arquivo", "es": u"Programas pasados del archivo",
        "it": u"Programmi passati dall’archivio", "de": u"Vergangene Sendungen aus dem Archiv",
        "pl": u"Minione audycje z archiwum dostawcy", "cs": u"Minulé pořady z archivu poskytovatele",
        "tr": u"Sağlayıcı arşivinden geçmiş programlar", "he": u"תוכניות קודמות מארכיון הספק",
        "ar": u"برامج سابقة من أرشيف المزود", "ja": u"プロバイダのアーカイブから過去の番組を再生",
        "zh": u"播放提供商回看中的往期节目", "th": u"รายการย้อนหลังจากคลังของผู้ให้บริการ",
    },
    u"Ошибка %d": {
        "en": u"Error %d", "fr": u"Erreur %d", "pt": u"Erro %d", "es": u"Error %d",
        "it": u"Errore %d", "de": u"Fehler %d", "pl": u"Błąd %d", "cs": u"Chyba %d",
        "tr": u"Hata %d", "he": u"שגיאה %d", "ar": u"خطأ %d", "ja": u"エラー %d",
        "zh": u"错误 %d", "th": u"ข้อผิดพลาด %d",
    },
    u"Сервис Dreaminator не отвечает": {
        "en": u"Dreaminator service is not responding", "fr": u"Le service Dreaminator ne répond pas",
        "pt": u"O serviço Dreaminator não responde", "es": u"El servicio Dreaminator no responde",
        "it": u"Il servizio Dreaminator non risponde", "de": u"Dreaminator-Dienst antwortet nicht",
        "pl": u"Usługa Dreaminator nie odpowiada", "cs": u"Služba Dreaminator neodpovídá",
        "tr": u"Dreaminator hizmeti yanıt vermiyor", "he": u"שירות Dreaminator אינו מגיב",
        "ar": u"خدمة Dreaminator لا تستجيب", "ja": u"Dreaminator サービスが応答しません",
        "zh": u"Dreaminator 服务无响应", "th": u"บริการ Dreaminator ไม่ตอบสนอง",
    },
    u"Не удалось обработать ответ сервиса": {
        "en": u"Failed to process service response", "fr": u"Impossible de traiter la réponse du service",
        "pt": u"Não foi possível processar a resposta", "es": u"No se pudo procesar la respuesta del servicio",
        "it": u"Impossibile elaborare la risposta", "de": u"Dienstantwort konnte nicht verarbeitet werden",
        "pl": u"Nie udało się przetworzyć odpowiedzi", "cs": u"Nepodařilo se zpracovat odpověď služby",
        "tr": u"Hizmet yanıtı işlenemedi", "he": u"עיבוד תגובת השירות נכשל",
        "ar": u"تعذرت معالجة استجابة الخدمة", "ja": u"サービス応答を処理できませんでした",
        "zh": u"无法处理服务响应", "th": u"ประมวลผลคำตอบจากบริการไม่สำเร็จ",
    },
}

# Server-originated strings (catalog categories, errors, etc.).
SERVER_TRANSLATIONS = {
    u"Все каналы": {
        "en": u"All channels", "fr": u"Toutes les chaînes", "pt": u"Todos os canais", "es": u"Todos los canales",
        "it": u"Tutti i canali", "de": u"Alle Kanäle", "pl": u"Wszystkie kanały", "cs": u"Všechny kanály",
        "tr": u"Tüm kanallar", "he": u"כל הערוצים", "ar": u"كل القنوات", "ja": u"すべてのチャンネル",
        "zh": u"全部频道", "th": u"ทุกช่อง",
    },
    u"Все": {
        "en": u"All", "fr": u"Tout", "pt": u"Todos", "es": u"Todo",
        "it": u"Tutti", "de": u"Alle", "pl": u"Wszystkie", "cs": u"Vše",
        "tr": u"Tümü", "he": u"הכל", "ar": u"الكل", "ja": u"すべて",
        "zh": u"全部", "th": u"ทั้งหมด",
    },
    u"Каналы": {
        "en": u"Channels", "fr": u"Chaînes", "pt": u"Canais", "es": u"Canales",
        "it": u"Canali", "de": u"Kanäle", "pl": u"Kanały", "cs": u"Kanály",
        "tr": u"Kanallar", "he": u"ערוצים", "ar": u"القنوات", "ja": u"チャンネル",
        "zh": u"频道", "th": u"ช่อง",
    },
    u"Фильмы и сериалы": {
        "en": u"Movies & series", "fr": u"Films et séries", "pt": u"Filmes e séries", "es": u"Películas y series",
        "it": u"Film e serie", "de": u"Filme & Serien", "pl": u"Filmy i seriale", "cs": u"Filmy a seriály",
        "tr": u"Film ve diziler", "he": u"סרטים וסדרות", "ar": u"الأفلام والمسلسلات", "ja": u"映画とシリーズ",
        "zh": u"电影和剧集", "th": u"ภาพยนตร์และซีรีส์",
    },
    u"Без группы": {
        "en": u"Ungrouped", "fr": u"Sans groupe", "pt": u"Sem grupo", "es": u"Sin grupo",
        "it": u"Senza gruppo", "de": u"Ohne Gruppe", "pl": u"Bez grupy", "cs": u"Bez skupiny",
        "tr": u"Grupsuz", "he": u"ללא קבוצה", "ar": u"بدون مجموعة", "ja": u"グループなし",
        "zh": u"未分组", "th": u"ไม่มีกลุ่ม",
    },
    u"Избранное": TRANSLATIONS[u"Избранное"],
    u"каталог обновился — вернитесь к категориям и откройте список заново": {
        "en": u"catalog refreshed — return to categories and reopen the list",
        "fr": u"catalogue mis à jour — revenez aux catégories et rouvrez la liste",
        "pt": u"catálogo atualizado — volte às categorias e reabra a lista",
        "es": u"catálogo actualizado — vuelve a categorías y abre la lista de nuevo",
        "it": u"catalogo aggiornato — torna alle categorie e riapri la lista",
        "de": u"Katalog aktualisiert — zurück zu Kategorien und Liste neu öffnen",
        "pl": u"katalog odświeżony — wróć do kategorii i otwórz listę ponownie",
        "cs": u"katalog aktualizován — vraťte se ke kategoriím a seznam znovu otevřete",
        "tr": u"katalog yenilendi — kategorilere dönüp listeyi yeniden açın",
        "he": u"הקטלוג התעדכן — חזור לקטגוריות ופתח את הרשימה מחדש",
        "ar": u"تم تحديث الدليل — عد إلى الفئات وأعد فتح القائمة",
        "ja": u"カタログが更新されました — カテゴリに戻って開き直してください",
        "zh": u"目录已刷新 — 请返回分类并重新打开列表",
        "th": u"แคตตาล็อกอัปเดตแล้ว — กลับไปที่หมวดหมู่และเปิดรายการใหม่",
    },
    u"этой передачи нет в архиве": {
        "en": u"this programme is not in the archive", "fr": u"ce programme n’est pas dans les archives",
        "pt": u"este programa não está no arquivo", "es": u"este programa no está en el archivo",
        "it": u"questo programma non è in archivio", "de": u"diese Sendung ist nicht im Archiv",
        "pl": u"tej audycji nie ma w archiwum", "cs": u"tento pořad není v archivu",
        "tr": u"bu program arşivde yok", "he": u"תוכנית זו אינה בארכיון",
        "ar": u"هذا البرنامج غير موجود بالأرشيف", "ja": u"この番組はアーカイブにありません",
        "zh": u"该节目不在回看中", "th": u"รายการนี้ไม่มีในย้อนหลัง",
    },
    u"источник не отдал программу передач": {
        "en": u"source returned no program guide", "fr": u"la source n’a pas fourni de guide TV",
        "pt": u"a fonte não devolveu o guia TV", "es": u"la fuente no devolvió la guía",
        "it": u"la fonte non ha restituito la guida", "de": u"Quelle lieferte keinen EPG",
        "pl": u"źródło nie zwróciło programu TV", "cs": u"zdroj nevrátil program",
        "tr": u"kaynak program rehberi vermedi", "he": u"המקור לא החזיר מדריך שידורים",
        "ar": u"المصدر لم يرجع دليل البرامج", "ja": u"ソースから番組表を取得できませんでした",
        "zh": u"源未提供节目单", "th": u"แหล่งที่มาไม่ส่งข้อมูลผังรายการ",
    },
    u"не удалось получить ссылку на видео — проверьте подключение и подписку": {
        "en": u"could not get stream URL — check connection and subscription",
        "fr": u"impossible d’obtenir le lien — vérifiez connexion et abonnement",
        "pt": u"não foi possível obter o link — verifique ligação e subscrição",
        "es": u"no se pudo obtener el enlace — revisa conexión y suscripción",
        "it": u"impossibile ottenere il link — verifica connessione e abbonamento",
        "de": u"Videolink nicht erhalten — Verbindung und Abo prüfen",
        "pl": u"nie udało się pobrać linku — sprawdź połączenie i subskrypcję",
        "cs": u"nepodařilo se získat odkaz — zkontrolujte připojení a předplatné",
        "tr": u"video bağlantısı alınamadı — bağlantıyı ve aboneliği kontrol edin",
        "he": u"לא ניתן לקבל קישור וידאו — בדוק חיבור ומנוי",
        "ar": u"تعذر الحصول على رابط الفيديو — تحقق من الاتصال والاشتراك",
        "ja": u"動画URLを取得できません — 接続と契約を確認してください",
        "zh": u"无法获取视频链接 — 请检查网络和订阅",
        "th": u"รับลิงก์วิดีโอไม่สำเร็จ — ตรวจสอบการเชื่อมต่อและการสมัครสมาชิก",
    },
}


def tr(value, lang=None):
    """Translate a static UI string (Russian key -> active language native str/bytes)."""
    if not value:
        return ui_api.text(value)
    if lang is None:
        lang = get_language()
    if lang == "ru":
        return ui_api.text(value)
    key = ui_api.unicode_text(value)
    entry = TRANSLATIONS.get(key)
    if entry and lang in entry:
        return ui_api.text(entry[lang])
    if entry and "en" in entry:
        return ui_api.text(entry["en"])
    return ui_api.text(value)


def _tr_ru_when(text, lang):
    """Translate server-generated Russian time fragments from humanWhen()."""
    if text == u"только что":
        return {
            "en": u"just now", "fr": u"à l’instant", "pt": u"agora mesmo", "es": u"ahora mismo",
            "it": u"proprio ora", "de": u"gerade eben", "pl": u"przed chwilą", "cs": u"právě teď",
            "tr": u"az önce", "he": u"ממש עכשיו", "ar": u"الآن", "ja": u"たった今",
            "zh": u"刚刚", "th": u"เมื่อสักครู่",
        }.get(lang, text)
    m = re.match(u"^(\\d+) мин назад$", text)
    if m:
        n = int(m.group(1))
        fmt = {
            "en": u"%d min ago", "fr": u"il y a %d min", "pt": u"há %d min", "es": u"hace %d min",
            "it": u"%d min fa", "de": u"vor %d Min.", "pl": u"%d min temu", "cs": u"před %d min",
            "tr": u"%d dk önce", "he": u"לפני %d דק׳", "ar": u"منذ %d د", "ja": u"%d分前",
            "zh": u"%d 分钟前", "th": u"%d นาทีที่แล้ว",
        }.get(lang)
        return fmt % n if fmt else text
    m = re.match(u"^сегодня в (\\d\\d:\\d\\d)$", text)
    if m:
        tm = m.group(1)
        fmt = {
            "en": u"today at %s", "fr": u"aujourd’hui à %s", "pt": u"hoje às %s", "es": u"hoy a las %s",
            "it": u"oggi alle %s", "de": u"heute um %s", "pl": u"dzisiaj o %s", "cs": u"dnes v %s",
            "tr": u"bugün %s", "he": u"היום ב-%s", "ar": u"اليوم %s", "ja": u"今日 %s",
            "zh": u"今天 %s", "th": u"วันนี้ %s",
        }.get(lang)
        return fmt % tm if fmt else text
    m = re.match(u"^вчера в (\\d\\d:\\d\\d)$", text)
    if m:
        tm = m.group(1)
        fmt = {
            "en": u"yesterday at %s", "fr": u"hier à %s", "pt": u"ontem às %s", "es": u"ayer a las %s",
            "it": u"ieri alle %s", "de": u"gestern um %s", "pl": u"wczoraj o %s", "cs": u"včera v %s",
            "tr": u"dün %s", "he": u"אתמול ב-%s", "ar": u"أمس %s", "ja": u"昨日 %s",
            "zh": u"昨天 %s", "th": u"เมื่อวาน %s",
        }.get(lang)
        return fmt % tm if fmt else text
    m = re.match(u"^(\\d{1,2}) ([^\\s]+) в (\\d\\d:\\d\\d)$", text)
    if m and m.group(2) in _MONTHS["ru"]:
        d_num, idx, tm = int(m.group(1)), _MONTHS["ru"].index(m.group(2)), m.group(3)
        mo = _MONTHS.get(lang, _MONTHS["en"])[idx]
        if lang in ("ja", "zh"):
            return u"%s%d日 %s" % (mo, d_num, tm)
        fmt = {
            "en": u"%d %s at %s", "fr": u"%d %s à %s", "pt": u"%d de %s às %s", "es": u"%d de %s a las %s",
            "it": u"%d %s alle %s", "de": u"%d. %s um %s", "pl": u"%d %s o %s", "cs": u"%d. %s v %s",
            "tr": u"%d %s %s", "he": u"%d ב%s ב-%s", "ar": u"%d %s %s", "th": u"%d %s %s",
        }.get(lang, u"%d %s at %s")
        return fmt % (d_num, mo, tm)
    return text


def tr_server(value, lang=None):
    """Translate standard server-returned strings (categories, summary texts, errors)."""
    if not value:
        return ui_api.text(value)
    if lang is None:
        lang = get_language()
    if lang == "ru":
        return ui_api.text(value)
    u = ui_api.unicode_text(value)
    entry = SERVER_TRANSLATIONS.get(u)
    if entry and lang in entry:
        return ui_api.text(entry[lang])
    entry = TRANSLATIONS.get(u)
    if entry and lang in entry:
        return ui_api.text(entry[lang])
    if u == u"обновляется…":
        return ui_api.text({
            "en": u"updating…", "fr": u"mise à jour…", "pt": u"a atualizar…", "es": u"actualizando…",
            "it": u"aggiornamento…", "de": u"wird aktualisiert…", "pl": u"aktualizacja…", "cs": u"aktualizuje se…",
            "tr": u"güncelleniyor…", "he": u"מתעדכן…", "ar": u"جارٍ التحديث…", "ja": u"更新中…",
            "zh": u"更新中…", "th": u"กำลังอัปเดต…",
        }.get(lang, u))
    if u == u"в списках пока нет каналов":
        return ui_api.text({
            "en": u"no channels in lists yet", "fr": u"aucune chaîne dans les listes",
            "pt": u"sem canais nas listas ainda", "es": u"sin canales en las listas aún",
            "it": u"nessun canale nelle liste", "de": u"noch keine Kanäle in Listen",
            "pl": u"brak kanałów na listach", "cs": u"v seznamech zatím nejsou kanály",
            "tr": u"listelerde henüz kanal yok", "he": u"אין ערוצים ברשימות עדיין",
            "ar": u"لا توجد قنوات في القوائم بعد", "ja": u"リストにチャンネルがありません",
            "zh": u"列表中暂无频道", "th": u"ยังไม่มีช่องในรายการ",
        }.get(lang, u))
    if u == u"ещё не загружалась":
        return ui_api.text({
            "en": u"not loaded yet", "fr": u"pas encore chargé", "pt": u"ainda não carregado",
            "es": u"aún no cargada", "it": u"non ancora caricata", "de": u"noch nicht geladen",
            "pl": u"jeszcze nie pobrano", "cs": u"ještě nenačteno", "tr": u"henüz yüklenmedi",
            "he": u"טרם נטען", "ar": u"لم يُحمّل بعد", "ja": u"未読込",
            "zh": u"尚未加载", "th": u"ยังไม่ได้โหลด",
        }.get(lang, u))
    m = re.match(u"^есть у (\\d+) из (\\d+) каналов, обновлена (.+)$", u)
    if m:
        matched, wanted, when_txt = int(m.group(1)), int(m.group(2)), _tr_ru_when(m.group(3), lang)
        fmt = {
            "en": u"%d of %d channels, updated %s", "fr": u"%d sur %d chaînes, mis à jour %s",
            "pt": u"%d de %d canais, atualizado %s", "es": u"%d de %d canales, actualizada %s",
            "it": u"%d su %d canali, aggiornata %s", "de": u"%d von %d Kanälen, aktualisiert %s",
            "pl": u"%d z %d kanałów, zaktualizowano %s", "cs": u"%d z %d kanálů, aktualizováno %s",
            "tr": u"%d / %d kanal, güncelleme %s", "he": u"%d מתוך %d ערוצים, עודכן %s",
            "ar": u"%d من %d قناة، حُدث %s", "ja": u"%d/%d チャンネル、更新 %s",
            "zh": u"%d/%d 个频道，更新于 %s", "th": u"%d จาก %d ช่อง, อัปเดต %s",
        }.get(lang)
        if fmt:
            return ui_api.text(fmt % (matched, wanted, when_txt))
    tr_when = _tr_ru_when(u, lang)
    if tr_when != u:
        return ui_api.text(tr_when)
    if u == u"без срока":
        return ui_api.text({
            "en": u"unlimited", "fr": u"illimité", "pt": u"ilimitado", "es": u"ilimitado", "it": u"illimitato",
            "de": u"unbegrenzt", "pl": u"bez limitu", "cs": u"bez omezení", "tr": u"süresiz",
            "he": u"ללא הגבלה", "ar": u"غير محدود", "ja": u"無期限", "zh": u"无限期", "th": u"ไม่จำกัด",
        }.get(lang, u))
    m = re.match(u"^(?:«(.+?)» )?до (\\d{1,2}) ([^\\s]+) (\\d{4})$", u)
    if m and m.group(3) in _MONTHS["ru"]:
        src_name, d_num, idx, yr = m.group(1), int(m.group(2)), _MONTHS["ru"].index(m.group(3)), m.group(4)
        mo = _MONTHS.get(lang, _MONTHS["en"])[idx]
        if lang in ("ja", "zh"):
            dt = u"%s年%s%d日" % (yr, mo, d_num)
        elif lang in ("de", "cs"):
            dt = u"%d. %s %s" % (d_num, mo, yr)
        elif lang in ("pt", "es"):
            dt = u"%d de %s de %s" % (d_num, mo, yr)
        else:
            dt = u"%d %s %s" % (d_num, mo, yr)
        until_fmt = {
            "en": u"until %s", "fr": u"jusqu’au %s", "pt": u"até %s", "es": u"hasta %s",
            "it": u"fino al %s", "de": u"bis %s", "pl": u"do %s", "cs": u"do %s",
            "tr": u"%s tarihine kadar", "he": u"עד %s", "ar": u"حتى %s",
            "ja": u"%sまで", "zh": u"至 %s", "th": u"ถึง %s",
        }.get(lang, u"until %s") % dt
        if src_name:
            return ui_api.text(u"%s · %s" % (src_name, until_fmt))
        return ui_api.text(until_fmt)
    return ui_api.text(value)


def duration_text(minutes, lang=None):
    try:
        m = int(minutes)
    except (TypeError, ValueError):
        return ""
    if m <= 0:
        return ""
    if lang is None:
        lang = get_language()
    min_u = {
        "ru": u"мин", "en": u"min", "fr": u"min", "pt": u"min", "es": u"min",
        "it": u"min", "de": u"Min.", "pl": u"min", "cs": u"min", "tr": u"dk",
        "he": u"דק׳", "ar": u"د", "ja": u"分", "zh": u"分钟", "th": u"นาที",
    }.get(lang, u"min")
    hr_u = {
        "ru": u"ч", "en": u"h", "fr": u"h", "pt": u"h", "es": u"h",
        "it": u"h", "de": u"Std.", "pl": u"godz.", "cs": u"h", "tr": u"sa",
        "he": u"ש׳", "ar": u"س", "ja": u"時間", "zh": u"小时", "th": u"ชม.",
    }.get(lang, u"h")
    if m < 60:
        return ui_api.text(u"%d %s" % (m, min_u))
    return ui_api.text(u"%d %s %02d %s" % (m // 60, hr_u, m % 60, min_u))


def days_text(n, lang=None):
    n = int(n)
    if lang is None:
        lang = get_language()
    if lang == "ru":
        if n % 10 == 1 and n % 100 != 11:
            word = u"день"
        elif 2 <= n % 10 <= 4 and not 12 <= n % 100 <= 14:
            word = u"дня"
        else:
            word = u"дней"
        return ui_api.text(u"%d %s" % (n, word))
    if lang == "pl":
        word = u"dzień" if n == 1 else u"dni"
        return ui_api.text(u"%d %s" % (n, word))
    if lang == "cs":
        word = u"den" if n == 1 else (u"dny" if 2 <= n <= 4 else u"dnů")
        return ui_api.text(u"%d %s" % (n, word))
    if lang in ("ja", "zh", "th"):
        unit = {"ja": u"日", "zh": u"天", "th": u"วัน"}[lang]
        return ui_api.text(u"%d %s" % (n, unit)) if lang == "th" else ui_api.text(u"%d%s" % (n, unit))
    forms = {
        "en": (u"day", u"days"),
        "fr": (u"jour", u"jours"),
        "pt": (u"dia", u"dias"),
        "es": (u"día", u"días"),
        "it": (u"giorno", u"giorni"),
        "de": (u"Tag", u"Tage"),
        "tr": (u"gün", u"gün"),
        "he": (u"יום", u"ימים"),
        "ar": (u"يوم", u"أيام"),
    }
    one, many = forms.get(lang, (u"day", u"days"))
    word = one if (n == 1 or (lang == "fr" and n <= 1)) else many
    return ui_api.text(u"%d %s" % (n, word))


_WEEKDAYS = {
    "ru": (u"понедельник", u"вторник", u"среда", u"четверг", u"пятница", u"суббота", u"воскресенье"),
    "en": (u"Monday", u"Tuesday", u"Wednesday", u"Thursday", u"Friday", u"Saturday", u"Sunday"),
    "fr": (u"lundi", u"mardi", u"mercredi", u"jeudi", u"vendredi", u"samedi", u"dimanche"),
    "pt": (u"segunda-feira", u"terça-feira", u"quarta-feira", u"quinta-feira", u"sexta-feira", u"sábado", u"domingo"),
    "es": (u"lunes", u"martes", u"miércoles", u"jueves", u"viernes", u"sábado", u"domingo"),
    "it": (u"lunedì", u"martedì", u"mercoledì", u"giovedì", u"venerdì", u"sabato", u"domenica"),
    "de": (u"Montag", u"Dienstag", u"Mittwoch", u"Donnerstag", u"Freitag", u"Samstag", u"Sonntag"),
    "pl": (u"poniedziałek", u"wtorek", u"środa", u"czwartek", u"piątek", u"sobota", u"niedziela"),
    "cs": (u"pondělí", u"úterý", u"středa", u"čtvrtek", u"pátek", u"sobota", u"neděle"),
    "tr": (u"Pazartesi", u"Salı", u"Çarşamba", u"Perşembe", u"Cuma", u"Cumartesi", u"Pazar"),
    "he": (u"יום שני", u"יום שלישי", u"יום רביעי", u"יום חמישי", u"יום שישי", u"שבת", u"יום ראשון"),
    "ar": (u"الإثنين", u"الثلاثاء", u"الأربعاء", u"الخميس", u"الجمعة", u"السبت", u"الأحد"),
    "ja": (u"月曜日", u"火曜日", u"水曜日", u"木曜日", u"金曜日", u"土曜日", u"日曜日"),
    "zh": (u"星期一", u"星期二", u"星期三", u"星期四", u"星期五", u"星期六", u"星期日"),
    "th": (u"วันจันทร์", u"วันอังคาร", u"วันพุธ", u"วันพฤหัสบดี", u"วันศุกร์", u"วันเสาร์", u"วันอาทิตย์"),
}

_WEEKDAYS_CAP = {
    "ru": (u"Понедельник", u"Вторник", u"Среда", u"Четверг", u"Пятница", u"Суббота", u"Воскресенье"),
    "fr": (u"Lundi", u"Mardi", u"Mercredi", u"Jeudi", u"Vendredi", u"Samedi", u"Dimanche"),
    "pt": (u"Segunda-feira", u"Terça-feira", u"Quarta-feira", u"Quinta-feira", u"Sexta-feira", u"Sábado", u"Domingo"),
    "es": (u"Lunes", u"Martes", u"Miércoles", u"Jueves", u"Viernes", u"Sábado", u"Domingo"),
    "it": (u"Lunedì", u"Martedì", u"Mercoledì", u"Giovedì", u"Venerdì", u"Sabato", u"Domenica"),
    "pl": (u"Poniedziałek", u"Wtorek", u"Środa", u"Czwartek", u"Piątek", u"Sobota", u"Niedziela"),
    "cs": (u"Pondělí", u"Úterý", u"Středa", u"Čtvrtek", u"Pátek", u"Sobota", u"Neděle"),
}

_MONTHS = {
    "ru": (u"января", u"февраля", u"марта", u"апреля", u"мая", u"июня", u"июля", u"августа", u"сентября", u"октября", u"ноября", u"декабря"),
    "en": (u"January", u"February", u"March", u"April", u"May", u"June", u"July", u"August", u"September", u"October", u"November", u"December"),
    "fr": (u"janvier", u"février", u"mars", u"avril", u"mai", u"juin", u"juillet", u"août", u"septembre", u"octobre", u"novembre", u"décembre"),
    "pt": (u"janeiro", u"fevereiro", u"março", u"abril", u"maio", u"junho", u"julho", u"agosto", u"setembro", u"outubro", u"novembro", u"dezembro"),
    "es": (u"enero", u"febrero", u"marzo", u"abril", u"mayo", u"junio", u"julio", u"agosto", u"septiembre", u"octubre", u"noviembre", u"diciembre"),
    "it": (u"gennaio", u"febbraio", u"marzo", u"aprile", u"maggio", u"giugno", u"luglio", u"agosto", u"settembre", u"ottobre", u"novembre", u"dicembre"),
    "de": (u"Januar", u"Februar", u"März", u"April", u"Mai", u"Juni", u"Juli", u"August", u"September", u"Oktober", u"November", u"Dezember"),
    "pl": (u"stycznia", u"lutego", u"marca", u"kwietnia", u"maja", u"czerwca", u"lipca", u"sierpnia", u"września", u"października", u"listopada", u"grudnia"),
    "cs": (u"ledna", u"února", u"března", u"dubna", u"května", u"června", u"července", u"srpna", u"září", u"října", u"listopadu", u"prosince"),
    "tr": (u"Ocak", u"Şubat", u"Mart", u"Nisan", u"Mayıs", u"Haziran", u"Temmuz", u"Ağustos", u"Eylül", u"Ekim", u"Kasım", u"Aralık"),
    "he": (u"ינואר", u"פברואר", u"מרץ", u"אפריל", u"מאי", u"יוני", u"יולי", u"אוגוסט", u"ספטמבר", u"אוקטובר", u"נובמבר", u"דצמבר"),
    "ar": (u"يناير", u"فبراير", u"مارس", u"أبريل", u"مايو", u"يونيو", u"يوليو", u"أغسطس", u"سبتمبر", u"أكتوبر", u"نوفمبر", u"ديسمبر"),
    "ja": (u"1月", u"2月", u"3月", u"4月", u"5月", u"6月", u"7月", u"8月", u"9月", u"10月", u"11月", u"12月"),
    "zh": (u"1月", u"2月", u"3月", u"4月", u"5月", u"6月", u"7月", u"8月", u"9月", u"10月", u"11月", u"12月"),
    "th": (u"มกราคม", u"กุมภาพันธ์", u"มีนาคม", u"เมษายน", u"พฤษภาคม", u"มิถุนายน", u"กรกฎาคม", u"สิงหาคม", u"กันยายน", u"ตุลาคม", u"พฤศจิกายน", u"ธันวาคม"),
}

_RELATIVE = {
    "ru": {-1: u"Вчера", 0: u"Сегодня", 1: u"Завтра"},
    "en": {-1: u"Yesterday", 0: u"Today", 1: u"Tomorrow"},
    "fr": {-1: u"Hier", 0: u"Aujourd’hui", 1: u"Demain"},
    "pt": {-1: u"Ontem", 0: u"Hoje", 1: u"Amanhã"},
    "es": {-1: u"Ayer", 0: u"Hoy", 1: u"Mañana"},
    "it": {-1: u"Ieri", 0: u"Oggi", 1: u"Domani"},
    "de": {-1: u"Gestern", 0: u"Heute", 1: u"Morgen"},
    "pl": {-1: u"Wczoraj", 0: u"Dzisiaj", 1: u"Jutro"},
    "cs": {-1: u"Včera", 0: u"Dnes", 1: u"Zítra"},
    "tr": {-1: u"Dün", 0: u"Bugün", 1: u"Yarın"},
    "he": {-1: u"אתמול", 0: u"היום", 1: u"מחר"},
    "ar": {-1: u"أمس", 0: u"اليوم", 1: u"غداً"},
    "ja": {-1: u"昨日", 0: u"今日", 1: u"明日"},
    "zh": {-1: u"昨天", 0: u"今天", 1: u"明天"},
    "th": {-1: u"เมื่อวาน", 0: u"วันนี้", 1: u"พรุ่งนี้"},
}


def relative_day(offset, lang=None):
    if lang is None:
        lang = get_language()
    rel = _RELATIVE.get(lang, _RELATIVE["en"])
    return ui_api.text(rel.get(offset, u""))


def day_label(offset, date, lang=None):
    if lang is None:
        lang = get_language()
    rel = _RELATIVE.get(lang, _RELATIVE["en"])
    try:
        y, m, d = [int(x) for x in ui_api.text(date).split("-")]
        weekday = datetime.date(y, m, d).weekday()
    except (ValueError, TypeError, AttributeError):
        return ui_api.text(rel.get(offset, u""))
    wd_list = _WEEKDAYS.get(lang, _WEEKDAYS["en"])
    wd_cap_list = _WEEKDAYS_CAP.get(lang, wd_list)
    mo_list = _MONTHS.get(lang, _MONTHS["en"])
    if lang in ("ja", "zh"):
        date_part = u"%s%d日 (%s)" % (mo_list[m - 1], d, wd_list[weekday])
    elif lang == "de":
        date_part = u"%s, %d. %s" % (wd_cap_list[weekday], d, mo_list[m - 1])
    elif lang == "en":
        date_part = u"%s, %s %d" % (wd_cap_list[weekday], mo_list[m - 1], d)
    else:
        date_part = u"%s, %d %s" % (wd_list[weekday] if offset in rel else wd_cap_list[weekday], d, mo_list[m - 1])
    if offset in rel:
        if lang in ("ja", "zh"):
            return ui_api.text(u"%s %s" % (rel[offset], date_part))
        return ui_api.text(u"%s, %s" % (rel[offset], date_part))
    return ui_api.text(date_part)


def favorite_message(item, result, name_of_fn, lang=None):
    if lang is None:
        lang = get_language()
    added = bool(result.get("fav"))
    item_name = ui_api.unicode_text(name_of_fn(item))
    if not result.get("channel"):
        if added:
            t_fmt = {
                "ru": u"«%s» — в избранном", "en": u"“%s” — added to favorites", "fr": u"« %s » — ajouté aux favoris",
                "pt": u"«%s» — nos favoritos", "es": u"«%s» — en favoritos", "it": u"«%s» — nei preferiti",
                "de": u"„%s“ — in Favoriten", "pl": u"„%s” — w ulubionych", "cs": u"„%s“ — v oblíbených",
                "tr": u"«%s» — favorilere eklendi", "he": u"«%s» — נוסף למועדפים", "ar": u"«%s» — أضيف للمفضلة",
                "ja": u"「%s」をお気に入りに追加", "zh": u"“%s” — 已收藏", "th": u"“%s” — เพิ่มในโปรดแล้ว",
            }.get(lang, u"“%s” — added to favorites")
            h_txt = {
                "ru": u"Раздел «Избранное» — вверху панели слева", "en": u"Favorites section is at the top of the left rail",
                "fr": u"La section Favoris est en haut du menu gauche", "pt": u"Secção Favoritos no topo do painel esquerdo",
                "es": u"La sección Favoritos está arriba a la izquierda", "it": u"La sezione Preferiti è in alto a sinistra",
                "de": u"Der Bereich Favoriten ist oben in der Seitenleiste", "pl": u"Sekcja Ulubione u góry panelu po lewej",
                "cs": u"Sekce Oblíbené je nahoře v levém panelu", "tr": u"Favoriler bölümü sol panelin üstündedir",
                "he": u"מדור המועדפים נמצא בראש התפריט משמאל", "ar": u"قسم المفضلة في أعلى القائمة",
                "ja": u"「お気に入り」は左パネルの上部にあります", "zh": u"“收藏夹”位于左侧栏顶部",
                "th": u"หมวด “รายการโปรด” อยู่ที่ด้านบนของแถบซ้าย",
            }.get(lang, u"Favorites section is at the top of the left rail")
            return ui_api.text(t_fmt % item_name), ui_api.text(h_txt)
        t_fmt = {
            "ru": u"«%s» убран из избранного", "en": u"“%s” removed from favorites", "fr": u"« %s » retiré des favoris",
            "pt": u"«%s» removido dos favoritos", "es": u"«%s» quitado de favoritos", "it": u"«%s» rimosso dai preferiti",
            "de": u"„%s“ aus Favoriten entfernt", "pl": u"„%s” usunięto z ulubionych", "cs": u"„%s“ odebráno z oblíbených",
            "tr": u"«%s» favorilerden çıkarıldı", "he": u"«%s» הוסר מהמועדפים", "ar": u"تمت إزالة «%s» من المفضلة",
            "ja": u"「%s」をお気に入りから削除", "zh": u"“%s”已取消收藏", "th": u"นำ “%s” ออกจากโปรดแล้ว",
        }.get(lang, u"“%s” removed from favorites")
        return ui_api.text(t_fmt % item_name), ""

    raw_list = result.get("list") or u"Избранное"
    lst_name = ui_api.unicode_text(tr_server(raw_list, lang) if raw_list == u"Избранное" else name_of_fn({"name": raw_list}, 20))
    short_item = ui_api.unicode_text(name_of_fn(item, 20 if added else 18))
    if added:
        t_fmt = {
            "ru": u"«%s» — в списке «%s»", "en": u"“%s” — in list “%s”", "fr": u"« %s » — dans « %s »",
            "pt": u"«%s» — na lista «%s»", "es": u"«%s» — en lista «%s»", "it": u"«%s» — nella lista «%s»",
            "de": u"„%s“ — in Liste „%s“", "pl": u"„%s” — na liście „%s”", "cs": u"„%s“ — v seznamu „%s“",
            "tr": u"«%s» — «%s» listesinde", "he": u"«%s» — ברשימה «%s»", "ar": u"«%s» — في قائمة «%s»",
            "ja": u"「%s」を「%s」に追加", "zh": u"“%s” — 已加入“%s”", "th": u"“%s” — อยู่ในรายการ “%s”",
        }.get(lang, u"“%s” — in list “%s”")
        h_txt = {
            "ru": u"В списке каналов ТВ — после закрытия каталога", "en": u"Updates TV channel list when catalog closes",
            "fr": u"Sur la liste TV après fermeture du catalogue", "pt": u"Na lista de TV após fechar o catálogo",
            "es": u"En la lista de TV al cerrar el catálogo", "it": u"Nella lista TV alla chiusura del catalogo",
            "de": u"Auf dem TV nach Schließen des Katalogs", "pl": u"Na liście kanałów TV po zamknięciu katalogu",
            "cs": u"V seznamu TV po zavření katalogu", "tr": u"Katalog kapanınca TV listesine yansır",
            "he": u"יתעדכן ברשימת הערוצים עם סגירת הקטלוג", "ar": u"يتحدث في قائمة التلفاز عند إغلاق الدليل",
            "ja": u"カタログ終了後にTVのリストへ反映されます", "zh": u"关闭目录后将更新到电视频道列表",
            "th": u"อัปเดตในรายการช่องทีวีหลังปิดแคตตาล็อก",
        }.get(lang, u"Updates TV channel list when catalog closes")
    else:
        t_fmt = {
            "ru": u"«%s» убран из списка «%s»", "en": u"“%s” removed from “%s”", "fr": u"« %s » retiré de « %s »",
            "pt": u"«%s» removido de «%s»", "es": u"«%s» quitado de «%s»", "it": u"«%s» rimosso da «%s»",
            "de": u"„%s“ aus „%s“ entfernt", "pl": u"„%s” usunięto z „%s”", "cs": u"„%s“ odebráno z „%s“",
            "tr": u"«%s» «%s» listesinden çıkarıldı", "he": u"«%s» הוסר מ-«%s»", "ar": u"أزيل «%s» من «%s»",
            "ja": u"「%s」を「%s」から削除", "zh": u"“%s”已从“%s”移除", "th": u"นำ “%s” ออกจาก “%s” แล้ว",
        }.get(lang, u"“%s” removed from “%s”")
        h_txt = {
            "ru": u"Список каналов ТВ обновится после закрытия каталога", "en": u"TV channel list updates when catalog closes",
            "fr": u"La liste TV sera mise à jour à la fermeture", "pt": u"A lista de TV atualiza ao fechar o catálogo",
            "es": u"La lista de TV se actualizará al cerrar el catálogo", "it": u"La lista TV si aggiornerà alla chiusura",
            "de": u"TV-Kanalliste wird beim Schließen aktualisiert", "pl": u"Lista kanałów TV odświeży się po zamknięciu",
            "cs": u"Seznam kanálů TV se obnoví po zavření", "tr": u"Katalog kapanınca TV listesi güncellenir",
            "he": u"רשימת הערוצים תתעדכן עם סגירת הקטלוג", "ar": u"تتحدث قائمة التلفاز عند إغلاق الدليل",
            "ja": u"カタログ終了後にTVのチャンネル一覧が更新されます", "zh": u"关闭目录后电视频道列表将自动更新",
            "th": u"รายการช่องทีวีจะอัปเดตเมื่อปิดแคตตาล็อก",
        }.get(lang, u"TV channel list updates when catalog closes")
    text = t_fmt % (short_item, lst_name)
    if result.get("created"):
        c_fmt = {
            "ru": u"Список «%s» создан заново · на ТВ — после закрытия каталога",
            "en": u"List “%s” recreated · on TV after closing catalog",
            "fr": u"Liste « %s » recréée · sur TV après fermeture",
            "pt": u"Lista «%s» recriada · na TV ao fechar",
            "es": u"Lista «%s» creada de nuevo · en TV al cerrar",
            "it": u"Lista «%s» ricreata · su TV alla chiusura",
            "de": u"Liste „%s“ neu erstellt · auf TV nach Schließen",
            "pl": u"Lista „%s” utworzona ponownie · na TV po zamknięciu",
            "cs": u"Seznam „%s“ znovu vytvořen · na TV po zavření",
            "tr": u"«%s» yeniden oluşturuldu · kapanınca TV'de",
            "he": u"רשימה «%s» נוצרה מחדש · בטלוויזיה בסגירה",
            "ar": u"أُعيد إنشاء «%s» · على التلفاز بعد الإغلاق",
            "ja": u"リスト「%s」を再作成 · 終了後にTVへ反映",
            "zh": u"列表“%s”已重建 · 关闭目录后同步至电视",
            "th": u"สร้างรายการ “%s” ใหม่แล้ว · อัปเดตบนทีวีเมื่อปิด",
        }.get(lang, u"List “%s” recreated · on TV after closing catalog")
        h_txt = c_fmt % lst_name
    elif result.get("unsent"):
        h_txt = {
            "ru": u"При закрытии на ТВ уйдут и неотправленные правки с планшета",
            "en": u"Unsent tablet changes will also sync to TV on close",
            "fr": u"Les modifs tablette en attente seront aussi envoyées",
            "pt": u"Alterações pendentes do tablet também serão enviadas",
            "es": u"Cambios pendientes de la tablet también se enviarán",
            "it": u"Anche le modifiche in sospeso verranno inviate alla TV",
            "de": u"Ungesendete Browser-Änderungen werden mit gesendet",
            "pl": u"Niewysłane zmiany z tabletu też trafią na TV",
            "cs": u"Odešlou se i neuložené úpravy z tabletu",
            "tr": u"Kapanışta bekleyen tablet değişiklikleri de gönderilir",
            "he": u"בסגירה יישלחו גם שינויים שטרם נשלחו מהטאבלט",
            "ar": u"عند الإغلاق ستُرسل أيضاً تعديلات المتصفح غير المرسلة",
            "ja": u"終了時にタブレットの未送信変更も送信されます",
            "zh": u"关闭时也会同步网页端未发送的修改",
            "th": u"การแก้ไขที่ยังไม่ส่งจากแท็บเล็ตจะถูกส่งไปด้วยเมื่อปิด",
        }.get(lang, h_txt)
    return ui_api.text(text), ui_api.text(h_txt)


def missing_message(item, name_of_fn, lang=None):
    if lang is None:
        lang = get_language()
    item_name = ui_api.unicode_text(name_of_fn(item))
    t_fmt = {
        "ru": u"«%s» нет в плейлисте", "en": u"“%s” is not in playlist", "fr": u"« %s » absent de la playlist",
        "pt": u"«%s» não está na playlist", "es": u"«%s» no está en la lista", "it": u"«%s» non è in playlist",
        "de": u"„%s“ nicht in Playlist", "pl": u"„%s” brak na playliście", "cs": u"„%s“ není v playlistu",
        "tr": u"«%s» listede yok", "he": u"«%s» אינו בפלייליסט", "ar": u"«%s» غير موجود بالقائمة",
        "ja": u"「%s」はプレイリストにありません", "zh": u"“%s”不在播放列表中", "th": u"“%s” ไม่อยู่ในเพลย์ลิสต์",
    }.get(lang, u"“%s” is not in playlist")
    h_txt = {
        "ru": u"Провайдер убрал канал · долгое OK — убрать из списка",
        "en": u"Provider removed channel · long OK — remove from list",
        "fr": u"Chaîne retirée · OK long — retirer de la liste",
        "pt": u"Canal removido pelo fornecedor · OK longo — remover",
        "es": u"Canal quitado por el proveedor · OK largo — quitar",
        "it": u"Canale rimosso dal provider · OK lungo — rimuovi",
        "de": u"Anbieter hat Kanal entfernt · lang OK — entfernen",
        "pl": u"Dostawca usunął kanał · długie OK — usuń z listy",
        "cs": u"Poskytovatel kanál odebral · dlouhé OK — odebrat",
        "tr": u"Sağlayıcı kanalı kaldırdı · uzun OK — listeden çıkar",
        "he": u"הספק הסיר את הערוץ · OK ארוך — הסר מהרשימה",
        "ar": u"أزال المزود القناة · OK مطول — إزالة من القائمة",
        "ja": u"プロバイダが削除しました · OK長押し — リストから削除",
        "zh": u"提供商已移除该频道 · 长按 OK — 从列表删除",
        "th": u"ผู้ให้บริการลบช่องแล้ว · กด OK ค้าง — ลบออกจากรายการ",
    }.get(lang, u"Provider removed channel · long OK — remove from list")
    return ui_api.text(t_fmt % item_name), ui_api.text(h_txt)
