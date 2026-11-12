package subtitles

import (
	"errors"
	"net/http"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/subtitles/container"
)

func translateError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrNoTranslator), errors.Is(err, ErrHasSubtitle), errors.Is(err, ErrPicture), errors.Is(err, ErrNoSource),
		errors.Is(err, ErrKeyElsewhere), errors.Is(err, ErrNoVault):
		webserver.Fail(w, http.StatusConflict, err.Error())
	default:
		subtitleError(w, err)
	}
}

func (a *Handlers) getTranslator(w http.ResponseWriter, r *http.Request) {
	webserver.WriteJSON(w, http.StatusOK, a.svc.Translator(r.Context()))
}

func (a *Handlers) putTranslator(w http.ResponseWriter, r *http.Request) {
	cur := a.svc.Translator(r.Context()) // fields the request leaves out keep their value
	in := TranslatorInput{Kind: cur.Kind, URL: cur.URL, Model: cur.Model, BuiltInAuto: cur.BuiltInAuto}
	if !webserver.Decode(w, r, &in, `send any of kind (openai, lingarr or "" for off), url, model, key, clear_key and built_in_auto`) {
		return
	}
	t, err := a.svc.SaveTranslator(r.Context(), in, actorOf(r))
	if err != nil {
		translateError(w, err)
		return
	}
	webserver.WriteJSON(w, http.StatusOK, t)
}

func (a *Handlers) testTranslator(w http.ResponseWriter, r *http.Request) {
	webserver.WriteJSON(w, http.StatusOK, a.svc.TestTranslator(r.Context()))
}

// translateSubtitle queues a translation a person asked for. It checks what
// it can at once, so a missing translator or a picture track is said here;
// the Job reports how the translation went.
func (a *Handlers) translateSubtitle(w http.ResponseWriter, r *http.Request) {
	id, ok := subID(r)
	if !ok {
		webserver.Fail(w, http.StatusNotFound, "no such File")
		return
	}
	var in TranslateRequest
	if !webserver.Decode(w, r, &in, `send {"language": "pt-br"}, optionally with "from": "en", "forced", and "source": "beside" with "path" or "built_in" with "track"`) {
		return
	}
	ctx := r.Context()
	fc, err := a.svc.load(ctx, id)
	if err != nil {
		subtitleError(w, err)
		return
	}
	if in.Language = NormLanguage(in.Language); in.Language == "" {
		webserver.Fail(w, http.StatusBadRequest, "unknown language")
		return
	}
	if in.From != "" {
		if in.From = NormLanguage(in.From); in.From == "" || in.From == in.Language {
			webserver.Fail(w, http.StatusBadRequest, "from must be another known language")
			return
		}
	}
	switch in.Source {
	case "", SourceBeside, SourceBuiltIn:
	default:
		webserver.Fail(w, http.StatusBadRequest, "source must be beside, built_in or left out")
		return
	}
	if !a.svc.Translator(ctx).Ready() {
		translateError(w, ErrNoTranslator)
		return
	}
	if in.Source == SourceBuiltIn {
		rows, _ := ListFor(ctx, a.store.Read(), fc.file.ID)
		for _, row := range rows {
			if row.Place == "built_in" && row.State == "present" && row.TrackIndex != nil && *row.TrackIndex == in.Track &&
				(row.Picture || row.Codec == nil || !container.TextCodec(*row.Codec)) {
				translateError(w, ErrPicture)
				return
			}
		}
	}
	in.ByHand = true
	in.SiteID, in.SiteKey, in.SiteLink, in.SiteFormat, in.SiteLanguage, in.SiteHI, in.SiteScore = 0, "", "", "", "", false, 0
	if err := a.svc.QueueTranslate(ctx, fc.file.ID, in); err != nil {
		webserver.Internal(w, "api", err)
		return
	}
	out := map[string]any{"queued": true}
	if jid, ok := a.jobByKey(ctx, translateKey(fc.file.ID, in)); ok {
		out["job_id"] = jid
	}
	webserver.WriteJSON(w, http.StatusAccepted, out)
}
