<script lang="ts">
  import { auth } from '$lib/stores/auth';
  import { goto } from '$app/navigation';
  import { onDestroy } from 'svelte';
  let username = '';
  let password = '';
  let totpCode = '';
  let unsub: any;
  let state: any;
  unsub = auth.subscribe(s => (state = s));
  onDestroy(() => unsub());

  let totpInputs: HTMLInputElement[] = [];
  let digits = ['', '', '', '', '', ''];

  function handleDigit(index: number, e: Event) {
    const input = e.target as HTMLInputElement;
    const val = input.value.replace(/\D/g, '');
    digits[index] = val.slice(-1);
    digits = [...digits];
    totpCode = digits.join('');

    if (val && index < 5) {
      totpInputs[index + 1]?.focus();
    }
    if (totpCode.length === 6) {
      submitTOTP();
    }
  }

  function handleKeydown(index: number, e: KeyboardEvent) {
    if (e.key === 'Backspace' && !digits[index] && index > 0) {
      totpInputs[index - 1]?.focus();
    }
  }

  function handlePaste(e: ClipboardEvent) {
    e.preventDefault();
    const text = (e.clipboardData?.getData('text') || '').replace(/\D/g, '').slice(0, 6);
    for (let i = 0; i < 6; i++) {
      digits[i] = text[i] || '';
    }
    digits = [...digits];
    totpCode = digits.join('');
    if (totpCode.length === 6) {
      submitTOTP();
    }
  }

  async function submit(e: Event) {
    e.preventDefault();
    const result = await auth.login(username, password);
    if (result === true) goto('/');
  }

  async function submitTOTP() {
    const ok = await auth.verifyTOTP(totpCode);
    if (ok) goto('/');
  }

  function backToLogin() {
    auth.cancelTOTP();
    digits = ['', '', '', '', '', ''];
    totpCode = '';
  }
</script>

<div class="page">
  {#if state && state.totpRequired}
    <!-- Phase 2: TOTP code entry -->
    <div class="login-card">
      <div class="login-logo">
        <div class="logo-icon"></div>
        <span>Docker CC</span>
      </div>
      <h2>Two-factor authentication</h2>
      <p class="login-subtitle">Enter the 6-digit code from your authenticator app</p>

      <div class="totp-inputs" on:paste={handlePaste}>
        {#each digits as digit, i}
          <input
            bind:this={totpInputs[i]}
            class="totp-digit"
            type="text"
            inputmode="numeric"
            maxlength="1"
            value={digit}
            on:input={(e) => handleDigit(i, e)}
            on:keydown={(e) => handleKeydown(i, e)}
            autocomplete="one-time-code"
          />
        {/each}
      </div>

      {#if state.error}
        <div class="error">{state.error}</div>
      {/if}

      <button class="submit-btn" on:click={submitTOTP} disabled={totpCode.length !== 6 || state.loading}>
        {#if state.loading}Verifying...{:else}Verify{/if}
      </button>
      <button class="back-btn" on:click={backToLogin}>Back to login</button>
    </div>
  {:else}
    <!-- Phase 1: Password -->
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
  {/if}
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

  /* TOTP input */
  .totp-inputs {
    display: flex;
    gap: 8px;
    justify-content: center;
    margin-bottom: 24px;
  }
  .totp-digit {
    width: 44px;
    height: 52px;
    text-align: center;
    font-size: 22px;
    font-weight: 700;
    font-family: 'SF Mono', 'Fira Code', monospace;
    background: var(--bg, #0b0e14);
    border: 1px solid var(--border, #1e2334);
    border-radius: 10px;
    color: var(--text);
    outline: none;
    transition: all 150ms ease;
    caret-color: var(--accent);
  }
  .totp-digit:focus {
    border-color: var(--accent, #3b82f6);
    box-shadow: 0 0 0 3px rgba(59, 130, 246, 0.15);
  }
  .back-btn {
    width: 100%;
    padding: 10px 16px;
    border: 1px solid var(--border, #1e2334);
    border-radius: 8px;
    background: transparent;
    color: var(--text-secondary, #94a3b8);
    font-size: 13px;
    font-family: inherit;
    cursor: pointer;
    margin-top: 8px;
    transition: all 150ms ease;
  }
  .back-btn:hover { background: rgba(255,255,255,0.04); color: var(--text); }
</style>
