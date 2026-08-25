// Model rescan + Hugging Face verification helpers for the Models dashboard.
//
// The rescan UI calls POST /api/hugs/hf/rescan?verify=1, which scans the local
// HF hub cache, matches cached repos against config references, and (with
// verify=1) queries the PUBLIC Hugging Face API for every configured model
// that references an HF repo — exact-matching each repo id and deriving
// conservative capability findings (vision/audio/image/tools/MTP). These
// helpers turn that raw response into the summary the dashboard renders.

export interface HFRepoFile {
  name: string;
  path: string;
  size_bytes: number;
}

export interface HFRepoReport {
  repo_id: string;
  matched: boolean;
  config_model?: string;
  reason?: string;
  revision?: string;
  total_bytes: number;
  files: HFRepoFile[];
}

export interface HFCapabilities {
  vision: boolean;
  audio: boolean;
  image: boolean;
  tools: boolean;
  mtp: boolean;
}

export type HFModelStatus = "matched" | "unmatched" | "unauthorized" | "error" | "no_ref";

export interface HFModelResult {
  model_id: string;
  repo_id?: string;
  status: HFModelStatus;
  http_status?: number;
  error?: string;
  reason?: string;
  capabilities?: HFCapabilities;
  pipeline_tag?: string;
  evidence?: string[];
}

export interface HFVerifySummary {
  checked: number;
  matched: number;
  unmatched: number;
  unauthorized: number;
  errors: number;
  no_ref: number;
  models: HFModelResult[];
}

export interface HFConfigRef {
  model_id: string;
  repo_id: string;
  file?: string;
  raw?: string;
}

export interface HFRescanResponse {
  cache_root: string;
  cache_root_exists: boolean;
  scanned_at_unix: number;
  repos: HFRepoReport[];
  matched_repos: number;
  unmatched_repos: number;
  unmatched: string[];
  config_refs: HFConfigRef[];
  verify?: HFVerifySummary;
  meta_updated?: string[];
  tags_updated?: string[];
  persisted_at_unix?: number;
}

export interface ScanSummary {
  root: string;
  cacheExists: boolean;
  scannedAtUnix: number;
  /** Cache section: total GGUF bytes across cached repos. */
  totalBytes: number;
  /** Cache section: total GGUF files across cached repos. */
  totalFiles: number;
  matchedRepos: number;
  unmatchedRepos: number;
  /** Sorted, de-duplicated cached repos with no config reference. */
  unmatchedRepoNames: string[];
  /** HF verification section; null when verify=1 was not requested. */
  hf: HFVerifySummary | null;
  /** Models whose hf:* tags were rewritten by this scan. */
  tagsUpdated: string[];
}

/** Canonical ordering of HF-derived capability keys. */
export const hfCapabilityKeys: ReadonlyArray<keyof HFCapabilities> = [
  "vision",
  "audio",
  "image",
  "tools",
  "mtp",
];

/** Human labels for the HF-derived capability findings. */
export const hfCapabilityLabels: Record<string, string> = {
  vision: "Vision",
  audio: "Audio",
  image: "Image Gen",
  tools: "Tools",
  mtp: "MTP",
};

/** Muted pastel badge classes per HF-derived capability key. */
export const hfCapabilityBadgeClass: Record<string, string> = {
  vision: "bg-violet-500/15 text-violet-700 dark:text-violet-300",
  audio: "bg-amber-500/15 text-amber-700 dark:text-amber-300",
  image: "bg-fuchsia-500/15 text-fuchsia-700 dark:text-fuchsia-300",
  tools: "bg-emerald-500/15 text-emerald-700 dark:text-emerald-300",
  mtp: "bg-cyan-500/15 text-cyan-700 dark:text-cyan-300",
};

/** Human labels per HF model verification status. */
export const hfStatusLabels: Record<HFModelStatus, string> = {
  matched: "Matched",
  unmatched: "Unmatched",
  unauthorized: "Gated",
  error: "Error",
  no_ref: "No HF ref",
};

const numericCompare = (a: string, b: string) =>
  a.localeCompare(b, undefined, { numeric: true });

/**
 * Summarizes POST /api/hugs/hf/rescan?verify=1 output for the dashboard:
 * cache totals, explicit unmatched repos, and (when present) the per-model
 * HF verification results.
 */
export function summarizeHFRescan(data: HFRescanResponse): ScanSummary {
  const repos = data?.repos ?? [];
  const unmatchedRepoNames = [...new Set(data?.unmatched ?? [])].sort(numericCompare);
  const totalBytes = repos.reduce((sum, r) => sum + (r.total_bytes ?? 0), 0);
  const totalFiles = repos.reduce((sum, r) => sum + (r.files?.length ?? 0), 0);

  const verify = data?.verify;
  const hf: HFVerifySummary | null = verify
    ? {
        checked: verify.checked ?? 0,
        matched: verify.matched ?? 0,
        unmatched: verify.unmatched ?? 0,
        unauthorized: verify.unauthorized ?? 0,
        errors: verify.errors ?? 0,
        no_ref: verify.no_ref ?? 0,
        models: [...(verify.models ?? [])].sort((a, b) =>
          numericCompare(a.model_id, b.model_id),
        ),
      }
    : null;

  return {
    root: data?.cache_root ?? "",
    cacheExists: data?.cache_root_exists ?? false,
    scannedAtUnix: data?.scanned_at_unix ?? 0,
    totalBytes,
    totalFiles,
    matchedRepos: data?.matched_repos ?? 0,
    unmatchedRepos: data?.unmatched_repos ?? 0,
    unmatchedRepoNames,
    hf,
    tagsUpdated: [...new Set(data?.tags_updated ?? [])].sort(numericCompare),
  };
}

/** Returns the capability keys positively found on a model result. */
export function foundCapabilities(caps?: HFCapabilities): Array<keyof HFCapabilities> {
  const out: Array<keyof HFCapabilities> = [];
  if (!caps) return out;
  for (const key of hfCapabilityKeys) {
    if (caps[key]) out.push(key);
  }
  return out;
}
