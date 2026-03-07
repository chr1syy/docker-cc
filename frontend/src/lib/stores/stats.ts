import { writable } from 'svelte/store';

type MetricsMap = Record<string, any>;

function createStatsStore() {
  const { subscribe, set, update } = writable<MetricsMap>({});
  let ws: WebSocket | null = null;
  let shouldStop = false;

  let backoff = 1000;

  function connect() {
    const proto = location.protocol === 'https:' ? 'wss' : 'ws';
    ws = new WebSocket(`${proto}://${location.host}/api/stats/stream`);

    ws.onopen = () => {
      backoff = 1000;
      try { const el = document.getElementById('reconnectBanner'); if (el) (el as HTMLElement).style.display = 'none'; } catch(_){}
    };
    ws.onmessage = (ev) => {
      try {
        const data = JSON.parse(ev.data);
        if (data.error) return;
        const map: MetricsMap = {};
        for (const m of data) {
          map[m.container_id] = m;
        }
        set(map);
      } catch (e) {
        // ignore
      }
    };
    ws.onclose = () => {
      ws = null;
      // show reconnect banner if present
      try { const el = document.getElementById('reconnectBanner'); if (el) (el as HTMLElement).style.display = 'block'; } catch(_){}
      if (shouldStop) return;
      setTimeout(() => {
        backoff = Math.min(backoff * 2, 10000);
        connect();
      }, backoff);
    };
    ws.onerror = () => {
      ws?.close();
    };
  }

  connect();

  return {
    subscribe,
    stop() {
      shouldStop = true;
      ws?.close();
    }
  };
}

export const stats = createStatsStore();
