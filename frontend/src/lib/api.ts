import type { Container, ContainerDetail } from './types';

async function fetchAPI(path: string, opts: RequestInit = {}) {
  const res = await fetch(path, opts);
  if (!res.ok) {
    const text = await res.text().catch(() => '');
    const err: any = new Error(res.statusText || 'API Error');
    err.status = res.status;
    err.body = text;
    throw err;
  }
  const contentType = res.headers.get('content-type') || '';
  if (contentType.includes('application/json')) return res.json();
  return res.text();
}

export async function getContainers(): Promise<Container[]> {
  return fetchAPI('/api/containers') as Promise<Container[]>;
}

export async function getContainer(id: string): Promise<ContainerDetail> {
  return fetchAPI(`/api/containers/${encodeURIComponent(id)}`) as Promise<ContainerDetail>;
}

export { fetchAPI };

export async function startContainer(id: string) {
  return fetchAPI(`/api/containers/${encodeURIComponent(id)}/start`, { method: 'POST' });
}

export async function stopContainer(id: string) {
  return fetchAPI(`/api/containers/${encodeURIComponent(id)}/stop`, { method: 'POST' });
}

export async function restartContainer(id: string) {
  return fetchAPI(`/api/containers/${encodeURIComponent(id)}/restart`, { method: 'POST' });
}

export async function removeContainer(id: string) {
  return fetchAPI(`/api/containers/${encodeURIComponent(id)}`, { method: 'DELETE' });
}
