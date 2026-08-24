<script lang="ts">
  import { onMount } from "svelte";
  import { Trophy, RefreshCw } from "@lucide/svelte";
  import { Button } from "$lib/components/ui/button/index.js";
  import { Input } from "$lib/components/ui/input/index.js";

  interface LeaderboardRow {
    model: string;
    task: string;
    best_tokens_per_s: number;
    run_at_unix: number;
    run_count: number;
  }

  let rows = $state<LeaderboardRow[]>([]);
  let loading = $state(true);
  let error = $state("");
  let filter = $state("");

  async function load(): Promise<void> {
    loading = true;
    error = "";
    try {
      const res = await fetch("/api/hugs/bench/leaderboard");
      if (!res.ok) throw new Error(`HTTP ${res.status}`);
      const data = await res.json();
      rows = (data.leaderboard ?? []) as LeaderboardRow[];
    } catch (cause) {
      error = cause instanceof Error ? cause.message : String(cause);
    } finally {
      loading = false;
    }
  }

  onMount(load);

  const filtered = $derived(
    filter.trim() === ""
      ? rows
      : rows.filter((r) => r.model.toLowerCase().includes(filter.toLowerCase()) || r.task.toLowerCase().includes(filter.toLowerCase())),
  );

  function bestForTask(task: string): number {
    return Math.max(...rows.filter((r) => r.task === task).map((r) => r.best_tokens_per_s));
  }
</script>

<div class="space-y-4 p-4">
  <div class="flex items-center justify-between">
    <h2 class="text-lg font-semibold flex items-center gap-2">
      <Trophy class="size-5" /> Benchmark Leaderboard
    </h2>
    <div class="flex items-center gap-2">
      <Input placeholder="Filter model or task…" bind:value={filter} class="w-56" />
      <Button variant="outline" size="sm" onclick={load} disabled={loading}>
        <RefreshCw class="size-4 {loading ? 'animate-spin' : ''}" />
        Refresh
      </Button>
    </div>
  </div>

  {#if loading}
    <p class="text-sm text-muted-foreground">Loading…</p>
  {:else if error}
    <p class="text-sm text-destructive">{error}</p>
  {:else}
    <div class="rounded-lg border overflow-auto max-h-[70vh]">
      <table class="w-full text-sm">
        <thead class="sticky top-0 bg-background border-b">
          <tr class="text-left text-muted-foreground">
            <th class="px-3 py-2 font-medium">#</th>
            <th class="px-3 py-2 font-medium">Model</th>
            <th class="px-3 py-2 font-medium">Task</th>
            <th class="px-3 py-2 font-medium text-right">Best tokens/s</th>
            <th class="px-3 py-2 font-medium text-right">Runs</th>
            <th class="px-3 py-2 font-medium">Last run</th>
          </tr>
        </thead>
        <tbody>
          {#each filtered as r, i (r.model + "/" + r.task)}
            <tr class="border-b last:border-0 hover:bg-muted/50 {r.best_tokens_per_s === bestForTask(r.task) ? 'bg-muted/30' : ''}">
              <td class="px-3 py-1.5 text-muted-foreground">{i + 1}</td>
              <td class="px-3 py-1.5 font-mono text-xs">{r.model}</td>
              <td class="px-3 py-1.5">{r.task}</td>
              <td class="px-3 py-1.5 text-right font-medium">
                {r.best_tokens_per_s.toFixed(1)}
                {#if r.best_tokens_per_s === bestForTask(r.task)}<span class="ml-1 text-xs text-yellow-500">★</span>{/if}
              </td>
              <td class="px-3 py-1.5 text-right text-muted-foreground">{r.run_count}</td>
              <td class="px-3 py-1.5 whitespace-nowrap text-muted-foreground">
                {new Date(r.run_at_unix * 1000).toLocaleDateString()}
              </td>
            </tr>
          {:else}
            <tr><td colspan="6" class="px-3 py-6 text-center text-muted-foreground">No benchmark data ingested.</td></tr>
          {/each}
        </tbody>
      </table>
    </div>
  {/if}
</div>
