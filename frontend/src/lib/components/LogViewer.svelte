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
  let viewHeight = 600;
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
      // Backend returns {lines: [...]}
      const arr = Array.isArray(res) ? res : (res?.lines ?? []);
      lines.set(arr.map((l:any) => ({ timestamp: l.timestamp, stream: l.stream, message: String(l.message ?? l.msg ?? l) })))
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
  .controls { display:flex; gap:8px; align-items:center; flex-wrap:wrap; margin-bottom:10px }
  .controls select, .controls input[type="text"], .controls input[type="datetime-local"] { background:var(--bg, #0b0e14); border:1px solid var(--border); padding:6px 10px; color:var(--text); border-radius:6px; font-size:12px }
  .controls label { font-size:12px; color:var(--text-secondary, #94a3b8); display:flex; align-items:center; gap:4px }
  .controls button { background:transparent; border:1px solid var(--border); color:var(--text-secondary); padding:6px 12px; border-radius:6px; font-size:12px; cursor:pointer; transition:all 150ms ease }
  .controls button:hover { background:rgba(255,255,255,0.04); color:var(--text); border-color:var(--text-muted) }
  .log-window { height:600px; overflow:auto; background:var(--bg, #0b0e14); border:1px solid var(--border); border-radius:var(--radius-sm, 6px); font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, 'Roboto Mono', monospace; font-size:12px; }
  .line { display:flex; gap:8px; padding:2px 10px; align-items:flex-start; white-space:pre-wrap }
  .line:hover { background:rgba(255,255,255,0.02) }
  .ts { color:var(--text-muted, #64748b); white-space:nowrap; flex-shrink:0; font-size:11px }
  .stream { width:6px; height:6px; border-radius:50%; margin-top:6px; flex-shrink:0 }
  .stdout { background:var(--accent) }
  .stderr { background:var(--danger) }
  .msg { flex:1; min-width:0; word-break:break-all }
  .live-badge { background: rgba(16,185,129,0.12); color:var(--success, #10b981); padding:4px 10px; border-radius:999px; font-size:11px; font-weight:600; animation:pulse 2s ease-in-out infinite }
  @keyframes pulse { 0%,100%{ opacity:1 } 50%{ opacity:0.6 } }
  @media (max-width:768px) {
    .controls { gap:6px }
    .log-window { height:400px; font-size:11px }
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

    <input type="text" placeholder="Search logs..." bind:value={filterText} on:keydown={(e)=>{ if (e.key==='Enter') loadOnce(); }} />
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
