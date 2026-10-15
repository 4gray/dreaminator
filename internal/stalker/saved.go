package stalker

// SavedItem is a catalog entry as the library keeps it: enough to show its
// row, to list a series' seasons and to mint a fresh link, without the page
// it was found on. Cmd and Poster stay inside the service; they are written
// to the receiver's own flash only.
type SavedItem struct {
	Name         string   `json:"name"`
	Kind         string   `json:"kind"`
	Type         string   `json:"type"`
	ID           string   `json:"id"`
	MovieID      string   `json:"movieId,omitempty"`
	SeasonID     string   `json:"seasonId,omitempty"`
	Episode      string   `json:"episode,omitempty"`
	Cmd          string   `json:"cmd,omitempty"`
	NeedsLink    bool     `json:"needsLink,omitempty"`
	Poster       string   `json:"poster,omitempty"`
	Year         string   `json:"year,omitempty"`
	Duration     string   `json:"duration,omitempty"`
	OriginalName string   `json:"originalName,omitempty"`
	Genres       string   `json:"genres,omitempty"`
	Description  string   `json:"description,omitempty"`
	Ratings      []Rating `json:"ratings,omitempty"`
	Age          string   `json:"age,omitempty"`
	Director     string   `json:"director,omitempty"`
	Actors       string   `json:"actors,omitempty"`
	Country      string   `json:"country,omitempty"`
	Number       string   `json:"number,omitempty"`
	TMDBID       string   `json:"tmdbId,omitempty"`
	Embedded     []string `json:"embedded,omitempty"` // episode numbers of a VOD series without seasons
	Lazy         bool     `json:"lazy,omitempty"`     // a VOD series whose seasons the portal lists on request
}

// savedDescription keeps library.json small: the card shows the rest from
// TMDB when it is matched.
const savedDescription = 600

func (item CatalogItem) Saved() SavedItem {
	return SavedItem{Name: item.Name, Kind: item.Kind, Type: item.Type, ID: item.ID, MovieID: item.MovieID, SeasonID: item.SeasonID,
		Episode: item.Episode, Cmd: item.Cmd, NeedsLink: item.NeedsLink, Poster: item.Poster, Year: item.Year, Duration: item.Duration,
		OriginalName: item.OriginalName, Genres: item.Genres, Description: clip(item.Description, savedDescription), Ratings: item.Ratings,
		Age: item.Age, Director: item.Director, Actors: item.Actors, Country: item.Country, Number: item.Number, TMDBID: item.TMDBID,
		Embedded: item.embedded, Lazy: item.lazy}
}

// Item turns a saved entry back into a catalog entry (without a navigation key).
func (s SavedItem) Item() CatalogItem {
	return CatalogItem{Name: s.Name, Kind: s.Kind, Type: s.Type, ID: s.ID, MovieID: s.MovieID, SeasonID: s.SeasonID, Episode: s.Episode,
		Cmd: s.Cmd, NeedsLink: s.NeedsLink, Poster: s.Poster, HasPoster: s.Poster != "", Year: s.Year, Duration: s.Duration,
		OriginalName: s.OriginalName, Genres: s.Genres, Description: s.Description, Ratings: s.Ratings, Age: s.Age, Director: s.Director,
		Actors: s.Actors, Country: s.Country, Number: s.Number, TMDBID: s.TMDBID, embedded: s.Embedded, lazy: s.Lazy}
}

// Same reports whether two saved copies would play and show alike, so a
// library entry is rewritten only when the provider changed something.
func (s SavedItem) Same(o SavedItem) bool {
	if s.Name != o.Name || s.Cmd != o.Cmd || s.Poster != o.Poster || s.Year != o.Year || s.MovieID != o.MovieID ||
		s.Lazy != o.Lazy || len(s.Embedded) != len(o.Embedded) || len(s.Ratings) != len(o.Ratings) {
		return false
	}
	for i := range s.Embedded {
		if s.Embedded[i] != o.Embedded[i] {
			return false
		}
	}
	for i := range s.Ratings {
		if s.Ratings[i] != o.Ratings[i] {
			return false
		}
	}
	return true
}
