package main

import (
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// ShowResult is a looked-up show as the lookup route answers it, with the
// open ask read back as an askBrief.
type ShowResult struct {
	metadata.Show
	TitleID  int64     `json:"title_id,omitempty"`
	NeverAdd bool      `json:"never_add,omitempty"`
	Ask      *askBrief `json:"ask,omitempty"`
}
