package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/requests"
)

func reportOf(t *testing.T, b []byte) requests.ReportView {
	t.Helper()
	var x struct {
		Report requests.ReportView `json:"report"`
	}
	if err := json.Unmarshal(b, &x); err != nil {
		t.Fatal(err)
	}
	return x.Report
}

type reportList struct {
	Reports []requests.ReportView `json:"reports"`
	Open    *int                  `json:"open"`
}

func TestReportsOverHTTP(t *testing.T) {
	t.Parallel()
	r := rqNewAskRig(t)
	ctx := context.Background()
	f, v := rqMovieCopy(t, r.st)
	title := rqItoa(v.TitleID)

	// A member reports; the same open problem twice is refused.
	mine := reportOf(t, r.ana.call("POST", "/api/v1/reports", `{"title_id":`+title+`,"kind":"sound","note":"hiss"}`, 201))
	if mine.State != "open" || !mine.Mine || mine.FileID == nil || *mine.FileID != f.ID || mine.Note != "hiss" {
		t.Fatalf("report %+v", mine)
	}
	r.ana.call("POST", "/api/v1/reports", `{"title_id":`+title+`,"kind":"sound"}`, 409)
	r.ana.call("POST", "/api/v1/reports", `{"title_id":`+title+`,"kind":"smell"}`, 400)
	theirs := reportOf(t, r.bo.call("POST", "/api/v1/reports", `{"title_id":`+title+`,"kind":"picture"}`, 201))

	// Members see only their own; managers see all and the open count.
	var l reportList
	_ = json.Unmarshal(r.ana.call("GET", "/api/v1/reports", "", 200), &l)
	if len(l.Reports) != 1 || l.Reports[0].ID != mine.ID || l.Open != nil {
		t.Fatalf("ana's list %+v", l)
	}
	l = reportList{}
	_ = json.Unmarshal(r.owner.call("GET", "/api/v1/reports?state=open", "", 200), &l)
	if len(l.Reports) != 2 || l.Open == nil || *l.Open != 2 || l.Reports[0].ID != theirs.ID {
		t.Fatalf("owner's list %+v", l)
	}
	r.owner.call("GET", "/api/v1/reports?state=gone", "", 400)
	var msgs map[string]any
	_ = json.Unmarshal(r.owner.call("GET", "/api/v1/messages", "", 200), &msgs)
	if msgs["reports_open"] != float64(2) {
		t.Fatalf("owner messages %v", msgs)
	}
	msgs = nil
	_ = json.Unmarshal(r.ana.call("GET", "/api/v1/messages", "", 200), &msgs)
	if _, ok := msgs["reports_open"]; ok {
		t.Fatalf("ana sees the open count: %v", msgs)
	}

	// Only managers fix and reopen; the reply reaches ana as a message.
	r.ana.call("POST", "/api/v1/reports/"+rqItoa(mine.ID)+"/fix", `{}`, 403)
	r.ana.call("POST", "/api/v1/reports/"+rqItoa(mine.ID)+"/reopen", ``, 403)
	fixed := reportOf(t, r.owner.call("POST", "/api/v1/reports/"+rqItoa(mine.ID)+"/fix", `{"reply":"new copy is in"}`, 200))
	if fixed.State != "fixed" || fixed.Reply != "new copy is in" || fixed.ClosedBy == nil || fixed.ClosedBy.Name != "luis" {
		t.Fatalf("fixed %+v", fixed)
	}
	r.owner.call("POST", "/api/v1/reports/"+rqItoa(mine.ID)+"/fix", ``, 409)
	var inbox struct {
		Messages []requests.Message `json:"messages"`
		Unread   int                `json:"unread"`
	}
	_ = json.Unmarshal(r.ana.call("GET", "/api/v1/messages", "", 200), &inbox)
	if inbox.Unread != 1 || inbox.Messages[0].Kind != "fixed" || inbox.Messages[0].ReportID == nil || *inbox.Messages[0].ReportID != mine.ID {
		t.Fatalf("ana's inbox %+v", inbox)
	}
	if again := reportOf(t, r.owner.call("POST", "/api/v1/reports/"+rqItoa(mine.ID)+"/reopen", ``, 200)); again.State != "open" {
		t.Fatalf("reopened %+v", again)
	}
	r.owner.call("POST", "/api/v1/reports/"+rqItoa(mine.ID)+"/reopen", ``, 409)
	r.owner.call("POST", "/api/v1/reports/9999/fix", ``, 404)

	// A title with nothing in the Library yet, and one that does not exist.
	var wanted int64
	err := r.st.Write(ctx, func(tx *sql.Tx) error {
		ti := &mediafiles.Title{Kind: "movie", GateDriver: "none", Name: "Wanted Only", SortName: "wanted only", MetadataSource: "local", NewPartsPolicy: "all", ReleaseGate: "released",
			WhyAdded: "by_hand", Status: "released", State: "active", SourceIDs: map[string]string{}, Aliases: []string{}, MusicTypes: []string{}, Artwork: []string{}}
		if _, err := database.Insert(tx, "titles", ti, domain.System); err != nil {
			return err
		}
		wanted = ti.ID
		_, err := database.Insert(tx, "items", &mediafiles.Item{TitleID: ti.ID, Kind: "movie", Numbers: mediafiles.Numbers{}, KeepUpToDate: true, SourceIDs: map[string]string{}}, domain.System)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	r.ana.call("POST", "/api/v1/reports", `{"title_id":`+rqItoa(wanted)+`,"kind":"other"}`, 422)
	r.ana.call("POST", "/api/v1/reports", `{"title_id":9999,"kind":"other"}`, 404)

	// A Kid sees an unrated title as not there at all, as on GET /titles/{id}.
	if err := r.st.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE users SET max_rating = 'PG' WHERE id = ?", r.bo.id)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	r.bo.call("GET", "/api/v1/titles/"+title, "", 404)
	r.bo.call("POST", "/api/v1/reports", `{"title_id":`+title+`,"kind":"other"}`, 404)
	// Bo's own earlier report of it is hidden too once the limit is lower.
	l = reportList{}
	_ = json.Unmarshal(r.bo.call("GET", "/api/v1/reports", "", 200), &l)
	if len(l.Reports) != 0 {
		t.Fatalf("bo's list under a Kid limit %+v", l)
	}
}
