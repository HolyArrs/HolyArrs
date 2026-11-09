package model

// goldenLayouts pins the field layout of the shared records.
var goldenLayouts = map[string]string{
	"Actor":    "Kind string `json:\"kind\"`\nID int64 `json:\"id,omitempty\"`\n",
	"Meta":     "ID int64 `db:\"id\"`\nCreatedAt time.Time `db:\"created_at\"`\nCreatedBy Actor `db:\"created_by,json\"`\nUpdatedAt time.Time `db:\"updated_at\"`\nUpdatedBy Actor `db:\"updated_by,json\"`\n",
	"Subject":  "Type string `json:\"type\"`\nID int64 `json:\"id\"`\n",
	"Event":    "Meta Meta ``\nAt time.Time `db:\"at\"`\nActor Actor `db:\"actor,json\"`\nKind string `db:\"kind\"`\nTitleID *int64 `db:\"title_id\"`\nSlotID *int64 `db:\"slot_id\"`\nDownloadID *int64 `db:\"download_id\"`\nJobID *int64 `db:\"job_id\"`\nSummary string `db:\"summary\"`\nData map[string]interface {} `db:\"data,json\"`\nGroupKey string `db:\"group_key\"`\n",
	"Job":      "Meta Meta ``\nKind string `db:\"kind\"`\nSubject *Subject `db:\"subject,json\"`\nParams map[string]interface {} `db:\"params,json\"`\nPriority string `db:\"priority\"`\nState string `db:\"state\"`\nProgress float64 `db:\"progress\"`\nProgressNote *string `db:\"progress_note\"`\nAttempt int `db:\"attempt\"`\nMaxAttempts int `db:\"max_attempts\"`\nRunAfter time.Time `db:\"run_after\"`\nStartedAt *time.Time `db:\"started_at\"`\nEndedAt *time.Time `db:\"ended_at\"`\nResult map[string]interface {} `db:\"result,json\"`\nError *JobError `db:\"error,json\"`\nAskedBy Actor `db:\"asked_by,json\"`\nDedupeKey *string `db:\"dedupe_key\"`\n",
	"JobError": "Code string `json:\"code\"`\nMessage string `json:\"message\"`\n",
	"Schedule": "Meta Meta ``\nKind string `db:\"kind\"`\nEverySeconds int64 `db:\"every_seconds\"`\nLastRun *time.Time `db:\"last_run\"`\nNextRun *time.Time `db:\"next_run\"`\nEnabled bool `db:\"enabled\"`\n",
}
