package updates

import "context"

// Names the external update list tests reach.
const (
	SettingListBucket   = settingListBucket
	SettingListFailures = settingListFailures
	SettingListSerial   = settingListSerial
	SettingUpdateOn     = settingUpdateOn
	ListFailuresForCopy = listFailuresForCopy
	SignatureProblem    = signatureProblem
)

func (e *Checker) Setting(ctx context.Context, key string) string { return e.setting(ctx, key) }

func (e *Checker) SaveSetting(ctx context.Context, key, value string) error {
	return e.saveSetting(ctx, key, value)
}

func (e *Checker) Bucket(ctx context.Context) int { return e.bucket(ctx) }

func (e *Checker) ListMode() bool { return e.listMode() }
