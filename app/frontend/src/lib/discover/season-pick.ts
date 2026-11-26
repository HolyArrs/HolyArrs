export type SeasonPick = "all" | "future" | "latest" | "none";

/**
 * Derives the Seasons dropdown value from a show's kept seasons and policies.
 * Preserves "future" when kept seasons match upcoming/empty seasons, and uses
 * season_choice as a tie-breaker on 1-season shows so "latest" stays distinct.
 */
export function deriveSeasonPick(
  seasons: {
    number: number;
    keep_up_to_date: boolean;
    not_out_yet?: number;
    episodes?: number;
  }[],
  newSeasons: string | undefined,
  seasonChoice: string | undefined,
): SeasonPick | "custom" {
  const real = seasons.filter((x) => x.number > 0);
  const kept = real.filter((x) => x.keep_up_to_date);
  const newOn = newSeasons !== "none";
  const latest = Math.max(0, ...real.map((x) => x.number));

  if (
    seasonChoice === "future" &&
    newOn &&
    !real.some((x) => !x.keep_up_to_date && (x.not_out_yet ?? 0) > 0) &&
    !(
      kept.length === real.length &&
      real.some((x) => (x.episodes ?? 0) > 0 && (x.not_out_yet ?? 0) === 0)
    )
  ) {
    return "future";
  }
  if (kept.length === 0 && !newOn) return "none";
  if (real.length === 1 && kept.length === 1 && newOn) {
    return seasonChoice === "latest" ? "latest" : "all";
  }
  if (kept.length === real.length && newOn) return "all";
  if (kept.length === 1 && kept[0].number === latest && newOn) return "latest";
  return "custom";
}
