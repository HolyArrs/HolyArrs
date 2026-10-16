# Protected copies

Protection belongs to an installed movie or show file. Use Copy details on its title page to protect it. Protection persists across restart and rescans, including when that file is temporarily missing. Search settlement and import placement both recheck the current protection state, so an older search or queued download cannot replace it.

Keep this copy and add another Version protects the existing owned file and creates the chosen Version in one database transaction. Choose its label, Library folder and Pick rules explicitly. Failure leaves both the old protection and Versions unchanged. This action does not enqueue a download or copy the existing bytes into the new Version.

Removing protection requires a separate acknowledgment in Copy details. Normal title and movie-series removal retains a protected copy and skips the entire title, keeping its Library record. Explicit managed-file removal can include the separate protected-copy acknowledgment. Only imported or adopted files go to the holding folder; unowned files stay untouched. A successful acknowledged move clears protection after the move, allowing the ordinary holding cleanup later. A failed move retains protection.

Holding cleanup skips protected rows. Multi-episode show files retain their shared-file safeguards: a protected file cannot be partially replaced by a pack. Protection does not change unrelated titles, Versions or Pick rules.

These checks coordinate local file operations and database state. They do not prevent another program or a person outside HolyArrs from changing a file.
