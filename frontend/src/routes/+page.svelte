<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import { getContainers } from '$lib/api';
  import type { Container } from '$lib/types';
  import Skeleton from '$lib/components/Skeleton.svelte';
  import ErrorState from '$lib/components/ErrorState.svelte';
  import ActionButton from '$lib/components/ActionButton.svelte';

  let containers: Container[] = [];
  let loading = true;
  let error: string | null = null;
  let filter = '';
  let intervalId: number;
  import { stats } from '$lib/stores/stats';

  async function load() {
    loading = true;
    error = null;
    try {
      containers = await getContainers();
    } catch (e) {
      error = e instanceof Error ? e.message : String(e);
    } finally {
      loading = false;
    }
  }

  onMount(() => {
    load();
    intervalId = setInterval(load, 5000) as unknown as number;
  });
  onDestroy(() => clearInterval(intervalId));

  $: filtered = containers.filter(c => {
    const q = filter.toLowerCase();
    return !q || c.name.toLowerCase().includes(q) || c.image.toLowerCase().includes(q);
  });

  function statusClass(state: string) {
    if (!state) return 'status-other';
    if (state.toLowerCase() === 'running') return 'status-running';
    if (state.toLowerCase() === 'exited') return 'status-exited';
    return 'status-other';
  }

  function formatBytes(n: number) {
    if (n < 1024) return n + 'B';
    if (n < 1024 * 1024) return (n / 1024).toFixed(1) + 'KB';
    if (n < 1024 * 1024 * 1024) return (n / (1024 * 1024)).toFixed(1) + 'MB';
    return (n / (1024 * 1024 * 1024)).toFixed(1) + 'GB';
  }
</script>

<div class="header-bar">
  <div>
    <h1>Containers</h1>
    <div class="card">{containers.filter(c=>c.state==='running').length} running, {containers.filter(c=>c.state==='exited').length} stopped, {containers.length} total</div>
  </div>
  <div>
    <input placeholder="Search by name or image" bind:value={filter} />
  </div>
</div>

 {#if loading}
   <div class="card">
     <Skeleton variant="line" height="24px" />
     <div style="height:8px"></div>
     <Skeleton variant="line" height="16px" />
   </div>
{:else if error}
  <div class="card">
    <ErrorState message={error} on:retry={load} />
  </div>
{:else}
  <div class="card table-card">
    <table class="containers-table" style="width:100%;border-collapse:collapse">
      <thead>
        <tr style="text-align:left;border-bottom:1px solid var(--border)">
          <th>Status</th>
          <th>Name</th>
          <th>Image</th>
          <th>Status Text</th>
      <th>CPU</th>
      <th>Memory</th>
      <th>Net</th>
      <th>Ports</th>
      </tr>
      </thead>
  <tbody>
        {#each filtered as c}
          <tr style="border-bottom:1px solid var(--border)">
            <td><span class="status-dot {statusClass(c.state)}"></span></td>
            <td><a href={`/container/${c.id}`}>{c.name}</a></td>
            <td>{c.image.length>40?c.image.slice(0,37)+'...':c.image}</td>
            <td>{c.status}</td>
            <td>
              {#if $stats[c.id]}
                <div class="cpu-percent">{Math.round($stats[c.id].cpu_percent)}%<span class="cpu-bar" style={`--p:${Math.min(100,Math.round($stats[c.id].cpu_percent))}%`}></span></div>
              {:else}-{/if}
            </td>
            <td>
              {#if $stats[c.id]}
                {formatBytes($stats[c.id].memory_usage)} / {formatBytes($stats[c.id].memory_limit)} ({Math.round($stats[c.id].memory_percent)}%)
              {:else}-{/if}
            </td>
            <td>
              {#if $stats[c.id]}
                {formatBytes($stats[c.id].network_rx_bytes)}/{formatBytes($stats[c.id].network_tx_bytes)}
              {:else}-{/if}
            </td>
            <td>{#if c.ports}{c.ports.map(p=> (p.publicPort? `${p.publicPort}:${p.privatePort}/${p.type}` : `${p.privatePort}/${p.type}`)).join(', ')}{:else}-{/if}</td>
            <td><a title="Logs" href={`/container/${c.id}#logs`}>📝</a></td>
            <td>
              <!-- Actions dropdown simplified to inline buttons -->
              {#if c.state === 'running'}
                <ActionButton action="stop" containerId={c.id} containerName={c.name} on:refresh={load} />
                <ActionButton action="restart" containerId={c.id} containerName={c.name} on:refresh={load} />
              {:else}
                <ActionButton action="start" containerId={c.id} containerName={c.name} on:refresh={load} />
              {/if}
            </td>
          </tr>
        {/each}
      </tbody>
    </table>
  </div>
{/if}

<style>
.cpu-bar{display:inline-block;height:8px;width:80px;background:linear-gradient(90deg,green,yellow,red);margin-left:8px;vertical-align:middle}
.cpu-percent{font-size:12px}

/* Responsive table -> card on small screens */
@media (max-width: 768px) {
  .containers-table, .containers-table thead, .containers-table tbody, .containers-table th, .containers-table td, .containers-table tr { display:block; width:100%; }
  .containers-table thead { display:none; }
  .containers-table tr { margin-bottom:12px; padding:12px; border-radius:8px; background:linear-gradient(180deg, rgba(255,255,255,0.02), transparent); }
  .containers-table td { display:flex; justify-content:space-between; padding:6px 0; border:none; }
  .containers-table td a { color:var(--text) }
  .containers-table .status-dot { margin-right:6px }
  .cpu-bar{ width:60px }
}

/* Hide less-critical columns on medium screens */
@media (max-width: 1024px) and (min-width: 769px) {
  /* hide Net (7) and Ports (8) - zero-based index: 6 and 7 in td/selectors are 1-based here */
  .containers-table th:nth-child(7), .containers-table td:nth-child(7),
  .containers-table th:nth-child(8), .containers-table td:nth-child(8) { display:none; }
}
</style>

