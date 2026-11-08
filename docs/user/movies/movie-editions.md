# Movie editions

A movie keeps one Title identity. Each Version can choose an edition independently from its Pick rules: for example, Theatrical HD, Theatrical 4K and Director cut HD. An empty edition is unconstrained. Existing Versions and files are not relabelled theatrical when the database is upgraded.

Add an edition on the movie's Version form. Searches use the selected edition; imports check it again before placement. An unknown, conflicting or ambiguous edition waits for a decision. On Needs a decision, a person with Library permission can confirm an edition they checked and retry the current finished movie download. This confirmation is recorded. It cannot bypass audio requirements, protection, ownership, path checks or a removed Title.

Copy details distinguish Version requirements from the edition recorded for an actual file. A checked correction updates that file's recorded edition. It does not rename its current path. Rename still uses a reviewed plan and the ordinary collision guards.

To adopt another existing copy, add an empty Version. Choose its existing folder name if it differs from the generated folder, then use Adopt existing copy with a relative file path inside that Version's folder. The file is inspected and recorded in place; adoption does not move its bytes. A constrained edition needs a matching stated edition or an explicit checked confirmation. The selected Version must be empty and have no active download.

## Media-server naming

Newly placed or explicitly renamed movies with a known edition receive the Plex filename tag `{edition-Name}`. Existing filenames stay unchanged until a reviewed rename. This follows [Plex's edition naming](https://support.plex.tv/articles/multiple-editions/). Plex's edition support also depends on its account, agent and server requirements. Producing this filename is not proof that a particular server has scanned and recognized it.

[Jellyfin](https://jellyfin.org/docs/general/server/media/movies/) and [Emby](https://emby.media/support/articles/Movie-Naming.html) describe generic versions grouped by a shared folder and filename prefix. Those are different recognition contracts. This addition does not claim that the Plex edition tag creates a native edition identity on either server. Live recognition and any server-specific arrangement require separate fixture or lab evidence; no installed server or media folder was changed for this build.
