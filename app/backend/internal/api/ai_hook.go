package api

import "github.com/HolyArrs/holyarrs/app/backend/internal/ai"

// AIHandlers is the AI connection handlers; full-server tests move its clock.
func (a *API) AIHandlers() *ai.Handlers { return a.ai }
