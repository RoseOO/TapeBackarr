<script lang="ts">
  import { onMount } from 'svelte';
  import { api } from '$lib/api/client';

  interface ProxmoxGuest {
    vmid: number;
    name: string;
    type: string;
    status: string;
    node: string;
    maxmem: number;
    maxdisk: number;
  }

  interface ProxmoxBackup {
    id: number;
    vmid: number;
    vm_name: string;
    node: string;
    status: string;
    tape_label: string;
    start_time: string;
    end_time: string;
    file_size: number;
    compressed: boolean;
    encrypted: boolean;
  }

  interface ProxmoxJob {
    id: number;
    name: string;
    vmids: string;
    schedule_cron: string;
    pool_id: number;
    pool_name?: string;
    enabled: boolean;
    compression: string;
    backup_mode: string;
    retention_days: number;
    notify_on_success: boolean;
    notify_on_failure: boolean;
    notes: string;
    last_run_at: string;
  }

  let guests: ProxmoxGuest[] = [];
  let backups: ProxmoxBackup[] = [];
  let jobs: ProxmoxJob[] = [];
  let loading = true;
  let error = '';
  let successMsg = '';
  let proxmoxEnabled = false;
  let activeTab = 'guests';
  let showCreateJobModal = false;
  let showBackupModal = false;
  let showEditJobModal = false;
  let showRestoreModal = false;
  let editingJob: ProxmoxJob | null = null;
  let backupTarget: ProxmoxGuest | null = null;
  let backupForm = { pool_id: 0, mode: 'snapshot', compress: 'zstd' };
  let tapes: any[] = [];
  let pools: any[] = [];
  let drives: { id: number; device_path: string; display_name: string; vendor: string; model: string; status: string; enabled: boolean; current_tape: string }[] = [];
  let restoreTarget: ProxmoxBackup | null = null;
  let restoreStep: 'config' | 'running' | 'done' | 'error' = 'config';
  let restoreError = '';
  let restoreResult: any = null;
  let restoreForm = {
    drive_id: null as number | null,
    target_vmid: 0,
    storage: 'local',
    overwrite: false,
    start_after: false,
  };

  const defaultJobForm = {
    name: '',
    vmids: '',
    schedule_cron: '0 2 * * *',
    pool_id: 0,
    compression: 'zstd',
    backup_mode: 'snapshot',
    retention_days: 30,
    notify_on_success: false,
    notify_on_failure: true,
    notes: '',
  };

  let jobForm = { ...defaultJobForm };

  onMount(async () => {
    await loadData();
  });

  async function loadData() {
    loading = true;
    error = '';
    try {
      const [guestResult, backupResult, jobResult] = await Promise.allSettled([
        api.get('/proxmox/guests'),
        api.get('/proxmox/backups'),
        api.get('/proxmox/jobs'),
      ]);

      if (guestResult.status === 'fulfilled') {
        const data = guestResult.value;
        const vmList = Array.isArray(data?.vms) ? data.vms.map((v: any) => ({...v, type: v.type || 'qemu'})) : [];
        const lxcList = Array.isArray(data?.lxcs) ? data.lxcs.map((c: any) => ({...c, type: c.type || 'lxc'})) : [];
        guests = [...vmList, ...lxcList];
        proxmoxEnabled = true;
      }
      if (backupResult.status === 'fulfilled') {
        backups = Array.isArray(backupResult.value) ? backupResult.value : [];
      }
      if (jobResult.status === 'fulfilled') {
        jobs = Array.isArray(jobResult.value) ? jobResult.value : [];
      }

      if (guestResult.status === 'rejected') {
        const msg = guestResult.reason?.message || '';
        if (msg.includes('not configured') || msg.includes('not enabled') || msg.includes('disabled') || msg.includes('Proxmox integration')) {
          proxmoxEnabled = false;
        } else {
          error = msg || 'Failed to load Proxmox data';
        }
      }

      // Also load tapes, pools, and drives for backup/restore forms
      try {
        const [tapeResult, poolResult, driveResult] = await Promise.allSettled([
          api.get('/tapes'),
          api.get('/pools'),
          api.get('/drives'),
        ]);
        if (tapeResult.status === 'fulfilled') tapes = Array.isArray(tapeResult.value) ? tapeResult.value : [];
        if (poolResult.status === 'fulfilled') pools = Array.isArray(poolResult.value) ? poolResult.value : [];
        if (driveResult.status === 'fulfilled') {
          drives = (Array.isArray(driveResult.value) ? driveResult.value : []).filter((d: any) => d.enabled);
          if (drives.length > 0 && restoreForm.drive_id === null) {
            restoreForm.drive_id = drives[0].id;
          }
        }
      } catch {}
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to load data';
    } finally {
      loading = false;
    }
  }

  function openBackupModal(guest: ProxmoxGuest) {
    backupTarget = guest;
    backupForm = { pool_id: 0, mode: 'snapshot', compress: 'zstd' };
    showBackupModal = true;
  }

  async function handleBackupGuest() {
    if (!backupTarget) return;
    if (!backupForm.pool_id) {
      error = 'Please select a media pool for the backup';
      return;
    }
    try {
      await api.post('/proxmox/backups', {
        node: backupTarget.node,
        vmid: backupTarget.vmid,
        guest_type: backupTarget.type,
        guest_name: backupTarget.name,
        pool_id: backupForm.pool_id,
        backup_mode: backupForm.mode,
        compress: backupForm.compress,
      });
      showBackupModal = false;
      backupTarget = null;
      showSuccess('Backup started');
      await loadData();
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to start backup';
    }
  }

  function showSuccess(msg: string) {
    successMsg = msg;
    setTimeout(() => successMsg = '', 3000);
  }

  async function handleCreateJob() {
    try {
      await api.post('/proxmox/jobs', jobForm);
      showCreateJobModal = false;
      resetJobForm();
      showSuccess('Backup job created');
      await loadData();
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to create job';
    }
  }

  function resetJobForm() {
    jobForm = { ...defaultJobForm };
  }

  async function handleDeleteJob(id: number) {
    if (!confirm('Delete this Proxmox backup job?')) return;
    try {
      await api.delete(`/proxmox/jobs/${id}`);
      showSuccess('Job deleted');
      await loadData();
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to delete job';
    }
  }

  async function handleRunJob(id: number) {
    try {
      await api.post(`/proxmox/jobs/${id}/run`);
      showSuccess('Job started');
      await loadData();
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to run job';
    }
  }

  function openEditJobModal(job: ProxmoxJob) {
    editingJob = job;
    jobForm = {
      name: job.name,
      vmids: job.vmids || '',
      schedule_cron: job.schedule_cron || '0 2 * * *',
      pool_id: job.pool_id || 0,
      compression: job.compression || 'zstd',
      backup_mode: job.backup_mode || 'snapshot',
      retention_days: job.retention_days || 30,
      notify_on_success: job.notify_on_success || false,
      notify_on_failure: job.notify_on_failure !== false,
      notes: job.notes || '',
    };
    showEditJobModal = true;
  }

  async function handleUpdateJob() {
    if (!editingJob) return;
    try {
      await api.put(`/proxmox/jobs/${editingJob.id}`, { ...jobForm, enabled: editingJob.enabled });
      showEditJobModal = false;
      editingJob = null;
      resetJobForm();
      showSuccess('Job updated');
      await loadData();
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to update job';
    }
  }

  async function toggleJobEnabled(job: ProxmoxJob) {
    try {
      await api.put(`/proxmox/jobs/${job.id}`, { enabled: !job.enabled });
      showSuccess(job.enabled ? 'Job disabled' : 'Job enabled');
      await loadData();
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to toggle job';
    }
  }

  function formatBytes(bytes: number): string {
    if (!bytes || bytes === 0) return '0 B';
    const k = 1024;
    const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
  }

  function formatDate(d: string | null): string {
    if (!d) return '-';
    return new Date(d).toLocaleString();
  }

  function openRestoreModal(backup: ProxmoxBackup) {
    restoreTarget = backup;
    restoreForm = {
      drive_id: drives.length > 0 ? drives[0].id : null,
      target_vmid: backup.vmid,
      storage: 'local',
      overwrite: false,
      start_after: false,
    };
    restoreStep = 'config';
    restoreError = '';
    restoreResult = null;
    showRestoreModal = true;
  }

  async function executeProxmoxRestore() {
    if (!restoreTarget) return;
    restoreStep = 'running';
    restoreError = '';
    try {
      const payload: any = {
        backup_id: restoreTarget.id,
        target_vmid: restoreForm.target_vmid || undefined,
        storage: restoreForm.storage,
        overwrite: restoreForm.overwrite,
        start_after: restoreForm.start_after,
      };
      if (restoreForm.drive_id) {
        payload.drive_id = restoreForm.drive_id;
      }
      restoreResult = await api.post('/proxmox/restores', payload);
      restoreStep = 'done';
    } catch (e) {
      restoreError = e instanceof Error ? e.message : 'Restore failed';
      restoreStep = 'error';
    }
  }
