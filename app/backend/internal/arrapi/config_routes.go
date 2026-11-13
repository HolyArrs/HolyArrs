package arrapi

import (
	"net/http"
	"slices"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// configRoutes adds the settings routes both sides answer (see config.go).
func (h *handler) configRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /delayprofile", h.listDelay)
	mux.HandleFunc("GET /delayprofile/{id}", h.getDelay)
	mux.HandleFunc("POST /delayprofile", h.addDelay)
	mux.HandleFunc("PUT /delayprofile/{id}", h.putDelay)
	mux.HandleFunc("DELETE /delayprofile/{id}", h.deleteDelay)
	mux.HandleFunc("GET /qualitydefinition", h.listSizes)
	mux.HandleFunc("GET /qualitydefinition/{id}", h.getSize)
	mux.HandleFunc("PUT /qualitydefinition/update", h.putSizes)
	mux.HandleFunc("PUT /qualitydefinition/{id}", h.putSize)
	mux.HandleFunc("GET /config/mediamanagement", h.getMedia)
	mux.HandleFunc("GET /config/mediamanagement/{id}", h.getMedia)
	mux.HandleFunc("PUT /config/mediamanagement", h.putMedia)
	mux.HandleFunc("PUT /config/mediamanagement/{id}", h.putMedia)
	mux.HandleFunc("GET /config/naming", h.getNaming)
	mux.HandleFunc("GET /config/naming/{id}", h.getNaming)
	mux.HandleFunc("PUT /config/naming", h.putNaming)
	mux.HandleFunc("PUT /config/naming/{id}", h.putNaming)
}

// Delay rules. Id 1 is the default rule every app has and cannot be removed.

func defaultDelay() obj {
	return obj{"id": 1, "enableUsenet": true, "enableTorrent": true, "preferredProtocol": "usenet", "usenetDelay": 0, "torrentDelay": 0,
		"bypassIfHighestQuality": false, "bypassIfAboveCustomFormatScore": false, "minimumCustomFormatScore": 0, "order": 2147483647, "tags": []any{}}
}

func (h *handler) delayRules(w http.ResponseWriter, r *http.Request) ([]obj, bool) {
	var l []obj
	if _, err := readConfig(r.Context(), h.Store, h.side, "delay", &l); err != nil {
		internalError(w, err)
		return nil, false
	}
	if !slices.ContainsFunc(l, func(o obj) bool { return sameID(o, 1) }) {
		l = append([]obj{defaultDelay()}, l...)
	}
	return l, true
}

func (h *handler) listDelay(w http.ResponseWriter, r *http.Request) {
	if l, ok := h.delayRules(w, r); ok {
		writeJSON(w, http.StatusOK, l)
	}
}

func (h *handler) getDelay(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	l, ok2 := h.delayRules(w, r)
	if !ok || !ok2 {
		return
	}
	if i := slices.IndexFunc(l, func(o obj) bool { return sameID(o, id) }); i >= 0 {
		writeJSON(w, http.StatusOK, l[i])
		return
	}
	notFound(w)
}

func (h *handler) addDelay(w http.ResponseWriter, r *http.Request) {
	var in obj
	if !mayWriteRules(w, r) || !readJSONBody(w, r, &in) {
		return
	}
	l, ok := h.delayRules(w, r)
	if !ok {
		return
	}
	next := int64(1)
	for _, o := range l {
		next = max(next, int64(num(o["id"])))
	}
	in["id"] = next + 1
	if !h.saveAndFold(w, r, "delay", append(l, in)) {
		return
	}
	writeJSON(w, http.StatusCreated, in)
}

func (h *handler) putDelay(w http.ResponseWriter, r *http.Request) {
	if !mayWriteRules(w, r) {
		return
	}
	id, ok := pathID(w, r)
	var in obj
	if !ok || !readJSONBody(w, r, &in) {
		return
	}
	l, ok := h.delayRules(w, r)
	if !ok {
		return
	}
	i := slices.IndexFunc(l, func(o obj) bool { return sameID(o, id) })
	if i < 0 {
		notFound(w)
		return
	}
	in["id"] = id
	l[i] = in
	if h.saveAndFold(w, r, "delay", l) {
		writeJSON(w, http.StatusAccepted, in)
	}
}

