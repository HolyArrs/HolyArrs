// Package arrapi lets apps built for Sonarr and Radarr talk to HolyArrs
// without changes. Request apps, subtitle apps, unpackers and start pages
// call /sonarr/ and /radarr/ as they would call the real apps, and
// Prowlarr writes its indexers in as Sources. The package keeps only a tag
// table of its own. Every answer is read from the Library and every change
// goes through the same services the app uses, so both paths behave the
// same. Sign-in, key, permission and part checks run before a request gets
// here, and the field names those apps expect stay inside this package.
package arrapi
