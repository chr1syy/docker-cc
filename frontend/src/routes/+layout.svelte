<script lang="ts">
  import "../lib/styles/global.css";
  import MobileNav from '$lib/components/MobileNav.svelte';
  import Toast from '$lib/components/Toast.svelte';
  import { onMount, onDestroy } from 'svelte';
  import { auth } from '$lib/stores/auth';
  import { stats } from '$lib/stores/stats';
  import { goto } from '$app/navigation';

  let state: any;
  const unsub = auth.subscribe(s => {
    state = s;
    // Start/stop stats WebSocket based on auth state
    if (s.isAuthenticated) {
      stats.start();
    } else {
      stats.stop();
    }
  });

  onMount(async () => {
    const ok = await auth.checkAuth();
    if (!ok && location.pathname !== '/login') {
      goto('/login');
    }
    if (ok && location.pathname === '/login') {
      goto('/');
    }
  });

  onDestroy(() => { unsub(); stats.stop(); });
</script>

<MobileNav />

<div class="app-grid">
  <aside class="sidebar">
    <div class="logo">Docker CC</div>
    <nav class="nav">
      <a href="/" class="active">Dashboard</a>
      <a href="/logs">Logs</a>
    </nav>
    <div class="reconnect-banner" id="reconnectBanner" style="display:none;padding:8px;background:#fff3bf;border-top:1px solid var(--border);text-align:center">Reconnecting…</div>
    <div class="sidebar-footer">
      {#if state && state.loading}
        <div>Checking auth…</div>
      {:else if state && state.isAuthenticated}
        <div>
          <small>{state.user}</small>
          <button on:click={() => auth.logout()}>Logout</button>
        </div>
      {/if}
    </div>
</aside>
  
  <main class="main">
    {#if state && state.loading}
      <div class="center-loading">Loading…</div>
    {:else}
      <slot />
    {/if}
  </main>
</div>


<style>
  .center-loading {
    display: flex;
    align-items: center;
    justify-content: center;
    height: 100%;
    color: #cbd5e1;
    font-size: 1.1rem;
  }
</style>
