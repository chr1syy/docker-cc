<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import { auth } from '$lib/stores/auth';
  import { fetchAPI } from '$lib/api';
  import { pushToast } from '$lib/stores/toast';

  let state: any;
  const unsub = auth.subscribe(s => (state = s));
  onDestroy(() => unsub());

  let totpEnabled = false;
  let setupURI = '';
  let setupSecret = '';
  let setupStep: 'idle' | 'scanning' | 'confirming' = 'idle';
  let confirmCode = '';
  let disableCode = '';
  let loading = false;
  let qrDataUrl = '';

  let confirmInputs: HTMLInputElement[] = [];
  let confirmDigits = ['', '', '', '', '', ''];
  let disableInputs: HTMLInputElement[] = [];
  let disableDigits = ['', '', '', '', '', ''];

  function handleDigits(digits: string[], inputs: HTMLInputElement[], index: number, e: Event, onComplete: () => void) {
    const input = e.target as HTMLInputElement;
    const val = input.value.replace(/\D/g, '');
    digits[index] = val.slice(-1);
    const code = digits.join('');
    if (val && index < 5) inputs[index + 1]?.focus();
    if (code.length === 6) onComplete();
    return digits;
  }

  function handleKeydown(digits: string[], inputs: HTMLInputElement[], index: number, e: KeyboardEvent) {
    if (e.key === 'Backspace' && !digits[index] && index > 0) {
      inputs[index - 1]?.focus();
    }
  }

  async function loadStatus() {
    try {
      const res = await fetch('/api/auth/2fa/status');
      if (res.ok) {
        const body = await res.json();
        totpEnabled = body.enabled;
      }
    } catch (e) {
      // ignore
    }
  }

  async function startSetup() {
    loading = true;
    try {
      const res = await fetch('/api/auth/2fa/setup', { method: 'POST' });
      const body = await res.json();
      if (!body.ok) {
        pushToast(body.error || 'Setup failed', 'error');
        loading = false;
        return;
      }
      setupURI = body.uri;
      setupSecret = body.secret;
      setupStep = 'scanning';

      // Generate QR code
      const QRCode = await import('qrcode');
      qrDataUrl = await QRCode.toDataURL(setupURI, {
        width: 200,
        margin: 2,
        color: { dark: '#e2e8f0', light: '#12151e' },
      });
    } catch (e) {
      pushToast('Failed to start 2FA setup', 'error');
    } finally {
      loading = false;
    }
  }

  async function confirmSetup() {
    confirmCode = confirmDigits.join('');
    if (confirmCode.length !== 6) return;
    loading = true;
    try {
      const res = await fetch('/api/auth/2fa/confirm', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ code: confirmCode }),
      });
      const body = await res.json();
      if (!body.ok) {
        pushToast(body.error || 'Invalid code', 'error');
        loading = false;
        return;
      }
      pushToast('Two-factor authentication enabled', 'success');
      totpEnabled = true;
      setupStep = 'idle';
      setupURI = '';
      setupSecret = '';
      confirmDigits = ['', '', '', '', '', ''];
      confirmCode = '';
      qrDataUrl = '';
      await auth.checkAuth();
    } catch (e) {
      pushToast('Failed to confirm', 'error');
    } finally {
      loading = false;
    }
  }

  async function disable2FA() {
    disableCode = disableDigits.join('');
    if (disableCode.length !== 6) return;
    loading = true;
    try {
      const res = await fetch('/api/auth/2fa/disable', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ code: disableCode }),
      });
      const body = await res.json();
      if (!body.ok) {
        pushToast(body.error || 'Invalid code', 'error');
        loading = false;
        return;
      }
      pushToast('Two-factor authentication disabled', 'success');
      totpEnabled = false;
      disableDigits = ['', '', '', '', '', ''];
      disableCode = '';
      await auth.checkAuth();
    } catch (e) {
      pushToast('Failed to disable 2FA', 'error');
    } finally {
      loading = false;
    }
  }

  function cancelSetup() {
    setupStep = 'idle';
    setupURI = '';
    setupSecret = '';
    confirmDigits = ['', '', '', '', '', ''];
    confirmCode = '';
    qrDataUrl = '';
  }

  onMount(() => { loadStatus(); });
</script>

<div class="page-header">
  <h1>Settings</h1>
  <p class="subtitle">Manage your account and security settings</p>
</div>

