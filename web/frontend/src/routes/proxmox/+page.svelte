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
    enabled: boolean;
    compression: string;
    last_run_at: string;
  }

  let guests: ProxmoxGuest[] = [];
  let backups: ProxmoxBackup[] = [];
  let jobs: ProxmoxJob[] = [];
  let loading = true;
  let error = '';
  let proxmoxEnabled = false;
  let activeTab = 'guests';
  let showCreateJobModal = false;

  let jobForm = {
    name: '',
    vmids: '',
    schedule_cron: '',
    pool_id: 0,
    compression: 'gzip',
  };

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
        guests = Array.isArray(guestResult.value) ? guestResult.value : [];
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
        if (msg.includes('not configured') || msg.includes('not enabled') || msg.includes('disabled')) {
          proxmoxEnabled = false;
        } else {
          error = msg || 'Failed to load Proxmox data';
        }
      }
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to load data';
    } finally {
      loading = false;
    }
  }

  async function handleBackupGuest(vmid: number) {
    if (!confirm(`Start backup for VM/LXC ${vmid}?`)) return;
    try {
      await api.post('/proxmox/backups', { vmid });
      alert('Proxmox backup started!');
      await loadData();
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to start backup';
    }
  }

  async function handleBackupAll() {
    if (!confirm('Start backup for ALL VMs and containers?')) return;
    try {
      await api.post('/proxmox/backups/all');
      alert('Proxmox backup all started!');
      await loadData();
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to start backup all';
    }
  }

  async function handleCreateJob() {
    try {
      await api.post('/proxmox/jobs', jobForm);
      showCreateJobModal = false;
      jobForm = { name: '', vmids: '', schedule_cron: '', pool_id: 0, compression: 'gzip' };
      await loadData();
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to create job';
    }
  }

  async function handleDeleteJob(id: number) {
    if (!confirm('Delete this Proxmox backup job?')) return;
    try {
      await api.delete(`/proxmox/jobs/${id}`);
      await loadData();
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to delete job';
    }
  }

  async function handleRunJob(id: number) {
    try {
      await api.post(`/proxmox/jobs/${id}/run`);
      alert('Proxmox job started!');
      await loadData();
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to run job';
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
</script>

<div class="page-header">
  <h1>🖥️ Proxmox Integration</h1>
  {#if proxmoxEnabled}
    <div style="display: flex; gap: 0.5rem;">
      <button class="btn btn-success" on:click={handleBackupAll}>Backup All</button>
      <button class="btn btn-primary" on:click={() => showCreateJobModal = true}>+ Create Job</button>
    </div>
  {/if}
</div>

{#if error}
  <div class="card" style="background: var(--badge-danger-bg); color: var(--badge-danger-text);">
    <p>{error}</p>
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
                <button class="btn btn-success" on:click={() => handleBackupGuest(guest.vmid)}>
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
            </tr>
          {/each}
          {#if backups.length === 0}
            <tr><td colspan="8" style="text-align:center; color: var(--text-muted);">No Proxmox backups found</td></tr>
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
            <th>Compression</th>
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
              <td>{job.compression || 'none'}</td>
              <td>
                <span class="badge {job.enabled ? 'badge-success' : 'badge-danger'}">
                  {job.enabled ? 'Enabled' : 'Disabled'}
                </span>
              </td>
              <td>{formatDate(job.last_run_at)}</td>
              <td>
                <div style="display:flex;gap:0.5rem;">
                  <button class="btn btn-success" on:click={() => handleRunJob(job.id)}>Run</button>
                  <button class="btn btn-danger" on:click={() => handleDeleteJob(job.id)}>Delete</button>
                </div>
              </td>
            </tr>
          {/each}
          {#if jobs.length === 0}
            <tr><td colspan="7" style="text-align:center; color: var(--text-muted);">No scheduled Proxmox jobs. Create one above.</td></tr>
          {/if}
        </tbody>
      </table>
    </div>
  {/if}
{/if}

{#if showCreateJobModal}
  <div class="modal-overlay" on:click={() => showCreateJobModal = false}>
    <div class="modal" on:click|stopPropagation={() => {}}>
      <h2>Create Proxmox Backup Job</h2>
      <form on:submit|preventDefault={handleCreateJob}>
        <div class="form-group">
          <label for="pxjob-name">Job Name</label>
          <input type="text" id="pxjob-name" bind:value={jobForm.name} required placeholder="e.g., Nightly VM Backup" />
        </div>
        <div class="form-group">
          <label for="pxjob-vmids">VM IDs (comma-separated, leave empty for all)</label>
          <input type="text" id="pxjob-vmids" bind:value={jobForm.vmids} placeholder="e.g., 100,101,200" />
        </div>
        <div class="form-group">
          <label for="pxjob-schedule">Schedule (cron)</label>
          <input type="text" id="pxjob-schedule" bind:value={jobForm.schedule_cron} placeholder="e.g., 0 2 * * *" />
        </div>
        <div class="form-group">
          <label for="pxjob-compression">Compression</label>
          <select id="pxjob-compression" bind:value={jobForm.compression}>
            <option value="none">None</option>
            <option value="gzip">Gzip</option>
            <option value="zstd">Zstd</option>
          </select>
        </div>
        <div class="modal-actions">
          <button type="button" class="btn btn-secondary" on:click={() => showCreateJobModal = false}>Cancel</button>
          <button type="submit" class="btn btn-primary">Create</button>
        </div>
      </form>
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

  .modal h2 {
    margin: 0 0 1.5rem;
  }

  .modal-actions {
    display: flex;
    gap: 0.75rem;
    justify-content: flex-end;
    margin-top: 1.5rem;
  }
</style>
