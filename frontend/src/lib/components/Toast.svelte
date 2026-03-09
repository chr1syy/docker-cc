<script lang="ts">
  import toasts from '$lib/stores/toast';
</script>

<div class="toast-wrapper">
  {#each $toasts as t (t.id)}
    <div class="toast {t.type}">
      {#if t.type === 'success'}
        <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5"><polyline points="20 6 9 17 4 12"/></svg>
      {:else if t.type === 'error'}
        <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5"><circle cx="12" cy="12" r="10"/><line x1="15" y1="9" x2="9" y2="15"/><line x1="9" y1="9" x2="15" y2="15"/></svg>
      {:else}
        <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5"><circle cx="12" cy="12" r="10"/><line x1="12" y1="16" x2="12" y2="12"/><line x1="12" y1="8" x2="12.01" y2="8"/></svg>
      {/if}
      <span>{t.message}</span>
    </div>
  {/each}
</div>

<style>
  .toast-wrapper {
    position: fixed;
    right: 16px;
    bottom: 16px;
    display: flex;
    flex-direction: column;
    gap: 8px;
    z-index: 9999;
    pointer-events: none;
  }
  .toast {
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 12px 16px;
    border-radius: 8px;
    font-size: 13px;
    font-weight: 500;
    color: white;
    min-width: 200px;
    max-width: 380px;
    box-shadow: 0 8px 24px rgba(0,0,0,0.3);
    animation: slideIn 200ms ease-out;
    pointer-events: auto;
  }
  .success { background: #059669; }
  .error { background: #dc2626; }
  .info { background: #2563eb; }

  @keyframes slideIn {
    from { transform: translateY(8px); opacity: 0; }
    to { transform: translateY(0); opacity: 1; }
  }
</style>
