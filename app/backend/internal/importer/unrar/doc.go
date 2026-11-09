// Package unrar lists and unpacks RAR sets. It reads stored RAR 4 and RAR 5
// files, including sets split over many volumes, and hands compressed sets to
// ExtractCompressed. Encrypted sets are reported and never unpacked. The files
// are untrusted, so sizes are capped, every header is checked against the
// volume length, and unsafe entry paths are refused.
package unrar
