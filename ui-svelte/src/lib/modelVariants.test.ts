import { describe, expect, it } from "vitest";
import type { Model } from "./types";
import { modelFamily, resolveModelFamily, variantLabel } from "./modelVariants";

const model = (id: string, overrides: Partial<Model> = {}): Model => ({
  id,
  state: "stopped",
  name: id,
  description: "",
  unlisted: false,
  peerID: "",
  ...overrides,
});

describe("model variants", () => {
  it("groups ROCm and CUDA implementations into one family", () => {
    expect(modelFamily(model("hugs-gemma-3-12b"))).toBe("gemma-3-12b");
    expect(modelFamily(model("hugs-gemma-3-12b-cuda"))).toBe("gemma-3-12b");
  });

  it("uses explicit server family metadata", () => {
    expect(modelFamily(model("custom", { family: "Gemma-3-12B" }))).toBe("gemma-3-12b");
  });

  it("labels server-emitted engine and driver values", () => {
    expect(variantLabel(model("hugs-gemma-3-12b", { backend: "llama.cpp", driver: "rocm" }))).toBe("llama.cpp · ROCm");
    expect(variantLabel(model("hugs-gemma-3-12b-cuda", { backend: "vllm", driver: "cuda" }))).toBe("vLLM · CUDA");
  });

  it("preserves peer model detail resolution", () => {
    const peer = model("remote/model", { peerID: "remote" });
    expect(resolveModelFamily([peer], peer.id)?.selected).toBe(peer);
  });

  it("preselects an exact variant and resolves a family route", () => {
    const variants = [
      model("hugs-gemma-3-12b"),
      model("hugs-gemma-3-12b-cuda", { state: "ready" }),
    ];
    expect(resolveModelFamily(variants, "hugs-gemma-3-12b")?.selected.id).toBe("hugs-gemma-3-12b");
    expect(resolveModelFamily(variants, "gemma-3-12b")?.selected.id).toBe("hugs-gemma-3-12b-cuda");
    expect(resolveModelFamily(variants, "missing")).toBeUndefined();
  });
});
