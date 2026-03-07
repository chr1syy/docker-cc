<script lang="ts">
  import { createEventDispatcher } from 'svelte';
  export let message: string = 'An error occurred';
  export let retryText: string = 'Retry';
  const dispatch = createEventDispatcher();
  function retry(){ dispatch('retry'); }
</script>

<div class="error-state">
  <div class="msg">{message}</div>
  <div class="controls">
    <button on:click={retry}>{retryText}</button>
  </div>
</div>

<style>
  .error-state{ padding:12px; border:1px solid var(--danger); background:var(--surface); color:var(--text); border-radius:8px }
  .msg{ margin-bottom:8px; color:var(--danger) }
  .controls button{ background:transparent;border:1px solid var(--border); padding:6px 10px; border-radius:6px }
</style>
