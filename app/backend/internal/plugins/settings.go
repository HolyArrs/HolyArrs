package plugins

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// Setting is one field a plugin declares in its manifest. HolyArrs shows it
// on the plugin's settings sheet and hands the saved value to the plugin as
// HOLYARRS_PLUGIN_SETTING_<KEY> when it starts.
type Setting struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	// Type is text, number, toggle, select or secret.
	Type string `json:"type"`
	// Default is the value used until the owner saves one.
	Default flexString `json:"default,omitempty"`
	// Options are the choices of a select.
	Options []string `json:"options,omitempty"`
	Help    string   `json:"help,omitempty"`
}

// flexString reads a JSON string, number or true/false as plain text.
type flexString string

func (f *flexString) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err == nil {
		*f = flexString(s)
		return nil
	}
	b = bytes.TrimSpace(b)
	if string(b) == "true" || string(b) == "false" {
		*f = flexString(b)
		return nil
	}
	if _, err := strconv.ParseFloat(string(b), 64); err == nil {
		*f = flexString(b)
		return nil
	}
	return fmt.Errorf("default must be text, a number or true/false")
}

const (
	maxSettings   = 100
	maxValue      = 2000
	settingsName  = "settings.json"
	sealedPrefix  = "sealed:"
	settingsLabel = "holyarrs plugin settings"
)

var settingKeyRe = regexp.MustCompile(`^[a-z][a-z0-9_]{0,39}$`)

// ErrNeedsNewerApp refuses a plugin that needs a newer HolyArrs.
var ErrNeedsNewerApp = errors.New("this plugin needs a newer HolyArrs")

// ErrSettings marks a refused settings save.
var ErrSettings = errors.New("plugin settings refused")

// checkSettings refuses unknown types, bad keys, markup and selects without options.
func checkSettings(list []Setting) error {
	bad := func(f string, a ...any) error {
		return fmt.Errorf("%w: settings: %s", ErrManifest, fmt.Sprintf(f, a...))
	}
	if len(list) > maxSettings {
		return bad("more than %d fields", maxSettings)
	}
	seen := map[string]bool{}
	plain := func(s string, max int) bool { return len(s) <= max && !strings.ContainsAny(s, "<>") }
	for _, s := range list {
		if !settingKeyRe.MatchString(s.Key) || seen[s.Key] {
			return bad("key %q must be unique lowercase letters, digits and _", s.Key)
		}
		seen[s.Key] = true
		if strings.TrimSpace(s.Label) == "" || !plain(s.Label, 60) || !plain(s.Help, maxShort) || !plain(string(s.Default), 200) {
			return bad("%q has an empty, long or marked-up label, help or default", s.Key)
		}
		switch s.Type {
		case "text", "secret":
		case "number":
			if s.Default != "" {
				if n, err := strconv.ParseFloat(string(s.Default), 64); err != nil || math.IsNaN(n) || math.IsInf(n, 0) {
					return bad("%q default is not a number", s.Key)
				}
			}
		case "toggle":
			if s.Default != "" && s.Default != "true" && s.Default != "false" {
				return bad("%q default must be true or false", s.Key)
			}
		case "select":
			if len(s.Options) == 0 || len(s.Options) > maxList {
				return bad("%q needs 1 to %d options", s.Key, maxList)
			}
			for _, o := range s.Options {
				if strings.TrimSpace(o) == "" || !plain(o, 60) {
					return bad("%q has an empty or long option", s.Key)
				}
			}
			if s.Default != "" && !contains(s.Options, string(s.Default)) {
				return bad("%q default is not one of its options", s.Key)
			}
		default:
			return bad("%q has unknown type %q", s.Key, s.Type)
		}
		if s.Type == "secret" && s.Default != "" {
			return bad("%q is a secret and can't have a default", s.Key)
		}
	}
	return nil
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// SettingsView is what the settings route shows: saved or default values for
// plain fields, and only whether each secret is set.
type SettingsView struct {
	Fields     []Setting         `json:"fields"`
	Values     map[string]string `json:"values"`
	SecretsSet map[string]bool   `json:"secrets_set"`
	// StartError is set when the settings were saved but the running plugin
	// could not start again with them (it is then stopped, with the reason).
	StartError string `json:"start_error,omitempty"`
}

// defaultValue is a field's value until the owner saves one: its default, or
// off for a toggle and the first choice for a select that has none.
func (f Setting) defaultValue() string {
	if f.Default != "" {
		return string(f.Default)
	}
	switch f.Type {
	case "toggle":
		return "false"
	case "select":
		if len(f.Options) > 0 {
			return f.Options[0]
		}
	}
	return ""
}

// ErrRestart is a settings save that was stored, but the plugin did not
// start again with it.
var ErrRestart = errors.New("settings saved, but the plugin could not start again")

// SettingsInput is a save: new values, and secrets to clear. A secret left
// out (or empty) keeps its saved value.
type SettingsInput struct {
	Values map[string]string `json:"values"`
	Clear  []string          `json:"clear"`
}

func (e *Engine) settingsPath(id string) string {
	return filepath.Join(e.Dir, id, settingsName)
}

func (e *Engine) loadSettings(id string) map[string]string {
	out := map[string]string{}
	b, err := os.ReadFile(e.settingsPath(id))
	if err == nil {
		_ = json.Unmarshal(b, &out)
	}
	return out
}

// fields is the installed plugin's declared settings.
func (e *Engine) fields(id string) ([]Setting, error) {
	if _, err := e.installed(id); err != nil {
		return nil, err
	}
	m := e.readManifest(id)
	if m == nil {
		return nil, nil
	}
	return m.Settings, nil
}

// Settings returns id's fields and values; secrets are never shown back.
func (e *Engine) Settings(id string) (*SettingsView, error) {
	if err := e.init(); err != nil {
		return nil, err
	}
	fields, err := e.fields(id)
	if err != nil {
		return nil, err
	}
	saved := e.loadSettings(id)
	v := &SettingsView{Fields: fields, Values: map[string]string{}, SecretsSet: map[string]bool{}}
	if v.Fields == nil {
		v.Fields = []Setting{}
	}
	for _, f := range fields {
		val, ok := saved[f.Key]
		if f.Type == "secret" {
			v.SecretsSet[f.Key] = ok && val != ""
			continue
		}
		if !ok {
			val = f.defaultValue()
		}
		v.Values[f.Key] = val
	}
	return v, nil
}

// SaveSettings checks in against id's fields, stores it (secrets sealed) and
// restarts the plugin when it runs, so it starts with the new values.
func (e *Engine) SaveSettings(id string, in SettingsInput) error {
	if err := e.init(); err != nil {
		return err
	}
	e.opMu.Lock()
	defer e.opMu.Unlock()
	fields, err := e.fields(id)
	if err != nil {
		return err
	}
	byKey := map[string]Setting{}
	for _, f := range fields {
		byKey[f.Key] = f
	}
	saved := e.loadSettings(id)
	for k, v := range in.Values {
		f, ok := byKey[k]
		if !ok {
			return fmt.Errorf("%w: unknown field %q", ErrSettings, k)
		}
		if len(v) > maxValue {
			return fmt.Errorf("%w: %q is too long", ErrSettings, f.Label)
		}
		switch f.Type {
		case "secret":
			if v == "" {
				continue // keeps the saved one
			}
			sealed, err := e.seal(v)
			if err != nil {
				return err
			}
			saved[k] = sealed
			continue
		case "number":
			if n, err := strconv.ParseFloat(v, 64); v != "" && (err != nil || math.IsNaN(n) || math.IsInf(n, 0)) {
				return fmt.Errorf("%w: %q must be a number", ErrSettings, f.Label)
			}
		case "toggle", "select":
			if v == "" {
				delete(saved, k) // not picked yet: the default stays
				continue
			}
		}
		switch f.Type {
		case "toggle":
			if v != "true" && v != "false" {
				return fmt.Errorf("%w: %q must be on or off", ErrSettings, f.Label)
			}
		case "select":
			if !contains(f.Options, v) {
				return fmt.Errorf("%w: %q must be one of its choices", ErrSettings, f.Label)
			}
		}
		if strings.ContainsAny(v, "\x00") {
			return fmt.Errorf("%w: %q has a null byte", ErrSettings, f.Label)
		}
		saved[k] = v
	}
	for _, k := range in.Clear {
		if f, ok := byKey[k]; !ok || f.Type != "secret" {
			return fmt.Errorf("%w: %q is not a secret field", ErrSettings, k)
		}
		delete(saved, k)
	}
	b, _ := json.MarshalIndent(saved, "", "  ")
	tmp := e.settingsPath(id) + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, e.settingsPath(id)); err != nil {
		return err
	}
	e.mu.Lock()
	_, running := e.procs[id]
	e.mu.Unlock()
	if running {
		if err := e.stopProc(id); err != nil {
			return fmt.Errorf("%w: %v", ErrRestart, err)
		}
		if err := e.start(id); err != nil {
			return fmt.Errorf("%w: %v", ErrRestart, err)
		}
	}
	return nil
}

