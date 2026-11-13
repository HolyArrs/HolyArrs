package model

// Event kinds used from M1 step 1 on. Older kinds (title_added,
// release_chosen, download_started and so on) keep their names.
const (
	EventTitleAdded       = "title_added"
	EventTitleEdited      = "title_edited"
	EventTitleRefreshed   = "title_refreshed"
	EventTitleDropped     = "title_dropped"         // the metadata source no longer knows it
	EventGateOpened       = "gate_opened"           // a Slot moved from not_out_yet to wanted
	EventGateClosed       = "gate_closed"           // new dates moved the gate later
	EventFolderScanned    = "folder_scanned"        // a Version folder was looked at after a refresh
	EventFolderHealth     = "library_folder_health" // a Library folder's health changed
	EventTitleMovePlanned = "title_move_planned"    // dry run of moving a Version to another Library folder
	EventTitleMoved       = "title_moved"
	EventNeverAddRefused  = "never_add_refused"
	EventVersionChanged   = "version_changed" // a Version was added, changed or removed
	EventListSynced       = "list_synced"     // an Outside list was read and its new movies added
)

// Event kinds for asks (Requests) and Alerts.
const (
	EventAskMade       = "ask_made"        // someone asked for a movie
	EventAskJoined     = "ask_joined"      // someone asked for a movie already asked for
	EventAskWithdrawn  = "ask_withdrawn"   // the person took their ask back
	EventAskApproved   = "ask_approved"    // approved by hand or by auto-approve
	EventAskTurnedDown = "ask_turned_down" // turned down with a note
	EventAskAvailable  = "ask_available"   // the movie is in the Library
	EventAskNotAdded   = "ask_not_added"   // approved, but the title could not be added: back in Waiting
	EventAlertFailed   = "alert_failed"    // an Alert could not be sent
)

// AskEventKinds are the asks' Event kinds. They name who asked and who
// decided, so only people who approve asks read them.
var AskEventKinds = []string{EventAskMade, EventAskJoined, EventAskWithdrawn, EventAskApproved, EventAskTurnedDown, EventAskAvailable, EventAskNotAdded, "ai_ask_made"}

// Event kinds for Reports (problems people report with titles).
const (
	EventReportMade     = "report_made"     // someone reported a problem
	EventReportFixed    = "report_fixed"    // an owner or admin marked it fixed
	EventReportReopened = "report_reopened" // a fixed report was opened again
)

// Event kinds for Organise.
const (
	EventFreeSpaceLow      = "free_space_low"             // an import was refused: the disk would be left too full
	EventRenamed           = "renamed"                    // a File got a new name
	EventRenamePlanned     = "rename_planned"             // a rename was started
	EventLibraryScanned    = "library_scanned"            // a Library folder was scanned for movies to adopt
	EventAdopted           = "adopted"                    // a movie folder found on disk was adopted
	EventFolderUnreachable = "library_folder_unreachable" // a rescan found the Library folder gone (mount down?)
	EventFileMissing       = "file_missing"               // a rescan did not see a File
	EventFileBack          = "file_back"                  // a missing File is there again
	EventTitleRemoved      = "title_removed"              // a movie was deleted from the Library
	EventFileHeld          = "file_held"                  // a File went to the holding folder
	EventHoldingCleaned    = "holding_cleaned"            // old files left the holding folder
)

// Event kinds for History, Alerts and health (5.1.10 to 5.1.14).
const (
	EventNeedsDecision    = "needs_decision"      // a Slot moved to needs_a_decision (same value as mediafiles.EventNeedsDecision)
	EventLibraryRescanned = "library_rescanned"   // the regular rescan of every Library folder ran
	EventHealthBroken     = "health_broken"       // a health check started failing
	EventHealthRecovered  = "health_recovered"    // a failing health check passes again
	EventMediaRefreshed   = "media_server_told"   // a media server was asked to look at a folder again
	EventMediaFailed      = "media_server_failed" // a media server could not be told
)

// EventControl is a control used by the owner or an admin (3.7.7): a part
// turned on, off or restarted, a stage paused, resumed or restarted, a
// linked app reconnected, or the app restarted or shut down. Data holds
// action, target and via (session or api_key).
const EventControl = "control_used"

// Event kinds for shows.
const (
	EventEpisodesChanged = "episodes_changed"       // a refresh added, changed or dropped seasons or episodes
	EventSourceSwitched  = "details_source_changed" // a show now follows another details source
	EventSeasonChanged   = "season_changed"         // Keep up to date was switched for a season or an episode
	EventEpisodesRemoved = "episodes_removed"       // a season or some episodes were deleted from the Library (5.2.3)
)

// Event kinds for music.
const (
	EventAlbumsChanged = "albums_changed" // a refresh added, changed or dropped albums, Pressings or tracks
	EventAlbumChanged  = "album_changed"  // Keep up to date or the Pressing was changed for an album
	EventArtistChanged = "artist_changed" // an artist's album kinds or new-album choice changed
)
