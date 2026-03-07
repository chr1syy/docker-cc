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

export interface ContainerDetail extends Container {
  fullState?: any;
  config?: any;
  networkSettings?: any;
  mounts?: Array<any>;
  restartCount?: number;
}
