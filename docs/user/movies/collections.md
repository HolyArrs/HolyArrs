# Movie collections

HolyArrs can store an exact collection identity from a movie details source and connect that collection to movies already in the Library. It does not infer collection membership from names, folders or release labels.

The collection screen is a reviewed shortcut over the normal safe movie removal. A preview lists the selected movie, Version and File IDs. It also includes a review token made from the complete current collection membership, File ownership and protection state, the selected movies and every removal choice. Confirmation is refused when any of that changes, so the person must review the current state again.

Keeping files is the default. When managed files are removed, the existing title removal path moves imported and adopted files to the holding folder. Files HolyArrs does not own remain untouched. The action never recursively deletes a collection folder. Removing a movie removes its Library entry even if the same movie belongs to another collection; the other collection and unrelated titles remain.

A movie with a protected present or missing copy is skipped as a whole while that copy would be retained. Its Library record and every other copy stay unchanged. Removing it requires both file removal and the separate protected-copy acknowledgment. Protection is checked again by the normal removal path under the same file-operation coordination used by imports.

Each selected movie reports `removed`, `skipped` or `failed`. A failure does not undo successful removals. Refresh the collection and preview only the remaining movies before retrying. Pending imports are coordinated by the normal title removal path. Unless Never-add is selected, an outside list may add a removed movie again later.
