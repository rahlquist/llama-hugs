import type { Model } from "./types";

export function modelFamily(model: Model): string {
  if (model.family?.trim()) return model.family.trim().toLowerCase();
  return model.id.toLowerCase().replace(/^hugs-/, "").replace(/(?:-|_)cuda$/, "");
}

function inferredBackend(model: Model): string {
  if (model.backend) return model.backend.toLowerCase();
  if (/^(?:hugs-)?v[rc]-/.test(model.id.toLowerCase())) return "vllm";
  return "llama.cpp";
}

function inferredDriver(model: Model): string {
  if (model.driver) return model.driver.toLowerCase();
  const id = model.id.toLowerCase();
  return id.endsWith("-cuda") || id.endsWith("_cuda") || /^(?:hugs-)?[lv]c-/.test(id) ? "cuda" : "rocm";
}

export function variantLabel(model: Model): string {
  const backendValue = inferredBackend(model);
  const driverValue = inferredDriver(model);
  const backend = backendValue === "v" || backendValue === "vllm" ? "vLLM" : backendValue === "l" || backendValue === "llama.cpp" ? "llama.cpp" : backendValue;
  const driver = driverValue === "c" || driverValue === "cuda" ? "CUDA" : driverValue === "r" || driverValue === "rocm" ? "ROCm" : driverValue.toUpperCase();
  return `${backend} · ${driver}`;
}

export interface ResolvedModelFamily {
  family: string;
  variants: Model[];
  selected: Model;
}

export function resolveModelFamily(models: Model[], idOrFamily: string): ResolvedModelFamily | undefined {
  const exact = models.find((model) => model.id === idOrFamily);
  const alias = exact ? undefined : models.find((model) => model.aliases?.includes(idOrFamily));
  const seed = exact ?? alias;
  const family = seed ? modelFamily(seed) : idOrFamily.toLowerCase().replace(/^hugs-/, "");
  if (seed?.peerID) return { family, variants: [seed], selected: seed };
  const variants = models
    .filter((model) => !model.peerID && modelFamily(model) === family)
    .sort((a, b) => variantLabel(a).localeCompare(variantLabel(b)) || a.id.localeCompare(b.id));
  if (variants.length === 0) return undefined;
  return { family, variants, selected: seed ?? variants.find((model) => model.state === "ready") ?? variants[0] };
}
