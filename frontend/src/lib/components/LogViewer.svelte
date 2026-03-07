<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import { writable } from 'svelte/store';
  import { fetchAPI } from '$lib/api';

  export let containerId: string;

  // Controls
  const ranges = [ ['15m', 15*60], ['1h', 60*60], ['6h', 6*60*60], ['24h', 24*60*60], ['custom', 0] ] as const;
  let selectedRange: string = '15m';
  let customSince: string = '';
  let customUntil: string = '';
  let filterText: string = '';
  let stderrOnly = false;
  let tail = 500;
  let live = false;
  let showTimestamps = true;

  // Log storage
  type LogLine = { timestamp?: string; stream?: string; message: string };
  const lines = writable<LogLine[]>([]);

  let ws: WebSocket | null = null;
  let containerEl: HTMLDivElement | null = null;
  const lineHeight = 20; // px
  let viewHeight = 400;
  let scrollTop = 0;

  // Virtual list derived values
  $: currentLines = $lines;
  $: total = currentLines.length;
  $: startIndex = Math.max(0, Math.floor(scrollTop / lineHeight) - 5);
  $: visibleCount = Math.ceil(viewHeight / lineHeight) + 10;
  $: endIndex = Math.min(total, startIndex + visibleCount);

  function computeSince() {
    if (selectedRange === 'custom') {
      return customSince ? new Date(customSince).toISOString() : undefined;
    }
    const r = ranges.find(r => r[0] === selectedRange);
    if (!r) return undefined;
    const s = new Date(Date.now() - r[1]*1000);
    return s.toISOString();
  }

  async function loadOnce() {
    const params: Record<string,string> = {};
    params.tail = String(tail);
    if (filterText) params.filter = filterText;
    if (stderrOnly) params.stream = 'stderr';
    const since = computeSince();
    if (since) params.since = since;
    if (selectedRange === 'custom' && customUntil) params.until = new Date(customUntil).toISOString();

    const qs = new URLSearchParams(params).toString();
    try {
      const res = await fetchAPI(`/api/containers/${encodeURIComponent(containerId)}/logs?${qs}`);
      // Expect array of {timestamp,stream,message}
      if (Array.isArray(res)) {
        lines.set(res.map((l:any) => ({ timestamp: l.timestamp, stream: l.stream, message: String(l.message ?? l.msg ?? l) })))
      } else if (typeof res === 'string') {
        // fallback: split lines
        lines.set(res.split('\n').filter(Boolean).map(s=>({message:s})))
      }
      // scroll to bottom after loading
      requestAnimationFrame(()=>{
        if (containerEl) containerEl.scrollTop = containerEl.scrollHeight;
      })
    } catch (e) {
      console.error('Failed to load logs', e);
    }
  }

  function startWS() {
    if (!containerId) return;
    stopWS();
    const protocol = location.protocol === 'https:' ? 'wss' : 'ws';
    const q = new URLSearchParams({ tail: String(tail), filter: filterText });
    const url = `${protocol}://${location.host}/api/containers/${encodeURIComponent(containerId)}/logs/stream?${q.toString()}`;
    ws = new WebSocket(url);
    ws.onopen = () => { try { const el = document.getElementById('reconnectBanner'); if (el) (el as HTMLElement).style.display = 'none'; } catch(_){} };
    ws.onmessage = (ev) => {
      try {
        const data = JSON.parse(ev.data);
        lines.update(arr => { arr.push({ timestamp: data.timestamp, stream: data.stream, message: String(data.message ?? data.msg ?? data.line ?? '') }); return arr; });
        // auto-scroll if near bottom
        if (containerEl) {
          const nearBottom = containerEl.scrollTop + containerEl.clientHeight + 50 >= containerEl.scrollHeight;
          if (nearBottom) containerEl.scrollTop = containerEl.scrollHeight;
        }
      } catch (err) {
        console.warn('ws message parse', err);
      }
    }
    ws.onclose = () => { ws = null; try { const el = document.getElementById('reconnectBanner'); if (el) (el as HTMLElement).style.display = 'block'; } catch(_){} };
    ws.onerror = (e) => { console.error('ws error', e); try { const el = document.getElementById('reconnectBanner'); if (el) (el as HTMLElement).style.display = 'block'; } catch(_){} };
  }
  function stopWS() {
    if (ws) { ws.close(); ws = null; }
  }

  $: if (live) {
    startWS();
  } else {
    stopWS();
  }

  onMount(()=>{
    loadOnce();
    const resize = () => { viewHeight = containerEl?.clientHeight ?? 400; };
    window.addEventListener('resize', resize);
    resize();
    return () => { window.removeEventListener('resize', resize); stopWS(); }
  });

  onDestroy(()=> stopWS());

  function onScroll(e:Event) {
    scrollTop = (e.target as HTMLElement).scrollTop;
  }

  // safe HTML escape
  function escapeHtml(s: string) { return s.replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;'); }
  function highlight(s: string) {
    if (!filterText) return escapeHtml(s);
    const q = filterText.replace(/[.*+?^${}()|[\]\\]/g,'\\$&');
    const re = new RegExp(q, 'ig');
    return escapeHtml(s).replace(re, (m) => `<mark>${m}</mark>`);
  }
</script>

<style>
  .controls { display:flex; gap:8px; align-items:center; flex-wrap:wrap; margin-bottom:8px }
  .controls select, .controls input { background:transparent; border:1px solid var(--border); padding:6px 8px; color:var(--text); border-radius:6px }
  .log-window { height:400px; overflow:auto; background:var(--surface); border:1px solid var(--border); font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, 'Roboto Mono', monospace; font-size:13px; }
  .line { display:flex; gap:8px; padding:2px 8px; align-items:flex-start; white-space:pre-wrap }
  .ts { color:var(--muted); width:170px; flex:0 0 170px }
  .stream { width:8px; height:8px; border-radius:50%; margin-top:6px }
  .stdout { background:var(--accent) }
  .stderr { background:var(--danger) }
  mark { background: #f6e05e; color: #000; padding:0 2px; border-radius:2px }
  .live-badge { background: rgba(59,130,246,0.12); color:var(--accent); padding:4px 8px; border-radius:999px; font-size:12px }
  @media (max-width:768px) {
    .controls { gap:6px }
    .log-window { font-size:12px }
    .ts { display:none }
  }
</style>

<div>
  <div class="controls">
    <label>Range
      <select bind:value={selectedRange} on:change={()=>{ if (selectedRange!=='custom') loadOnce(); }}>
        {#each ranges as r}
          <option value={r[0]}>{r[0]}</option>
        {/each}
      </select>
    </label>
    {#if selectedRange==='custom'}
      <label>Since <input type="datetime-local" bind:value={customSince} on:change={loadOnce} /></label>
      <label>Until <input type="datetime-local" bind:value={customUntil} on:change={loadOnce} /></label>
    {/if}

    <input placeholder="Search" bind:value={filterText} on:keydown={(e)=>{ if (e.key==='Enter') loadOnce(); }} />
    <label><input type="checkbox" bind:checked={stderrOnly} on:change={loadOnce} /> stderr only</label>
    <label>Tail
      <select bind:value={tail} on:change={loadOnce}>
        <option value={100}>100</option>
        <option value={500}>500</option>
        <option value={1000}>1000</option>
        <option value={5000}>5000</option>
      </select>
    </label>
    <label><input type="checkbox" bind:checked={showTimestamps} /> show timestamps</label>
    <label style="margin-left:auto"><button on:click={() => { live = !live; }}>{live ? 'Stop Live' : 'Live'}</button></label>
    {#if live}<div class="live-badge">Live</div>{/if}
    <button on:click={loadOnce}>Refresh</button>
  </div>

  <div bind:this={containerEl} class="log-window" on:scroll={onScroll}>
    <div style={`height:${total * lineHeight}px;position:relative`}> 
      {#each currentLines.slice(startIndex, endIndex) as line, i (startIndex+i)}
        <div class="line" style={`position:absolute;left:0;right:0;top:${(startIndex+i)*lineHeight}px;height:${lineHeight}px`}> 
          {#if showTimestamps}
            <div class="ts">{line.timestamp ?? ''}</div>
          {/if}
          <div class="stream {line.stream==='stderr'?'stderr':'stdout'}"></div>
          <div class="msg"><span>{@html highlight(line.message)}</span></div>
        </div>
      {/each}
    </div>
  </div>
</div>
