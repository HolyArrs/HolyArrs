package api

import "github.com/HolyArrs/holyarrs/app/backend/internal/health"

// The settings status rules live in health; these tests still read them by
// their old names.
type (
	statusInput  = health.StatusInput
	statusAnswer = health.StatusAnswer
	statusItem   = health.StatusItem
	namedState   = health.NamedState
)

var settingsStatus = health.SettingsStatus
var settingsChange = health.SettingsChange
