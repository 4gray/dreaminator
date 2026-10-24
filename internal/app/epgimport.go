package app

import (
	"strings"

	"dreaminator/internal/enigma"
)

// epgImportMapping uses the same IPTV references as the receiver's bouquets.
// Some EPGImport versions validate bare DVB references by trying to tune them;
// our synthetic triplets are not DVB services and would all be discarded.
func (a *App) epgImportMapping(mapping map[string][]string, serviceType int) map[string][]string {
	a.mu.Lock()
	byRef := map[string]string{}
	for _, b := range a.buildBouquets() {
		for _, svc := range b.Services {
			byRef[enigma.EPGRef(svc.Key)] = enigma.ServiceRef(serviceType, svc)
		}
	}
	a.mu.Unlock()

	out := make(map[string][]string, len(mapping))
	for id, refs := range mapping {
		for _, ref := range refs {
			if iptv, ok := byRef[ref]; ok {
				out[id] = append(out[id], iptv)
			} else {
				out[id] = append(out[id], ref)
			}
		}
	}
	return out
}

// epgImportKey recovers the stable cache reference from either an old bare
// mapping or a full IPTV reference. URL/name changes do not change that key.
func epgImportKey(ref string) string {
	parts := strings.SplitN(ref, ":", 11)
	if len(parts) == 11 {
		switch parts[0] {
		case "1", "4097", "5001", "5002":
			parts[0], parts[10] = "1", ""
			return strings.Join(parts, ":")
		}
	}
	return ref
}
