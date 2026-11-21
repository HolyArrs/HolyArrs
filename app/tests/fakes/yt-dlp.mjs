#!/usr/bin/env node
// A stand-in yt-dlp for the browser tests. It answers -J from the canned JSON
// the backend's own tests use, prints progress lines in the HAYT template
// HolyArrs asks for, and "downloads" a tiny playable clip with a thumbnail
// beside it. --version and -U answer like a current yt-dlp.
import { readFileSync, writeFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const data = join(
  dirname(fileURLToPath(import.meta.url)),
  "..",
  "..",
  "backend",
  "internal",
  "youtube",
  "testdata",
);

// Six seconds of plain colour, VP9 in MP4 (Playwright's Chromium has no H.264).
const clip = [
  "AAAAHGZ0eXBpc29tAAACAGlzb21pc28ybXA0MQAAA4ptb292AAAAbG12aGQAAAAAAAAAAAAAAAAAAAPoAAAXcAABAAABAAAAAAAAAAAAAAAAAQAAAAAAAAAA",
  "AAAAAAAAAAEAAAAAAAAAAAAAAAAAAEAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAACAAACtXRyYWsAAABcdGtoZAAAAAMAAAAAAAAAAAAAAAEAAAAA",
  "AAAXcAAAAAAAAAAAAAAAAAAAAAAAAQAAAAAAAAAAAAAAAAAAAAEAAAAAAAAAAAAAAAAAAEAAAAAAQAAAACQAAAAAACRlZHRzAAAAHGVsc3QAAAAAAAAAAQAA",
  "F3AAAAAAAAEAAAAAAi1tZGlhAAAAIG1kaGQAAAAAAAAAAAAAAAAAACgAAADwAFXEAAAAAAAtaGRscgAAAAAAAAAAdmlkZQAAAAAAAAAAAAAAAFZpZGVvSGFu",
  "ZGxlcgAAAAHYbWluZgAAABR2bWhkAAAAAQAAAAAAAAAAAAAAJGRpbmYAAAAcZHJlZgAAAAAAAAABAAAADHVybCAAAAABAAABmHN0YmwAAACoc3RzZAAAAAAA",
  "AAABAAAAmHZwMDkAAAAAAAAAAQAAAAAAAAAAAAAAAAAAAAAAQAAkAEgAAABIAAAAAAAAAAEXTGF2YzYzLjEuMTAyIGxpYnZweC12cDkAAAAAAAAAAAAY//8A",
  "AAAUdnBjQwEAAAAACoICAgIAAAAAAApmaWVsAQAAAAAQcGFzcAAAAAEAAAABAAAAFGJ0cnQAAAAAAAATiAAAAAAAAAAYc3R0cwAAAAAAAAABAAAAHgAACAAA",
  "AAAUc3RzcwAAAAAAAAABAAAAAQAAABxzdHNjAAAAAAAAAAEAAAABAAAAHgAAAAEAAACMc3RzegAAAAAAAAAAAAAAHgAAACoAAAAPAAAADwAAAA8AAAAPAAAA",
  "DwAAAA8AAAAPAAAADwAAAA8AAAAPAAAADwAAAA8AAAAPAAAADwAAAA8AAAAPAAAADwAAAA8AAAAPAAAADwAAAA8AAAAPAAAADwAAAA8AAAAPAAAADwAAAA8A",
  "AAAPAAAADwAAABRzdGNvAAAAAAAAAAEAAAO2AAAAYXVkdGEAAABZbWV0YQAAAAAAAAAhaGRscgAAAAAAAAAAbWRpcmFwcGwAAAAAAAAAAAAAAAAsaWxzdAAA",
  "ACSpdG9vAAAAHGRhdGEAAAABAAAAAExhdmY2My4xLjEwMgAAAAhmcmVlAAAB5W1kYXSCSYNCAAPwAjYAOCQcGEIAADBgAAATv//4fuAACP////gao/////XH",
  "QACGAECSnABJQAADIAAAVHCGAECSnABKwAADIAAAVHCGAECSnABJwAADIAAAVHCGAECSnABIoAADIAAAVHCGAECSnABHgAADIAAAVHCGAECSnABG4AADIAAA",
  "VHCGAECSnABGQAADIAAAVHCGAECSnABFwAADIAAAVHCGAECSnABFQAADIAAAVHCGAMCSnABCoAADIAAAVHCGAECSnABE4AADIAAAVHCGAECSnABEYAADIAAA",
  "VHCGAECSnABEAAADIAAAVHCGAECSnABDoAADIAAAVHCGAECSnABDQAADIAAAVHCGAECSnABDAAADIAAAVHCGAECSnABCwAADIAAAVHCGAECSnABCgAADIAAA",
  "VHCGAECSnABCQAADIAAAVHCGAMCSnABAIAADIAAAVHCGAECSnABCAAADIAAAVHCGAECSnABB4AADIAAAVHCGAECSnABBwAADIAAAVHCGAECSnABBgAADIAAA",
  "VHCGAECSnABBYAADIAAAVHCGAECSnABBIAADIAAAVHCGAECSnABBAAADIAAAVHCGAECSnABA4AADIAAAVHCGAECSnABA4AADIAAAVHA=",
].join("");
// A 64x36 JPEG of the same colour.
const thumb =
  "/9j/4AAQSkZJRgABAgAAAQABAAD//gAPTGF2YzYzLjEuMTAyAP/bAEMACBQUFxQXGxsbGxsbIB4gISEhICAgICEhISQkJCoqKiQkJCEhJCQoKCoqLi8uKysqKy8vMjIyPDw5OUZGSFZWZ//EAE0AAQEAAAAAAAAAAAAAAAAAAAAGAQEBAQAAAAAAAAAAAAAAAAAABQYQAQAAAAAAAAAAAAAAAAAAAAARAQAAAAAAAAAAAAAAAAAAAAD/wAARCAAkAEADASIAAhEAAxEA/9oADAMBAAIRAxEAPwCOAa1IAAAAAAAAAAAAAAAAAAAAAAf/2Q==";

const args = process.argv.slice(2);
const url = args.at(-1) ?? "";
const out = args[args.indexOf("-o") + 1];

/** @param {number} ms */
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

if (args.includes("--version")) {
  console.log("2026.10.26");
} else if (args.includes("-U")) {
  console.log("yt-dlp is up to date (2026.10.26)");
} else if (args.includes("-o") && out) {
  // The output name is a template: "<base>.%(ext)s" with "%" doubled.
  const base = out.replace(/\.%\(ext\)s$/, "").replaceAll("%%", "%");
  // Slow enough for the page to show the download going.
  for (const done of [100, 300, 500, 700, 900]) {
    console.log(`HAYT ${done} 1000 NA`);
    await sleep(700);
  }
  writeFileSync(`${base}.mp4`, Buffer.from(clip, "base64"));
  writeFileSync(`${base}.jpg`, Buffer.from(thumb, "base64"));
  console.log("HAYT 1000 1000 NA");
  console.log(`${base}.mp4`);
} else {
  /** @param {string} name */
  const canned = (name) => readFileSync(join(data, name), "utf8");
  const video = /watch\?v=([\w-]{11})$/.exec(url);
  if (video)
    process.stdout.write(
      video[1] === "dQw4w9WgXcQ"
        ? canned("video.json")
        : canned("info.json").replaceAll("@ID@", video[1]),
    );
  else if (url.includes("playlist?list="))
    process.stdout.write(canned("playlist.json"));
  else if (url.endsWith("/videos"))
    process.stdout.write(canned("channel_videos.json"));
  else if (url.endsWith("/streams"))
    process.stdout.write(canned("channel_streams.json"));
  else if (url.endsWith("/shorts"))
    process.stdout.write(canned("channel_shorts.json"));
  else {
    console.error(`ERROR: Unsupported URL: ${url}`);
    process.exit(1);
  }
}
