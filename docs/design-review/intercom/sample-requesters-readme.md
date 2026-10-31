# Sample requesters

Run `sample-requesters.py` only for the disposable preview database on 18747. The script checks its exact temp path and fake source URL. It adds disabled member identities with empty credentials and no permissions; existing users and passwords are unchanged. No login is possible for these sample people.

- Maya (sample): Game of Thrones, Breaking Bad, The Odyssey.
- Alex (sample): Queen, Hozier, Toy Story 5, and the shared Game of Thrones request.
- Jordan (sample): Fargo, Billie Eilish, Rick and Morty.

These names replace the preview owner's attribution on those sample requests. Other requests stay attributed to the preview owner, so Me remains useful. Pending and denied requests appear in Requests; only requests linked to Library titles participate in Library filtering. Library examples: Maya sees Game of Thrones; Alex sees Queen and Game of Thrones; Jordan sees Fargo and Billie Eilish.

Re-running is idempotent. Request states, approvals, artwork, downloads, slots and imports stay unchanged. Headless checks at 1440/834/390 cover each person's expected Library count, Me, search/list/reset, overflow and axe WCAG A/AA. Evidence: `.omx/artifacts/visual-ralph/intercom/sample-requesters-check/`.