<div class="card section">
  <h3>Two-Factor Authentication</h3>

  {#if totpEnabled && setupStep === 'idle'}
    <div class="status-row">
      <div class="status-indicator enabled">
        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5"><path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z"/></svg>
        <span>2FA is enabled</span>
      </div>
    </div>
    <p class="help-text">Your account is protected with two-factor authentication using an authenticator app.</p>

    <div class="disable-section">
      <p class="field-label">Enter your authenticator code to disable 2FA:</p>
      <div class="totp-inputs">
        {#each disableDigits as digit, i}
          <input
            bind:this={disableInputs[i]}
            class="totp-digit"
            type="text"
            inputmode="numeric"
            maxlength="1"
            value={digit}
            on:input={(e) => { disableDigits = [...handleDigits(disableDigits, disableInputs, i, e, disable2FA)]; }}
            on:keydown={(e) => handleKeydown(disableDigits, disableInputs, i, e)}
          />
        {/each}
      </div>
      <button class="btn-danger" on:click={disable2FA} disabled={loading || disableDigits.join('').length !== 6}>
        {#if loading}Disabling...{:else}Disable 2FA{/if}
      </button>
    </div>

  {:else if setupStep === 'idle'}
    <div class="status-row">
      <div class="status-indicator disabled">
        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z"/></svg>
        <span>2FA is not enabled</span>
      </div>
    </div>
    <p class="help-text">Add an extra layer of security by requiring a code from your authenticator app when you sign in.</p>
    <button class="btn-primary" on:click={startSetup} disabled={loading}>
      {#if loading}Setting up...{:else}Enable 2FA{/if}
    </button>

  {:else if setupStep === 'scanning'}
    <div class="setup-flow">
      <div class="step-header">
        <span class="step-number">1</span>
        <span>Scan QR code with your authenticator app</span>
      </div>

      {#if qrDataUrl}
        <div class="qr-container">
          <img src={qrDataUrl} alt="TOTP QR Code" width="200" height="200" />
        </div>
      {/if}

      <div class="manual-entry">
        <p class="field-label">Can't scan? Enter this key manually:</p>
        <code class="secret-key">{setupSecret}</code>
      </div>

      <div class="step-header" style="margin-top:24px">
        <span class="step-number">2</span>
        <span>Enter the 6-digit code from your app to confirm</span>
      </div>

      <div class="totp-inputs">
        {#each confirmDigits as digit, i}
          <input
            bind:this={confirmInputs[i]}
            class="totp-digit"
            type="text"
            inputmode="numeric"
            maxlength="1"
            value={digit}
            on:input={(e) => { confirmDigits = [...handleDigits(confirmDigits, confirmInputs, i, e, confirmSetup)]; }}
            on:keydown={(e) => handleKeydown(confirmDigits, confirmInputs, i, e)}
          />
        {/each}
      </div>

      <div class="setup-actions">
        <button class="btn-secondary" on:click={cancelSetup}>Cancel</button>
        <button class="btn-primary" on:click={confirmSetup} disabled={loading || confirmDigits.join('').length !== 6}>
          {#if loading}Verifying...{:else}Confirm & Enable{/if}
        </button>
      </div>
    </div>
  {/if}
</div>

<style>
  .page-header { margin-bottom: 24px; }
  .page-header h1 { margin: 0; }
  .subtitle { color: var(--text-muted); font-size: 13px; margin-top: 4px; }

  .section { margin-bottom: 16px; }

  .status-row { margin-bottom: 12px; }
  .status-indicator {
    display: inline-flex;
    align-items: center;
    gap: 8px;
    padding: 8px 14px;
    border-radius: 8px;
    font-size: 13px;
    font-weight: 500;
  }
  .status-indicator.enabled {
    background: var(--success-soft, rgba(16,185,129,0.1));
    color: var(--success, #10b981);
  }
  .status-indicator.disabled {
    background: rgba(255,255,255,0.04);
    color: var(--text-muted, #64748b);
  }

  .help-text {
    color: var(--text-secondary, #94a3b8);
    font-size: 13px;
    line-height: 1.6;
    margin-bottom: 20px;
  }

  .btn-primary {
    padding: 10px 20px;
    border: none;
    border-radius: 8px;
    background: var(--accent, #3b82f6);
    color: white;
    font-size: 13px;
    font-weight: 600;
    font-family: inherit;
    cursor: pointer;
    transition: all 150ms ease;
  }
  .btn-primary:hover:not(:disabled) { background: var(--accent-hover, #2563eb); }
  .btn-primary:disabled { opacity: 0.5; cursor: not-allowed; }

  .btn-secondary {
    padding: 10px 20px;
    border: 1px solid var(--border);
    border-radius: 8px;
    background: transparent;
    color: var(--text-secondary);
    font-size: 13px;
    font-family: inherit;
    cursor: pointer;
    transition: all 150ms ease;
  }
  .btn-secondary:hover { background: rgba(255,255,255,0.04); color: var(--text); }

  .btn-danger {
    padding: 10px 20px;
    border: none;
    border-radius: 8px;
    background: var(--danger, #ef4444);
    color: white;
    font-size: 13px;
    font-weight: 600;
    font-family: inherit;
    cursor: pointer;
    transition: all 150ms ease;
  }
  .btn-danger:hover:not(:disabled) { filter: brightness(1.1); }
  .btn-danger:disabled { opacity: 0.5; cursor: not-allowed; }

  /* Setup flow */
  .setup-flow { }
  .step-header {
    display: flex;
    align-items: center;
    gap: 12px;
    font-size: 14px;
    font-weight: 500;
    margin-bottom: 16px;
    color: var(--text);
  }
  .step-number {
    width: 28px;
    height: 28px;
    border-radius: 50%;
    background: var(--accent-soft, rgba(59,130,246,0.1));
    color: var(--accent, #3b82f6);
    display: flex;
    align-items: center;
    justify-content: center;
    font-size: 13px;
    font-weight: 700;
    flex-shrink: 0;
  }

  .qr-container {
    display: flex;
    justify-content: center;
    padding: 20px;
    background: var(--bg, #0b0e14);
    border: 1px solid var(--border);
    border-radius: 12px;
    margin-bottom: 16px;
  }
  .qr-container img { border-radius: 8px; }

  .manual-entry {
    margin-bottom: 8px;
  }
  .field-label {
    font-size: 12px;
    color: var(--text-muted);
    margin-bottom: 8px;
  }
  .secret-key {
    display: block;
    padding: 10px 14px;
    background: var(--bg, #0b0e14);
    border: 1px solid var(--border);
    border-radius: 8px;
    font-family: 'SF Mono', 'Fira Code', monospace;
    font-size: 14px;
    letter-spacing: 2px;
    color: var(--text);
    word-break: break-all;
    user-select: all;
  }

  .totp-inputs {
    display: flex;
    gap: 8px;
    justify-content: center;
    margin-bottom: 20px;
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

  .setup-actions {
    display: flex;
    gap: 8px;
    justify-content: flex-end;
  }

  .disable-section {
    margin-top: 20px;
    padding-top: 20px;
    border-top: 1px solid var(--border);
  }
</style>
