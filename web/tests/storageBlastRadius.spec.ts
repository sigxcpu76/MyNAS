import { describe, it, expect, beforeEach } from 'vitest';
import { computeStorageBlastRadius } from '../src/ui/utils/storageBlastRadius';
import { nasState } from '../src/ui/store';

describe('Storage Blast Radius Engine', () => {
  beforeEach(() => {
    nasState.systemConfig = {
      name: 'system/config',
      mainPool: 'zfsPools/tank',
      timezone: 'UTC',
      hostname: 'nas01',
    } as any;
    nasState.vms = [
      {
        name: 'virtualMachines/win10',
        vmName: 'win10',
        disks: [{ path: '/dev/zvol/tank/vms/win10-disk', bus: 'virtio', readOnly: false, bootOrder: 1, sizeBytes: 10737418240 }],
        state: 'RUNNING',
        cpuCores: 4,
        memoryBytes: 4294967296,
        nics: [],
      },
    ] as any;
    nasState.lxcContainers = [
      {
        name: 'lxcContainers/alpine',
        containerName: 'alpine',
        rootfs: '/mnt/tank/containers/alpine/rootfs',
        state: 'RUNNING',
        cpuCores: 2,
        memoryBytes: 1073741824,
        autostart: true,
        mounts: [],
      },
    ] as any;
    nasState.dockerApps = [
      {
        name: 'dockerApps/nextcloud',
        appId: 'nextcloud',
        title: 'Nextcloud Hub',
        state: 'RUNNING',
        composeYaml: 'services:\n  app:\n    image: nextcloud\n    volumes:\n      - /mnt/tank/apps/nextcloud:/var/www/html\n',
      },
    ] as any;
    nasState.cifsShares = [
      {
        name: 'cifsShares/documents',
        id: 'documents',
        path: '/mnt/tank/shares/documents',
        comment: '',
        browseable: true,
        readOnly: false,
        guestOk: false,
        validUsers: [],
        validGroups: [],
        createMask: '0664',
        directoryMask: '0775',
        forceCreateMode: '0000',
        forceDirectoryMode: '0000',
        shadowCopy: true,
        timeMachine: false,
        recycleBin: false,
        enabled: true,
      },
    ] as any;
    nasState.webdavShares = [
      {
        name: 'webdavShares/media',
        id: 'media',
        path: '/mnt/tank/shares/media',
        description: '',
        readOnly: true,
        enabled: true,
      },
    ] as any;
  });

  it('detects main pool and all dependent workloads and shares', () => {
    const blast = computeStorageBlastRadius('tank');
    expect(blast.isMainPool).toBe(true);
    expect(blast.hasDependencies).toBe(true);
    expect(blast.affectedVms).toContain('win10');
    expect(blast.affectedLxc).toContain('alpine');
    expect(blast.affectedApps).toContain('Nextcloud Hub');
    expect(blast.affectedCifs).toContain('documents');
    expect(blast.affectedWebdav).toContain('media');
    expect(blast.summaryWarning).toContain('MAIN SYSTEM POOL');
  });

  it('detects specific dataset dependencies accurately', () => {
    const blast = computeStorageBlastRadius('tank/apps/nextcloud');
    expect(blast.isMainPool).toBe(false);
    expect(blast.affectedApps).toContain('Nextcloud Hub');
    expect(blast.affectedVms.length).toBe(0);
  });

  it('returns clean result when target has no dependencies', () => {
    const blast = computeStorageBlastRadius('otherpool');
    expect(blast.isMainPool).toBe(false);
    expect(blast.hasDependencies).toBe(false);
    expect(blast.affectedVms.length).toBe(0);
    expect(blast.affectedApps.length).toBe(0);
    expect(blast.summaryWarning).toBe('');
  });

  it('detects custom mountpoint datasets, standalone docker containers, and cron jobs', () => {
    nasState.datasetsByPool['zfsPools/tank'] = [
      {
        name: 'zfsPools/tank/datasets/backups',
        datasetPath: 'tank/backups',
        mountpoint: '/srv/custom-backups',
      } as any,
    ];
    nasState.dockerContainers = [
      {
        containerId: 'abc',
        containerName: 'restic-worker',
        volumes: ['/srv/custom-backups:/data'],
      } as any,
    ];
    nasState.cronJobs = [
      {
        name: 'cronJobs/nightly-sync',
        title: 'Nightly Sync',
        volumes: [{ hostPath: '/srv/custom-backups/daily', mountPath: '/backup' }],
      } as any,
    ];

    const blast = computeStorageBlastRadius('tank/backups');
    expect(blast.hasDependencies).toBe(true);
    expect(blast.affectedContainers).toContain('restic-worker');
    expect(blast.affectedCronJobs).toContain('Nightly Sync');
  });
});
