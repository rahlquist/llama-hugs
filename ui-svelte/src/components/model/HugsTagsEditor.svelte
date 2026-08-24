<script lang="ts">
  import { onMount } from "svelte";
  import { Tag, Pencil, Check, X } from "@lucide/svelte";
  import { Button } from "$lib/components/ui/button/index.js";
  import { Input } from "$lib/components/ui/input/index.js";

  let { modelId }: { modelId: string } = $props();

  let tags = $state<string[]>([]);
  let loading = $state(true);
  let saving = $state(false);
  let editing = $state(false);
  let draft = $state("");
  let error = $state("");

  async function load(): Promise<void> {
    loading = true;
    error = "";
    try {
      const res = await fetch(`/api/hugs/meta/${encodeURIComponent(modelId)}`);
      if (!res.ok) throw new Error(`HTTP ${res.status}`);
      const data = await res.json();
      tags = (data.tags ?? "")
        .split(",")
        .map((t: string) => t.trim())
        .filter((t: string) => t !== "");
    } catch (cause) {
      error = cause instanceof Error ? cause.message : String(cause);
    } finally {
      loading = false;
    }
  }

  onMount(load);

  function startEdit(): void {
    draft = tags.join(", ");
    editing = true;
  }

  async function save(): Promise<void> {
    saving = true;
    error = "";
    try {
      const res = await fetch(`/api/hugs/meta/${encodeURIComponent(modelId)}`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ tags: draft }),
      });
      if (!res.ok) throw new Error(`HTTP ${res.status}`);
      editing = false;
      await load();
    } catch (cause) {
      error = cause instanceof Error ? cause.message : String(cause);
    } finally {
      saving = false;
    }
  }

  function cancel(): void {
    editing = false;
    draft = "";
    error = "";
  }
</script>

<div class="flex flex-wrap items-center gap-1.5">
  {#if loading}
    <span class="text-muted-foreground text-xs">…</span>
  {:else if editing}
    <div class="flex w-full max-w-md items-center gap-2">
      <Input
        bind:value={draft}
        placeholder="comma-separated tags"
        disabled={saving}
        onkeydown={(e) => {
          if (e.key === "Enter") save();
          else if (e.key === "Escape") cancel();
        }}
      />
      <Button variant="default" size="icon" onclick={save} disabled={saving} aria-label="Save tags">
        <Check class="size-4" />
      </Button>
      <Button variant="outline" size="icon" onclick={cancel} aria-label="Cancel editing">
        <X class="size-4" />
      </Button>
    </div>
  {:else}
    {#each tags as tag (tag)}
      <Tag class="bg-teal-500/15 text-teal-700 dark:text-teal-300">{tag}</Tag>
    {/each}
    <button
      type="button"
      class="text-muted-foreground hover:text-foreground cursor-pointer"
      onclick={startEdit}
      aria-label="Edit tags"
      title="Edit tags"
    >
      <Pencil class="size-3.5" />
    </button>
  {/if}
  {#if error}<span class="text-destructive text-xs">{error}</span>{/if}
</div>
