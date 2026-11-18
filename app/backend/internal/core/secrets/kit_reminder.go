package secrets

import "time"

// Key kit reminder levels.
const (
	KitRecommend = "recommend"
	KitWarn      = "warn"
)

// KitWarnAfter is how long saved secrets and backups may exist side by side
// before a missing key kit becomes a warning. A week gives a new owner time
// to finish setting up and get to it calmly, yet is shorter than most
// people go between disk or machine changes, which is when a backup without
// its key is lost for good.
const KitWarnAfter = 7 * 24 * time.Hour

// KitReminder says how loudly to ask for the key kit when none matches the
// current key: not at all while there is nothing to protect, a calm
// recommendation once a secret is saved or a backup is made by hand, and a
// warning once secrets and backups have existed together for KitWarnAfter.
// Zero times mean none. The scheduled backup every install makes on first
// start does not count on its own, or every new install would be asked.
func KitReminder(firstSecret, oldestBackup time.Time, byHand bool, now time.Time) string {
	if firstSecret.IsZero() && !byHand {
		return ""
	}
	if firstSecret.IsZero() || oldestBackup.IsZero() {
		return KitRecommend
	}
	both := firstSecret
	if oldestBackup.After(both) {
		both = oldestBackup
	}
	if now.Sub(both) >= KitWarnAfter {
		return KitWarn
	}
	return KitRecommend
}
