<script lang="ts">
  import { onMount } from "svelte";
  import type { Snippet } from "svelte";
  import { link } from "svelte-spa-router";
  import {
    activeProfile,
    fetchPlaygroundModels,
    models,
    profiles,
    selectorModels,
    unloadAllModels,
  } from "../stores/api";
  import { statusDotColor } from "../stores/modelLoad";
  import { showUnlistedModels as showUnlisted, showCapabilityTags } from "../stores/modelDisplay";
  import { listCapabilityBadges, capabilityBadgeClass } from "../lib/capabilities";
  import type { Model } from "../lib/types";
  import ModelLoadButton from "../components/ModelLoadButton.svelte";
  import Tag from "../components/Tag.svelte";
  import * as Card from "$lib/components/ui/card/index.js";
  import { Button } from "$lib/components/ui/button/index.js";
  import * as Switch from "$lib/components/ui/switch/index.js";
  import * as Label from "$lib/components/ui/label/index.js";
  import { PowerOff, Loader2, ExternalLink, SquareStack, RefreshCw, HardDrive, Globe, CircleCheck, CircleX, TriangleAlert } from "@lucide/svelte";
  import { modelServerPath } from "../lib/modelUtils";
  import { formatCapacity } from "../lib/format";
  import {
    summarizeHFRescan,
    foundCapabilities,
    hfStatusLabels,
    hfCapabilityLabels,
    hfCapabilityBadgeClass,
    type HFModelResult,
    type HFRescanResponse,
    type ScanSummary,
  } from "../lib/modelScan";

  let unloadingAll = $state(false);
  let hugTags = $state<Record<string, string>>({});

  let scanning = $state(false);
  let scanError = $state("");
  let scanSummary = $state<ScanSummary | null>(null);

  const hfStatusClass: Record<HFModelResult["status"], string> = {
    matched: "bg-emerald-500/15 text-emerald-700 dark:text-emerald-300",
    unmatched: "bg-destructive/15 text-destructive",
    unauthorized: "bg-amber-500/15 text-amber-700 dark:text-amber-300",
    error: "bg-orange-500/15 text-orange-700 dark:text-orange-300",
    no_ref: "bg-muted text-muted-foreground",
  };

  onMount(() => {
    void fetchPlaygroundModels();
    void loadHugTags();
    void rescanModels();
  });

  async function loadHugTags(): Promise<void> {
    try {
      const res = await fetch("/api/hugs/meta");
      const rows = res.ok ? ((await res.json()) as Array<{ model_id: string; tags: string }>) : [];
      const map: Record<string, string> = {};
      for (const row of rows ?? []) {
        if (row.tags) map[row.model_id] = row.tags;
      }
      hugTags = map;
    } catch {
      // tags are decoration; never block the model list
    }
  }

  async function rescanModels(): Promise<void> {
    scanning = true;
    scanError = "";
    try {
      const res = await fetch("/api/hugs/hf/rescan?verify=1", { method: "POST" });
      if (!res.ok) throw new Error(`scan: HTTP ${res.status}`);
      const data = (await res.json()) as HFRescanResponse;
      scanSummary = summarizeHFRescan(data);
      // The scan rewrites hf:* tags on model rows — refresh them.
      await loadHugTags();
    } catch (e) {
      scanError = e instanceof Error ? e.message : String(e);
      scanSummary = null;
    } finally {
      scanning = false;
    }
  }

  let visibleModels = $derived(
    $showUnlisted ? $models : $models.filter((m) => !m.unlisted)
  );
  let localModels = $derived(visibleModels.filter((model) => !model.peerID));
  let peerModels = $derived(visibleModels.filter((model) => model.peerID));
  let selectedProfile = $derived(
    $profiles.find((profile) => profile.id === $activeProfile)
  );
  let profileMappings = $derived(
    Object.entries(selectedProfile?.pins ?? {}).sort(([a], [b]) =>
      a.localeCompare(b, undefined, { numeric: true })
    )
  );

  let readyCount = $derived($models.filter((m) => m.state === "ready").length);
  let anyReady = $derived(readyCount > 0);

  async function handleUnloadAll(): Promise<void> {
    unloadingAll = true;
    try {
      await unloadAllModels();
    } catch (e) {
      console.error(e);
    } finally {
      unloadingAll = false;
    }
  }
