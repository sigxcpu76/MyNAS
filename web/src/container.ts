/**
 * Dependency Injection Container for the Hexagonal Frontend.
 * Wires concrete HTTP adapters into pure Application Services.
 */

import { AipClient } from './adapters/http/aipClient';
import {
  HttpAuthAdapter,
  HttpCatalogAdapter,
  HttpDiskAdapter,
  HttpDockerAdapter,
  HttpDockerAppAdapter,
  HttpIdentityAdapter,
  HttpKvmAdapter,
  HttpLxcAdapter,
  HttpPrerequisiteAdapter,
  HttpShareAdapter,
  HttpZfsAdapter,
} from './adapters/http/httpAdapters';
import {
  AuthService,
  CatalogService,
  DiskService,
  DockerAppService,
  DockerService,
  IdentityService,
  KvmService,
  LxcService,
  PrerequisiteService,
  ShareService,
  ZfsService,
} from './application/services';

export const aipClient = new AipClient('/v1');

export const authPort = new HttpAuthAdapter(aipClient);
export const prerequisitePort = new HttpPrerequisiteAdapter(aipClient);
export const diskPort = new HttpDiskAdapter(aipClient);
export const zfsPort = new HttpZfsAdapter(aipClient);
export const kvmPort = new HttpKvmAdapter(aipClient);
export const dockerPort = new HttpDockerAdapter(aipClient);
export const dockerAppPort = new HttpDockerAppAdapter(aipClient);
export const lxcPort = new HttpLxcAdapter(aipClient);
export const sharePort = new HttpShareAdapter(aipClient);
export const identityPort = new HttpIdentityAdapter(aipClient);
export const catalogPort = new HttpCatalogAdapter(aipClient);

export const services = {
  auth: new AuthService(authPort),
  prerequisites: new PrerequisiteService(prerequisitePort),
  disk: new DiskService(diskPort),
  zfs: new ZfsService(zfsPort),
  kvm: new KvmService(kvmPort),
  docker: new DockerService(dockerPort),
  apps: new DockerAppService(dockerAppPort),
  lxc: new LxcService(lxcPort),
  shares: new ShareService(sharePort),
  identity: new IdentityService(identityPort),
  catalog: new CatalogService(catalogPort),
};