func (h *handler) deleteDelay(w http.ResponseWriter, r *http.Request) {
	if !mayWriteRules(w, r) {
		return
	}
	id, ok := pathID(w, r)
	l, ok2 := h.delayRules(w, r)
	if !ok || !ok2 {
		return
	}
	if id == 1 {
		message(w, http.StatusBadRequest, "the default delay profile cannot be removed")
		return
	}
	i := slices.IndexFunc(l, func(o obj) bool { return sameID(o, id) })
	if i < 0 {
		notFound(w)
		return
	}
	if h.saveAndFold(w, r, "delay", slices.Delete(l, i, i+1)) {
		writeJSON(w, http.StatusOK, obj{})
	}
}

// The size table: one row per quality of the side, MB a minute.

func (h *handler) sizeRows(w http.ResponseWriter, r *http.Request) ([]obj, bool) {
	var l []obj
	found, err := readConfig(r.Context(), h.Store, h.side, "sizes", &l)
	if err != nil {
		internalError(w, err)
		return nil, false
	}
	if !found {
		for i, q := range sideQualities[h.side] {
			l = append(l, obj{"id": i + 1, "quality": obj{"id": q.ID, "name": q.Name}, "title": q.Name, "weight": i + 1,
				"minSize": 0, "maxSize": nil, "preferredSize": nil})
		}
	}
	return l, true
}

func (h *handler) listSizes(w http.ResponseWriter, r *http.Request) {
	if l, ok := h.sizeRows(w, r); ok {
		writeJSON(w, http.StatusOK, l)
	}
}

func (h *handler) getSize(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	l, ok2 := h.sizeRows(w, r)
	if !ok || !ok2 {
		return
	}
	if i := slices.IndexFunc(l, func(o obj) bool { return sameID(o, id) }); i >= 0 {
		writeJSON(w, http.StatusOK, l[i])
		return
	}
	notFound(w)
}

// takeSizes copies the sent sizes and title onto the matching row, by id
// and then by quality name; -1 when no row matches.
func takeSizes(l []obj, in obj) int {
	q, _ := in["quality"].(obj)
	name := strOf(q["name"])
	i := slices.IndexFunc(l, func(o obj) bool { return in["id"] != nil && sameID(o, int64(num(in["id"]))) })
	if i < 0 && name != "" {
		i = slices.IndexFunc(l, func(o obj) bool { oq, _ := o["quality"].(obj); return strOf(oq["name"]) == name })
	}
	if i < 0 {
		return -1
	}
	for _, k := range []string{"minSize", "maxSize", "preferredSize", "title"} {
		if v, ok := in[k]; ok {
			l[i][k] = v
		}
	}
	return i
}

func (h *handler) putSizes(w http.ResponseWriter, r *http.Request) {
	var in []obj
	if !mayWriteRules(w, r) || !readJSONBody(w, r, &in) {
		return
	}
	l, ok := h.sizeRows(w, r)
	if !ok {
		return
	}
	for _, row := range in {
		takeSizes(l, row)
	}
	if h.saveAndFold(w, r, "sizes", l) {
		writeJSON(w, http.StatusAccepted, l)
	}
}

func (h *handler) putSize(w http.ResponseWriter, r *http.Request) {
	if !mayWriteRules(w, r) {
		return
	}
	id, ok := pathID(w, r)
	var in obj
	if !ok || !readJSONBody(w, r, &in) {
		return
	}
	l, ok := h.sizeRows(w, r)
	if !ok {
		return
	}
	in["id"] = id
	i := takeSizes(l, in)
	if i < 0 {
		notFound(w)
		return
	}
	if h.saveAndFold(w, r, "sizes", l) {
		writeJSON(w, http.StatusAccepted, l[i])
	}
}

// Media switches: the repack choice maps; the rest is kept as sent.

func defaultMedia() obj {
	return obj{"id": 1, "downloadPropersAndRepacks": "preferAndUpgrade", "enableMediaInfo": true, "copyUsingHardlinks": true,
		"deleteEmptyFolders": false, "importExtraFiles": false, "extraFileExtensions": "srt", "fileDate": "none", "recycleBin": "",
		"recycleBinCleanupDays": 7, "setPermissionsLinux": false, "skipFreeSpaceCheckWhenImporting": false,
		"minimumFreeSpaceWhenImporting": 100, "rescanAfterRefresh": "always"}
}

func (h *handler) getMedia(w http.ResponseWriter, r *http.Request) {
	m := defaultMedia()
	if _, err := readConfig(r.Context(), h.Store, h.side, "media", &m); err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, m)
}

func (h *handler) putMedia(w http.ResponseWriter, r *http.Request) {
	var in obj
	if !mayWriteRules(w, r) || !readJSONBody(w, r, &in) {
		return
	}
	in["id"] = 1
	if h.saveAndFold(w, r, "media", in) {
		writeJSON(w, http.StatusAccepted, in)
	}
}

