<script lang="ts">
  import toasts from '$lib/stores/toast';
</script>

<style>
  .toast-wrapper { position: fixed; right: 12px; bottom: 12px; display:flex; flex-direction:column; gap:8px; z-index:9999 }
  .toast { padding:8px 12px; border-radius:6px; color:white; min-width:160px }
  .success { background:#2d9a4a }
  .error { background:#d9534f }
  .info { background:#2f86d6 }
</style>

<div class="toast-wrapper">
  {#each $toasts as t (t.id)}
    <div class="toast {t.type}">{t.message}</div>
  {/each}
</div>
