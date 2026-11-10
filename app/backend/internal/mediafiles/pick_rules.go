package mediafiles

// Ladder steps, low to high (pick-rules.md section 5).
var Ladder = []string{
	"cam", "telesync", "sd_tv", "sd_dvd",
	"720p_tv", "720p_web", "720p_bluray",
	"1080p_tv", "1080p_web", "1080p_bluray", "1080p_remux",
	"2160p_web", "2160p_bluray", "2160p_remux",
}

// Step is one rung of the quality ladder.
type Step struct {
	Quality string `json:"quality"`
	Allowed bool   `json:"allowed"`
}

// TraitPoints gives points when a Trait matches.
type TraitPoints struct {
	Trait  string `json:"trait"`
	Points int    `json:"points"`
}

// SizeLimit bounds a Release's size in bytes. With PerMinute the limits are
// per minute of runtime.
type SizeLimit struct {
	Min       int64 `json:"min"`
	Max       int64 `json:"max"`
	PerMinute bool  `json:"per_minute"`
}

// ProtocolRule is the protocol preference and wait.
type ProtocolRule struct {
	Prefer               string `json:"prefer"` // torrent, usenet, either
	WaitMinutesTorrent   int    `json:"wait_minutes_torrent"`
	WaitMinutesUsenet    int    `json:"wait_minutes_usenet"`
	SkipWaitAtGoodEnough bool   `json:"skip_wait_at_good_enough"`
}

// LanguageRule says which languages are wanted.
type LanguageRule struct {
	Wanted     []string `json:"wanted"`
	OriginalOK bool     `json:"original_ok"`
}

// ProperPolicy says how fixed re-releases count.
type ProperPolicy struct {
	Mode       string `json:"mode"` // take_any, same_group_only, prefer_only, ignore
	MaxAgeDays int    `json:"max_age_days,omitempty"`
}

// PickRulesBody is the rule body stored in the pick_rules.body JSON column.
type PickRulesBody struct {
	Steps            []Step               `json:"steps"`
	GoodEnoughAt     string               `json:"good_enough_at"`
	Traits           []TraitPoints        `json:"traits"`
	MustHave         []string             `json:"must_have"`
	Never            []string             `json:"never"`
	MinPoints        int                  `json:"min_points"`
	GoodEnoughPoints *int                 `json:"good_enough_points,omitempty"`
	SizeLimits       map[string]SizeLimit `json:"size_limits"` // by step; "*" for every step
	Protocol         ProtocolRule         `json:"protocol"`
	Language         LanguageRule         `json:"language"`
	Order            string               `json:"order"` // quality_then_points, points_then_quality
	UpgradeMinPoints int                  `json:"upgrade_min_points"`
	ProperPolicy     ProperPolicy         `json:"proper_policy"`
	// Synced names the TRaSH Guides profile last applied, for re-syncing.
	Synced *SyncedProfile `json:"synced,omitempty"`
}

// SyncedProfile is a TRaSH Guides profile applied to Pick rules.
type SyncedProfile struct {
	From      string `json:"from"` // trash
	App       string `json:"app"`  // sonarr or radarr
	ProfileID string `json:"profile_id"`
	Name      string `json:"name"`
	At        string `json:"at"`
}

// GoodEverydayName is the default preset's name.
const GoodEverydayName = "Standard"

const gb = int64(1) << 30

// GoodEveryday returns the default preset (pick-rules.md section 5).
func GoodEveryday() PickRulesBody {
	allowed := map[string]bool{"720p_tv": true, "720p_web": true, "720p_bluray": true,
		"1080p_tv": true, "1080p_web": true, "1080p_bluray": true}
	steps := make([]Step, len(Ladder))
	for i, q := range Ladder {
		steps[i] = Step{Quality: q, Allowed: allowed[q]}
	}
	return PickRulesBody{
		Steps:        steps,
		GoodEnoughAt: "1080p_web",
		Traits: []TraitPoints{
			{Trait: "hevc", Points: 10},
			{Trait: "ddp_audio", Points: 5},
			{Trait: "web_dl", Points: 5},
			{Trait: "upscaled", Points: -50},
			{Trait: "dubbed_only", Points: -30},
		},
		MustHave: []string{},
		Never: []string{"sample", "executable", "password_protected", "disc_image",
			"hardcoded_subs", "3d", "cam", "telesync"},
		MinPoints: 0,
		// 0.5 to 6 GB per hour of video.
		SizeLimits:       map[string]SizeLimit{"*": {Min: gb / 2 / 60, Max: 6 * gb / 60, PerMinute: true}},
		Protocol:         ProtocolRule{Prefer: "either"},
		Language:         LanguageRule{Wanted: []string{}, OriginalOK: true},
		Order:            "quality_then_points",
		UpgradeMinPoints: 10,
		ProperPolicy:     ProperPolicy{Mode: "take_any"},
	}
}

