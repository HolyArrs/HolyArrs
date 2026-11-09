import { liveByTitle, liveFacts } from "./live.ts";
import { lifecycle } from "./lifecycle.ts";
import lab from "./testdata/live-steps.lab.json";
import { describe, expect, it } from "vitest";
import type { Ask } from "../core/types.ts";
import type { SearchView } from "../core/types.ts";
import type { DownloadRow } from "../core/types.ts";

type Snap = {
  searches: { running?: SearchView[] };
  downloads: { downloads: DownloadRow[] };
  ask: Pick<Ask, "state" | "title_id"> & { title_id: number };
};
const snaps = lab as unknown as Snap[];

// Responses kept from a lab run of an approved ask for Dune: Part Two: GET /api/v1/searches and GET /api/v1/downloads/view, read as the Requests page reads them.
describe("Requests card during a real lab download (#139)", () => {
  const labels = snaps.map((s) => {
    const live = liveByTitle(s.downloads.downloads, s.searches.running ?? []);
    const ask = s.ask;
    const facts = liveFacts(live.get(ask.title_id));
    return {
      life:
        ask.state === "approved" && !facts.phase
          ? lifecycle(ask)
          : lifecycle(ask, facts),
      facts,
    };
  });
  const seq = labels
    .map((l) => l.life.label)
    .filter((l, i, all) => l !== all[i - 1]);

  it("goes Searching, Downloading n%, Importing, then Ready", () => {
    expect(seq[0]).toBe("Processing");
    expect(seq.slice(1, 2)).toEqual(["Searching"]);
    const dl = seq.filter((l) => l.startsWith("Downloading · "));
    expect(
      dl.some((l) => l !== "Downloading · 0%" && l !== "Downloading · 100%"),
    ).toBe(true);
    const at = (l: string) => seq.findIndex((x) => x === l || x.startsWith(l));
    expect(at("Searching")).toBeLessThan(at("Downloading"));
    expect(at("Downloading")).toBeLessThan(at("Importing"));
    expect(at("Importing")).toBeLessThan(at("Ready"));
    expect(seq.at(-1)).toBe("Ready");
  });

  it("the bar only moves forward", () => {
    const pct = labels.map((l) => l.life.pct);
    for (let i = 1; i < pct.length; i++)
      expect(pct[i]).toBeGreaterThanOrEqual(pct[i - 1]);
  });
});
