import { describe, expect, it, vi } from 'vitest';
import { mount } from '@vue/test-utils';
import { HttpCronJobAdapter } from '../src/adapters/http/httpAdapters';
import type { AipClient } from '../src/adapters/http/aipClient';
import CronJobsView from '../src/ui/views/CronJobsView.vue';
import { nasState } from '../src/ui/store';
import type { CronJob } from '../src/domain/models';

describe('CronJobs Domain & Adapter', () => {
  it('maps cron job and handles snake_case / camelCase serialization', async () => {
    const mockClient = {
      get: vi.fn().mockResolvedValue({
        cron_jobs: [
          {
            name: 'cronJobs/rclone-pictures',
            id: 'rclone-pictures',
            title: 'Rclone Google Drive Backup',
            description: 'Nightly backup of pictures',
            schedule: '0 23 * * *',
            image: 'rclone/rclone:1',
            args: ['sync', '/data/library/pictures', 'google_drive:/Backups/Pictures'],
            enabled: true,
            concurrency_policy: 'Forbid',
            restart_policy: 'Never',
            timeout_seconds: 3600,
            state: 'IDLE',
            volumes: [
              { host_path: '/data/library', mount_path: '/data/library', read_only: false },
              { host_path: '/config/rclone', mount_path: '/config/rclone', read_only: true },
            ],
            last_run_status: 'SUCCESS',
            last_run_duration: '45.2s',
            last_run_exit_code: 0,
          },
        ],
      }),
      post: vi.fn().mockResolvedValue({
        name: 'cronJobs/rclone-pictures',
        id: 'rclone-pictures',
      }),
      patch: vi.fn().mockResolvedValue({
        name: 'cronJobs/rclone-pictures',
        id: 'rclone-pictures',
      }),
      delete: vi.fn().mockResolvedValue({}),
      customMethod: vi.fn().mockResolvedValue({
        run_id: 'rclone-pictures-12345',
        status: 'SUCCESS',
        duration: '10.5s',
      }),
    } as unknown as AipClient;

    const adapter = new HttpCronJobAdapter(mockClient);
    const jobs = await adapter.listCronJobs();

    expect(jobs).toHaveLength(1);
    const j = jobs[0];
    expect(j.id).toBe('rclone-pictures');
    expect(j.title).toBe('Rclone Google Drive Backup');
    expect(j.schedule).toBe('0 23 * * *');
    expect(j.image).toBe('rclone/rclone:1');
    expect(j.args).toEqual(['sync', '/data/library/pictures', 'google_drive:/Backups/Pictures']);
    expect(j.volumes).toHaveLength(2);
    expect(j.volumes![0].hostPath).toBe('/data/library');
    expect(j.volumes![1].readOnly).toBe(true);
    expect(j.lastRunStatus).toBe('SUCCESS');
    expect(j.lastRunDuration).toBe('45.2s');
    expect(j.timeoutSeconds).toBe(3600);

    // Test createCronJob
    await adapter.createCronJob('rclone-pictures', {
      title: 'Sync Pictures',
      schedule: '0 23 * * *',
      image: 'rclone/rclone:1',
      timeoutSeconds: 7200,
      args: ['sync', '/data/library/pictures', 'google_drive:/Backups/Pictures'],
      volumes: [{ hostPath: '/mnt/storage/library', mountPath: '/data/library', readOnly: true }],
      env: [{ name: 'RCLONE_CONFIG_DRIVE_TYPE', value: 'drive' }],
    });

    expect(mockClient.post).toHaveBeenCalledWith(
      'cronJobs',
      expect.objectContaining({
        name: 'cronJobs/rclone-pictures',
        id: 'rclone-pictures',
        title: 'Sync Pictures',
        schedule: '0 23 * * *',
        image: 'rclone/rclone:1',
        timeoutSeconds: 7200,
        args: ['sync', '/data/library/pictures', 'google_drive:/Backups/Pictures'],
        volumes: [{ hostPath: '/mnt/storage/library', mountPath: '/data/library', readOnly: true }],
        env: [{ name: 'RCLONE_CONFIG_DRIVE_TYPE', value: 'drive' }],
      }),
      { cronJobId: 'rclone-pictures' }
    );

    // Test updateCronJob
    await adapter.updateCronJob('rclone-pictures', {
      title: 'Sync Pictures Updated',
    }, ['title']);

    expect(mockClient.patch).toHaveBeenCalledWith(
      'cronJobs/rclone-pictures',
      expect.objectContaining({
        name: 'cronJobs/rclone-pictures',
        id: 'rclone-pictures',
        title: 'Sync Pictures Updated',
      }),
      ['title']
    );
  });

  it('renders CronJobsView with list of tasks and formatted schedule', async () => {
    const testJob: CronJob = {
      name: 'cronJobs/rclone-pictures',
      id: 'rclone-pictures',
      title: 'Nightly Rclone Picture Sync',
      description: 'Backs up local pictures to Google Drive',
      schedule: '0 23 * * *',
      image: 'rclone/rclone:1',
      args: ['sync', '/data/library/pictures', 'google_drive:/Backups/Pictures'],
      enabled: true,
      concurrencyPolicy: 'Forbid',
      restartPolicy: 'Never',
      timeoutSeconds: 7200,
      state: 'IDLE',
      lastRunStatus: 'SUCCESS',
      lastRunDuration: '1m 12s',
      nextScheduleTime: new Date(Date.now() + 3600000).toISOString(),
    };

    nasState.cronJobs = [testJob];

    const wrapper = mount(CronJobsView);
    expect(wrapper.text()).toContain('Scheduled Tasks');
    expect(wrapper.text()).toContain('Nightly Rclone Picture Sync');
    expect(wrapper.text()).toContain('Daily at 23:00');
    expect(wrapper.text()).toContain('0 23 * * *');
    expect(wrapper.text()).toContain('2h');
    expect(wrapper.text()).toContain('rclone/rclone:1');
    expect(wrapper.text()).toContain('SUCCESS');
  });

  it('renders RUNNING state with disabled running button when a task is active', async () => {
    const runningJob: CronJob = {
      name: 'cronJobs/active-task',
      id: 'active-task',
      title: 'Active Backup Task',
      description: 'Currently running backup',
      schedule: '0 2 * * *',
      image: 'rclone/rclone:1',
      args: ['sync', '/data', 'remote:/data'],
      enabled: true,
      concurrencyPolicy: 'Forbid',
      restartPolicy: 'Never',
      state: 'RUNNING',
      lastRunStatus: 'RUNNING',
      lastScheduleTime: new Date(Date.now() - 45000).toISOString(),
    };

    nasState.cronJobs = [runningJob];

    const wrapper = mount(CronJobsView);
    expect(wrapper.text()).toContain('Active Backup Task');
    expect(wrapper.text()).toContain('RUNNING');
    expect(wrapper.text()).toContain('Running...');
    const btn = wrapper.find('button[disabled]');
    expect(btn.exists()).toBe(true);
    expect(btn.text()).toContain('Running...');
  });
});
