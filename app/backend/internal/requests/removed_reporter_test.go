package requests

import (
	"context"
	"database/sql"
	"testing"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// Fixing a deleted person's report tells nobody and names them plainly in History.
func TestFixReportOfDeletedPerson(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	c := addCopies(t, r.st, "movie", "Arrival", []mediafiles.Numbers{{}}, []string{"HD"}, true)
	v, err := r.svc.Report(ctx, r.ana, ReportInput{TitleID: c.title, Kind: "sound"})
	if err != nil {
		t.Fatal(err)
	}
	if err := r.svc.Store.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE users SET name = name || ?, removed_at = 'x' WHERE id = ?", "⁣"+"7", r.ana.UserID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	// Report's own alert goes out in the background; let it land first so it
	// is not counted as one sent by the fix.
	r.svc.Wait()
	before := len(r.fake.all())
	if _, err := r.svc.FixReport(ctx, r.luis, v.ID, "done"); err != nil {
		t.Fatal(err)
	}
	r.svc.Wait()
	if sent := r.fake.all()[before:]; len(sent) != 0 {
		t.Fatalf("a deleted reporter was alerted: %+v", sent)
	}
	if ms, _, _ := Messages(ctx, r.st.Read(), r.ana.UserID, 10); len(ms) != 0 {
		t.Fatalf("a deleted reporter got a message: %+v", ms)
	}
	if e := hasEvent(r.events(t), domain.EventReportFixed); e == nil || e.Data["made_by"] != "ana (deleted)" {
		t.Fatalf("fixed event %+v", e)
	}
	key := webserver.ContextWith(ctx, webserver.Caller{UserID: r.luis.UserID, KeyID: 3})
	rv, err := r.svc.reportView(key, v.ID)
	if err != nil {
		t.Fatal(err)
	}
	if rv.MadeBy == nil || rv.MadeBy.Name != "ana (deleted)" {
		t.Fatalf("a key holder read the reporter as %+v", rv.MadeBy)
	}
}
