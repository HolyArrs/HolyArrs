# Find a title in the Library

You want to find a movie, show or artist you already added. Use the search box on the Library page. To find something new to add, use the Search page.

## Steps

1. Open **Library**.
2. Type in the search box. It says **Search library**, or **Search movies**, **Search TV shows** or **Search music** if you picked a kind.
3. Wait a moment. The list updates as you type.
4. To see everything again, clear the box.

## How matching works

- It matches names, original names, other names from metadata, and years.
- Accents are optional. `amelie` finds `Amélie`.
- You can type the start of a word. `star` finds `Stargate`.
- Each extra word narrows the results. Every word must match.
- Symbols like `OR` and `*` are treated as plain text.

The kind and state buttons still apply. For example, pick Shows and Missing, then type part of a name.

Searching never adds a title, starts a download or changes a file. Removed titles never show up.

## Limits

The box takes up to 256 characters and 16 words.

## Troubleshooting

- **Nothing shows up.** Check the kind and state buttons. They may hide the title. Try fewer words.
- **An error shows on the page.** Shorten or change what you typed, then try again.

## For scripts

`GET /api/v1/titles?q=amelie` returns matching titles. It uses the same matching rules. An empty query lists the whole Library. See [The API](../../dev/api.md) for how to sign in and use keys.

See also [Add Sources, and link Jackett or Prowlarr](sources-bridge.md).
