<script lang="ts">
  import "../lib/styles/global.css";
  import MobileNav from '$lib/components/MobileNav.svelte';
  import Toast from '$lib/components/Toast.svelte';
  import { onMount, onDestroy } from 'svelte';
  import { auth } from '$lib/stores/auth';
  import { stats } from '$lib/stores/stats';
  import { getVersion } from '$lib/api';
  import { goto } from '$app/navigation';
  import { page } from '$app/stores';

  let appVersion = '';

  let state: any;
  const unsub = auth.subscribe(s => {
    state = s;
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
    getVersion().then(v => appVersion = v).catch(() => {});
  });

  onDestroy(() => { unsub(); stats.stop(); });

  $: currentPath = $page.url.pathname;
</script>

<MobileNav />
<Toast />

<div class="app-grid">
  <aside class="sidebar">
    <div class="logo">Docker CC</div>
    <nav class="nav">
      <a href="/" class:active={currentPath === '/'}>
        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="3" y="3" width="7" height="7"/><rect x="14" y="3" width="7" height="7"/><rect x="3" y="14" width="7" height="7"/><rect x="14" y="14" width="7" height="7"/></svg>
        Dashboard
      </a>
      <a href="/logs" class:active={currentPath === '/logs'}>
        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/><polyline points="14 2 14 8 20 8"/><line x1="16" y1="13" x2="8" y2="13"/><line x1="16" y1="17" x2="8" y2="17"/></svg>
        Logs
      </a>
      <a href="/settings" class:active={currentPath === '/settings'}>
        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="3"/><path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-4 0v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83-2.83l.06-.06A1.65 1.65 0 0 0 4.68 15a1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1 0-4h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 2.83-2.83l.06.06A1.65 1.65 0 0 0 9 4.68a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 2.83l-.06.06A1.65 1.65 0 0 0 19.4 9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1z"/></svg>
        Settings
      </a>
    </nav>
    <div class="reconnect-banner" id="reconnectBanner" style="display:none;padding:8px 12px;background:var(--warning-soft);color:var(--warning);border-radius:var(--radius-sm);font-size:12px;text-align:center">
      Reconnecting...
    </div>
    <div class="sidebar-footer">
      {#if state && state.loading}
        <div style="color:var(--text-muted);font-size:12px">Checking auth...</div>
      {:else if state && state.isAuthenticated}
        <div style="display:flex;align-items:center;justify-content:space-between">
          <small>{state.user}</small>
          <button on:click={() => auth.logout()}>Logout</button>
        </div>
      {/if}
      {#if appVersion}
        <div style="color:var(--text-muted);font-size:11px;margin-top:6px">{appVersion}</div>
      {/if}
    </div>
  </aside>

  <main class="main">
    {#if state && state.loading}
      <div class="center-loading">
        <div class="loading-spinner"></div>
        <span>Loading...</span>
      </div>
    {:else}
      <slot />
    {/if}
  </main>
</div>

<style>
  .center-loading {
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    height: 60vh;
    gap: 16px;
    color: var(--text-muted);
    font-size: 13px;
  }
  .loading-spinner {
    width: 24px;
    height: 24px;
    border: 2px solid var(--border);
    border-top-color: var(--accent);
    border-radius: 50%;
    animation: spin 0.6s linear infinite;
  }
  @keyframes spin { to { transform: rotate(360deg); } }
</style>
