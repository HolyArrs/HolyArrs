import { describe, expect, it } from "vitest";
import { deriveSeasonPick } from "./season-pick.ts";

describe("deriveSeasonPick", () => {
  it("keeps future after the followed season finishes airing", () => {
    const done = [
      { number: 1, keep_up_to_date: false, episodes: 10, not_out_yet: 0 },
      { number: 2, keep_up_to_date: true, episodes: 8, not_out_yet: 0 },
    ];
    expect(deriveSeasonPick(done, "all", "future")).toBe("future");
  });

  it("preserves future when all existing seasons are aired and unkept", () => {
    const aired = [
      { number: 1, keep_up_to_date: false, episodes: 10, not_out_yet: 0 },
      { number: 2, keep_up_to_date: false, episodes: 10, not_out_yet: 0 },
    ];
    expect(deriveSeasonPick(aired, "all", "future")).toBe("future");
    expect(deriveSeasonPick(aired, "none", "future")).toBe("none");
  });

  it("preserves future when ongoing or empty placeholder seasons are kept", () => {
    const mixed = [
      { number: 1, keep_up_to_date: false, episodes: 10, not_out_yet: 0 },
      { number: 2, keep_up_to_date: true, episodes: 8, not_out_yet: 2 },
      { number: 3, keep_up_to_date: true, episodes: 0, not_out_yet: 0 },
    ];
    expect(deriveSeasonPick(mixed, "all", "future")).toBe("future");
  });

  it("returns custom when future is manually overridden per season", () => {
    const overridden = [
      { number: 1, keep_up_to_date: true, episodes: 10, not_out_yet: 0 },
      { number: 2, keep_up_to_date: true, episodes: 8, not_out_yet: 2 },
    ];
    expect(deriveSeasonPick(overridden, "all", "future")).toBe("all");

    const partiallyOff = [
      { number: 1, keep_up_to_date: true, episodes: 10, not_out_yet: 0 },
      { number: 2, keep_up_to_date: false, episodes: 8, not_out_yet: 2 },
    ];
    expect(deriveSeasonPick(partiallyOff, "all", "future")).toBe("custom");
  });

  it("preserves latest vs all on a single-season show", () => {
    const one = [
      { number: 1, keep_up_to_date: true, episodes: 8, not_out_yet: 0 },
    ];
    expect(deriveSeasonPick(one, "all", "latest")).toBe("latest");
    expect(deriveSeasonPick(one, "all", "all")).toBe("all");
    expect(deriveSeasonPick(one, "all", undefined)).toBe("all");
  });

  it("handles multi-season all, latest, none, and custom", () => {
    expect(
      deriveSeasonPick(
        [
          { number: 1, keep_up_to_date: true },
          { number: 2, keep_up_to_date: true },
        ],
        "all",
        "all",
      ),
    ).toBe("all");

    expect(
      deriveSeasonPick(
        [
          { number: 1, keep_up_to_date: false },
          { number: 2, keep_up_to_date: true },
        ],
        "all",
        "latest",
      ),
    ).toBe("latest");

    expect(
      deriveSeasonPick(
        [
          { number: 1, keep_up_to_date: true },
          { number: 2, keep_up_to_date: false },
        ],
        "all",
        "all",
      ),
    ).toBe("custom");

    expect(
      deriveSeasonPick(
        [
          { number: 1, keep_up_to_date: false },
          { number: 2, keep_up_to_date: false },
        ],
        "none",
        "none",
      ),
    ).toBe("none");
  });

  it("ignores specials (season 0) when deriving the pick", () => {
    const withSpecials = [
      { number: 0, keep_up_to_date: false, episodes: 2, not_out_yet: 1 },
      { number: 1, keep_up_to_date: false, episodes: 8, not_out_yet: 0 },
    ];
    expect(deriveSeasonPick(withSpecials, "all", "future")).toBe("future");
  });
});
