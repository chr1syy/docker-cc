<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import { page } from '$app/stores';
  import { getContainer } from '$lib/api';
  import type { ContainerDetail } from '$lib/types';
  import { stats } from '$lib/stores/stats';
  import type { ContainerHistory } from '$lib/stores/stats';
  import MetricChart from '$lib/components/MetricChart.svelte';
  import LogViewer from '$lib/components/LogViewer.svelte';
  import ActionButton from '$lib/components/ActionButton.svelte';
  import Skeleton from '$lib/components/Skeleton.svelte';
  import ErrorState from '$lib/components/ErrorState.svelte';

  let id: string;
  let detail: ContainerDetail | null = null;
  let loading = true;
  let error: string | null = null;
  let showEnv = false;

  let hist: ContainerHistory | undefined;
  const unsubHistory = stats.history.subscribe(h => { hist = h[id]; });
  onDestroy(() => unsubHistory());

  $: id = $page.params.id;

  async function reload() {
    try { detail = await getContainer(id); error = null; } catch(e){ error = e instanceof Error ? e.message : String(e); }
  }

  onMount(async () => {
    loading = true;
    try {
      detail = await getContainer(id);
    } catch (e) {
      error = e instanceof Error ? e.message : String(e);
    } finally {
      loading = false;
    }
  });
</script>

<a href="/" class="back-link">
  <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="15 18 9 12 15 6"/></svg>
  Back to Dashboard
</a>

