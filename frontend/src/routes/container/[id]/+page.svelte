<script lang="ts">
  import { onMount } from 'svelte';
  import { page } from '$app/stores';
  import { getContainer } from '$lib/api';
  import type { ContainerDetail } from '$lib/types';

  let id: string;
  let detail: ContainerDetail | null = null;
  let loading = true;
  let error: string | null = null;
  let showEnv = false;
  import { stats } from '$lib/stores/stats';
  import MetricChart from '$lib/components/MetricChart.svelte';
  import LogViewer from '$lib/components/LogViewer.svelte';
  import ActionButton from '$lib/components/ActionButton.svelte';
  import Skeleton from '$lib/components/Skeleton.svelte';
  import ErrorState from '$lib/components/ErrorState.svelte';

  // per-container rolling buffers
  const cpuBuffer: {ts:number,value:number}[] = [];
  const memBuffer: {ts:number,value:number}[] = [];
  const netRxBuffer: {ts:number,value:number}[] = [];
  const netTxBuffer: {ts:number,value:number}[] = [];
  const blkReadBuffer: {ts:number,value:number}[] = [];
  const blkWriteBuffer: {ts:number,value:number}[] = [];

  $: if ($stats && id) {
    const m = $stats[id];
    if (m) {
      const ts = new Date(m.timestamp).getTime();
      cpuBuffer.push({ts,value: Math.round(m.cpu_percent)});
      memBuffer.push({ts,value: Math.round(m.memory_percent)});
      netRxBuffer.push({ts,value: m.network_rx_bytes});
      netTxBuffer.push({ts,value: m.network_tx_bytes});
      blkReadBuffer.push({ts,value: m.block_read_bytes});
      blkWriteBuffer.push({ts,value: m.block_write_bytes});
      // trim to 60
      if (cpuBuffer.length>60) cpuBuffer.shift();
      if (memBuffer.length>60) memBuffer.shift();
      if (netRxBuffer.length>60) netRxBuffer.shift();
      if (netTxBuffer.length>60) netTxBuffer.shift();
      if (blkReadBuffer.length>60) blkReadBuffer.shift();
      if (blkWriteBuffer.length>60) blkWriteBuffer.shift();
    }
  }

  $: id = $page.params.id;

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

<a href="/">← Back to Dashboard</a>

{#if loading}
  <div class="card"><Skeleton variant="line" height="20px" /><div style="height:8px"></div><Skeleton variant="rect" height="80px" /></div>
{:else if error}
  <div class="card"><ErrorState message={error} on:retry={async () => { loading = true; try { detail = await getContainer(id); error = null; } catch(e){ error = e instanceof Error ? e.message : String(e);} finally { loading=false } }} /></div>
{:else if detail}
  <div class="card">
    <h2>{detail.name} <span class="status-dot {detail.state==='running'?'status-running':detail.state==='exited'?'status-exited':'status-other'}"></span></h2>
    <div style="display:flex;gap:12px;align-items:center">
      <div>{detail.image} • {detail.status}</div>
      <div style="margin-left:auto">
        {#if detail.state === 'running'}
          <ActionButton action="stop" containerId={id} containerName={detail.name} on:refresh={async () => { detail = await getContainer(id); }} />
          <ActionButton action="restart" containerId={id} containerName={detail.name} on:refresh={async () => { detail = await getContainer(id); }} />
        {:else}
          <ActionButton action="start" containerId={id} containerName={detail.name} on:refresh={async () => { detail = await getContainer(id); }} />
        {/if}
      </div>
    </div>
  </div>

  <div class="card" style="margin-top:12px">
    <h3>Info</h3>
    <div style="display:grid;grid-template-columns:repeat(2,1fr);gap:8px">
      <div>Created: {detail.created}</div>
      <div>Restarts: {detail.restartCount ?? '-'}</div>
      <div>Platform: {detail.config?.Platform ?? '-'}</div>
      <div>Command: {detail.config?.Cmd ? detail.config.Cmd.join(' ') : '-'}</div>
    </div>
  </div>

  <div class="card" style="margin-top:12px">
    <h3>Network</h3>
    <table style="width:100%">
      <thead><tr><th>Network</th><th>IP</th><th>Gateway</th><th>Ports</th></tr></thead>
      <tbody>
        {#each Object.entries(detail.networkSettings?.Networks ?? {}) as [name, net]}
          <tr>
            <td>{name}</td>
            <td>{net?.IPAddress ?? '-'}</td>
            <td>{net?.Gateway ?? '-'}</td>
            <td>{detail.ports? JSON.stringify(detail.ports) : '-'}</td>
          </tr>
        {/each}
      </tbody>
    </table>
  </div>

  <div class="card" style="margin-top:12px">
    <h3>Live Metrics</h3>
    <div class="metrics-grid" style="display:grid;grid-template-columns:repeat(2,1fr);gap:12px">
      <MetricChart data={cpuBuffer} />
      <MetricChart data={memBuffer} />
      <MetricChart data={netRxBuffer} />
      <MetricChart data={blkReadBuffer} />
    </div>
  </div>
  <div class="card" style="margin-top:12px">
    <h3>Mounts</h3>
    <table style="width:100%">
      <thead><tr><th>Source</th><th>Destination</th><th>Mode</th></tr></thead>
      <tbody>
        {#each detail.mounts ?? [] as m}
          <tr><td>{m.Source}</td><td>{m.Destination}</td><td>{m.Mode}</td></tr>
        {/each}
      </tbody>
    </table>
  </div>

  <div id="logs" class="card" style="margin-top:12px">
    <h3>Logs</h3>
    <LogViewer containerId={id} />
  </div>

  <div class="card" style="margin-top:12px">
    <h3>Labels</h3>
    <table style="width:100%">
      <tbody>
        {#each Object.entries(detail.config?.Labels ?? {}) as [k,v]}
          <tr><td>{k}</td><td>{v}</td></tr>
        {/each}
      </tbody>
    </table>
  </div>

  <div class="card" style="margin-top:12px">
    <h3>Environment</h3>
    <button on:click={() => showEnv = !showEnv}>{showEnv? 'Hide' : 'Reveal' } values</button>
    <table style="width:100%;margin-top:8px">
      <thead><tr><th>Variable</th><th>Value</th></tr></thead>
      <tbody>
        {#each (detail.config?.Env ?? []) as e}
          {#if e}
            <tr>
              <td>{e.split('=')[0]}</td>
              <td>{showEnv ? e.split('=').slice(1).join('=') : '•••••' }</td>
            </tr>
          {/if}
        {/each}
      </tbody>
    </table>
  </div>

{/if}
