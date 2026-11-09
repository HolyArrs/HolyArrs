package main

// The time zone database is built in (about 450 KB), so TZ and each
// member's browser zone (Explore's time of day) work on an image with no
// /usr/share/zoneinfo; a system database, when there is one, is still used
// first.
import _ "time/tzdata"
