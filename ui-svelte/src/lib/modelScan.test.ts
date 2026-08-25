import { describe, it, expect } from "vitest";
import {
  summarizeHFRescan,
  foundCapabilities,
  hfCapabilityKeys,
} from "./modelScan";
import type { HFRescanResponse } from "./modelScan";

function makeRescan(overrides: Partial<HFRescanResponse> = {}): HFRescanResponse {
  return {
    cache_root: "/home/user/.cache/huggingface/hub",
    cache_root_exists: true,
    scanned_at_unix: 1_720_000_000,
    repos: [],
    matched_repos: 0,
    unmatched_repos: 0,
    unmatched: [],
    config_refs: [],
    ...overrides,
  };
}

describe("summarizeHFRescan", () => {
  it("returns zero cache counts for an empty scan", () => {
    const s = summarizeHFRescan(makeRescan());
    expect(s.totalFiles).toBe(0);
    expect(s.totalBytes).toBe(0);
    expect(s.matchedRepos).toBe(0);
    expect(s.unmatchedRepos).toBe(0);
    expect(s.unmatchedRepoNames).toEqual([]);
    expect(s.hf).toBeNull();
    expect(s.tagsUpdated).toEqual([]);
  });

  it("totals cache bytes and files across repos", () => {
    const data = makeRescan({
      repos: [
        { repo_id: "org/a", matched: true, total_bytes: 100, files: [{ name: "a.gguf", path: "/x", size_bytes: 100 }] },
        { repo_id: "org/b", matched: false, total_bytes: 250, reason: "no config reference", files: [{ name: "b.gguf", path: "/y", size_bytes: 250 }] },
      ],
    });
    const s = summarizeHFRescan(data);
    expect(s.totalBytes).toBe(350);
    expect(s.totalFiles).toBe(2);
  });

  it("counts matched and unmatched repos", () => {
    const s = summarizeHFRescan(
      makeRescan({
        matched_repos: 3,
        unmatched_repos: 2,
        unmatched: ["org/z", "org/a", "org/z"],
      }),
    );
    expect(s.matchedRepos).toBe(3);
    expect(s.unmatchedRepos).toBe(2);
    expect(s.unmatchedRepoNames).toEqual(["org/a", "org/z"]);
  });

  it("exposes the HF verify summary and sorts models by id", () => {
    const s = summarizeHFRescan(
      makeRescan({
        verify: {
          checked: 2,
          matched: 1,
          unmatched: 1,
          unauthorized: 0,
          errors: 0,
          no_ref: 1,
          models: [
            { model_id: "z-model", status: "matched", repo_id: "org/z" },
            { model_id: "a-model", status: "unmatched", repo_id: "org/gone", http_status: 404 },
          ],
        },
      }),
    );
    expect(s.hf).not.toBeNull();
    expect(s.hf!.matched).toBe(1);
    expect(s.hf!.unmatched).toBe(1);
    expect(s.hf!.no_ref).toBe(1);
    expect(s.hf!.models.map((m) => m.model_id)).toEqual(["a-model", "z-model"]);
  });

  it("returns null hf section when verify was not requested", () => {
    const s = summarizeHFRescan(makeRescan());
    expect(s.hf).toBeNull();
  });

  it("propagates root, cache existence and scan time", () => {
    const s = summarizeHFRescan(
      makeRescan({ cache_root: "/data/hub", cache_root_exists: false, scanned_at_unix: 99 }),
    );
    expect(s.root).toBe("/data/hub");
    expect(s.cacheExists).toBe(false);
    expect(s.scannedAtUnix).toBe(99);
  });

  it("sorts and de-duplicates tags_updated", () => {
    const s = summarizeHFRescan(
      makeRescan({ tags_updated: ["z", "a", "a"] }),
    );
    expect(s.tagsUpdated).toEqual(["a", "z"]);
  });
});

describe("foundCapabilities", () => {
  it("returns only positively-found keys in canonical order", () => {
    const caps = { vision: true, audio: false, image: true, tools: false, mtp: false };
    expect(foundCapabilities(caps)).toEqual(["vision", "image"]);
  });

  it("returns empty for undefined caps", () => {
    expect(foundCapabilities(undefined)).toEqual([]);
  });

  it("canonical order matches the exported key list", () => {
    expect(foundCapabilities({ vision: true, audio: true, image: true, tools: true, mtp: true })).toEqual(
      [...hfCapabilityKeys],
    );
  });
});
