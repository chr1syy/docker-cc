<script lang="ts">
  import { auth } from '$lib/stores/auth';
  import { goto } from '$app/navigation';
  import { onDestroy } from 'svelte';
  let username = '';
  let password = '';
  let unsub: any;
  let state: any;
  unsub = auth.subscribe(s => (state = s));
  onDestroy(() => unsub());

  async function submit(e: Event) {
    e.preventDefault();
    const ok = await auth.login(username, password);
    if (ok) goto('/');
  }
</script>

<div class="page">
  <form class="login-card" on:submit|preventDefault={submit}>
    <div class="login-logo">
      <div class="logo-icon"></div>
      <span>Docker CC</span>
    </div>
    <h2>Sign in</h2>
    <p class="login-subtitle">Enter your credentials to continue</p>

    <label class="field">
      <span class="field-label">Username</span>
      <input bind:value={username} required autocomplete="username" />
    </label>
    <label class="field">
      <span class="field-label">Password</span>
      <input type="password" bind:value={password} required autocomplete="current-password" />
    </label>

    {#if state && state.error}
      <div class="error">{state.error}</div>
    {/if}

    <button type="submit" class="submit-btn" disabled={state && state.loading}>
      {#if state && state.loading}Signing in...{:else}Sign in{/if}
    </button>
  </form>
</div>

<style>
  .page {
    display: flex;
    align-items: center;
    justify-content: center;
    min-height: 100vh;
    background: var(--bg, #0b0e14);
    padding: 20px;
  }
  .login-card {
    background: var(--surface, #12151e);
    border: 1px solid var(--border, #1e2334);
    padding: 40px;
    border-radius: 14px;
    width: 100%;
    max-width: 380px;
    box-shadow: 0 16px 48px rgba(0,0,0,0.3);
  }
  .login-logo {
    display: flex;
    align-items: center;
    gap: 10px;
    font-weight: 800;
    font-size: 16px;
    margin-bottom: 32px;
    color: var(--text);
  }
  .logo-icon {
    width: 28px;
    height: 28px;
    border-radius: 8px;
    background: linear-gradient(135deg, var(--accent, #3b82f6), #8b5cf6);
  }
  h2 {
    margin: 0 0 4px;
    font-size: 1.25rem;
  }
  .login-subtitle {
    color: var(--text-muted, #64748b);
    font-size: 13px;
    margin: 0 0 24px;
  }
  .field {
    display: block;
    margin-bottom: 16px;
  }
  .field-label {
    display: block;
    font-size: 12px;
    font-weight: 500;
    color: var(--text-secondary, #94a3b8);
    margin-bottom: 6px;
  }
  .field input {
    width: 100%;
    padding: 10px 14px;
    background: var(--bg, #0b0e14);
    border: 1px solid var(--border, #1e2334);
    border-radius: 8px;
    color: var(--text);
    font-size: 14px;
    font-family: inherit;
    outline: none;
    transition: all 150ms ease;
  }
  .field input:focus {
    border-color: var(--accent, #3b82f6);
    box-shadow: 0 0 0 3px rgba(59, 130, 246, 0.1);
  }
  .error {
    background: rgba(239, 68, 68, 0.1);
    color: #f87171;
    padding: 10px 14px;
    border-radius: 8px;
    font-size: 13px;
    margin-bottom: 16px;
  }
  .submit-btn {
    width: 100%;
    padding: 11px 16px;
    border: none;
    border-radius: 8px;
    background: var(--accent, #3b82f6);
    color: white;
    font-size: 14px;
    font-weight: 600;
    font-family: inherit;
    cursor: pointer;
    transition: all 150ms ease;
  }
  .submit-btn:hover:not(:disabled) { background: var(--accent-hover, #2563eb); }
  .submit-btn:disabled { opacity: 0.6; cursor: not-allowed; }
</style>
