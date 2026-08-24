<script lang="ts">
  import { onMount } from "svelte";
  import { HardDrive, RefreshCw, Flag, FlagOff, Trash2 } from "@lucide/svelte";
  import { Button } from "$lib/components/ui/button/index.js";

  interface FileEntry {
    path: string;
    name: string;
    size_bytes: number;
    mtime_unix: number;
  }
  interface DirReport {
    root: string;
    total_bytes: number;
    file_count: number;
    files?: FileEntry[];
    scanned_at_ms: number;
  }

  let report = $state<DirReport | null>(null);
  let orphans = $state<string[]>([]);
  let flagged = $state<Set<string>>(new Set());
  let selected = $state<Set<string>>(new Set());
  let loading = $state(true);
  let error = $state("");
  let actionError = $state("");
  let notice = $state("");
  let confirmDelete = $state(false);

  function formatBytes(bytes: number): string {
    if (bytes >= 1e9) return (bytes / 1e9).toFixed(2) + " GB";
    if (bytes >= 1e6) return (bytes / 1e6).toFixed(1) + " MB";
    return (bytes / 1e3).toFixed(0) + " KB";
  }

  async function load(): Promise<void> {
    loading = true;
    error = "";
    notice = "";
    try {
      const [diskRes, flagsRes] = await Promise.all([
        fetch("/api/hugs/disk?files=1"),
        fetch("/api/hugs/files/flags"),
      ]);
      if (!diskRes.ok) throw new Error(`disk: HTTP ${diskRes.status}`);
      const diskData = await diskRes.json();
      report = diskData.report as DirReport;
      orphans = (diskData.orphans ?? []) as string[];
      if (flagsRes.ok) {
        const flagsData = await flagsRes.json();
        flagged = new Set((flagsData.flags ?? []) as string[]);
      }
      selected = new Set();
    } catch (cause) {
      error = cause instanceof Error ? cause.message : String(cause);
    } finally {
      loading = false;
    }
  }

  onMount(load);

  const orphanSet = $derived(new Set(orphans));
  const files = $derived(report?.files ?? []);
  const allSelected = $derived(files.length > 0 && files.every((f) => selected.has(f.name)));
  const someSelected = $derived(files.some((f) => selected.has(f.name)));

  function toggleAll(): void {
    if (allSelected) {
      selected = new Set();
    } else {
      selected = new Set(files.map((f) => f.name));
    }
  }

  function toggle(name: string): void {
    const next = new Set(selected);
    if (next.has(name)) {
      next.delete(name);
    } else {
      next.add(name);
    }
    selected = next;
  }

  async function setFlags(flaggedState: boolean): Promise<void> {
    actionError = "";
    notice = "";
    const names = [...selected];
    if (names.length === 0) return;
    try {
      const res = await fetch("/api/hugs/files/flags", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ names, flagged: flaggedState }),
      });
      if (!res.ok) throw new Error(`HTTP ${res.status}`);
      const data = await res.json();
      flagged = new Set((data.flags ?? []) as string[]);
      notice = `${names.length} file(s) ${flaggedState ? "flagged for" : "removed from"} deletion.`;
    } catch (cause) {
      actionError = cause instanceof Error ? cause.message : String(cause);
    }
  }

  async function deleteSelected(): Promise<void> {
    if (!confirmDelete) {
      confirmDelete = true;
      return;
    }
    actionError = "";
    notice = "";
    const targets = files.filter((f) => selected.has(f.name));
    try {
      const res = await fetch("/api/hugs/files/delete", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ paths: targets.map((f) => f.path) }),
      });
      const data = await res.json();
      const deletedCount = (data.deleted ?? []).length;
      const errors: string[] = data.errors ?? [];
      if (errors.length > 0) actionError = errors.join("; ");
      if (deletedCount > 0) notice = `Deleted ${deletedCount} file(s).`;
      confirmDelete = false;
      await load();
    } catch (cause) {
      actionError = cause instanceof Error ? cause.message : String(cause);
      confirmDelete = false;
    }
  }
</script>