// SaveSpaceName is the small-files preset's name.
const SaveSpaceName = "Lite"

// SaveSpace returns the small-files preset: 720p to 1080p web, good enough at
// 1080p web, x265 earns points, 0.4 to 2.5 GB per hour of video.
func SaveSpace() PickRulesBody {
	b := GoodEveryday()
	allowed := map[string]bool{"720p_tv": true, "720p_web": true, "720p_bluray": true, "1080p_tv": true, "1080p_web": true}
	for i := range b.Steps {
		b.Steps[i].Allowed = allowed[b.Steps[i].Quality]
	}
	b.Traits = []TraitPoints{
		{Trait: "hevc", Points: 20},
		{Trait: "web_dl", Points: 5},
		{Trait: "upscaled", Points: -50},
		{Trait: "dubbed_only", Points: -30},
	}
	b.SizeLimits = map[string]SizeLimit{"*": {Min: 2 * gb / 5 / 60, Max: 5 * gb / 2 / 60, PerMinute: true}}
	return b
}

// BestName is the best-copy preset's name.
const BestName = "Best"

// Best returns the best-copy preset: 4K first with 1080p Blu-ray as the
// fallback, good enough at 2160p Blu-ray, HDR and Atmos earn points.
func Best() PickRulesBody {
	b := FourK()
	allowed := map[string]bool{"1080p_bluray": true, "2160p_web": true, "2160p_bluray": true, "2160p_remux": true}
	for i := range b.Steps {
		b.Steps[i].Allowed = allowed[b.Steps[i].Quality]
	}
	b.GoodEnoughAt = "2160p_bluray"
	return b
}

// FourKName is the 4K preset's name, meant for a second Version of a movie.
const FourKName = "4K"

// FourK returns the 4K preset: 2160p steps only, good enough at 2160p web,
// HDR and Atmos earn points (pick-rules.md: 4K via Versions).
func FourK() PickRulesBody {
	allowed := map[string]bool{"2160p_web": true, "2160p_bluray": true, "2160p_remux": true}
	steps := make([]Step, len(Ladder))
	for i, q := range Ladder {
		steps[i] = Step{Quality: q, Allowed: allowed[q]}
	}
	return PickRulesBody{
		Steps:        steps,
		GoodEnoughAt: "2160p_web",
		Traits: []TraitPoints{
			{Trait: "dv", Points: 20},
			{Trait: "hdr10plus", Points: 15},
			{Trait: "hdr10", Points: 10},
			{Trait: "atmos", Points: 10},
			{Trait: "hevc", Points: 5},
			{Trait: "upscaled", Points: -100},
		},
		MustHave: []string{},
		Never: []string{"sample", "executable", "password_protected", "disc_image",
			"hardcoded_subs", "3d", "cam", "telesync"},
		MinPoints: 0,
		// 3 to 60 GB per hour of video.
		SizeLimits:       map[string]SizeLimit{"*": {Min: 3 * gb / 60, Max: 60 * gb / 60, PerMinute: true}},
		Protocol:         ProtocolRule{Prefer: "either"},
		Language:         LanguageRule{Wanted: []string{}, OriginalOK: true},
		Order:            "quality_then_points",
		UpgradeMinPoints: 10,
		ProperPolicy:     ProperPolicy{Mode: "take_any"},
	}
}

