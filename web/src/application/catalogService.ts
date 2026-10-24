import type { CatalogApp } from '../domain/models';
import type { CatalogPort } from '../ports/outbound';

export class CatalogService {
  constructor(private readonly port: CatalogPort) {}

  async listApps(train?: string, category?: string, query?: string): Promise<{ apps: CatalogApp[]; totalSize: number; lastSyncTime?: string }> {
    return this.port.listApps(train, category, query);
  }

  async getApp(appId: string, train?: string): Promise<CatalogApp> {
    return this.port.getApp(appId, train);
  }

  async syncCatalog(force = false): Promise<{ success: boolean; message: string; totalApps: number; lastSyncTime: string }> {
    return this.port.syncCatalog(force);
  }
}