{#if loading}
  <div class="card" style="margin-top:16px">
    <Skeleton variant="line" height="24px" />
    <div style="height:12px"></div>
    <Skeleton variant="rect" height="120px" />
  </div>
{:else if error}
  <div class="card" style="margin-top:16px">
    <ErrorState message={error} on:retry={async () => { loading = true; await reload(); loading = false; }} />
  </div>
{:else if detail}

  <!-- Header -->
  <div class="detail-header">
    <div class="detail-title-row">
      <span class="status-dot {detail.state==='running'?'status-running':detail.state==='exited'?'status-exited':'status-other'}"></span>
      <h1>{detail.name}</h1>
      <span class="state-badge" class:running={detail.state==='running'} class:exited={detail.state==='exited'}>{detail.state}</span>
    </div>
    <div class="detail-meta">
      <span class="meta-image">{detail.image}</span>
      <span class="meta-sep"></span>
      <span>{detail.status}</span>
    </div>
    <div class="detail-actions">
      {#if detail.state === 'running'}
        <ActionButton action="stop" containerId={id} containerName={detail.name} on:refresh={reload} />
        <ActionButton action="restart" containerId={id} containerName={detail.name} on:refresh={reload} />
      {:else}
        <ActionButton action="start" containerId={id} containerName={detail.name} on:refresh={reload} />
      {/if}
    </div>
  </div>

  <!-- Info grid -->
  <div class="card section">
    <h3>Container Info</h3>
    <div class="info-grid">
      <div class="info-item">
        <span class="info-key">Created</span>
        <span class="info-val">{detail.created}</span>
      </div>
      <div class="info-item">
        <span class="info-key">Restarts</span>
        <span class="info-val">{detail.restartCount ?? '-'}</span>
      </div>
      <div class="info-item">
        <span class="info-key">Platform</span>
        <span class="info-val">{detail.config?.Platform ?? '-'}</span>
      </div>
      <div class="info-item">
        <span class="info-key">Command</span>
        <span class="info-val mono">{detail.config?.Cmd ? detail.config.Cmd.join(' ') : '-'}</span>
      </div>
    </div>
  </div>

  <!-- Live Metrics -->
  <div class="card section">
    <h3>Live Metrics</h3>
    <div class="metrics-grid">
      <MetricChart data={hist?.cpu ?? []} label="CPU" unit="%" color="#3b82f6" />
      <MetricChart data={hist?.mem ?? []} label="Memory" unit="%" color="#10b981" />
      <MetricChart data={hist?.netRx ?? []} label="Network RX" unit="bytes" color="#f59e0b" />
      <MetricChart data={hist?.netTx ?? []} label="Network TX" unit="bytes" color="#f97316" />
      <MetricChart data={hist?.blkRead ?? []} label="Disk Read" unit="bytes" color="#a855f7" />
      <MetricChart data={hist?.blkWrite ?? []} label="Disk Write" unit="bytes" color="#ec4899" />
    </div>
  </div>

  <!-- Network -->
  <div class="card section">
    <h3>Network</h3>
    <table>
      <thead><tr><th>Network</th><th>IP Address</th><th>Gateway</th><th>Ports</th></tr></thead>
      <tbody>
        {#each Object.entries(detail.networkSettings?.Networks ?? {}) as [name, net]}
          <tr>
            <td>{name}</td>
            <td class="mono">{net?.IPAddress ?? '-'}</td>
            <td class="mono">{net?.Gateway ?? '-'}</td>
            <td>{detail.ports ? detail.ports.join(', ') : '-'}</td>
          </tr>
        {/each}
      </tbody>
    </table>
  </div>

  <!-- Mounts -->
  {#if detail.mounts && detail.mounts.length}
  <div class="card section">
    <h3>Mounts</h3>
    <table>
      <thead><tr><th>Source</th><th>Destination</th><th>Mode</th></tr></thead>
      <tbody>
        {#each detail.mounts as m}
          <tr>
            <td class="mono">{m.Source}</td>
            <td class="mono">{m.Destination}</td>
            <td>{m.Mode || 'rw'}</td>
          </tr>
        {/each}
      </tbody>
    </table>
  </div>
  {/if}

  <!-- Logs -->
  <div id="logs" class="card section">
    <h3>Logs</h3>
    <LogViewer containerId={id} />
  </div>

  <!-- Labels -->
  {#if Object.keys(detail.config?.Labels ?? {}).length}
  <div class="card section">
    <h3>Labels</h3>
    <table>
      <thead><tr><th>Key</th><th>Value</th></tr></thead>
      <tbody>
        {#each Object.entries(detail.config?.Labels ?? {}) as [k, v]}
          <tr>
            <td class="mono">{k}</td>
            <td>{v}</td>
          </tr>
        {/each}
      </tbody>
    </table>
  </div>
  {/if}

  <!-- Environment -->
  <div class="card section">
    <div class="section-header">
      <h3 style="margin-bottom:0">Environment</h3>
      <button class="toggle-btn" on:click={() => showEnv = !showEnv}>
        {showEnv ? 'Hide values' : 'Reveal values'}
      </button>
    </div>
    <table>
      <thead><tr><th>Variable</th><th>Value</th></tr></thead>
      <tbody>
        {#each (detail.config?.Env ?? []) as e}
          {#if e}
            <tr>
              <td class="mono">{e.split('=')[0]}</td>
              <td>{showEnv ? e.split('=').slice(1).join('=') : '•••••••'}</td>
            </tr>
          {/if}
        {/each}
      </tbody>
    </table>
  </div>

{/if}

<style>
  .back-link {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    color: var(--text-secondary);
    font-size: 13px;
    font-weight: 500;
    transition: color var(--transition);
  }
  .back-link:hover { color: var(--text); text-decoration: none; }

  .detail-header {
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: var(--radius);
    padding: 24px;
    margin-top: 16px;
    margin-bottom: 16px;
  }
  .detail-title-row {
    display: flex;
    align-items: center;
    gap: 12px;
  }
  .detail-title-row h1 { margin: 0; font-size: 1.35rem; }
  .state-badge {
    font-size: 11px;
    padding: 3px 10px;
    border-radius: 999px;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.04em;
    background: var(--warning-soft);
    color: var(--warning);
  }
  .state-badge.running { background: var(--success-soft); color: var(--success); }
  .state-badge.exited { background: var(--danger-soft); color: var(--danger); }
  .detail-meta {
    display: flex;
    align-items: center;
    gap: 10px;
    margin-top: 8px;
    font-size: 13px;
    color: var(--text-secondary);
  }
  .meta-image {
    font-family: 'SF Mono', 'Fira Code', monospace;
    color: var(--text-muted);
    font-size: 12px;
  }
  .meta-sep {
    width: 3px;
    height: 3px;
    border-radius: 50%;
    background: var(--text-muted);
  }
  .detail-actions {
    display: flex;
    gap: 6px;
    margin-top: 16px;
  }

  .section { margin-bottom: 16px; }

  .info-grid {
    display: grid;
    grid-template-columns: repeat(2, 1fr);
    gap: 16px;
  }
  .info-item { display: flex; flex-direction: column; gap: 2px; }
  .info-key { font-size: 11px; color: var(--text-muted); text-transform: uppercase; letter-spacing: 0.05em; font-weight: 500; }
  .info-val { font-size: 14px; color: var(--text); }

  .metrics-grid {
    display: grid;
    grid-template-columns: repeat(2, 1fr);
    gap: 16px;
  }

  .mono {
    font-family: 'SF Mono', 'Fira Code', 'Cascadia Code', monospace;
    font-size: 12px;
  }

  .section-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    margin-bottom: 12px;
  }
  .toggle-btn {
    background: transparent;
    border: 1px solid var(--border);
    color: var(--text-secondary);
    padding: 5px 12px;
    border-radius: var(--radius-sm);
    font-size: 12px;
  }
  .toggle-btn:hover {
    background: rgba(255,255,255,0.04);
    color: var(--text);
    border-color: var(--text-muted);
  }

  @media (max-width: 768px) {
    .info-grid { grid-template-columns: 1fr; }
    .metrics-grid { grid-template-columns: 1fr; }
  }
</style>
