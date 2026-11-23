package arrapi

import (
	"net/http"
)

// hookSchema answers the one kind of record HolyArrs keeps, the webhook,
// in the shape the old apps list their notification kinds. Phone apps
// such as Helmarr read it before they add their own webhook. Every event
// switch is there; all but application updates are sent (HolyArrs
// is updated with its image) and import complete (each file is its own
// import event).
func (h *handler) hookSchema(w http.ResponseWriter, r *http.Request) {
	events := []string{"onGrab", "onDownload", "onUpgrade", "onRename", "onHealthIssue", "onHealthRestored", "onApplicationUpdate", "onManualInteractionRequired"}
	if h.side == "sonarr" {
		events = append(events, "onImportComplete", "onSeriesAdd", "onSeriesDelete", "onEpisodeFileDelete", "onEpisodeFileDeleteForUpgrade")
	} else {
		events = append(events, "onMovieAdded", "onMovieDelete", "onMovieFileDelete", "onMovieFileDeleteForUpgrade")
	}
	m := map[string]any{
		"name": "", "implementation": "Webhook", "implementationName": "Webhook", "configContract": "WebhookSettings",
		"infoLink": "https://github.com/HolyArrs/HolyArrs", "includeHealthWarnings": false, "tags": []int64{}, "presets": []any{},
		"fields": []map[string]any{
			{"order": 0, "name": "url", "label": "Webhook URL", "type": "url", "advanced": false, "privacy": "normal", "isFloat": false},
			{"order": 1, "name": "method", "label": "Method", "helpText": "Which HTTP method to use submit to the Webservice", "value": 1, "type": "select", "advanced": false,
				"selectOptions": []map[string]any{{"value": 1, "name": "POST", "order": 1, "dividerAfter": false}, {"value": 2, "name": "PUT", "order": 2, "dividerAfter": false}},
				"privacy":       "normal", "isFloat": false},
			{"order": 2, "name": "username", "label": "Username", "type": "textbox", "advanced": false, "privacy": "userName", "isFloat": false},
			{"order": 3, "name": "password", "label": "Password", "type": "password", "advanced": false, "privacy": "password", "isFloat": false},
			{"order": 4, "name": "headers", "label": "Headers", "value": []any{}, "type": "keyValueList", "advanced": true, "privacy": "normal", "isFloat": false},
		},
	}
	for _, e := range events {
		m[e] = false
		m["supportsO"+e[1:]] = e != "onApplicationUpdate" && e != "onImportComplete"
	}
	writeJSON(w, http.StatusOK, []map[string]any{m})
}