</script>

{#snippet modelRow(model: Model)}
  <div class="hover:bg-muted/50 flex items-center gap-3 px-4 py-2.5">
    {#if !model.peerID}
      <span class={`size-2.5 shrink-0 rounded-full ${statusDotColor(model)}`}></span>
    {/if}
    <a
      href="/models/{encodeURIComponent(model.id)}"
      use:link
      class="min-w-0 flex-1"
    >
      <div class="truncate text-sm font-medium">{model.name || model.id}</div>
      <div class="text-muted-foreground truncate text-xs">
        {model.id}
        {#if model.aliases && model.aliases.length > 0}
          · {model.aliases.join(", ")}
        {/if}
      </div>
    </a>
    {#if $showCapabilityTags}
      {@const badges = listCapabilityBadges(model)}
      {#if badges.length > 0}
        <div class="hidden min-w-0 flex-wrap items-center gap-1 sm:flex">
          {#each badges as badge (badge.key)}
            <Tag class={`px-1.5 text-[0.625rem] ${capabilityBadgeClass[badge.key] ?? ""}`}>{badge.label}</Tag>
          {/each}
          {#each (hugTags[model.id] ?? "").split(",").map((t) => t.trim()).filter((t) => t !== "") as hugTag (hugTag)}
            <Tag class="bg-teal-500/15 text-teal-700 dark:text-teal-300 px-1.5 text-[0.625rem]">{hugTag}</Tag>
          {/each}
        </div>
      {:else if hugTags[model.id]}
        <div class="hidden min-w-0 flex-wrap items-center gap-1 sm:flex">
          {#each (hugTags[model.id] ?? "").split(",").map((t) => t.trim()).filter((t) => t !== "") as hugTag (hugTag)}
            <Tag class="bg-teal-500/15 text-teal-700 dark:text-teal-300 px-1.5 text-[0.625rem]">{hugTag}</Tag>
          {/each}
        </div>
      {/if}
    {/if}
    <span class="text-muted-foreground text-xs uppercase tracking-wide">
      {model.state}
    </span>
    {#if model.unlisted}
      <Tag class="px-1.5 text-[0.625rem] uppercase">unlisted</Tag>
    {/if}
    {#if !model.peerID}
      <a
        href={modelServerPath(model.id)}
        target="_blank"
        rel="noopener noreferrer"
        class="text-muted-foreground hover:text-foreground"
        title="Open model server"
        aria-label="Open model server"
      >
        <ExternalLink class="size-4" />
      </a>
      <ModelLoadButton {model} />
    {/if}
  </div>
{/snippet}

{#snippet modelSection(title: string, sectionModels: Model[], headerExtra?: Snippet)}
  <Card.Root class="shrink-0 gap-0 overflow-hidden py-0">
    <Card.Header class="shrink-0 border-b px-4 py-2.5">
      <div class="flex items-center gap-2">
        <Card.Title class="text-sm">{title}</Card.Title>
        <span class="text-muted-foreground text-xs">{sectionModels.length}</span>
        {#if headerExtra}
          <div class="ml-auto flex items-center gap-2">
            {@render headerExtra()}
          </div>
        {/if}
      </div>
    </Card.Header>
    <Card.Content class="p-0">
      {#if sectionModels.length === 0}
        <div class="text-muted-foreground px-4 py-6 text-center text-sm">
          No {title.toLowerCase()} available
        </div>
      {:else}
        <div class="divide-y">
          {#each sectionModels as model (model.id)}
            {@render modelRow(model)}
          {/each}
        </div>
      {/if}
    </Card.Content>
  </Card.Root>
{/snippet}

{#snippet unlistedToggle()}
  <Label.Root for="show-unlisted-toggle" class="text-sm">
    Show unlisted models
  </Label.Root>
  <Switch.Root
    id="show-unlisted-toggle"
    checked={$showUnlisted}
    onCheckedChange={(v) => showUnlisted.set(v)}
  />
  <span class="text-muted-foreground text-xs">
    {$models.filter((m) => m.unlisted).length} unlisted
  </span>
{/snippet}

<div class="flex h-full flex-col gap-4 overflow-y-auto p-2">
  <Card.Root class="shrink-0 gap-0 overflow-hidden py-0">
    <Card.Header class="shrink-0 gap-2 border-b px-4 py-3">
      <div class="flex items-center gap-2">
        <SquareStack class="size-5" />
        <Card.Title class="text-lg">Models</Card.Title>
        <span class="text-muted-foreground text-sm">
          ({visibleModels.length} of {$models.length})
        </span>
        <span class="text-muted-foreground text-xs uppercase tracking-wide">
          {readyCount} ready
        </span>
        <div class="ml-auto flex items-center gap-2">
          <Button
            variant="outline"
            size="sm"
            onclick={rescanModels}
            disabled={scanning}
            title="Scan the local HF cache and verify each model against Hugging Face (exact match, vision/audio/image/tools/MTP)"
          >
            {#if scanning}
              <Loader2 class="size-3.5 animate-spin" />
            {:else}
              <RefreshCw class="size-3.5" />
            {/if}
            Rescan &amp; Verify
          </Button>
          <Button
            variant="outline"
            size="sm"
            onclick={handleUnloadAll}
            disabled={!anyReady || unloadingAll}
          >
            {#if unloadingAll}
              <Loader2 class="size-3.5 animate-spin" />
            {:else}
              <PowerOff class="size-3.5" />
            {/if}
            Unload All
          </Button>
        </div>
      </div>
    </Card.Header>
  </Card.Root>

  <div class="flex min-h-0 shrink-0 flex-col gap-4">
    {#if scanning}
      <Card.Root class="shrink-0 gap-0 overflow-hidden py-0">
        <Card.Header class="shrink-0 border-b px-4 py-2.5">
          <div class="flex items-center gap-2">
            <Loader2 class="size-4 animate-spin" />
            <Card.Title class="text-sm">HF cache &amp; model verification</Card.Title>
            <span class="text-muted-foreground text-xs">Scanning…</span>
          </div>
        </Card.Header>
      </Card.Root>
    {:else if scanError}
      <Card.Root class="shrink-0 gap-0 overflow-hidden py-0">
        <Card.Header class="shrink-0 border-b px-4 py-2.5">
          <div class="flex items-center gap-2">
            <HardDrive class="size-4" />
            <Card.Title class="text-sm">HF cache &amp; model verification</Card.Title>
            <span class="text-destructive ml-auto text-xs">{scanError}</span>
          </div>
        </Card.Header>
      </Card.Root>
    {:else if scanSummary}
      <Card.Root class="shrink-0 gap-0 overflow-hidden py-0">
        <Card.Header class="shrink-0 border-b px-4 py-2.5">
          <div class="flex items-center gap-2">
            <HardDrive class="size-4" />
            <Card.Title class="text-sm">HF cache &amp; model verification</Card.Title>
            <span class="text-muted-foreground ml-auto text-xs" title={scanSummary.root}>
              {scanSummary.scannedAtUnix > 0
                ? `scanned ${new Date(scanSummary.scannedAtUnix * 1000).toLocaleTimeString()}`
                : "scan pending"}
            </span>
          </div>
        </Card.Header>
        <Card.Content class="p-0">
          <div class="grid grid-cols-2 divide-x divide-y sm:grid-cols-4 sm:divide-y-0">
            <div class="px-4 py-2.5">
              <p class="text-muted-foreground text-xs uppercase tracking-wide">Total size</p>
              <p class="text-sm font-semibold">{formatCapacity(scanSummary.totalBytes)}</p>
            </div>
            <div class="px-4 py-2.5">
              <p class="text-muted-foreground text-xs uppercase tracking-wide">Files on disk</p>
              <p class="text-sm font-semibold">{scanSummary.totalFiles}</p>
            </div>
            <div class="px-4 py-2.5">
              <p class="text-muted-foreground text-xs uppercase tracking-wide">Repos matched</p>
              <p class="text-sm font-semibold">{scanSummary.matchedRepos}</p>
            </div>
            <div class="px-4 py-2.5">
              <p class="text-muted-foreground text-xs uppercase tracking-wide">Repos unmatched</p>
              <p
                class="text-sm font-semibold"
                class:text-destructive={scanSummary.unmatchedRepos > 0}
              >
                {scanSummary.unmatchedRepos}
              </p>
            </div>
          </div>
          {#if scanSummary.unmatchedRepoNames.length > 0}
            <div class="flex flex-wrap items-center gap-1.5 border-t px-4 py-2.5">
              <span class="text-destructive text-xs">Cached repos with no config reference:</span>
              {#each scanSummary.unmatchedRepoNames as name (name)}
                <Tag class="bg-destructive/15 text-destructive px-1.5 text-[0.625rem]">
                  <span class="inline-block max-w-64 truncate" title={name}>{name}</span>
                </Tag>
              {/each}
            </div>
          {:else}
            <p class="text-muted-foreground border-t px-4 py-2.5 text-xs">
              Every cached repo is referenced by a config entry.
            </p>
          {/if}

          {#if scanSummary.hf}
            <div class="border-t">
              <div class="flex items-center gap-2 px-4 pt-2.5">
                <Globe class="size-4" />
                <p class="text-sm font-semibold">Hugging Face verification</p>
                {#if scanSummary.tagsUpdated.length > 0}
                  <span class="text-muted-foreground ml-auto text-xs">
                    updated {scanSummary.tagsUpdated.length}
                    model{scanSummary.tagsUpdated.length === 1 ? "" : "s"}
                  </span>
                {/if}
              </div>
              <div class="grid grid-cols-3 divide-x divide-y sm:grid-cols-6 sm:divide-y-0">
                <div class="px-4 py-2">
                  <p class="text-muted-foreground text-xs uppercase tracking-wide">Checked</p>
                  <p class="text-sm font-semibold">{scanSummary.hf.checked}</p>
                </div>
                <div class="px-4 py-2">
                  <p class="text-muted-foreground text-xs uppercase tracking-wide">Matched</p>
                  <p class="text-sm font-semibold">{scanSummary.hf.matched}</p>
                </div>
                <div class="px-4 py-2">
                  <p class="text-muted-foreground text-xs uppercase tracking-wide">Unmatched</p>
                  <p
                    class="text-sm font-semibold"
                    class:text-destructive={scanSummary.hf.unmatched > 0}
                  >
                    {scanSummary.hf.unmatched}
                  </p>
                </div>
                <div class="px-4 py-2">
                  <p class="text-muted-foreground text-xs uppercase tracking-wide">Gated</p>
                  <p class="text-sm font-semibold">{scanSummary.hf.unauthorized}</p>
                </div>
                <div class="px-4 py-2">
                  <p class="text-muted-foreground text-xs uppercase tracking-wide">Errors</p>
                  <p class="text-sm font-semibold">{scanSummary.hf.errors}</p>
                </div>
                <div class="px-4 py-2">
                  <p class="text-muted-foreground text-xs uppercase tracking-wide">No HF ref</p>
                  <p class="text-sm font-semibold">{scanSummary.hf.no_ref}</p>
                </div>
              </div>
              <div class="divide-y border-t">
                {#each scanSummary.hf.models as m (m.model_id)}
                  <div class="flex flex-wrap items-center gap-x-2 gap-y-1 px-4 py-2">
                    <span class="min-w-0 truncate text-xs font-medium">{m.model_id}</span>
                    {#if m.repo_id}
                      <span class="text-muted-foreground max-w-48 truncate text-[0.625rem]" title={m.repo_id}>
                        {m.repo_id}
                      </span>
                    {/if}
                    <Tag class={`ml-auto px-1.5 text-[0.625rem] ${hfStatusClass[m.status] ?? ""}`}>
                      {#if m.status === "matched"}
                        <CircleCheck class="size-3" />
                      {:else if m.status === "unmatched"}
                        <CircleX class="size-3" />
                      {:else if m.status === "error"}
                        <TriangleAlert class="size-3" />
                      {/if}
                      {hfStatusLabels[m.status]}
                    </Tag>
                    {#each foundCapabilities(m.capabilities) as key (key)}
                      <Tag class={`px-1.5 text-[0.625rem] ${hfCapabilityBadgeClass[key] ?? ""}`}>
                        <span title={m.evidence?.find((e) => e.startsWith(key))}>
                          {hfCapabilityLabels[key] ?? key}
                        </span>
                      </Tag>
                    {/each}
                    {#if m.reason || m.error}
                      <span class="text-muted-foreground w-full truncate text-[0.625rem]" title={m.reason ?? m.error}>
                        {m.reason ?? m.error}
                      </span>
                    {/if}
                  </div>
                {/each}
              </div>
            </div>
          {/if}
        </Card.Content>
      </Card.Root>
    {/if}

    {#if $profiles.length > 0}
      <Card.Root class="shrink-0 gap-0 overflow-hidden py-0">
        <Card.Header class="shrink-0 border-b px-4 py-2.5">
          <div class="flex items-center gap-2">
            <Card.Title class="text-sm">Profiles</Card.Title>
            {#if selectedProfile}
              <Tag>{$activeProfile}</Tag>
              <Tag class="bg-success/15 text-success">Active</Tag>
            {/if}
            <span class="text-muted-foreground ml-auto text-xs">
              {profileMappings.length} {profileMappings.length === 1 ? "mapping" : "mappings"}
            </span>
          </div>
          {#if selectedProfile?.description}
            <p class="text-muted-foreground text-xs">{selectedProfile.description}</p>
          {/if}
        </Card.Header>
        <Card.Content class="p-0">
          {#if !selectedProfile}
            <div class="text-muted-foreground px-4 py-6 text-center text-sm">
              No active profile
            </div>
          {:else}
            <div class="divide-y">
              {#each profileMappings as [modelID, target] (modelID)}
                <div class="hover:bg-muted/50 flex items-center gap-2 px-4 py-2.5">
                  <span class="max-w-[45%] truncate text-sm font-medium">{modelID}</span>
                  <span class="text-muted-foreground text-xs" aria-hidden="true">→</span>
                  {#if target}
                    <span class="min-w-0 truncate text-sm">{target}</span>
                  {:else}
                    <Tag class="px-1.5 text-[0.625rem] uppercase">disabled</Tag>
                  {/if}
                </div>
              {/each}
            </div>
          {/if}
        </Card.Content>
      </Card.Root>
    {/if}

    {#if $selectorModels.length > 0}
      <Card.Root class="shrink-0 gap-0 overflow-hidden py-0">
        <Card.Header class="shrink-0 border-b px-4 py-2.5">
          <div class="flex items-center gap-2">
            <Card.Title class="text-sm">Selectors</Card.Title>
            <span class="text-muted-foreground ml-auto text-xs">
              {$selectorModels.length} {$selectorModels.length === 1 ? "selector" : "selectors"}
            </span>
          </div>
        </Card.Header>
        <Card.Content class="p-0">
          <div class="divide-y">
            {#each $selectorModels as selector (selector.id)}
              <div class="hover:bg-muted/50 flex items-center gap-2 px-4 py-2.5">
                <div class="min-w-0 flex-1">
                  <div class="truncate text-sm font-medium">
                    {selector.name ? `${selector.id} - ${selector.name}` : selector.id}
                  </div>
                  {#if selector.description}
                    <div class="text-muted-foreground truncate text-xs">
                      {selector.description}
                    </div>
                  {/if}
                  <div class="text-muted-foreground flex flex-wrap items-center gap-x-1 text-xs">
                    <span>targets:</span>
                    {#each selector.targets ?? [] as target, i (target)}
                      {#if i > 0}<span>,</span>{/if}
                      <a
                        href="/models/{encodeURIComponent(target)}"
                        use:link
                        class="hover:text-foreground hover:underline"
                      >{target}</a>
                    {/each}
                  </div>
                </div>
                {#if selector.strategy === "spillover" && selector.spillover}
                  <Tag>spillover {selector.spillover}</Tag>
                {/if}
                <Tag class="px-1.5 text-[0.625rem] uppercase">{selector.strategy}</Tag>
              </div>
            {/each}
          </div>
        </Card.Content>
      </Card.Root>
    {/if}

    {@render modelSection("Local models", localModels, unlistedToggle)}
    {@render modelSection("Peer models", peerModels)}
  </div>
</div>
