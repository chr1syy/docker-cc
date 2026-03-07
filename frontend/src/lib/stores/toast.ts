import { writable } from 'svelte/store';

export type Toast = { id: number; message: string; type: 'success' | 'error' | 'info' };

const toasts = writable<Toast[]>([]);
let nextId = 1;

export function pushToast(message: string, type: Toast['type'] = 'info') {
  const id = nextId++;
  toasts.update((t) => [...t, { id, message, type }]);
  setTimeout(() => {
    toasts.update((t) => t.filter((x) => x.id !== id));
  }, 4000);
}

export default toasts;
