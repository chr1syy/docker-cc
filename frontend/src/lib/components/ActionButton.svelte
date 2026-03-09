<script lang="ts">
  import { createEventDispatcher } from 'svelte';
  import { startContainer, stopContainer, restartContainer } from '$lib/api';
  import { pushToast } from '$lib/stores/toast';

  export let action: 'start'|'stop'|'restart';
  export let containerId: string;
  export let containerName: string;
  export let disabled: boolean = false;

  const dispatch = createEventDispatcher();
  let confirming = false;
  let loading = false;

  async function confirm() {
    confirming = true;
  }

  async function doAction() {
    loading = true;
    try {
      if (action === 'start') await startContainer(containerId);
      if (action === 'stop') await stopContainer(containerId);
      if (action === 'restart') await restartContainer(containerId);
      pushToast(`Container ${containerName} ${action}ed`, 'success');
      dispatch('refresh');
    } catch (e) {
      const err = e as any;
      if (err.status === 403) {
        pushToast('Container actions are disabled. Set ALLOW_ACTIONS=true on the server.', 'error');
      } else {
        pushToast(err.message || 'Action failed', 'error');
      }
    } finally {
      loading = false;
      confirming = false;
    }
  }
  function cancel() { confirming = false; }

  $: variant = action === 'start' ? 'success' : action === 'stop' ? 'danger' : 'default';
</script>

<button class="action-btn {variant}" on:click={confirm} disabled={disabled || loading}>
  {#if loading}
    <span class="spinner"></span>
  {:else}
    {#if action === 'start'}
      <svg width="12" height="12" viewBox="0 0 24 24" fill="currentColor"><polygon points="5 3 19 12 5 21 5 3"/></svg>
    {:else if action === 'stop'}
      <svg width="12" height="12" viewBox="0 0 24 24" fill="currentColor"><rect x="4" y="4" width="16" height="16" rx="2"/></svg>
    {:else}
      <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><polyline points="23 4 23 10 17 10"/><path d="M20.49 15a9 9 0 1 1-2.12-9.36L23 10"/></svg>
    {/if}
    {action}
  {/if}
</button>

{#if confirming}
  <div class="confirm-overlay" on:click|self={cancel} on:keydown={e => e.key === 'Escape' && cancel()} role="dialog" tabindex="-1">
    <div class="confirm-card">
      <p>Are you sure you want to <strong>{action}</strong> <strong>{containerName}</strong>?</p>
      <div class="confirm-actions">
        <button class="btn-cancel" on:click={cancel}>Cancel</button>
        <button class="btn-confirm {variant}" on:click={doAction}>
          {#if loading}<span class="spinner"></span>{:else}Confirm {action}{/if}
        </button>
      </div>
    </div>
  </div>
{/if}

<style>
  .action-btn {
    display: inline-flex;
    align-items: center;
    gap: 5px;
    padding: 6px 12px;
    border-radius: var(--radius-sm, 6px);
    font-size: 12px;
    font-weight: 500;
    border: 1px solid var(--border, #1e2334);
    background: transparent;
    color: var(--text-secondary, #94a3b8);
    cursor: pointer;
    transition: all 150ms ease;
    text-transform: capitalize;
    white-space: nowrap;
  }
  .action-btn:hover:not(:disabled) {
    background: rgba(255,255,255,0.04);
    border-color: var(--text-muted, #64748b);
    color: var(--text, #e2e8f0);
  }
  .action-btn.success:hover:not(:disabled) {
    background: rgba(16, 185, 129, 0.1);
    border-color: rgba(16, 185, 129, 0.3);
    color: #10b981;
  }
  .action-btn.danger:hover:not(:disabled) {
    background: rgba(239, 68, 68, 0.1);
    border-color: rgba(239, 68, 68, 0.3);
    color: #ef4444;
  }
  .action-btn:disabled {
    opacity: 0.4;
    cursor: not-allowed;
  }

  .spinner {
    width: 12px;
    height: 12px;
    border: 1.5px solid rgba(255,255,255,0.2);
    border-top-color: currentColor;
    border-radius: 50%;
    animation: spin 0.5s linear infinite;
    display: inline-block;
  }
  @keyframes spin { to { transform: rotate(360deg); } }

  .confirm-overlay {
    position: fixed;
    inset: 0;
    background: rgba(0,0,0,0.5);
    backdrop-filter: blur(4px);
    display: flex;
    align-items: center;
    justify-content: center;
    z-index: 100;
  }
  .confirm-card {
    background: var(--surface, #12151e);
    border: 1px solid var(--border, #1e2334);
    border-radius: var(--radius, 10px);
    padding: 24px;
    max-width: 380px;
    width: 90%;
    box-shadow: 0 16px 48px rgba(0,0,0,0.4);
  }
  .confirm-card p {
    margin: 0 0 20px;
    font-size: 14px;
    line-height: 1.5;
    color: var(--text-secondary, #94a3b8);
  }
  .confirm-actions {
    display: flex;
    gap: 8px;
    justify-content: flex-end;
  }
  .btn-cancel {
    padding: 8px 16px;
    border-radius: var(--radius-sm, 6px);
    border: 1px solid var(--border, #1e2334);
    background: transparent;
    color: var(--text-secondary, #94a3b8);
    font-size: 13px;
    cursor: pointer;
  }
  .btn-cancel:hover { background: rgba(255,255,255,0.04); color: var(--text); }
  .btn-confirm {
    padding: 8px 16px;
    border-radius: var(--radius-sm, 6px);
    border: none;
    font-size: 13px;
    font-weight: 500;
    cursor: pointer;
    display: inline-flex;
    align-items: center;
    gap: 6px;
    background: var(--accent, #3b82f6);
    color: white;
  }
  .btn-confirm.success { background: #10b981; }
  .btn-confirm.danger { background: #ef4444; }
  .btn-confirm:hover { filter: brightness(1.1); }
</style>
