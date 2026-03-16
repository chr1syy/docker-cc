<script lang="ts">
  import { onMount } from 'svelte';
  import LogViewer from '$lib/components/LogViewer.svelte';
  import { getContainers } from '$lib/api';
  let containers: import('$lib/types').Container[] = [];
  let selected = '';
  onMount(async ()=>{ containers = await getContainers(); if (containers[0]) selected = containers[0].id; })
</script>

<div class="card">
  <h2>Logs</h2>
  <div style="display:flex;gap:8px;align-items:center;margin-bottom:8px">
    <select bind:value={selected}>
      {#each containers as c}
        <option value={c.id}>{c.name} — {c.image}</option>
      {/each}
    </select>
  </div>
  {#if selected}
    <LogViewer containerId={selected} />
  {:else}
    <div class="card">Select a container to view logs</div>
  {/if}
  
</div>
