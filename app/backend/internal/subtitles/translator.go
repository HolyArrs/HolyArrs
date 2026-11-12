package subtitles

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/subtitles/translate"
)

// Kinds of translator.
const (
	TranslatorOpenAI  = "openai"  // any server with the OpenAI chat API: cloud services, Ollama, LM Studio
	TranslatorLingarr = "lingarr" // a Lingarr server
)

// The settings rows of the translator. The key lives in the vault; its row
// holds only a reference.
const (
	trGroup    = "what_to_pick"
	trKind     = "subtitle_translator_kind"
	trURL      = "subtitle_translator_url"
	trModel    = "subtitle_translator_model"
	trKey      = "subtitle_translator_key"
	trBuiltIn  = "subtitle_translate_built_in"
	trMaxModel = 200
	trMaxKey   = 1000
)

// Translator is the owner's own translator. The key is never sent back:
// HasKey only says one is kept.
type Translator struct {
	Kind  string `json:"kind"` // "" (off), openai or lingarr
	URL   string `json:"url"`
	Model string `json:"model"`
	// HasKey reports that a key is kept in the vault.
	HasKey bool `json:"has_key"`
	// BuiltInAuto lets the timed search translate text tracks inside the video
	// without asking. Off, they are translated only by hand.
	BuiltInAuto bool `json:"built_in_auto"`
}

// Ready reports whether the translator can be called.
func (t Translator) Ready() bool {
	switch t.Kind {
	case TranslatorOpenAI:
		return t.URL != "" && t.Model != ""
	case TranslatorLingarr:
		return t.URL != ""
	}
	return false
}

// Name is how saved subtitles and Events name the translator: the model, or
// Lingarr.
func (t Translator) Name() string {
	if t.Kind == TranslatorLingarr {
		return "Lingarr"
	}
	return t.Model
}

// TranslatorInput changes the translator. Fields left out keep their value;
// an empty key keeps the stored one and ClearKey removes it.
type TranslatorInput struct {
	Kind        string `json:"kind"`
	URL         string `json:"url"`
	Model       string `json:"model"`
	Key         string `json:"key"`
	ClearKey    bool   `json:"clear_key"`
	BuiltInAuto bool   `json:"built_in_auto"`
}

// Errors from the translator settings.
var (
	ErrNoTranslator = errors.New("no translator is set up: add one in Settings > Subtitles")
	ErrKeyElsewhere = errors.New("the translator address moved to another host: give the key again or clear it")
)

// readTranslator reads the translator settings, without looking in the
// vault. Odd values read as off.
func readTranslator(ctx context.Context, db *sql.DB) Translator {
	get := func(k string) string { return database.SettingOr(ctx, db, trGroup, k, "") }
	t := Translator{Kind: get(trKind), URL: get(trURL), Model: get(trModel)}
	if t.Kind != TranslatorOpenAI && t.Kind != TranslatorLingarr {
		t.Kind = ""
	}
	if translate.CheckAddress(t.URL) != nil {
		t.URL = ""
	}
	t.BuiltInAuto, _ = strconv.ParseBool(get(trBuiltIn))
	return t
}

// Translator reads the translator settings and whether a key is kept.
func (s *Service) Translator(ctx context.Context) Translator {
	t := readTranslator(ctx, s.Store.Read())
	key, err := s.translatorKey(ctx)
	t.HasKey = err == nil && key != ""
	return t
}

// check cleans an input and refuses odd values.
func (in *TranslatorInput) check() error {
	in.Kind, in.URL, in.Model, in.Key = strings.TrimSpace(in.Kind), strings.TrimSpace(in.URL), strings.TrimSpace(in.Model), strings.TrimSpace(in.Key)
	switch in.Kind {
	case "", TranslatorOpenAI, TranslatorLingarr:
	default:
		return fmt.Errorf("kind must be %s, %s or empty for off", TranslatorOpenAI, TranslatorLingarr)
	}
	if in.Kind != "" && in.URL == "" {
		return errors.New("the translator needs its address")
	}
	if in.URL != "" {
		if err := translate.CheckAddress(in.URL); err != nil {
			return err
		}
	}
	if in.Kind == TranslatorOpenAI && in.Model == "" {
		return errors.New("the translator needs a model name")
	}
	if len(in.Model) > trMaxModel || strings.ContainsAny(in.Model, "\r\n") {
		return errors.New("the model name is too long")
	}
	if len(in.Key) > trMaxKey || strings.ContainsAny(in.Key, " \r\n") {
		return errors.New("the key is too long or holds spaces")
	}
	return nil
}

