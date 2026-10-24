# Dreaminator archive from bouquets

The approved entry point is **channel list → Menu → Архив Dreaminator**. It opens the existing Dreaminator daily guide for the selected channel, without browsing the catalog. Past recordings play with OK; the current programme can start from its beginning. Back returns to the same channel list. Existing catalog behavior stays unchanged.

openATV 7.5 provides `WHERE_CHANNEL_CONTEXT_MENU`. The installed OpenHDF on DM520 lacks that hook; a session-start adapter appends one menu item after the original ChannelContextMenu initializer. Installation is idempotent and skips directories, markers and edit mode. No receiver system file is edited. Channel parental protection is checked before opening the archive.

A local POST resolver accepts only the service identity (the first ten reference fields, no stream URL). The service matches a current Dreaminator bouquet channel by its stable EPG identity and returns its source and catalog card. It registers the channel in the catalog cache so the existing guide works after a cold start. Missing, ambiguous or non-archive channels produce a readable error; no name-based matching or credential-bearing data is returned.

A small asynchronous loading screen calls the resolver off the Enigma2 thread, ignores results after closing and opens GuideScreen in archive mode. In this mode OK on an available programme starts its recording; a future programme explains that it has not started. Existing playback and same-row restoration are reused.

Verification covers reference normalization, cold caches, duplicate bouquet membership, missing/non-archive channels, stale callbacks, the native hook, the legacy adapter, archive-only key behavior and Python 2.7/3. Build both receiver packages. Back up and update only Vu+, restart the GUI if idle, open the entry from its actual bouquet and play a past programme. Observe decoder/video progress and capture the receiver output; distinguish verified playback from sound or seeking that cannot be confirmed remotely. Dreambox receives no installation or GUI restart in this task.
