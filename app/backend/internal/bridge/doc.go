// Package bridge connects HolyArrs to a Jackett or Prowlarr that the person
// runs. For Jackett, HolyArrs reads the configured sites and keeps one
// Torznab Source per site. For Prowlarr, HolyArrs adds itself as an app and
// Prowlarr's own app sync writes the Sources.
package bridge