<div class="space-y-4 p-4">
  <div class="flex items-center justify-between">
    <h2 class="text-lg font-semibold flex items-center gap-2">
      <HardDrive class="size-5" /> Disk &amp; Orphans
    </h2>
    <Button variant="outline" size="sm" onclick={load} disabled={loading}>
      <RefreshCw class="size-4 {loading ? 'animate-spin' : ''}" />
      Refresh
    </Button>
  </div>

  {#if loading}
    <p class="text-sm text-muted-foreground">Scanning…</p>
  {:else if error}
    <p class="text-sm text-destructive">{error}</p>
  {:else if report}
    <div class="grid grid-cols-3 gap-3">
      <div class="rounded-lg border p-3">
        <p class="text-xs text-muted-foreground">Total GGUF size</p>
        <p class="text-xl font-semibold">{formatBytes(report.total_bytes)}</p>
      </div>
      <div class="rounded-lg border p-3">
        <p class="text-xs text-muted-foreground">Files on disk</p>
        <p class="text-xl font-semibold">{report.file_count}</p>
      </div>
      <div class="rounded-lg border p-3">
        <p class="text-xs text-muted-foreground">Orphans (no config entry)</p>
        <p class="text-xl font-semibold" class:text-destructive={orphans.length > 0}>{orphans.length}</p>
      </div>
    </div>

    {#if someSelected}
      <div class="flex items-center gap-2 rounded-lg border p-2">
        <span class="text-sm text-muted-foreground mr-auto">{selected.size} selected</span>
        <Button variant="outline" size="sm" onclick={() => setFlags(true)}>
          <Flag class="size-4" /> Flag for deletion
        </Button>
        <Button variant="outline" size="sm" onclick={() => setFlags(false)}>
          <FlagOff class="size-4" /> Remove flag
        </Button>
        <Button variant="destructive" size="sm" onclick={deleteSelected}>
          <Trash2 class="size-4" />
          {confirmDelete ? "Really delete?" : "Delete immediately"}
        </Button>
      </div>
    {/if}

    {#if notice}<p class="text-sm text-green-600">{notice}</p>{/if}
    {#if actionError}<p class="text-sm text-destructive">{actionError}</p>{/if}

    <div class="rounded-lg border overflow-auto max-h-[60vh]">
      <table class="w-full text-sm">
        <thead class="sticky top-0 bg-background border-b">
          <tr class="text-left text-muted-foreground">
            <th class="px-3 py-2 font-medium">File</th>
            <th class="px-3 py-2 font-medium">Size</th>
            <th class="px-3 py-2 font-medium">Modified</th>
            <th class="px-3 py-2 font-medium">Status</th>
            <th class="px-3 py-2 font-medium w-10">
              <input
                type="checkbox"
                class="size-4 accent-primary cursor-pointer"
                checked={allSelected}
                onclick={toggleAll}
                aria-label="Select all"
              />
            </th>
          </tr>
        </thead>
        <tbody>
          {#each files as f (f.name)}
            <tr
              class="border-b last:border-0 hover:bg-muted/50 {selected.has(f.name) ? 'bg-muted/40' : ''}"
            >
              <td class="px-3 py-1.5 font-mono text-xs">
                {f.name}
                {#if flagged.has(f.name)}<span class="ml-1 text-xs text-orange-500">⚑ flagged</span>{/if}
              </td>
              <td class="px-3 py-1.5 whitespace-nowrap">{formatBytes(f.size_bytes)}</td>
              <td class="px-3 py-1.5 whitespace-nowrap text-muted-foreground">
                {new Date(f.mtime_unix * 1000).toLocaleDateString()}
              </td>
              <td class="px-3 py-1.5">
                {#if orphanSet.has(f.name)}
                  <span class="text-destructive">orphan</span>
                {:else}
                  <span class="text-muted-foreground">registered</span>
                {/if}
              </td>
              <td class="px-3 py-1.5">
                <input
                  type="checkbox"
                  class="size-4 accent-primary cursor-pointer"
                  checked={selected.has(f.name)}
                  onclick={() => toggle(f.name)}
                  aria-label="Select {f.name}"
                />
              </td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>

    <p class="text-xs text-muted-foreground">
      Deletion is permanent and restricted to .gguf files in the model cache. Registered models will break if their file is deleted — prefer orphans.
    </p>
  {/if}
</div>
