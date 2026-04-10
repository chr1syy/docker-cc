<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  export let data: {ts:number,value:number}[] = [];
  export let label = '';
  export let unit = '';
  export let formatValue: ((v: number) => string) | null = null;
  export let height = 120;
  export let color = '#4FC3F7';
  let canvas: HTMLCanvasElement;
  let raf = 0;

  function defaultFormat(v: number): string {
    if (formatValue) return formatValue(v);
    if (unit === '%') return Math.round(v) + '%';
    if (unit === 'bytes') return fmtBytes(v);
    return String(Math.round(v * 100) / 100);
  }

  function fmtBytes(n: number): string {
    if (n < 1024) return n + ' B';
    if (n < 1024 * 1024) return (n / 1024).toFixed(1) + ' KB';
    if (n < 1024 * 1024 * 1024) return (n / (1024 * 1024)).toFixed(1) + ' MB';
    return (n / (1024 * 1024 * 1024)).toFixed(1) + ' GB';
  }

  function draw() {
    if (!canvas) return;
    const ctx = canvas.getContext('2d');
    if (!ctx) return;
    const w = canvas.width = canvas.clientWidth * (window.devicePixelRatio || 1);
    const h = canvas.height = height * (window.devicePixelRatio || 1);
    ctx.scale(window.devicePixelRatio || 1, window.devicePixelRatio || 1);
    const dw = canvas.clientWidth;
    const dh = height;
    ctx.clearRect(0, 0, dw, dh);

    const points = data.slice(-150);
    if (points.length < 2) return;
    const vals = points.map(p => p.value);
    const min = Math.min(...vals, 0);
    const max = Math.max(...vals, 1);
    const range = max - min || 1;

    ctx.beginPath();
    for (let i = 0; i < points.length; i++) {
      const x = (i / (points.length - 1)) * dw;
      const y = dh - ((points[i].value - min) / range) * (dh * 0.85);
      if (i === 0) ctx.moveTo(x, y); else ctx.lineTo(x, y);
    }
    ctx.strokeStyle = color;
    ctx.lineWidth = 2;
    ctx.stroke();

    // gradient fill
    const grad = ctx.createLinearGradient(0, 0, 0, dh);
    const rgb = hexToRgb(color);
    grad.addColorStop(0, `rgba(${rgb},0.3)`);
    grad.addColorStop(1, `rgba(${rgb},0.02)`);
    ctx.lineTo(dw, dh);
    ctx.lineTo(0, dh);
    ctx.fillStyle = grad;
    ctx.fill();
  }

  function hexToRgb(hex: string): string {
    const r = parseInt(hex.slice(1,3),16);
    const g = parseInt(hex.slice(3,5),16);
    const b = parseInt(hex.slice(5,7),16);
    return `${r},${g},${b}`;
  }

  function loop() { draw(); raf = requestAnimationFrame(loop); }

  onMount(() => { loop(); });
  onDestroy(() => cancelAnimationFrame(raf));

  $: currentValue = data.length ? defaultFormat(data[data.length - 1].value) : '-';
</script>

<div class="metric-chart">
  {#if label}
    <div class="metric-label">{label}</div>
  {/if}
  <div class="metric-value">{currentValue}</div>
  <canvas bind:this={canvas} style="width:100%;height:{height}px"></canvas>
</div>

<style>
  .metric-chart {
    width: 100%;
  }
  .metric-label {
    font-size: 12px;
    color: var(--text-muted, #888);
    text-transform: uppercase;
    letter-spacing: 0.5px;
    margin-bottom: 2px;
  }
  .metric-value {
    font-size: 20px;
    font-weight: 600;
    margin-bottom: 6px;
  }
</style>
