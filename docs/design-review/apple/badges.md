# Poster badges (2026-10-19)

References looked at on Mobbin: Hulu (My Stuff wide cards), Tubi (poster grid), Apple TV (detail marks). Behaviour and feel only, in our words.

1. **Dark glass, not colour blocks.** The best apps put a small translucent dark pill over art. The art shows through; the pill never fights the poster.
2. **Colour lives in the glyph.** The word stays white; only a tiny line icon carries the state tint (green check, blue ring, amber clock).
3. **Sentence case, small, calm.** Labels read like words, not shouting caps. One size and height for every badge on every card.
4. **Two at most, in the corners.** Type top-left, status top-right. When a card is narrow the status folds to its glyph and keeps its full sentence as the accessible name and tooltip.
5. **Progress as a ring.** A download shows a thin ring and a percent instead of a separate bar.

Built as `web/app/src/lib/ui/ArtPill.svelte`; `Badge` and `StatusPill` use it whenever `on="art"`, so the Coming soon day tag, type, status and Top 10 status all match.
