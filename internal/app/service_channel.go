package app

import (
	"dreaminator/internal/enigma"
	"dreaminator/internal/playlist"
)

// ServiceChannel is the playlist channel behind a service the receiver
// plays from a channel list: the TV's EPG key opens its programme and
// archive with it, as the catalog's guide does.
type ServiceChannel struct {
	Source CatalogSource `json:"source"`
	Item   CatalogCard   `json:"item"`
}

// ChannelForService finds the channel of a service reference. A channel of
// our lists is found by the ids its key gave it, so a stream address that
// changed since the lists were sent (a new token) still finds it; any other
// list's entry by its stream address.
func (a *App) ChannelForService(ref string) (ServiceChannel, error) {
	p, ok := enigma.ParseServiceRef(ref)
	if !ok {
		return ServiceChannel{}, ErrNotFound
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	var src *Source
	var found *playlist.Channel
search:
	for _, s := range a.st.Sources {
		list := a.channels[s.ID]
		for i := range list {
			ch := &list[i]
			if p.Ours {
				if sid, tsid := enigma.ServiceIDs(s.ID + "/" + ch.ID); sid == p.SID && tsid == p.TSID {
					src, found = s, ch
					break search
				}
			}
			// Another plugin's line keeps the address as it is (Enigma2
			// escapes '%'); ours only escaped ':', so Enigma2 decoded it.
			if found == nil && (ch.URL == p.Stream || p.Ours && enigma.StreamPath(ch.URL) == p.Stream) {
				// An address match waits: our ids, if any, decide first.
				src, found = s, ch
				if !p.Ours {
					break search
				}
			}
		}
	}
	if found == nil {
		return ServiceChannel{}, ErrNotFound
	}
	cache, _, err := a.catalogLocked(src.ID)
	if err != nil {
		return ServiceChannel{}, err
	}
	key, rec, ok := a.catalogRecordByIDLocked(src.ID, cache, catalogID(src.ID, "itv", found.ID))
	if !ok {
		return ServiceChannel{}, ErrNotFound
	}
	card := CatalogCard{Key: key, ID: rec.item.CatalogID, Name: found.Name, Kind: "channel"}
	if found.Catchup != nil {
		card.Archive = found.Catchup.Days
	}
	return ServiceChannel{Source: catalogSource(src), Item: card}, nil
}
