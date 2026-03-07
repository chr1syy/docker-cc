<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  export let data: {ts:number,value:number}[] = [];
  export let height = 120;
  let canvas: HTMLCanvasElement;
  let raf = 0;

  function draw() {
    if (!canvas) return;
    const ctx = canvas.getContext('2d');
    if (!ctx) return;
  const w = canvas.width = canvas.clientWidth;
  const h = canvas.height = canvas.clientHeight = height;
  ctx.clearRect(0,0,w,h);
    const points = data.slice(-60);
    if (points.length === 0) return;
    const vals = points.map(p=>p.value);
    const min = Math.min(...vals);
    const max = Math.max(...vals);
    const range = max - min || 1;

    ctx.beginPath();
    for (let i=0;i<points.length;i++){
      const x = (i/(points.length-1)) * w;
      const y = h - ((points[i].value - min)/range)*h;
      if (i===0) ctx.moveTo(x,y); else ctx.lineTo(x,y);
    }
    ctx.strokeStyle = '#4FC3F7';
    ctx.lineWidth = 2;
    ctx.stroke();

    // gradient fill
    const grad = ctx.createLinearGradient(0,0,0,h);
    grad.addColorStop(0,'rgba(79,195,247,0.4)');
    grad.addColorStop(1,'rgba(79,195,247,0.05)');
    ctx.lineTo(w,h);
    ctx.lineTo(0,h);
    ctx.fillStyle = grad;
    ctx.fill();
  }

  function loop(){ draw(); raf = requestAnimationFrame(loop); }

  onMount(()=>{ loop(); });
  onDestroy(()=> cancelAnimationFrame(raf));
</script>

<div style="width:100%">
  <div style="font-size:18px;margin-bottom:6px">{data.length? data[data.length-1].value : '-'} </div>
  <canvas bind:this={canvas} style="width:100%;height:{height}px"></canvas>
</div>