</script>

<div class="page-header">
  <h1>🖥️ Proxmox Integration</h1>
  {#if proxmoxEnabled}
    <div style="display: flex; gap: 0.5rem;">
      <button class="btn btn-primary" on:click={() => { resetJobForm(); showCreateJobModal = true; }}>+ Create Backup Job</button>
    </div>
  {/if}
</div>

{#if error}
  <div class="card" style="background: var(--badge-danger-bg); color: var(--badge-danger-text); display: flex; justify-content: space-between; align-items: center;">
    <p style="margin:0">{error}</p>
    <button class="btn btn-secondary" on:click={() => error = ''}>×</button>
  </div>
{/if}

{#if successMsg}
  <div class="card" style="background: var(--badge-success-bg); color: var(--badge-success-text);">
    <p style="margin:0">{successMsg}</p>
  </div>
{/if}

{#if loading}
  <p>Loading...</p>
{:else if !proxmoxEnabled}
  <div class="card">
    <h2>Proxmox Not Configured</h2>
    <p>Proxmox VE integration is not enabled. To use this feature:</p>
    <ol>
      <li>Go to <a href="/settings">Settings</a></li>
      <li>Enable Proxmox integration</li>
      <li>Enter your Proxmox VE host, API token, and other settings</li>
      <li>Save and restart TapeBackarr</li>
    </ol>
    <p>Proxmox integration allows you to backup and restore VMs and LXC containers directly to tape.</p>
  </div>
{:else}
  <div class="tab-bar">
    <button class="tab" class:active={activeTab === 'guests'} on:click={() => activeTab = 'guests'}>
      VMs & Containers ({guests.length})
    </button>
    <button class="tab" class:active={activeTab === 'backups'} on:click={() => activeTab = 'backups'}>
      Backups ({backups.length})
    </button>
    <button class="tab" class:active={activeTab === 'jobs'} on:click={() => activeTab = 'jobs'}>
      Scheduled Jobs ({jobs.length})
    </button>
  </div>

  {#if activeTab === 'guests'}
    <div class="card">
      <table>
        <thead>
          <tr>
            <th>VMID</th>
            <th>Name</th>
            <th>Type</th>
            <th>Node</th>
            <th>Status</th>
            <th>Memory</th>
            <th>Disk</th>
            <th>Actions</th>
          </tr>
        </thead>
        <tbody>
          {#each guests as guest}
            <tr>
              <td><strong>{guest.vmid}</strong></td>
              <td>{guest.name}</td>
              <td><span class="badge badge-info">{guest.type}</span></td>
              <td>{guest.node}</td>
              <td>
                <span class="badge {guest.status === 'running' ? 'badge-success' : 'badge-warning'}">
                  {guest.status}
                </span>
              </td>
              <td>{formatBytes(guest.maxmem)}</td>
              <td>{formatBytes(guest.maxdisk)}</td>
              <td>
                <button class="btn btn-success" on:click={() => openBackupModal(guest)}>
                  Backup
                </button>
              </td>
            </tr>
          {/each}
          {#if guests.length === 0}
            <tr><td colspan="8" style="text-align:center; color: var(--text-muted);">No VMs or containers found</td></tr>
          {/if}
        </tbody>
      </table>
    </div>
  {/if}

  {#if activeTab === 'backups'}
    <div class="card">
      <table>
        <thead>
          <tr>
            <th>VMID</th>
            <th>Name</th>
            <th>Node</th>
            <th>Tape</th>
            <th>Size</th>
            <th>Status</th>
            <th>Started</th>
            <th>Completed</th>
            <th>Actions</th>
          </tr>
        </thead>
        <tbody>
          {#each backups as backup}
            <tr>
              <td>{backup.vmid}</td>
              <td>{backup.vm_name}</td>
              <td>{backup.node}</td>
              <td><code>{backup.tape_label || '-'}</code></td>
              <td>{formatBytes(backup.file_size)}</td>
              <td>
                <span class="badge {backup.status === 'completed' ? 'badge-success' : backup.status === 'running' ? 'badge-info' : 'badge-danger'}">
                  {backup.status}
                </span>
                {#if backup.compressed}<span class="badge badge-info" style="margin-left:0.25rem">compressed</span>{/if}
                {#if backup.encrypted}<span class="badge badge-warning" style="margin-left:0.25rem">encrypted</span>{/if}
              </td>
              <td>{formatDate(backup.start_time)}</td>
              <td>{formatDate(backup.end_time)}</td>
              <td>
                {#if backup.status === 'completed'}
                  <button class="btn btn-primary btn-sm" on:click={() => openRestoreModal(backup)}>
                    🔄 Restore
                  </button>
                {/if}
              </td>
            </tr>
          {/each}
          {#if backups.length === 0}
            <tr><td colspan="9" style="text-align:center; color: var(--text-muted);">No Proxmox backups found</td></tr>
          {/if}
        </tbody>
      </table>
    </div>
  {/if}

  {#if activeTab === 'jobs'}
    <div class="card">
      <table>
        <thead>
          <tr>
            <th>Name</th>
            <th>VMIDs</th>
            <th>Schedule</th>
            <th>Mode</th>
            <th>Pool</th>
            <th>Retention</th>
            <th>Status</th>
            <th>Last Run</th>
            <th>Actions</th>
          </tr>
        </thead>
        <tbody>
          {#each jobs as job}
            <tr>
              <td><strong>{job.name}</strong></td>
              <td><code>{job.vmids || 'All'}</code></td>
              <td><code>{job.schedule_cron || 'Manual'}</code></td>
              <td>{job.backup_mode || '-'}</td>
              <td>{job.pool_name || (job.pool_id ? `Pool #${job.pool_id}` : 'None')}</td>
              <td>{job.retention_days ? `${job.retention_days}d` : '-'}</td>
              <td>
                <span class="badge {job.enabled ? 'badge-success' : 'badge-danger'}">
                  {job.enabled ? 'Enabled' : 'Disabled'}
                </span>
              </td>
              <td>{formatDate(job.last_run_at)}</td>
              <td>
                <div style="display:flex;gap:0.25rem;flex-wrap:wrap;">
                  <button class="btn btn-success btn-sm" on:click={() => handleRunJob(job.id)}>▶ Run</button>
                  <button class="btn btn-secondary btn-sm" on:click={() => openEditJobModal(job)}>✏️ Edit</button>
                  <button class="btn btn-secondary btn-sm" on:click={() => toggleJobEnabled(job)}>
                    {job.enabled ? '⏸ Disable' : '▶ Enable'}
                  </button>
                  <button class="btn btn-danger btn-sm" on:click={() => handleDeleteJob(job.id)}>🗑️</button>
                </div>
              </td>
            </tr>
          {/each}
          {#if jobs.length === 0}
            <tr><td colspan="9" style="text-align:center; color: var(--text-muted);">No scheduled Proxmox jobs. Create one above.</td></tr>
          {/if}
        </tbody>
      </table>
    </div>
  {/if}
{/if}

{#if showCreateJobModal}
  <div class="modal-overlay" on:click={() => showCreateJobModal = false}>
    <div class="modal modal-lg" on:click|stopPropagation={() => {}}>
      <h2>Create Proxmox Backup Job</h2>
      <form on:submit|preventDefault={handleCreateJob}>
        <div class="form-group">
          <label for="pxjob-name">Job Name</label>
          <input type="text" id="pxjob-name" bind:value={jobForm.name} required placeholder="e.g., Nightly VM Backup" />
        </div>
        <div class="form-group">
          <label for="pxjob-vmids">VM IDs (comma-separated, leave empty for all)</label>
          <input type="text" id="pxjob-vmids" bind:value={jobForm.vmids} placeholder="e.g., 100,101,200" />
          <small style="color: var(--text-muted)">Leave empty to include all VMs and containers</small>
        </div>
        <div class="form-row">
          <div class="form-group">
            <label for="pxjob-schedule">Schedule (cron)</label>
            <input type="text" id="pxjob-schedule" bind:value={jobForm.schedule_cron} placeholder="0 2 * * *" />
            <small style="color: var(--text-muted)">Daily at 2 AM: <code>0 2 * * *</code></small>
          </div>
          <div class="form-group">
            <label for="pxjob-mode">Backup Mode</label>
            <select id="pxjob-mode" bind:value={jobForm.backup_mode}>
              <option value="snapshot">Snapshot (live, no downtime)</option>
              <option value="suspend">Suspend (brief pause)</option>
              <option value="stop">Stop (full shutdown)</option>
            </select>
          </div>
        </div>
        <div class="form-row">
          <div class="form-group">
            <label for="pxjob-pool">Media Pool</label>
            <select id="pxjob-pool" bind:value={jobForm.pool_id} required>
              <option value={0}>Select a pool...</option>
              {#each pools as pool}
                <option value={pool.id}>{pool.name}</option>
              {/each}
            </select>
            <small style="color: var(--text-muted)">Tapes are automatically selected from the pool</small>
          </div>
          <div class="form-group">
            <label for="pxjob-compression">Compression</label>
            <select id="pxjob-compression" bind:value={jobForm.compression}>
              <option value="none">None</option>
              <option value="gzip">Gzip</option>
              <option value="zstd">Zstd (recommended)</option>
            </select>
          </div>
        </div>
        <div class="form-row">
          <div class="form-group">
            <label for="pxjob-retention">Retention (days)</label>
            <input type="number" id="pxjob-retention" bind:value={jobForm.retention_days} min="1" max="3650" />
          </div>
          <div class="form-group" style="display:flex;flex-direction:column;gap:0.5rem;justify-content:center;">
            <label style="display:flex;align-items:center;gap:0.5rem;cursor:pointer;">
              <input type="checkbox" bind:checked={jobForm.notify_on_failure} style="width:auto;" />
              Notify on failure
            </label>
            <label style="display:flex;align-items:center;gap:0.5rem;cursor:pointer;">
              <input type="checkbox" bind:checked={jobForm.notify_on_success} style="width:auto;" />
              Notify on success
            </label>
          </div>
        </div>
        <div class="form-group">
          <label for="pxjob-notes">Notes</label>
          <textarea id="pxjob-notes" bind:value={jobForm.notes} placeholder="Optional notes about this job" rows="2"></textarea>
        </div>
        <div class="modal-actions">
          <button type="button" class="btn btn-secondary" on:click={() => showCreateJobModal = false}>Cancel</button>
          <button type="submit" class="btn btn-primary" disabled={!jobForm.pool_id}>Create Job</button>
        </div>
      </form>
    </div>
  </div>
{/if}

{#if showEditJobModal && editingJob}
  <div class="modal-overlay" on:click={() => showEditJobModal = false}>
    <div class="modal modal-lg" on:click|stopPropagation={() => {}}>
      <h2>Edit Backup Job: {editingJob.name}</h2>
      <form on:submit|preventDefault={handleUpdateJob}>
        <div class="form-group">
          <label for="edit-name">Job Name</label>
          <input type="text" id="edit-name" bind:value={jobForm.name} required />
        </div>
        <div class="form-group">
          <label for="edit-vmids">VM IDs (comma-separated, leave empty for all)</label>
          <input type="text" id="edit-vmids" bind:value={jobForm.vmids} placeholder="e.g., 100,101,200" />
        </div>
        <div class="form-row">
          <div class="form-group">
            <label for="edit-schedule">Schedule (cron)</label>
            <input type="text" id="edit-schedule" bind:value={jobForm.schedule_cron} />
          </div>
          <div class="form-group">
            <label for="edit-mode">Backup Mode</label>
            <select id="edit-mode" bind:value={jobForm.backup_mode}>
              <option value="snapshot">Snapshot</option>
              <option value="suspend">Suspend</option>
              <option value="stop">Stop</option>
            </select>
          </div>
        </div>
        <div class="form-row">
          <div class="form-group">
            <label for="edit-pool">Media Pool</label>
            <select id="edit-pool" bind:value={jobForm.pool_id} required>
              <option value={0}>Select a pool...</option>
              {#each pools as pool}
                <option value={pool.id}>{pool.name}</option>
              {/each}
            </select>
          </div>
          <div class="form-group">
            <label for="edit-compression">Compression</label>
            <select id="edit-compression" bind:value={jobForm.compression}>
              <option value="none">None</option>
              <option value="gzip">Gzip</option>
              <option value="zstd">Zstd</option>
            </select>
          </div>
        </div>
        <div class="form-row">
          <div class="form-group">
            <label for="edit-retention">Retention (days)</label>
            <input type="number" id="edit-retention" bind:value={jobForm.retention_days} min="1" max="3650" />
          </div>
          <div class="form-group" style="display:flex;flex-direction:column;gap:0.5rem;justify-content:center;">
            <label style="display:flex;align-items:center;gap:0.5rem;cursor:pointer;">
              <input type="checkbox" bind:checked={jobForm.notify_on_failure} style="width:auto;" />
              Notify on failure
            </label>
            <label style="display:flex;align-items:center;gap:0.5rem;cursor:pointer;">
              <input type="checkbox" bind:checked={jobForm.notify_on_success} style="width:auto;" />
              Notify on success
            </label>
          </div>
        </div>
        <div class="form-group">
          <label for="edit-notes">Notes</label>
          <textarea id="edit-notes" bind:value={jobForm.notes} rows="2"></textarea>
        </div>
        <div class="modal-actions">
          <button type="button" class="btn btn-secondary" on:click={() => showEditJobModal = false}>Cancel</button>
          <button type="submit" class="btn btn-primary" disabled={!jobForm.pool_id}>Save Changes</button>
        </div>
      </form>
    </div>
  </div>
{/if}

{#if showBackupModal && backupTarget}
  <div class="modal-overlay" on:click={() => showBackupModal = false}>
    <div class="modal" on:click|stopPropagation={() => {}}>
      <h2>Backup {backupTarget.type === 'lxc' ? 'Container' : 'VM'}: {backupTarget.name} (VMID {backupTarget.vmid})</h2>
      <form on:submit|preventDefault={handleBackupGuest}>
        <div class="form-group">
          <label for="bk-pool">Media Pool</label>
          <select id="bk-pool" bind:value={backupForm.pool_id} required>
            <option value={0}>Select a pool...</option>
            {#each pools as pool}
              <option value={pool.id}>{pool.name}</option>
            {/each}
          </select>
          <small style="color: var(--text-muted)">A tape will be automatically selected from this pool</small>
        </div>
        <div class="form-group">
          <label for="bk-mode">Backup Mode</label>
          <select id="bk-mode" bind:value={backupForm.mode}>
            <option value="snapshot">Snapshot (live)</option>
            <option value="suspend">Suspend</option>
            <option value="stop">Stop</option>
          </select>
        </div>
        <div class="form-group">
          <label for="bk-compress">Compression</label>
          <select id="bk-compress" bind:value={backupForm.compress}>
            <option value="zstd">Zstd</option>
            <option value="gzip">Gzip</option>
            <option value="lzo">LZO</option>
            <option value="">None</option>
          </select>
        </div>
        <div class="modal-actions">
          <button type="button" class="btn btn-secondary" on:click={() => showBackupModal = false}>Cancel</button>
          <button type="submit" class="btn btn-success" disabled={!backupForm.pool_id}>Start Backup</button>
        </div>
      </form>
    </div>
  </div>
{/if}

{#if showRestoreModal && restoreTarget}
  <div class="modal-overlay" on:click={() => { if (restoreStep !== 'running') showRestoreModal = false; }}>
    <div class="modal modal-lg" on:click|stopPropagation={() => {}}>
      {#if restoreStep === 'config'}
        <h2>🔄 Restore: {restoreTarget.vm_name} (VMID {restoreTarget.vmid})</h2>
        <div style="margin-bottom: 1rem;">
          <div style="display: flex; gap: 1rem; flex-wrap: wrap; font-size: 0.85rem; color: var(--text-secondary);">
            <span>📼 Tape: <strong>{restoreTarget.tape_label || '-'}</strong></span>
            <span>📦 Size: <strong>{formatBytes(restoreTarget.file_size)}</strong></span>
            <span>🖥️ Node: <strong>{restoreTarget.node}</strong></span>
          </div>
        </div>
        <form on:submit|preventDefault={executeProxmoxRestore}>
          <div class="form-group">
            <label for="rx-drive">Tape Drive</label>
            <select id="rx-drive" bind:value={restoreForm.drive_id}>
              {#each drives as drive}
                <option value={drive.id}>
                  {drive.display_name || drive.device_path}
                  {#if drive.vendor || drive.model} ({drive.vendor} {drive.model}){/if}
                  {#if drive.current_tape} — 📼 {drive.current_tape}{/if}
                </option>
              {/each}
              {#if drives.length === 0}
                <option value={undefined}>No drives available</option>
              {/if}
            </select>
            <small style="color: var(--text-muted)">Select the tape drive to read from. The correct tape must be loaded.</small>
          </div>
          <div class="form-row">
            <div class="form-group">
              <label for="rx-vmid">Target VMID</label>
              <input type="number" id="rx-vmid" bind:value={restoreForm.target_vmid} min="100" />
              <small style="color: var(--text-muted)">Use original VMID or assign a new one</small>
            </div>
            <div class="form-group">
              <label for="rx-storage">Target Storage</label>
              <input type="text" id="rx-storage" bind:value={restoreForm.storage} placeholder="local" />
              <small style="color: var(--text-muted)">Proxmox storage for restored disks</small>
            </div>
          </div>
          <div class="form-group" style="display:flex;flex-direction:column;gap:0.5rem;">
            <label style="display:flex;align-items:center;gap:0.5rem;cursor:pointer;">
              <input type="checkbox" bind:checked={restoreForm.overwrite} style="width:auto;" />
              Overwrite if VMID exists
            </label>
            <label style="display:flex;align-items:center;gap:0.5rem;cursor:pointer;">
              <input type="checkbox" bind:checked={restoreForm.start_after} style="width:auto;" />
              Start VM/container after restore
            </label>
          </div>
          <div class="modal-actions">
            <button type="button" class="btn btn-secondary" on:click={() => showRestoreModal = false}>Cancel</button>
            <button type="submit" class="btn btn-success" disabled={drives.length === 0}>🚀 Start Restore</button>
          </div>
        </form>
      {:else if restoreStep === 'running'}
        <h2>🔄 Restoring...</h2>
        <div style="text-align: center; padding: 2rem 0;">
          <div style="font-size: 2rem; margin-bottom: 1rem;">📼</div>
          <p>Restoring <strong>{restoreTarget.vm_name}</strong> (VMID {restoreTarget.vmid}) from tape...</p>
          <p style="color: var(--text-muted); font-size: 0.85rem;">Please wait. Do not eject the tape.</p>
        </div>
      {:else if restoreStep === 'done'}
        <h2>✅ Restore Complete</h2>
        <div style="text-align: center; padding: 2rem 0;">
          <div style="font-size: 2rem; margin-bottom: 1rem;">✅</div>
          <p><strong>{restoreTarget.vm_name}</strong> (VMID {restoreForm.target_vmid}) has been restored successfully.</p>
          <div class="modal-actions" style="justify-content: center;">
            <button class="btn btn-primary" on:click={() => showRestoreModal = false}>Done</button>
          </div>
        </div>
      {:else if restoreStep === 'error'}
        <h2>❌ Restore Failed</h2>
        <div style="padding: 1rem 0;">
          <div style="background: var(--badge-danger-bg); color: var(--badge-danger-text); padding: 1rem; border-radius: 8px; margin-bottom: 1rem;">
            {restoreError}
          </div>
          <div class="modal-actions">
            <button class="btn btn-secondary" on:click={() => showRestoreModal = false}>Close</button>
            <button class="btn btn-primary" on:click={() => restoreStep = 'config'}>← Try Again</button>
          </div>
        </div>
      {/if}
    </div>
  </div>
{/if}

<style>
  .tab-bar {
    display: flex;
    gap: 0.25rem;
    margin-bottom: 1rem;
  }

  .tab {
    padding: 0.5rem 1rem;
    border: 1px solid var(--border-color);
    background: var(--bg-card);
    color: var(--text-secondary);
    border-radius: 6px 6px 0 0;
    cursor: pointer;
    font-size: 0.875rem;
    transition: all 0.2s;
  }

  .tab.active {
    background: var(--accent-primary);
    color: white;
    border-color: var(--accent-primary);
  }

  .tab:hover:not(.active) {
    background: var(--bg-card-hover);
  }

  code {
    background: var(--code-bg);
    padding: 0.15rem 0.4rem;
    border-radius: 4px;
    font-size: 0.8rem;
  }

  .modal-overlay {
    position: fixed;
    top: 0; left: 0; right: 0; bottom: 0;
    background: rgba(0,0,0,0.5);
    display: flex;
    justify-content: center;
    align-items: center;
    z-index: 1000;
  }

  .modal {
    background: var(--bg-card);
    padding: 2rem;
    border-radius: 12px;
    width: 100%;
    max-width: 450px;
  }

  .modal.modal-lg {
    max-width: 600px;
  }

  .modal h2 {
    margin: 0 0 1.5rem;
  }

  .modal-actions {
    display: flex;
    gap: 0.75rem;
    justify-content: flex-end;
    margin-top: 1.5rem;
  }

  .form-row {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 1rem;
  }

  .btn-sm {
    padding: 0.25rem 0.5rem;
    font-size: 0.75rem;
  }

  code {
    background: var(--code-bg);
    padding: 0.15rem 0.4rem;
    border-radius: 4px;
    font-size: 0.8rem;
  }

  small {
    display: block;
    margin-top: 0.25rem;
  }

  @media (max-width: 768px) {
    .form-row {
      grid-template-columns: 1fr;
    }
  }
</style>
