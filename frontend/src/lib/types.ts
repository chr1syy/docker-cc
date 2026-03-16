export interface Port {
  ip?: string | null;
  privatePort: number;
  publicPort?: number | null;
  type: string;
}

export interface Container {
  id: string;
  name: string;
  image: string;
  state: string;
  status: string;
  ports?: Port[];
  created?: string | number;
}

export interface NetworkInfo {
  IPAddress?: string;
  Gateway?: string;
  [key: string]: unknown;
}

export interface ContainerDetail extends Container {
  fullState?: Record<string, unknown>;
  config?: { Cmd?: string[]; Platform?: string; Env?: string[]; [key: string]: unknown };
  networkSettings?: { Networks?: Record<string, NetworkInfo> };
  mounts?: Array<Record<string, unknown>>;
  restartCount?: number;
}
