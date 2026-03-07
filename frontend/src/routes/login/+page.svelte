<script lang="ts">
  import { auth } from '$lib/stores/auth';
  import { goto } from '$app/navigation';
  import { onDestroy } from 'svelte';
  let username = '';
  let password = '';
  let unsub: any;
  let state;
  unsub = auth.subscribe(s => (state = s));
  onDestroy(() => unsub());

  async function submit(e: Event) {
    e.preventDefault();
    const ok = await auth.login(username, password);
    if (ok) goto('/');
  }
</script>

<style>
  .page {
    display: flex;
    align-items: center;
    justify-content: center;
    height: 100vh;
    background: linear-gradient(180deg,#0f1724,#071025);
    color: #e6eef8;
  }
  .card {
    background: #0b1220;
    padding: 2rem;
    border-radius: 8px;
    width: 320px;
    box-shadow: 0 6px 18px rgba(2,6,23,0.6);
  }
  input { width: 100%; padding: 0.5rem; margin-top: 0.5rem; border-radius: 4px; border: 1px solid #223; background: #08101a; color: #fff }
  button { width: 100%; padding: 0.5rem; margin-top: 1rem; border-radius: 4px; background: #1f6feb; color: white; border: none }
  .error { color: #ff7b7b; margin-top: 0.5rem }
</style>

<div class="page">
  <form class="card" on:submit|preventDefault={submit}>
    <h2>Sign in</h2>
    <label>Username
      <input bind:value={username} required />
    </label>
    <label>Password
      <input type="password" bind:value={password} required />
    </label>
    {#if state && state.error}
      <div class="error">{state.error}</div>
    {/if}
    <button type="submit">Sign in</button>
  </form>
</div>
