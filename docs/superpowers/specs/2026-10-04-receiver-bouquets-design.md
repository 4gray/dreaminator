# Receiver bouquet editor

Approved in conversation: show receiver bouquets separately from Dreaminator lists, allow viewing, renaming, reordering and removing entries, make independent copies, and retain a recoverable backup. Existing provider lists must remain untouched until explicitly saved.

The Lists page has two sections, Dreaminator and Receiver bouquets. The latter reads registered TV bouquets from bouquets.tv, excluding Dreaminator-managed files. Missing bouquets remain visible with an explanation. Raw service references, markers and other directives are preserved; the editor never converts DVB or provider IPTV references into Dreaminator playlist channels. DVB names may be resolved through OpenWebif when absent from the file.

Edits are drafts until Save to TV. Saving changes only the selected bouquet file, stores one undo snapshot, and reloads Enigma2 through OpenWebif. File revision checks reject stale saves/undo after external changes. Unknown entry IDs, duplicate IDs, path traversal and symlink targets are rejected. Copies use a separate filename namespace and preserve the existing index. Reload failures are reported as saved-but-not-reloaded with a retry action.

Provider plugins may replace their own bouquets later. The editor explains this and offers Create copy. Copies remain receiver bouquets and are not removed by Dreaminator playlist synchronization. There is no native bouquet deletion, stream URL editing or automatic migration into playlist state in this change.

Verification uses mixed DVB/IPTV/marker fixtures, byte-level service block preservation, stale edits, undo, missing files, traversal/symlinks and managed-writer coexistence; API and browser flows are checked against a scratch receiver. Deployment verifies reading real bouquets without altering provider originals.
