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
</script>

<button on:click={confirm} disabled={disabled || loading}>
  {#if loading}Working...{:else}{action}{/if}
</button>

{#if confirming}
  <div class="confirm">
    <div>Are you sure you want to {action} {containerName}?</div>
    <button on:click={doAction}>Confirm</button>
    <button on:click={cancel}>Cancel</button>
  </div>
{/if}

<style>
  .confirm { background:var(--surface); padding:8px; border:1px solid var(--border); margin-top:8px }
</style>
