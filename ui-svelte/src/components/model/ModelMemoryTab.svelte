<script lang="ts">
  import { onMount } from "svelte";
  import * as Card from "$lib/components/ui/card/index.js";
  import type { Model } from "../../lib/types";

  interface Smoke { id: number; run_at: number; gpu_backend: string; context_size: number; gpu_vram_before_bytes: number; gpu_vram_peak_bytes: number; gpu_vram_after_bytes: number; success: number; error?: string; notes?: string }
  let { model }: { model: Model } = $props();
  let rows = $state<Smoke[]>([]);
  let loading = $state(true);
  let error = $state("");
  const gib = (n: number) => n > 0 ? `${(n / 1073741824).toFixed(2)} GiB` : "—";
  onMount(async () => {
    try {
      const r = await fetch(`/api/hugs/models/${encodeURIComponent(model.id)}/smoke`);
      if (!r.ok) throw new Error(`HTTP ${r.status}`);
      rows = (await r.json()) as Smoke[];
    } catch (e) { error = e instanceof Error ? e.message : String(e); }
    finally { loading = false; }
  });
</script>
<Card.Root class="shrink-0 gap-0 overflow-hidden py-0">
  <Card.Header class="border-b px-4 py-2"><Card.Title class="text-sm font-semibold">Memory</Card.Title></Card.Header>
  <Card.Content class="p-3">
    {#if loading}<span class="text-sm text-muted-foreground">Loading…</span>
    {:else if error}<span class="text-sm text-destructive">{error}</span>
    {:else if rows.length === 0}<span class="text-sm text-muted-foreground">No recorded memory measurements.</span>
    {:else}<div class="overflow-x-auto"><table class="w-full text-xs"><thead><tr class="border-b text-left text-muted-foreground"><th class="py-2 pr-3">Run</th><th class="py-2 pr-3">GPU</th><th class="py-2 pr-3 text-right">Before</th><th class="py-2 pr-3 text-right">Peak</th><th class="py-2 text-right">After</th></tr></thead><tbody>{#each rows as row (row.id)}<tr class="border-b last:border-0"><td class="py-2 pr-3">{new Date(row.run_at * 1000).toLocaleString()}</td><td class="py-2 pr-3">{row.gpu_backend}</td><td class="py-2 pr-3 text-right">{gib(row.gpu_vram_before_bytes)}</td><td class="py-2 pr-3 text-right font-medium">{gib(row.gpu_vram_peak_bytes)}</td><td class="py-2 text-right">{gib(row.gpu_vram_after_bytes)}</td></tr>{/each}</tbody></table></div>{/if}
  </Card.Content>
</Card.Root>