// settingsEnv gives id's settings as HOLYARRS_PLUGIN_SETTING_<KEY>=value,
// defaults filled in and secrets opened. A secret that can't be opened is left out.
func (e *Engine) settingsEnv(id string) []string {
	m := e.readManifest(id)
	if m == nil || len(m.Settings) == 0 {
		return nil
	}
	saved := e.loadSettings(id)
	var env []string
	for _, f := range m.Settings {
		v, ok := saved[f.Key]
		if !ok {
			v = f.defaultValue()
		}
		if f.Type == "secret" && v != "" {
			var err error
			if v, err = e.open(v); err != nil {
				continue
			}
		}
		env = append(env, "HOLYARRS_PLUGIN_SETTING_"+strings.ToUpper(f.Key)+"="+v)
	}
	return env
}

func (e *Engine) aead() (cipher.AEAD, error) {
	if len(e.SettingsKey) == 0 {
		return nil, fmt.Errorf("%w: secrets can't be saved in this build", ErrSettings)
	}
	mac := hmac.New(sha256.New, e.SettingsKey)
	mac.Write([]byte(settingsLabel))
	block, err := aes.NewCipher(mac.Sum(nil))
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

func (e *Engine) seal(plain string) (string, error) {
	a, err := e.aead()
	if err != nil {
		return "", err
	}
	nonce := make([]byte, a.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	return sealedPrefix + base64.StdEncoding.EncodeToString(a.Seal(nonce, nonce, []byte(plain), nil)), nil
}

func (e *Engine) open(stored string) (string, error) {
	a, err := e.aead()
	if err != nil {
		return "", err
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(stored, sealedPrefix))
	if err != nil || len(raw) < a.NonceSize() {
		return "", errors.New("sealed setting unreadable")
	}
	out, err := a.Open(nil, raw[:a.NonceSize()], raw[a.NonceSize():], nil)
	return string(out), err
}
