import { writable } from 'svelte/store';

type AuthState = {
  user: string | null;
  isAuthenticated: boolean;
  loading: boolean;
  error: string | null;
};

function createAuthStore() {
  const { subscribe, set, update } = writable<AuthState>({ user: null, isAuthenticated: false, loading: true, error: null });

  async function checkAuth() {
    update(s => ({ ...s, loading: true, error: null }));
    try {
      const res = await fetch('/api/auth/check');
      if (res.ok) {
        const body = await res.json();
        set({ user: body.username || null, isAuthenticated: true, loading: false, error: null });
        return true;
      }
      set({ user: null, isAuthenticated: false, loading: false, error: null });
      return false;
    } catch (err) {
      set({ user: null, isAuthenticated: false, loading: false, error: 'network error' });
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
      if (!res.ok) {
        const body = await res.json().catch(() => ({}));
        update(s => ({ ...s, loading: false, error: body.error || 'login failed' }));
        return false;
      }
      const body = await res.json();
      set({ user: body.username, isAuthenticated: true, loading: false, error: null });
      return true;
    } catch (e) {
      update(s => ({ ...s, loading: false, error: 'network error' }));
      return false;
    }
  }

  async function logout() {
    update(s => ({ ...s, loading: true, error: null }));
    try {
      await fetch('/api/logout', { method: 'POST' });
    } catch (e) {
      // ignore
    }
    set({ user: null, isAuthenticated: false, loading: false, error: null });
  }

  return {
    subscribe,
    checkAuth,
    login,
    logout,
  };
}

export const auth = createAuthStore();