// MusicLadder is the music quality ladder, low to high (pick-rules.md
// section 8): lossy under 192 kbps, lossy 192 to 256 kbps, lossy at its best
// (MP3 320 or V0, AAC 256 and up, Opus 160 and up), lossless (16-bit or
// depth not given) and lossless hi-res (24-bit or a rate over 48 kHz).
var MusicLadder = []string{"lossy_low", "lossy_mid", "lossy_high", "lossless", "lossless_hires"}

// Music preset names (stage 3.3.1).
const (
	LosslessFirstName = "Lossless first"
	Lossy320Name      = "Lossy 320 or V0"
	AnyMusicName      = "Any music"
	// MusicDefaultName is the Pick rules a new artist gets when nothing else is set.
	MusicDefaultName = LosslessFirstName
)

const mb = int64(1) << 20

// IsMusicRules says Pick rules judge music (their ladder is the music one).
func IsMusicRules(b PickRulesBody) bool {
	for _, s := range b.Steps {
		if s.Quality == "lossless" {
			return true
		}
	}
	return false
}

// musicSizes are the believable album sizes per minute of music, by step.
func musicSizes() map[string]SizeLimit {
	return map[string]SizeLimit{
		"lossy_low":      {Min: mb * 3 / 10, Max: mb * 3 / 2, PerMinute: true}, // 0.3 to 1.5 MB a minute
		"lossy_mid":      {Min: mb, Max: mb * 5 / 2, PerMinute: true},          // 1 to 2.5 MB
		"lossy_high":     {Min: mb * 6 / 5, Max: mb * 7 / 2, PerMinute: true},  // 1.2 to 3.5 MB
		"lossless":       {Min: 3 * mb, Max: 15 * mb, PerMinute: true},         // 3 to 15 MB
		"lossless_hires": {Min: 6 * mb, Max: 80 * mb, PerMinute: true},         // 6 to 80 MB
	}
}

func musicPreset(allowed map[string]bool, goodEnough string, traits []TraitPoints) PickRulesBody {
	steps := make([]Step, len(MusicLadder))
	for i, q := range MusicLadder {
		steps[i] = Step{Quality: q, Allowed: allowed[q]}
	}
	return PickRulesBody{
		Steps:            steps,
		GoodEnoughAt:     goodEnough,
		Traits:           traits,
		MustHave:         []string{},
		Never:            []string{"sample", "executable", "password_protected", "single_image", "lossy_transcode"},
		MinPoints:        -10, // a vinyl rip (-10) still passes; it only loses to a clean copy
		SizeLimits:       musicSizes(),
		Protocol:         ProtocolRule{Prefer: "either"},
		Language:         LanguageRule{Wanted: []string{}, OriginalOK: true},
		Order:            "quality_then_points",
		UpgradeMinPoints: 10,
		ProperPolicy:     ProperPolicy{Mode: "take_any"},
	}
}

// LosslessFirst wants lossless and takes lossy at its best until a lossless
// copy turns up; Good enough at lossless (CD quality).
func LosslessFirst() PickRulesBody {
	return musicPreset(map[string]bool{"lossy_high": true, "lossless": true, "lossless_hires": true}, "lossless",
		[]TraitPoints{{Trait: "cd_source", Points: 10}, {Trait: "log_cue", Points: 5}, {Trait: "web_source", Points: 5}, {Trait: "vinyl_source", Points: -10}})
}

// Lossy320 wants small files: lossy only, Good enough at MP3 320 or V0 (or
// AAC 256, Opus 160).
func Lossy320() PickRulesBody {
	return musicPreset(map[string]bool{"lossy_mid": true, "lossy_high": true}, "lossy_high",
		[]TraitPoints{{Trait: "mp3", Points: 5}, {Trait: "web_source", Points: 5}, {Trait: "cd_source", Points: 5}})
}

// AnyMusic takes any audio, best first; Good enough at lossless.
func AnyMusic() PickRulesBody {
	return musicPreset(map[string]bool{"lossy_low": true, "lossy_mid": true, "lossy_high": true, "lossless": true, "lossless_hires": true}, "lossless",
		[]TraitPoints{{Trait: "cd_source", Points: 10}, {Trait: "log_cue", Points: 5}, {Trait: "web_source", Points: 5}, {Trait: "vinyl_source", Points: -10}})
}
