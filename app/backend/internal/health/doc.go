// Package health runs the health checks, the doctor, the Health page data,
// logs and the support bundle. A check that breaks or recovers, and holds
// for a few runs in a row, writes a health Event that Alerts speak for.
package health
