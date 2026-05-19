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
      const arr = Array.isArray(res) ? res : (res?.lines ?? []);
      lines.set(arr.map((l:any) => ({ timestamp: l.timestamp, stream: l.stream, message: String(l.message ?? l.msg ?? l) })))
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
    return () => { stopWS(); }
  });

  onDestroy(()=> stopWS());

  // ---------- Message parsing ----------

  // Many log lines embed their own leading timestamp (Go's default logger
  // emits "YYYY/MM/DD HH:MM:SS ", Docker access logs emit RFC3339, etc.).
  // We already render the Docker-provided timestamp in its own column, so
  // strip a leading timestamp from the message body to avoid showing two.
  const LEADING_TS = /^(?:\d{4}[/-]\d{2}[/-]\d{2}[T ]\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:?\d{2})?)\s+/;

  function shortTs(ts?: string): string {
    if (!ts) return '';
    const m = ts.match(/T(\d{2}:\d{2}:\d{2})/);
    if (m) return m[1];
    const d = new Date(ts);
    if (!isNaN(d.getTime())) return d.toISOString().substr(11, 8);
    return ts;
  }

  type Parsed =
    | { kind: 'access'; method: string; path: string; status?: number; duration_ms?: number; level?: string; extra?: string }
    | { kind: 'structured'; level?: string; message: string; extra?: Record<string, unknown> }
    | { kind: 'raw'; text: string };

  function parseMessage(raw: string): Parsed {
    const stripped = raw.replace(LEADING_TS, '');
    const trimmed = stripped.trim();

    if (trimmed.startsWith('{') && trimmed.endsWith('}')) {
      try {
        const obj = JSON.parse(trimmed);
        if (obj && typeof obj === 'object') {
          const method = typeof obj.method === 'string' ? obj.method : undefined;
          const path = typeof obj.path === 'string' ? obj.path : undefined;
          const status = typeof obj.status === 'number' ? obj.status : undefined;
          const duration_ms = typeof obj.duration_ms === 'number' ? obj.duration_ms
            : typeof obj.duration === 'number' ? obj.duration : undefined;
          const level = typeof obj.level === 'string' ? obj.level : undefined;
          if (method && path) {
            const known = new Set(['method','path','status','duration_ms','duration','level','time','ts','timestamp']);
            const extras = Object.entries(obj).filter(([k]) => !known.has(k));
            const extra = extras.length
              ? extras.map(([k,v]) => `${k}=${typeof v === 'string' ? v : JSON.stringify(v)}`).join(' ')
              : undefined;
            return { kind: 'access', method, path, status, duration_ms, level, extra };
          }
          const message = typeof obj.msg === 'string' ? obj.msg
            : typeof obj.message === 'string' ? obj.message
            : typeof obj.error === 'string' ? obj.error
            : '';
          if (message || level) {
            const known = new Set(['msg','message','error','level','time','ts','timestamp']);
            const extras: Record<string, unknown> = {};
            for (const [k, v] of Object.entries(obj)) if (!known.has(k)) extras[k] = v;
            return { kind: 'structured', level, message, extra: Object.keys(extras).length ? extras : undefined };
          }
        }
      } catch (_) { /* fall through */ }
    }
    return { kind: 'raw', text: stripped };
  }

  function statusBucket(status?: number): string {
    if (status === undefined) return '';
    if (status >= 500) return 'st-5xx';
    if (status >= 400) return 'st-4xx';
    if (status >= 300) return 'st-3xx';
    if (status >= 200) return 'st-2xx';
    return 'st-1xx';
  }

  function levelClass(level?: string): string {
    if (!level) return '';
    const l = level.toLowerCase();
    if (l === 'error' || l === 'fatal' || l === 'panic') return 'lvl-error';
    if (l === 'warn' || l === 'warning') return 'lvl-warn';
    if (l === 'info') return 'lvl-info';
    if (l === 'debug' || l === 'trace') return 'lvl-debug';
    return '';
  }

  function methodClass(method: string): string {
    const m = method.toUpperCase();
    if (m === 'GET' || m === 'HEAD') return 'mth-get';
    if (m === 'POST') return 'mth-post';
    if (m === 'PUT' || m === 'PATCH') return 'mth-put';
    if (m === 'DELETE') return 'mth-delete';
    return 'mth-other';
  }

  function rawSeverity(text: string): string {
    if (/\b(error|fatal|panic|exception)\b/i.test(text)) return 'lvl-error';
    if (/\b(warn|warning)\b/i.test(text)) return 'lvl-warn';
    return '';
  }

  function escapeHtml(s: string) { return s.replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;'); }
  function highlight(s: string) {
    if (!filterText) return escapeHtml(s);
    const q = filterText.replace(/[.*+?^${}()|[\]\\]/g,'\\$&');
    const re = new RegExp(q, 'ig');
    return escapeHtml(s).replace(re, (m) => `<mark>${m}</mark>`);
  }

  function formatDur(ms?: number): string {
    if (ms === undefined) return '';
    if (ms < 1) return '<1ms';
    if (ms < 1000) return Math.round(ms) + 'ms';
    return (ms / 1000).toFixed(2) + 's';
  }
</script>

<style>
  .controls { display:flex; gap:8px; align-items:center; flex-wrap:wrap; margin-bottom:10px }
  .controls select, .controls input[type="text"], .controls input[type="datetime-local"] { background:var(--bg, #0b0e14); border:1px solid var(--border); padding:6px 10px; color:var(--text); border-radius:6px; font-size:12px }
  .controls label { font-size:12px; color:var(--text-secondary, #94a3b8); display:flex; align-items:center; gap:4px }
  .controls button { background:transparent; border:1px solid var(--border); color:var(--text-secondary); padding:6px 12px; border-radius:6px; font-size:12px; cursor:pointer; transition:all 150ms ease }
  .controls button:hover { background:rgba(255,255,255,0.04); color:var(--text); border-color:var(--text-muted) }
  .search-input { flex: 1 1 200px; min-width: 0 }
  .right-actions { display: flex; align-items: center; gap: 8px; margin-left: auto }

  .log-window {
    height: 600px;
    overflow: auto;
    background: var(--bg, #0b0e14);
    border: 1px solid var(--border);
    border-radius: var(--radius-sm, 6px);
    font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, 'Roboto Mono', monospace;
    font-size: 12px;
  }
  .line {
    display: grid;
    grid-template-columns: 8px auto 1fr;
    gap: 10px;
    padding: 4px 12px;
    align-items: baseline;
    white-space: pre-wrap;
    word-break: break-word;
    overflow-wrap: anywhere;
    border-bottom: 1px solid rgba(255, 255, 255, 0.025);
  }
  .line:last-child { border-bottom: none; }
  .line:hover { background: rgba(255, 255, 255, 0.025); }
  .line.has-error { background: rgba(239, 68, 68, 0.04); }
  .line.has-error:hover { background: rgba(239, 68, 68, 0.07); }
  .line.has-warn { background: rgba(245, 158, 11, 0.03); }
  .line.has-warn:hover { background: rgba(245, 158, 11, 0.06); }

  .stream {
    width: 6px;
    height: 6px;
    border-radius: 50%;
    align-self: center;
    margin-top: 1px;
  }
  .stream.stdout { background: var(--accent); }
  .stream.stderr { background: var(--danger); }

  .ts {
    color: var(--text-muted, #64748b);
    font-variant-numeric: tabular-nums;
    font-size: 11px;
    white-space: nowrap;
  }
  .msg {
    min-width: 0;
    word-break: break-all;
    overflow-wrap: anywhere;
    display: flex;
    flex-wrap: wrap;
    align-items: baseline;
    gap: 8px;
  }
  .msg-text { color: var(--text); }
  .msg-text.dim { color: var(--text-secondary); }

  .chip {
    display: inline-flex;
    align-items: center;
    padding: 1px 6px;
    border-radius: 4px;
    font-size: 11px;
    font-weight: 600;
    font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, monospace;
    letter-spacing: 0.02em;
    line-height: 1.5;
    flex-shrink: 0;
  }

  .mth-get    { background: rgba(59, 130, 246, 0.15); color: #60a5fa; }
  .mth-post   { background: rgba(16, 185, 129, 0.15); color: #34d399; }
  .mth-put    { background: rgba(245, 158, 11, 0.15); color: #fbbf24; }
  .mth-delete { background: rgba(239, 68, 68, 0.15); color: #f87171; }
  .mth-other  { background: rgba(148, 163, 184, 0.15); color: #94a3b8; }

  .st-2xx { background: rgba(16, 185, 129, 0.15); color: #34d399; }
  .st-3xx { background: rgba(59, 130, 246, 0.15); color: #60a5fa; }
  .st-4xx { background: rgba(245, 158, 11, 0.15); color: #fbbf24; }
  .st-5xx { background: rgba(239, 68, 68, 0.18); color: #f87171; }
  .st-1xx { background: rgba(148, 163, 184, 0.15); color: #94a3b8; }

  .path {
    color: var(--text);
    font-weight: 500;
    word-break: break-all;
  }
  .dur {
    color: var(--text-muted);
    font-size: 11px;
    font-variant-numeric: tabular-nums;
  }
  .extra {
    color: var(--text-muted);
    font-size: 11px;
  }

  .lvl-error { background: rgba(239, 68, 68, 0.18); color: #f87171; }
  .lvl-warn  { background: rgba(245, 158, 11, 0.18); color: #fbbf24; }
  .lvl-info  { background: rgba(59, 130, 246, 0.15); color: #60a5fa; }
  .lvl-debug { background: rgba(148, 163, 184, 0.15); color: #94a3b8; }

  .msg-text :global(mark) {
    background: rgba(245, 158, 11, 0.4);
    color: var(--text);
    padding: 0 2px;
    border-radius: 2px;
  }

  .live-badge { background: rgba(16,185,129,0.12); color:var(--success, #10b981); padding:4px 10px; border-radius:999px; font-size:11px; font-weight:600; animation:pulse 2s ease-in-out infinite }
  @keyframes pulse { 0%,100%{ opacity:1 } 50%{ opacity:0.6 } }

  @media (max-width:768px) {
    .controls { gap:6px 8px }
    .controls label { font-size: 11px }
    .controls select, .controls input[type="text"], .controls input[type="datetime-local"] { padding: 8px 10px; font-size: 13px; min-height: 36px }
    .controls button { padding: 8px 12px; font-size: 13px; min-height: 36px }
    .search-input { flex-basis: 100% }
    .right-actions { margin-left: 0; flex: 0 0 100%; justify-content: flex-end }
    .log-window { height: 60vh; min-height: 320px; font-size: 11px }
    .line { grid-template-columns: 8px 1fr; gap: 8px; padding: 4px 10px }
    .ts { display: none }
  }
  @media (max-width: 420px) {
    .log-window { height: 55vh; min-height: 280px }
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

    <input type="text" class="search-input" placeholder="Search logs..." bind:value={filterText} on:keydown={(e)=>{ if (e.key==='Enter') loadOnce(); }} />
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
    <div class="right-actions">
      <button on:click={() => { live = !live; }}>{live ? 'Stop Live' : 'Live'}</button>
      {#if live}<div class="live-badge">Live</div>{/if}
      <button on:click={loadOnce}>Refresh</button>
    </div>
  </div>

  <div bind:this={containerEl} class="log-window">
    {#each $lines as line}
      {@const parsed = parseMessage(line.message)}
      {@const sev = parsed.kind === 'access' ? statusBucket(parsed.status).replace('st-', 'lvl-mock-')
        : parsed.kind === 'structured' ? levelClass(parsed.level)
        : rawSeverity(parsed.text)}
      <div class="line"
        class:has-error={sev === 'lvl-error' || (parsed.kind === 'access' && (parsed.status ?? 0) >= 500)}
        class:has-warn={sev === 'lvl-warn' || (parsed.kind === 'access' && (parsed.status ?? 0) >= 400 && (parsed.status ?? 0) < 500)}
      >
        <div class="stream {line.stream==='stderr'?'stderr':'stdout'}"></div>
        {#if showTimestamps}
          <div class="ts" title={line.timestamp ?? ''}>{shortTs(line.timestamp)}</div>
        {:else}
          <div></div>
        {/if}
        <div class="msg">
          {#if parsed.kind === 'access'}
            <span class="chip {methodClass(parsed.method)}">{parsed.method.toUpperCase()}</span>
            <span class="path">{parsed.path}</span>
            {#if parsed.status !== undefined}
              <span class="chip {statusBucket(parsed.status)}">{parsed.status}</span>
            {/if}
            {#if parsed.duration_ms !== undefined}
              <span class="dur">{formatDur(parsed.duration_ms)}</span>
            {/if}
            {#if parsed.extra}
              <span class="extra">{parsed.extra}</span>
            {/if}
          {:else if parsed.kind === 'structured'}
            {#if parsed.level}
              <span class="chip {levelClass(parsed.level)}">{parsed.level.toUpperCase()}</span>
            {/if}
            <span class="msg-text">{@html highlight(parsed.message)}</span>
            {#if parsed.extra}
              <span class="extra">{Object.entries(parsed.extra).map(([k,v]) => `${k}=${typeof v === 'string' ? v : JSON.stringify(v)}`).join(' ')}</span>
            {/if}
          {:else}
            <span class="msg-text">{@html highlight(parsed.text)}</span>
          {/if}
        </div>
      </div>
    {/each}
  </div>
</div>
