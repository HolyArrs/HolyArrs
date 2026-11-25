package hooks

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"regexp"
	"strings"
	"unicode/utf8"
)

// Card is a Home widget or a title page section: plain data that HolyArrs
// draws with its own parts. A plugin never sends markup or script.
type Card struct {
	Title string    `json:"title"`
	Rows  []CardRow `json:"rows"`
}

// CardRow is one line of a Card. Link is a page inside HolyArrs only.
type CardRow struct {
	Text    string `json:"text"`
	Sub     string `json:"sub,omitempty"`
	TitleID int64  `json:"title_id,omitempty"`
	Link    string `json:"link,omitempty"`
}

// Tool is one assistant tool a plugin offers. Scopes are the plugin scopes
// the tool uses: it is listed only for people whose own rights cover them.
type Tool struct {
	Name   string         `json:"name"`
	About  string         `json:"about"`
	Scopes []string       `json:"scopes,omitempty"`
	Input  map[string]any `json:"input,omitempty"`
}

// Page is one plugin page, a file in the plugin's own pages folder.
type Page struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	File  string `json:"file"`
}

const (
	textMax  = 200
	rowsMax  = 20
	toolsMax = 20
	pagesMax = 8
)

var (
	nameRe = regexp.MustCompile(`^[a-z][a-z0-9_]{0,39}$`)
	pageRe = regexp.MustCompile(`^[a-z][a-z0-9-]{0,29}$`)
	linkRe = regexp.MustCompile(`^/[a-z0-9][a-z0-9/_\-?=&.]*$`)
)

// decodeStrict reads b into v, refusing unknown fields and trailing data.
func decodeStrict(b []byte, v any) error {
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		return fmt.Errorf("not valid: %w", err)
	}
	if d.More() {
		return errors.New("not valid: trailing data")
	}
	return nil
}

// plainText is short text with no markup.
func plainText(s string, need bool) error {
	switch {
	case need && strings.TrimSpace(s) == "":
		return errors.New("text is empty")
	case utf8.RuneCountInString(s) > textMax:
		return fmt.Errorf("text is over %d characters", textMax)
	case strings.ContainsAny(s, "<>"):
		return errors.New("text has markup")
	}
	return nil
}

// CheckCard reads a widget or section and refuses anything but plain data.
func CheckCard(b []byte) (Card, error) {
	var c Card
	if err := decodeStrict(b, &c); err != nil {
		return Card{}, err
	}
	if err := plainText(c.Title, true); err != nil {
		return Card{}, fmt.Errorf("title: %w", err)
	}
	if len(c.Rows) > rowsMax {
		return Card{}, fmt.Errorf("more than %d rows", rowsMax)
	}
	for i, r := range c.Rows {
		if err := plainText(r.Text, true); err != nil {
			return Card{}, fmt.Errorf("row %d: %w", i+1, err)
		}
		if err := plainText(r.Sub, false); err != nil {
			return Card{}, fmt.Errorf("row %d: %w", i+1, err)
		}
		if r.Link != "" && (!linkRe.MatchString(r.Link) || strings.Contains(r.Link, "..") || r.Link == "/api" || strings.HasPrefix(r.Link, "/api/")) {
			return Card{}, fmt.Errorf("row %d: links go to a HolyArrs page only", i+1)
		}
		if r.TitleID < 0 {
			return Card{}, fmt.Errorf("row %d: bad title id", i+1)
		}
	}
	if c.Rows == nil {
		c.Rows = []CardRow{}
	}
	return c, nil
}

// CheckTools reads a plugin's tool list. valid says whether a scope name is
// known.
func CheckTools(b []byte, valid func(string) bool) ([]Tool, error) {
	var ts []Tool
	if err := decodeStrict(b, &ts); err != nil {
		return nil, err
	}
	if len(ts) > toolsMax {
		return nil, fmt.Errorf("more than %d tools", toolsMax)
	}
	seen := map[string]bool{}
	for _, t := range ts {
		if !nameRe.MatchString(t.Name) || seen[t.Name] {
			return nil, fmt.Errorf("tool name %q: use a-z, 0-9 and _, once each", clip(t.Name, 40))
		}
		seen[t.Name] = true
		if err := plainText(t.About, true); err != nil {
			return nil, fmt.Errorf("tool %s: %w", t.Name, err)
		}
		if len(t.Scopes) == 0 {
			return nil, fmt.Errorf("tool %s: list the permissions it uses", t.Name)
		}
		for _, s := range t.Scopes {
			if !valid(s) {
				return nil, fmt.Errorf("tool %s: unknown permission %q", t.Name, clip(s, 60))
			}
		}
	}
	return ts, nil
}

// CheckPages reads a plugin's page list.
func CheckPages(b []byte) ([]Page, error) {
	var ps []Page
	if err := decodeStrict(b, &ps); err != nil {
		return nil, err
	}
	if len(ps) > pagesMax {
		return nil, fmt.Errorf("more than %d pages", pagesMax)
	}
	seen := map[string]bool{}
	for _, p := range ps {
		if !pageRe.MatchString(p.ID) || seen[p.ID] {
			return nil, fmt.Errorf("page id %q: use a-z, 0-9 and -, once each", clip(p.ID, 30))
		}
		seen[p.ID] = true
		if err := plainText(p.Title, true); err != nil {
			return nil, fmt.Errorf("page %s: %w", p.ID, err)
		}
		if !safeFile(p.File) || path.Ext(p.File) != ".html" {
			return nil, fmt.Errorf("page %s: file must be an .html file inside the pages folder", p.ID)
		}
	}
	return ps, nil
}

// safeFile is a relative slash path that stays inside its folder.
func safeFile(f string) bool {
	if f == "" || strings.HasPrefix(f, "/") || strings.Contains(f, "\\") || strings.ContainsRune(f, 0) {
		return false
	}
	c := path.Clean(f)
	return c == f && c != "." && !strings.HasPrefix(c, "../") && c != ".."
}
