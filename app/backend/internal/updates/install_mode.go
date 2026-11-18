package updates

import (
	"context"
	"errors"
	"log"
	"os"
	"slices"
	"strings"
)

// Update modes. Off is the default everywhere: only a notice is shown.
const (
	ModeOff  = "off"
	ModeAsk  = "ask"
	ModeAuto = "auto"
)

// Modes are the allowed update modes.
var Modes = []string{ModeOff, ModeAsk, ModeAuto}

// settingUpdateMode is the in-app update mode in the this_app group.
const settingUpdateMode = "update_mode"

// ErrModeLocked says HOLYARRS_UPDATES sets the mode, so Settings cannot.
var ErrModeLocked = errors.New("HOLYARRS_UPDATES sets the update mode, so it cannot be changed here")

// Mode is the update mode and whether HOLYARRS_UPDATES fixes it.
func (e *Checker) Mode(ctx context.Context) (mode string, locked bool) {
	if NoUpdater {
		return ModeOff, true
	}
	if v := strings.ToLower(strings.TrimSpace(os.Getenv(EnvMode))); slices.Contains(Modes, v) {
		return v, true
	}
	if v := e.setting(ctx, settingUpdateMode); slices.Contains(Modes, v) {
		return v, false
	}
	return ModeOff, false
}

// SetMode keeps the update mode chosen in Settings.
func (e *Checker) SetMode(ctx context.Context, mode string) error {
	if !slices.Contains(Modes, mode) {
		return errors.New("the update mode is off, ask or auto")
	}
	if _, locked := e.Mode(ctx); locked {
		return ErrModeLocked
	}
	return e.saveSetting(ctx, settingUpdateMode, mode)
}

// CanInstall says in-app installs can work in this build at all: keys are
// pinned, the build allows it and the system can start another program.
func (e *Checker) CanInstall() bool {
	return e.Installer != nil && !NoUpdater && e.listMode() && execSupported
}

// offered gets a release ready when the mode allows it: Ask me downloads
// and checks it so "Update now" is quick; Automatic also installs it, but
// only when the release is marked safe for that and is not one way.
func (e *Checker) offered(ctx context.Context, l *List, data, sig []byte, c Channel) {
	if !e.CanInstall() {
		return
	}
	mode, _ := e.Mode(ctx)
	if mode == ModeOff {
		return
	}
	in := e.Installer
	go func() {
		ctx := context.WithoutCancel(ctx)
		if err := in.Prepare(ctx, l, data, sig, c); err != nil {
			log.Printf("updates: %s is not installed: %v", c.Version, err)
			return
		}
		if mode == ModeAuto && c.AutoUpdateSafe && !c.OneWay {
			if err := in.Install(ctx); err != nil {
				log.Printf("updates: automatic update to %s waits: %v", c.Version, err)
			}
		}
	}()
}
