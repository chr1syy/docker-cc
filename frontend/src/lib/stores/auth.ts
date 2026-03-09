import { writable } from 'svelte/store';

type AuthState = {
  user: string | null;
  isAuthenticated: boolean;
  loading: boolean;
  error: string | null;
  totpRequired: boolean;
  totpToken: string | null;
  totpEnabled: boolean;
};

function createAuthStore() {
  const { subscribe, set, update } = writable<AuthState>({
    user: null, isAuthenticated: false, loading: true, error: null,
    totpRequired: false, totpToken: null, totpEnabled: false,
  });

  async function checkAuth() {
    update(s => ({ ...s, loading: true, error: null }));
    try {
      const res = await fetch('/api/auth/check');
      if (res.ok) {
        const body = await res.json();
        set({
          user: body.username || null, isAuthenticated: true, loading: false, error: null,
          totpRequired: false, totpToken: null, totpEnabled: !!body.totp_enabled,
        });
        return true;
      }
      set({ user: null, isAuthenticated: false, loading: false, error: null, totpRequired: false, totpToken: null, totpEnabled: false });
      return false;
    } catch (err) {
      set({ user: null, isAuthenticated: false, loading: false, error: 'network error', totpRequired: false, totpToken: null, totpEnabled: false });
      return false;
    }
  }

  async function login(username: string, password: string) {
    update(s => ({ ...s, loading: true, error: null }));
    try {
      const res = await fetch('/api/login', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ username, password }),
      });
      const body = await res.json().catch(() => ({}));

      if (body.requires_totp) {
        update(s => ({
          ...s, loading: false, error: null,
          totpRequired: true, totpToken: body.totp_token,
        }));
        return 'totp_required';
      }

      if (!res.ok) {
        update(s => ({ ...s, loading: false, error: body.error || 'login failed' }));
        return false;
      }

      set({
        user: body.username, isAuthenticated: true, loading: false, error: null,
        totpRequired: false, totpToken: null, totpEnabled: false,
      });
      return true;
    } catch (e) {
      update(s => ({ ...s, loading: false, error: 'network error' }));
      return false;
    }
  }

  async function verifyTOTP(code: string) {
    let token: string | null = null;
    const unsub = subscribe(s => { token = s.totpToken; });
    unsub();

    update(s => ({ ...s, loading: true, error: null }));
    try {
      const res = await fetch('/api/auth/totp/verify', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ token, code }),
      });
      const body = await res.json().catch(() => ({}));

      if (!res.ok) {
        update(s => ({ ...s, loading: false, error: body.error || 'invalid code' }));
        return false;
      }

      set({
        user: body.username, isAuthenticated: true, loading: false, error: null,
        totpRequired: false, totpToken: null, totpEnabled: true,
      });
      return true;
    } catch (e) {
      update(s => ({ ...s, loading: false, error: 'network error' }));
      return false;
    }
  }

  function cancelTOTP() {
    update(s => ({ ...s, totpRequired: false, totpToken: null, error: null }));
  }

  async function logout() {
    update(s => ({ ...s, loading: true, error: null }));
    try {
      await fetch('/api/logout', { method: 'POST' });
    } catch (e) {
      // ignore
    }
    set({ user: null, isAuthenticated: false, loading: false, error: null, totpRequired: false, totpToken: null, totpEnabled: false });
  }

  return {
    subscribe,
    checkAuth,
    login,
    verifyTOTP,
    cancelTOTP,
    logout,
  };
}

export const auth = createAuthStore();