// SaveTranslator stores the translator and keeps its key in the vault. A key
// never follows the translator to another host: moving the address with a
// key kept needs the key again or ClearKey.
func (s *Service) SaveTranslator(ctx context.Context, in TranslatorInput, by domain.Actor) (Translator, error) {
	if err := in.check(); err != nil {
		return Translator{}, err
	}
	cur := s.Translator(ctx)
	if in.URL == "" && in.Key == "" && cur.HasKey {
		in.ClearKey = true // with no address left, a kept key has nowhere to go
	}
	if cur.HasKey && in.Key == "" && !in.ClearKey && originOf(in.URL) != originOf(cur.URL) {
		return cur, ErrKeyElsewhere
	}
	if (in.Key != "" || in.ClearKey) && s.Vault == nil {
		return cur, ErrNoVault
	}
	err := s.Store.Write(ctx, func(tx *sql.Tx) error {
		ts := time.Now()
		for k, v := range map[string]string{trKind: in.Kind, trURL: in.URL, trModel: in.Model, trBuiltIn: strconv.FormatBool(in.BuiltInAuto)} {
			if err := database.PutSetting(tx, trGroup, k, v, by, ts); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return cur, err
	}
	switch {
	case in.Key != "":
		err = s.Vault.SetSetting(ctx, trGroup, trKey, in.Key, by)
	case in.ClearKey:
		err = s.Vault.SetSetting(ctx, trGroup, trKey, "", by)
	}
	return s.Translator(ctx), err
}

// translatorKey reads the key from the vault; "" when none is kept.
func (s *Service) translatorKey(ctx context.Context) (string, error) {
	if s.Vault == nil || database.SettingOr(ctx, s.Store.Read(), trGroup, trKey, "") == "" {
		return "", nil
	}
	return s.Vault.Setting(ctx, trGroup, trKey)
}

// translatorClient builds the client for the stored translator.
func (s *Service) translatorClient(ctx context.Context) (translate.Client, Translator, error) {
	t := readTranslator(ctx, s.Store.Read())
	if !t.Ready() {
		return nil, t, ErrNoTranslator
	}
	key, err := s.translatorKey(ctx)
	if err != nil {
		return nil, t, fmt.Errorf("the translator key cannot be read: %w", err)
	}
	if t.Kind == TranslatorLingarr {
		return &translate.Lingarr{URL: t.URL, Key: key, HTTP: s.TranslateHTTP}, t, nil
	}
	return &translate.OpenAI{URL: t.URL, Key: key, Model: t.Model, HTTP: s.TranslateHTTP}, t, nil
}

// TranslatorTest is how a translator test went, in plain words.
type TranslatorTest struct {
	OK      bool     `json:"ok"`
	Message string   `json:"message"`
	Models  []string `json:"models"`
}

// TestTranslator lists the server's models where it can, and translates two
// short lines to show the address, key and model work together.
func (s *Service) TestTranslator(ctx context.Context) TranslatorTest {
	res := TranslatorTest{Models: []string{}}
	c, t, err := s.translatorClient(ctx)
	if err != nil {
		res.Message = err.Error()
		return res
	}
	if o, ok := c.(*translate.OpenAI); ok {
		if ms, err := o.Models(ctx); err == nil {
			res.Models = ms
		}
	}
	b := translate.Batch{From: "en", To: "es", FromName: "English", ToName: "Spanish", Kind: "movie",
		Lines: []translate.Line{{N: 1, Text: "Good morning."}, {N: 2, Text: "See you tomorrow."}}}
	got, err := c.Translate(ctx, b)
	if err != nil {
		res.Message = translatorWords(err)
		return res
	}
	n := 0
	for _, l := range got {
		if (l.N == 1 || l.N == 2) && strings.TrimSpace(l.Text) != "" {
			n++
		}
	}
	if n < 2 {
		res.Message = "The translator answered, but not with the two numbered test lines. Try another model."
		return res
	}
	res.OK, res.Message = true, fmt.Sprintf("The translator answered: %s translated both test lines.", t.Name())
	return res
}

// translatorWords explains a translator failure in plain words.
func translatorWords(err error) string {
	var te *translate.Error
	if !errors.As(err, &te) {
		return "The translator could not be reached (" + err.Error() + ")."
	}
	switch te.Kind {
	case translate.KindRefused:
		return "The translator refused the call (" + te.Msg + "). Check the address, key and model."
	case translate.KindBusy:
		return "The translator is busy or failing (" + te.Msg + "). Try again later."
	case translate.KindCut:
		return "The translator did not answer in time."
	case translate.KindUnreadable:
		return "The translator answered, but HolyArrs could not read the answer (" + te.Msg + ")."
	}
	return "The translator could not be reached (" + te.Msg + ")."
}
