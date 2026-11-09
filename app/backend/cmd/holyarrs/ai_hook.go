package main

import "github.com/HolyArrs/holyarrs/app/backend/internal/ai"

// AIHandlers is the AI connection handlers; full-server tests move its clock.
func (a *handlers) AIHandlers() *ai.Handlers { return a.ai }
