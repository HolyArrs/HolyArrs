// Package importer places finished downloads into the Library. It finds the
// media files in a download, matches them to a movie, show or album, plans a
// copy step for each and runs the steps without overwriting anything. Packed
// releases are unpacked first, replaced files are held for a while before they
// are removed, and free space is checked up front so a full disk fails early
// with a plain reason. It also serves import by hand, limited to the folders
// the user allowed, because those paths come from the browser.
package importer
