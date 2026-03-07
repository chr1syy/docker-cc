<script lang="ts">
  import { writable } from 'svelte/store';
  const open = writable(false);
  function toggle() { open.update(v => !v); }
</script>

<style>
  .mobile-nav { display:none; align-items:center; gap:8px; padding:8px 12px; background:var(--surface); border-bottom:1px solid var(--border); }
  .logo { font-weight:700 }
  .hamburger { width:36px; height:36px; display:inline-flex; align-items:center; justify-content:center; border-radius:6px; background:transparent; border:1px solid var(--border) }
  .drawer { position:absolute; left:0; top:56px; right:0; background:var(--surface); border-top:1px solid var(--border); padding:12px; display:none; z-index:50 }
  .drawer a { display:block; padding:8px 6px; border-radius:6px }
  @media (max-width: 768px) {
    .mobile-nav{ display:flex; }
    .drawer[open] { display:block }
  }
</style>

<div class="mobile-nav">
  <div class="logo">Docker CC</div>
  <div style="margin-left:auto;display:flex;align-items:center;gap:8px">
    <button class="hamburger" on:click={toggle} aria-label="Toggle menu">☰</button>
  </div>
  {#if $open}
    <div class="drawer" open>
      <a href="/">Dashboard</a>
      <a href="/logs">Logs</a>
      <!-- Settings page not implemented yet -->
    </div>
  {/if}
</div>
