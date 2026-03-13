<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import { getContainers } from '$lib/api';
  import type { Container } from '$lib/types';
  import Skeleton from '$lib/components/Skeleton.svelte';
  import ErrorState from '$lib/components/ErrorState.svelte';
  import ActionButton from '$lib/components/ActionButton.svelte';
  import Sparkline from '$lib/components/Sparkline.svelte';
  import { stats } from '$lib/stores/stats';
  import type { ContainerHistory } from '$lib/stores/stats';

  let containers: Container[] = [];
  let loading = true;
  let error: string | null = null;
  let filter = '';
  let intervalId: number;
  let historyData: Record<string, ContainerHistory> = {};
  const unsubHistory = stats.history.subscribe(h => { historyData = h; });
  onDestroy(() => unsubHistory());

  async function load() {
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

  $: running = containers.filter(c => c.state === 'running').length;
  $: stopped = containers.filter(c => c.state === 'exited').length;

  function statusClass(state: string) {
    if (!state) return 'status-other';
    if (state.toLowerCase() === 'running') return 'status-running';
    if (state.toLowerCase() === 'exited') return 'status-exited';
    return 'status-other';
  }

  function formatBytes(n: number) {
    if (n < 1024) return n + ' B';
    if (n < 1024 * 1024) return (n / 1024).toFixed(1) + ' KB';
    if (n < 1024 * 1024 * 1024) return (n / (1024 * 1024)).toFixed(1) + ' MB';
    return (n / (1024 * 1024 * 1024)).toFixed(1) + ' GB';
  }
</script>

<div class="page-header">
  <div>
    <h1>Containers</h1>
    <p class="subtitle">Monitor and manage your Docker containers</p>
  </div>
  <div class="search-wrap">
    <svg class="search-icon" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="11" cy="11" r="8"/><line x1="21" y1="21" x2="16.65" y2="16.65"/></svg>
    <input class="search-input" placeholder="Search containers..." bind:value={filter} />
  </div>
</div>

{#if !loading}
<div class="stat-cards">
  <div class="stat-card">
    <div class="stat-value">{containers.length}</div>
    <div class="stat-label">Total</div>
  </div>
  <div class="stat-card stat-running">
    <div class="stat-value">{running}</div>
    <div class="stat-label">Running</div>
  </div>
  <div class="stat-card stat-stopped">
    <div class="stat-value">{stopped}</div>
    <div class="stat-label">Stopped</div>
  </div>
  <div class="stat-card stat-other">
    <div class="stat-value">{containers.length - running - stopped}</div>
    <div class="stat-label">Other</div>
  </div>
</div>
{/if}

{#if loading}
  <div class="card">
    <Skeleton variant="line" height="20px" />
    <div style="height:12px"></div>
    <Skeleton variant="rect" height="200px" />
  </div>
{:else if error}
  <div class="card">
    <ErrorState message={error} on:retry={load} />
  </div>
{:else}
  <div class="card table-card">
    <table class="containers-table">
      <thead>
        <tr>
          <th style="width:36px"></th>
          <th>Name</th>
          <th>Image</th>
          <th>Status</th>
          <th>CPU</th>
          <th>Memory</th>
          <th class="hide-md">Network</th>
          <th class="hide-md">Ports</th>
          <th style="width:1%"></th>
        </tr>
      </thead>
      <tbody>
        {#each filtered as c}
          <tr>
            <td><span class="status-dot {statusClass(c.state)}"></span></td>
            <td class="name-cell"><a href={`/container/${c.id}`}>{c.name}</a></td>
            <td class="image-cell" title={c.image}>{c.image.length > 35 ? c.image.slice(0, 32) + '...' : c.image}</td>
            <td><span class="status-badge {statusClass(c.state)}-badge">{c.status}</span></td>
            <td>
              {#if $stats[c.id]}
                <div class="metric-cell">
                  <span class="metric-val">{Math.round($stats[c.id].cpu_percent)}%</span>
                  {#if historyData[c.id]?.cpu?.length > 1}
                    <Sparkline data={historyData[c.id].cpu} color="#3b82f6" />
                  {/if}
                </div>
              {:else}<span class="text-muted">-</span>{/if}
            </td>
            <td>
              {#if $stats[c.id]}
                <div class="metric-cell">
                  <div>
                    <span class="metric-val">{formatBytes($stats[c.id].memory_usage)}</span>
                    <span class="metric-limit">/ {formatBytes($stats[c.id].memory_limit)}</span>
                  </div>
                  {#if historyData[c.id]?.mem?.length > 1}
                    <Sparkline data={historyData[c.id].mem} color="#10b981" />
                  {/if}
                </div>
              {:else}<span class="text-muted">-</span>{/if}
            </td>
            <td class="hide-md">
              {#if $stats[c.id]}
                <span class="net-label">
                  <span class="net-arrow up"></span>{formatBytes($stats[c.id].network_rx_bytes)}
                  <span class="net-arrow down"></span>{formatBytes($stats[c.id].network_tx_bytes)}
                </span>
              {:else}<span class="text-muted">-</span>{/if}
            </td>
            <td class="hide-md">
              {#if c.ports && c.ports.length}
                <span class="port-badges">{c.ports.join(', ')}</span>
              {:else}<span class="text-muted">-</span>{/if}
            </td>
            <td>
              <div class="action-group">
                {#if c.state === 'running'}
                  <ActionButton action="stop" containerId={c.id} containerName={c.name} on:refresh={load} />
                  <ActionButton action="restart" containerId={c.id} containerName={c.name} on:refresh={load} />
                {:else}
                  <ActionButton action="start" containerId={c.id} containerName={c.name} on:refresh={load} />
                  <ActionButton action="remove" containerId={c.id} containerName={c.name} on:refresh={load} />
                {/if}
              </div>
            </td>
          </tr>
        {/each}
      </tbody>
    </table>
    {#if filtered.length === 0}
      <div class="empty-state">No containers match your search.</div>
    {/if}
  </div>
{/if}

<style>
  .page-header {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: 16px;
    margin-bottom: 24px;
  }
  .page-header h1 { margin: 0; }
  .subtitle {
    color: var(--text-muted);
    font-size: 13px;
    margin-top: 4px;
  }
  .search-wrap {
    position: relative;
  }
  .search-icon {
    position: absolute;
    left: 12px;
    top: 50%;
    transform: translateY(-50%);
    color: var(--text-muted);
    pointer-events: none;
  }
  .search-input {
    padding-left: 34px;
    width: 260px;
  }

  /* Stat cards */
  .stat-cards {
    display: grid;
    grid-template-columns: repeat(4, 1fr);
    gap: 12px;
    margin-bottom: 20px;
  }
  .stat-card {
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: var(--radius);
    padding: 16px 20px;
  }
  .stat-value {
    font-size: 28px;
    font-weight: 700;
    letter-spacing: -0.02em;
    line-height: 1.1;
  }
  .stat-label {
    font-size: 12px;
    color: var(--text-muted);
    margin-top: 4px;
    text-transform: uppercase;
    letter-spacing: 0.05em;
    font-weight: 500;
  }
  .stat-running .stat-value { color: var(--success); }
  .stat-running { border-color: rgba(16, 185, 129, 0.2); background: linear-gradient(135deg, var(--surface), rgba(16, 185, 129, 0.03)); }
  .stat-stopped .stat-value { color: var(--danger); }
  .stat-stopped { border-color: rgba(239, 68, 68, 0.15); }
  .stat-other .stat-value { color: var(--warning); }

  /* Table */
  .table-card { padding: 0; overflow: hidden; }
  .containers-table { margin: 0; }
  .containers-table th { padding: 14px 16px; background: rgba(255,255,255,0.01); }
  .containers-table td { padding: 14px 16px; }

  .name-cell a {
    color: var(--text);
    font-weight: 500;
  }
  .name-cell a:hover {
    color: var(--accent);
  }
  .image-cell {
    font-family: 'SF Mono', 'Fira Code', 'Cascadia Code', monospace;
    font-size: 12px;
    color: var(--text-muted);
  }
  .status-badge {
    font-size: 12px;
    padding: 3px 10px;
    border-radius: 999px;
    font-weight: 500;
    white-space: nowrap;
  }
  .status-running-badge { background: var(--success-soft); color: var(--success); }
  .status-exited-badge { background: var(--danger-soft); color: var(--danger); }
  .status-other-badge { background: var(--warning-soft); color: var(--warning); }

  /* Metrics */
  .metric-cell { display: flex; align-items: center; gap: 8px; }
  .metric-val { font-size: 13px; font-weight: 500; white-space: nowrap; color: var(--text); font-variant-numeric: tabular-nums; }
  .metric-limit { color: var(--text-muted); font-size: 11px; }

  /* Network */
  .net-label { font-size: 12px; color: var(--text-secondary); white-space: nowrap; display: flex; align-items: center; gap: 4px; }
  .net-arrow { display: inline-block; width: 0; height: 0; border-left: 3px solid transparent; border-right: 3px solid transparent; }
  .net-arrow.up { border-bottom: 5px solid var(--success); }
  .net-arrow.down { border-top: 5px solid var(--accent); }

  .port-badges { font-size: 12px; color: var(--text-secondary); }
  .text-muted { color: var(--text-muted); }
  .action-group { display: flex; gap: 4px; white-space: nowrap; }

  .empty-state {
    text-align: center;
    padding: 40px 20px;
    color: var(--text-muted);
    font-size: 13px;
  }

  /* Responsive */
  .hide-md { }
  @media (max-width: 1024px) {
    .hide-md { display: none; }
    .stat-cards { grid-template-columns: repeat(2, 1fr); }
  }
  @media (max-width: 768px) {
    .page-header { flex-direction: column; }
    .search-input { width: 100%; }
    .stat-cards { grid-template-columns: repeat(2, 1fr); }
    .containers-table, .containers-table thead, .containers-table tbody, .containers-table th, .containers-table td, .containers-table tr { display: block; width: 100%; }
    .containers-table thead { display: none; }
    .containers-table tr { padding: 14px 16px; border-bottom: 1px solid var(--border); }
    .containers-table td { display: flex; justify-content: space-between; padding: 4px 0; border: none; }
  }
</style>
