import { writable } from 'svelte/store';

type MetricsMap = Record<string, any>;

function createStatsStore() {
  const { subscribe, set } = writable<MetricsMap>({});
  let ws: WebSocket | null = null;
  let shouldStop = false;
  let started = false;
  let backoff = 1000;

  function connect() {
    if (shouldStop) return;
    const proto = location.protocol === 'https:' ? 'wss' : 'ws';
    ws = new WebSocket(`${proto}://${location.host}/api/stats/stream`);

    ws.onopen = () => {
      backoff = 1000;
      try {
        const el = document.getElementById('reconnectBanner');
        if (el) (el as HTMLElement).style.display = 'none';
      } catch (_) {}
    };
    ws.onmessage = (ev) => {
      try {
        const data = JSON.parse(ev.data);
        if (data.error) return;
        const map: MetricsMap = {};
        if (Array.isArray(data)) {
          for (const m of data) {
            map[m.container_id] = m;
          }
        }
        set(map);
      } catch (_) {}
    };
    ws.onclose = () => {
      ws = null;
      try {
        const el = document.getElementById('reconnectBanner');
        if (el) (el as HTMLElement).style.display = 'block';
      } catch (_) {}
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

  return {
    subscribe,
    start() {
      if (started) return;
      started = true;
      shouldStop = false;
      connect();
    },
    stop() {
      shouldStop = true;
      started = false;
      ws?.close();
    }
  };
}

export const stats = createStatsStore();