// Naming. Colon handling is a word on Radarr and a number on Sonarr.

var colonWords = []string{"delete", "dash", "spaceDash", "spaceDashSpace", "smart"}

var colonOurs = []string{mediafiles.ColonRemove, mediafiles.ColonDash, mediafiles.ColonSpaceDash, mediafiles.ColonSpaceDashSpace, mediafiles.ColonSmart}

// multiOurs is our multi-episode style for each Sonarr number: extend,
// duplicate (none here), repeat, scene, range, prefixed range.
var multiOurs = []string{mediafiles.MultiShortRange, "", mediafiles.MultiEach, mediafiles.MultiEachDash, mediafiles.MultiShortRange, mediafiles.MultiRange}

func (h *handler) getNaming(w http.ResponseWriter, r *http.Request) {
	n, err := mediafiles.ReadNaming(r.Context(), h.Store.Read())
	if err != nil {
		internalError(w, err)
		return
	}
	m := obj{"id": 1, "replaceIllegalCharacters": true}
	if h.side == "radarr" {
		m["standardMovieFormat"], m["movieFolderFormat"] = n.FilePattern, n.FolderPattern
	} else {
		m["standardEpisodeFormat"], m["dailyEpisodeFormat"], m["animeEpisodeFormat"] = n.EpisodePattern, n.DailyPattern, n.AnimePattern
		m["seriesFolderFormat"], m["seasonFolderFormat"], m["specialsFolderFormat"] = n.ShowFolderPattern, n.SeasonFolderPattern, n.SpecialsFolder
		m["customColonReplacementFormat"] = ""
	}
	if _, err := readConfig(r.Context(), h.Store, h.side, "naming", &m); err != nil {
		internalError(w, err)
		return
	}
	// What HolyArrs uses wins over what was sent.
	m[map[string]string{"radarr": "renameMovies", "sonarr": "renameEpisodes"}[h.side]] = n.RenameOnImport
	m["replaceIllegalCharacters"] = n.Illegal == mediafiles.IllegalReplace
	if c := slices.Index(colonOurs, n.Colon); c >= 0 {
		if h.side == "radarr" {
			m["colonReplacementFormat"] = colonWords[c]
		} else if num(m["colonReplacementFormat"]) != 5 { // 5 is a custom text, kept as sent
			m["colonReplacementFormat"] = c
		}
	}
	if h.side == "sonarr" {
		if s := int(num(m["multiEpisodeStyle"])); s < 0 || s >= len(multiOurs) || multiOurs[s] != n.MultiEpisodeStyle {
			m["multiEpisodeStyle"] = map[string]int{mediafiles.MultiShortRange: 4, mediafiles.MultiRange: 5,
				mediafiles.MultiEach: 2, mediafiles.MultiEachDash: 3}[n.MultiEpisodeStyle]
		}
	}
	writeJSON(w, http.StatusOK, m)
}

func (h *handler) putNaming(w http.ResponseWriter, r *http.Request) {
	var in obj
	if !mayWriteRules(w, r) || !readJSONBody(w, r, &in) {
		return
	}
	in["id"] = 1
	ctx := r.Context()
	n, err := mediafiles.ReadNaming(ctx, h.Store.Read())
	if err != nil {
		internalError(w, err)
		return
	}
	if v, ok := in[map[string]string{"radarr": "renameMovies", "sonarr": "renameEpisodes"}[h.side]].(bool); ok {
		n.RenameOnImport = v
	}
	if v, ok := in["replaceIllegalCharacters"].(bool); ok {
		n.Illegal = map[bool]string{true: mediafiles.IllegalReplace, false: mediafiles.IllegalRemove}[v]
	}
	switch c := in["colonReplacementFormat"].(type) {
	case string:
		if i := slices.Index(colonWords, c); i >= 0 {
			n.Colon = colonOurs[i]
		}
	case float64:
		if i := int(c); i >= 0 && i < len(colonOurs) {
			n.Colon = colonOurs[i]
		}
	}
	if s, ok := in["multiEpisodeStyle"].(float64); ok && h.side == "sonarr" && s >= 0 && int(s) < len(multiOurs) && multiOurs[int(s)] != "" {
		n.MultiEpisodeStyle = multiOurs[int(s)]
	}
	if err := mediafiles.PutNamingSettings(ctx, h.Store, n, actor(r)); err != nil {
		internalError(w, err)
		return
	}
	if err := saveConfig(ctx, h.Store, h.side, "naming", in, actor(r)); err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, in)
}
