package migrate

import (
	"fmt"
	"sort"
	"strings"
)

// resultWords say each result in plain words for the report.
var resultWords = map[string]string{
	ResCreate: "would be added", ResCreated: "added", ResAdopt: "would be adopted in place", ResAdopted: "adopted in place",
	ResChange: "would change", ResChanged: "changed", ResMatched: "matched", ResAlready: "already here",
	ResSkipped: "left out", ResNotFound: "not found here", ResFailed: "failed", ResRemoved: "removed", ResKept: "kept",
}

// md keeps text from an old app from breaking the report's tables.
func md(s string) string {
	s = strings.NewReplacer("|", "\\|", "\n", " ", "\r", " ", "<", "&lt;", ">", "&gt;").Replace(s)
	return s
}

// Markdown writes the report as a Markdown file people can keep (the move
// screen offers it as a download).
func (r *Report) Markdown() string {
	var b strings.Builder
	title := "Import report"
	switch {
	case r.Undo:
		title = "Undo report"
	case r.Dry:
		title = "Import dry run"
	}
	fmt.Fprintf(&b, "# HolyArrs %s\n\n", strings.ToLower(title[:1])+title[1:])
	if r.Move != 0 {
		fmt.Fprintf(&b, "- Import: %d\n", r.Move)
	}
	if !r.At.IsZero() {
		fmt.Fprintf(&b, "- When: %s\n", r.At.Format("2006-01-02 15:04 MST"))
	}
	if len(r.Apps) > 0 {
		fmt.Fprintf(&b, "- From: %s\n", strings.Join(r.Apps, ", "))
	}
	keys := make([]string, 0, len(r.Counts))
	for k := range r.Counts {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var parts []string
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%d %s", r.Counts[k], resultWords[k]))
	}
	if len(parts) == 0 {
		// A move that failed before it started has no items.
		parts = []string{"nothing was moved"}
	}
	fmt.Fprintf(&b, "- In short: %s\n\n", strings.Join(parts, ", "))

	b.WriteString("## What to do next\n\n")
	for _, n := range r.Next {
		fmt.Fprintf(&b, "- %s\n", md(n))
	}
	if len(r.NeedsInput) > 0 {
		b.WriteString("\n## Needs your input\n\nThese keys and passwords never come over; type them again in HolyArrs.\n\n")
		for _, n := range r.NeedsInput {
			fmt.Fprintf(&b, "- %s\n", md(n))
		}
	}
	groups := map[string][]Line{}
	var order []string
	for _, l := range r.Lines {
		if _, ok := groups[l.Group]; !ok {
			order = append(order, l.Group)
		}
		groups[l.Group] = append(groups[l.Group], l)
	}
	if len(order) > 0 {
		b.WriteString("\n## What was imported\n")
	}
	for _, g := range order {
		label := GroupLabels[g]
		if label == "" {
			label = g
		}
		fmt.Fprintf(&b, "\n### %s\n\n| Item | Result | Details |\n| --- | --- | --- |\n", label)
		for _, l := range groups[g] {
			fmt.Fprintf(&b, "| %s | %s | %s |\n", md(l.Name), resultWords[l.Result], md(l.Detail))
		}
	}
	if len(r.NotMoved) > 0 {
		b.WriteString("\n## Not imported\n\nThese settings have no HolyArrs match. They stay in the old apps.\n\n| Setting | From | Why |\n| --- | --- | --- |\n")
		for _, n := range r.NotMoved {
			fmt.Fprintf(&b, "| %s | %s | %s |\n", md(n.What), md(n.From), md(n.Why))
		}
	}
	if len(r.Notes) > 0 {
		b.WriteString("\n## Notes from the read\n\n")
		for _, n := range r.Notes {
			fmt.Fprintf(&b, "- %s\n", md(n))
		}
	}
	b.WriteString("\nThe import never changes, moves, renames or deletes a media file, and never writes to the old apps.\n")
	return b.String()
}
