package app

import (
	"errors"
	"strings"
)

func (a *App) CreateBouquet(name string, refs []ChannelRef) (*Bouquet, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, errors.New("придумайте название списка")
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	b := &Bouquet{ID: newID(), Name: name}
	b.Channels = a.addRefs(nil, refs, nil)
	a.st.Bouquets = append(a.st.Bouquets, b)
	a.touch()
	cp := *b
	return &cp, a.save()
}

type BouquetPatch struct {
	Name     *string       `json:"name"`
	Channels *[]ChannelRef `json:"channels"`
}

func (a *App) UpdateBouquet(id string, p BouquetPatch) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	b := a.bouquet(id)
	if b == nil {
		return ErrNotFound
	}
	if p.Name != nil {
		name := strings.TrimSpace(*p.Name)
		if name == "" {
			return errors.New("название не может быть пустым")
		}
		b.Name = name
	}
	if p.Channels != nil {
		// Refs whose channel vanished from the playlist may stay (they come
		// back if the provider restores the channel) but only if they were
		// already in this bouquet.
		prev := map[string]ChannelRef{}
		for _, r := range b.Channels {
			prev[r.key()] = r
		}
		b.Channels = a.addRefs(nil, *p.Channels, prev)
	}
	a.touch()
	return a.save()
}

// ModifyBouquet adds and removes channels without the client having to send
// (and race on) the full list.
func (a *App) ModifyBouquet(id string, add, remove []ChannelRef) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	b := a.bouquet(id)
	if b == nil {
		return ErrNotFound
	}
	drop := map[string]bool{}
	for _, r := range remove {
		drop[r.key()] = true
	}
	kept := make([]ChannelRef, 0, len(b.Channels))
	for _, r := range b.Channels {
		if !drop[r.key()] {
			kept = append(kept, r)
		}
	}
	b.Channels = a.addRefs(kept, add, nil)
	a.touch()
	return a.save()
}

// addRefs appends refs to list, skipping duplicates and unknown channels and
// filling names from the channel cache. Refs found in keep are accepted even
// when their channel is currently missing.
func (a *App) addRefs(list, refs []ChannelRef, keep map[string]ChannelRef) []ChannelRef {
	seen := map[string]bool{}
	for _, r := range list {
		seen[r.key()] = true
	}
	if list == nil {
		list = []ChannelRef{}
	}
	for _, r := range refs {
		if r.Source == "" || r.ID == "" || seen[r.key()] {
			continue
		}
		if ch := a.channel(r.Source, r.ID); ch != nil {
			r.Name = ch.Name
		} else if old, ok := keep[r.key()]; ok {
			r.Name = old.Name
		} else {
			continue
		}
		seen[r.key()] = true
		list = append(list, r)
	}
	return list
}

func (a *App) DeleteBouquet(id string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	kept := a.st.Bouquets[:0]
	found := false
	for _, b := range a.st.Bouquets {
		if b.ID == id {
			found = true
			continue
		}
		kept = append(kept, b)
	}
	if !found {
		return ErrNotFound
	}
	a.st.Bouquets = kept
	a.touch()
	return a.save()
}

func (a *App) ReorderBouquets(ids []string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	pos := map[string]int{}
	for i, id := range ids {
		pos[id] = i
	}
	ordered := make([]*Bouquet, 0, len(a.st.Bouquets))
	var rest []*Bouquet
	for _, id := range ids {
		if b := a.bouquet(id); b != nil {
			ordered = append(ordered, b)
		}
	}
	for _, b := range a.st.Bouquets {
		if _, ok := pos[b.ID]; !ok {
			rest = append(rest, b)
		}
	}
	a.st.Bouquets = append(ordered, rest...)
	a.touch()
	return a.save()
}

func (a *App) UpdateSettings(s Settings) error {
	s.normalize()
	s.WebifURL = strings.TrimRight(strings.TrimSpace(s.WebifURL), "/")
	a.mu.Lock()
	defer a.mu.Unlock()
	prev := a.st.Settings
	a.st.Settings = s
	// These change what ends up in the bouquet files.
	if prev.ServiceType != s.ServiceType || prev.BouquetsOnTop != s.BouquetsOnTop {
		a.touch()
	}
	return a.save()
}
