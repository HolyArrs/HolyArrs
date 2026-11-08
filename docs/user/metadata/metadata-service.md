# Optional metadata service

By default HolyArrs uses the HolyArrs metadata server at
`https://holyarrs-api.coderluii.dev` and needs no keys (see
[Metadata](metadata-keys.md); `HOLYARRS_METADATA_URL` points it at another
server). This page covers a separate, optional standalone metadata service
with its own API. Its address is not a MusicBrainz mirror
address and must never be entered in `musicbrainz_url`.

After the application wiring is integrated, select the service at startup with
`HOLYARRS_METADATA_SERVICE_URL` and mount a separate installation credential file
at the path named by `HOLYARRS_METADATA_SERVICE_TOKEN_FILE`. The file takes
precedence over the encrypted vault setting `links/metadata_service_installation_token`.
The file is read again for each request, so rotation does not require a restart.
This feature does not add a browser enrollment or credential-setting route.
Keep the file private and outside exports, reports and source control.

Use a plain HTTPS origin without a path, user information, query or fragment.
Public destinations always require HTTPS. For an operator-controlled private
service or temporary SSH forward, `HOLYARRS_METADATA_SERVICE_ALLOW_LOCAL=true`
also allows local addresses. Plain HTTP is accepted only with that opt-in and a
private or loopback literal IP address. Cloud metadata and link-local addresses
remain blocked. Redirects and environment proxies are disabled.

The installation credential is sent only to the configured service. Direct
provider keys stay with their existing providers. Service answers are checked
for schema, source identity, size and freshness; HTML summaries are treated as
plain text. The app does not cache authenticated service answers locally, so a
revoked installation is checked again on the next lookup. An expired cached
service answer is refused. A refresh requests fresh data but cannot force the
service to refresh its upstream cache.

Movie and show name search use the first provider page, as the current
application interfaces do. Movie search with a year stays on the direct provider
because the service API has no year filter. The adapter validates the service's pagination and never follows
links from a reply. Show details must contain every advertised season and its
episodes within the configured bounds. A missing or mismatched set does not
replace a saved episode list.

An embedded artist album list is partial. An embedded album edition list returns
the existing partial-list signal. When available, the direct MusicBrainz provider
still supplies complete browse results. Missing albums or editions in a partial
answer do not remove saved records. Recording detail is used only when a track
needs a missing name or duration; the app has no separate recording lookup screen.

An unavailable service, a busy service, or an unsupported complete-catalog
operation can use the existing direct provider within the same 18-second lookup
budget. Revoked access, malformed data, identity mismatches and expired answers
are reported. One retry for a 429 or 503 respects a bounded wait and the caller's
deadline. Disabling the optional URL restores direct-provider selection.

Private synthetic integration checks do not establish public hosting eligibility,
provider permission, live-provider completeness or installed-version support.
