<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  export let data: {ts:number,value:number}[] = [];
  export let color = '#4FC3F7';
  export let width = 80;
  export let height = 24;
  let canvas: HTMLCanvasElement;
  let raf = 0;

  function draw() {
    if (!canvas) return;
    const ctx = canvas.getContext('2d');
    if (!ctx) return;
    const dpr = window.devicePixelRatio || 1;
    canvas.width = width * dpr;
    canvas.height = height * dpr;
    ctx.scale(dpr, dpr);
    ctx.clearRect(0, 0, width, height);

    const points = data.slice(-30);
    if (points.length < 2) return;
    const vals = points.map(p => p.value);
    const min = Math.min(...vals, 0);
    const max = Math.max(...vals, 1);
    const range = max - min || 1;

    ctx.beginPath();
    for (let i = 0; i < points.length; i++) {
      const x = (i / (points.length - 1)) * width;
      const y = height - ((points[i].value - min) / range) * (height * 0.8) - 1;
      if (i === 0) ctx.moveTo(x, y); else ctx.lineTo(x, y);
    }
    ctx.strokeStyle = color;
    ctx.lineWidth = 1.5;
    ctx.stroke();
  }

  function loop() { draw(); raf = requestAnimationFrame(loop); }
  onMount(() => { loop(); });
  onDestroy(() => cancelAnimationFrame(raf));
</script>

<canvas bind:this={canvas} style="width:{width}px;height:{height}px;vertical-align:middle"></canvas>
