<script lang="ts">
  import { writable } from 'svelte/store';
  import { auth } from '$lib/stores/auth';
  const open = writable(false);
  function toggle() { open.update(v => !v); }
  function close() { open.set(false); }
</script>

<div class="mobile-nav">
  <div class="logo">Docker CC</div>
  <div style="margin-left:auto;display:flex;align-items:center;gap:8px">
    <button class="hamburger" on:click={toggle} aria-label="Toggle menu">
      <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><line x1="3" y1="6" x2="21" y2="6"/><line x1="3" y1="12" x2="21" y2="12"/><line x1="3" y1="18" x2="21" y2="18"/></svg>
    </button>
  </div>
</div>

{#if $open}
  <div class="drawer-backdrop" on:click={close} on:keydown={e => e.key === 'Escape' && close()} role="button" tabindex="-1"></div>
  <div class="drawer">
    <a href="/" on:click={close}>Dashboard</a>
    <a href="/logs" on:click={close}>Logs</a>
    <a href="/settings" on:click={close}>Settings</a>
    <div class="drawer-footer">
      <button class="logout-btn" on:click={() => { auth.logout(); close(); }}>Logout</button>
    </div>
  </div>
{/if}

<style>
  .mobile-nav {
    display: none;
    align-items: center;
    gap: 8px;
    padding: 12px 16px;
    background: var(--surface, #12151e);
    border-bottom: 1px solid var(--border, #1e2334);
  }
  .logo { font-weight: 800; font-size: 15px; letter-spacing: -0.02em; }
  .hamburger {
    width: 36px;
    height: 36px;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    border-radius: 6px;
    background: transparent;
    border: 1px solid var(--border, #1e2334);
    color: var(--text-secondary, #94a3b8);
    cursor: pointer;
    transition: all 150ms ease;
  }
  .hamburger:hover {
    background: rgba(255,255,255,0.04);
    color: var(--text, #e2e8f0);
  }

  .drawer-backdrop {
    position: fixed;
    inset: 0;
    background: rgba(0,0,0,0.4);
    z-index: 49;
  }
  .drawer {
    position: fixed;
    left: 0;
    top: 0;
    bottom: 0;
    width: 260px;
    background: var(--surface, #12151e);
    border-right: 1px solid var(--border, #1e2334);
    padding: 20px 12px;
    z-index: 50;
    display: flex;
    flex-direction: column;
    gap: 2px;
    animation: slideRight 150ms ease-out;
  }
  .drawer a {
    display: block;
    padding: 10px 12px;
    border-radius: 6px;
    color: var(--text-secondary, #94a3b8);
    font-size: 14px;
    font-weight: 500;
    text-decoration: none;
  }
  .drawer a:hover {
    background: rgba(255,255,255,0.04);
    color: var(--text, #e2e8f0);
  }
  .drawer-footer {
    margin-top: auto;
    padding-top: 12px;
    border-top: 1px solid var(--border, #1e2334);
  }
  .logout-btn {
    width: 100%;
    padding: 8px;
    border-radius: 6px;
    background: transparent;
    border: 1px solid var(--border, #1e2334);
    color: var(--text-secondary, #94a3b8);
    font-size: 13px;
    cursor: pointer;
  }
  .logout-btn:hover { background: rgba(255,255,255,0.04); color: var(--text); }

  @keyframes slideRight {
    from { transform: translateX(-100%); }
    to { transform: translateX(0); }
  }

  @media (max-width: 768px) {
    .mobile-nav { display: flex; }
  }
</style>
