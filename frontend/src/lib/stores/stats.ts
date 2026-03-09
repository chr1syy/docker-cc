import { writable, derived } from 'svelte/store';

export interface ContainerMetrics {
  container_id: string;
  cpu_percent: number;
  memory_usage: number;
  memory_limit: number;
  memory_percent: number;
  network_rx_bytes: number;
  network_tx_bytes: number;
  block_read_bytes: number;
  block_write_bytes: number;
  timestamp: string;
}

export interface MetricPoint {
  ts: number;
  value: number;
}

export interface ContainerHistory {
  cpu: MetricPoint[];
  mem: MetricPoint[];
  memUsage: MetricPoint[];
  netRx: MetricPoint[];
  netTx: MetricPoint[];
  blkRead: MetricPoint[];
  blkWrite: MetricPoint[];
}

type MetricsMap = Record<string, ContainerMetrics>;
type HistoryMap = Record<string, ContainerHistory>;

const MAX_POINTS = 60;

function pushPoint(arr: MetricPoint[], point: MetricPoint) {
  arr.push(point);
  if (arr.length > MAX_POINTS) arr.shift();
}

function emptyHistory(): ContainerHistory {
  return { cpu: [], mem: [], memUsage: [], netRx: [], netTx: [], blkRead: [], blkWrite: [] };
}

function createStatsStore() {
  const current = writable<MetricsMap>({});
  const historyStore = writable<HistoryMap>({});
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
        current.set(map);

        // Update history
        historyStore.update(hist => {
          for (const m of Object.values(map)) {
            const ts = new Date(m.timestamp).getTime() || Date.now();
            if (!hist[m.container_id]) hist[m.container_id] = emptyHistory();
            const h = hist[m.container_id];
            pushPoint(h.cpu, { ts, value: m.cpu_percent });
            pushPoint(h.mem, { ts, value: m.memory_percent });
            pushPoint(h.memUsage, { ts, value: m.memory_usage });
            pushPoint(h.netRx, { ts, value: m.network_rx_bytes });
            pushPoint(h.netTx, { ts, value: m.network_tx_bytes });
            pushPoint(h.blkRead, { ts, value: m.block_read_bytes });
            pushPoint(h.blkWrite, { ts, value: m.block_write_bytes });
          }
          return { ...hist };
        });
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
    subscribe: current.subscribe,
    history: { subscribe: historyStore.subscribe },
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
