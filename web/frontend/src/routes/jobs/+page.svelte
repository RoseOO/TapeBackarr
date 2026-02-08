<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import * as api from '$lib/api/client';

  interface Job {
    id: number;
    name: string;
    source_id: number;
    source_name: string;
    pool_id: number;
    pool_name: string;
    backup_type: string;
    schedule_cron: string;
    retention_days: number;
    enabled: boolean;
    last_run_at: string | null;
    next_run_at: string | null;
  }

  interface ActiveJob {
    job_id: number;
    job_name: string;
    backup_set_id: number;
    phase: string;
    message: string;
    file_count: number;
    total_files: number;
    total_bytes: number;
    start_time: string;
    updated_at: string;
    log_lines: string[];
  }

  interface Source {
    id: number;
    name: string;
  }

  interface Pool {
    id: number;
    name: string;
  }

  interface Tape {
    id: number;
    label: string;
    status: string;
  }

  let jobs: Job[] = [];
  let sources: Source[] = [];
  let pools: Pool[] = [];
  let tapes: Tape[] = [];
  let activeJobs: ActiveJob[] = [];
  let loading = true;
  let error = '';
  let showCreateModal = false;
  let showRunModal = false;
  let selectedJob: Job | null = null;
  let pollInterval: ReturnType<typeof setInterval>;

  let formData = {
    name: '',
    source_id: 0,
    pool_id: 0,
    backup_type: 'full',
    schedule_cron: '',
    retention_days: 30,
  };

  let runFormData = {
    tape_id: 0,
    backup_type: 'full',
  };

  onMount(async () => {
    await loadData();
    await loadActiveJobs();
    pollInterval = setInterval(loadActiveJobs, 3000);
  });

  onDestroy(() => {
    if (pollInterval) clearInterval(pollInterval);
  });

  async function loadActiveJobs() {
    try {
      activeJobs = await api.getActiveJobs();
    } catch {
      // Silently ignore polling errors
    }
  }

  async function loadData() {
    try {
      [jobs, sources, pools, tapes] = await Promise.all([
        api.getJobs(),
        api.getSources(),
        api.getPools(),
        api.getTapes(),
      ]);
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to load data';
    } finally {
      loading = false;
    }
  }

  async function handleCreate() {
    try {
      await api.createJob(formData);
      showCreateModal = false;
      resetForm();
      await loadData();
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to create job';
    }
  }

  async function handleDelete(job: Job) {
    if (!confirm(`Delete job "${job.name}"?`)) return;
    try {
      await api.deleteJob(job.id);
      await loadData();
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to delete job';
    }
  }

  async function handleToggle(job: Job) {
    try {
      await api.updateJob(job.id, { enabled: !job.enabled });
      await loadData();
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to update job';
    }
  }

  async function handleRunJob() {
    if (!selectedJob) return;
    try {
      await api.runJob(selectedJob.id, runFormData.tape_id, runFormData.backup_type);
      showRunModal = false;
      alert('Backup job started!');
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to start job';
    }
  }

  function openRunModal(job: Job) {
    selectedJob = job;
    runFormData = {
      tape_id: 0,
      backup_type: job.backup_type,
    };
    showRunModal = true;
  }

  function resetForm() {
    formData = {
      name: '',
      source_id: 0,
      pool_id: 0,
      backup_type: 'full',
      schedule_cron: '',
      retention_days: 30,
    };
  }

  function formatDate(dateStr: string | null): string {
    if (!dateStr) return '-';
    return new Date(dateStr).toLocaleString();
  }

  function formatBytes(bytes: number): string {
    if (bytes === 0) return '0 B';
    const k = 1024;
    const sizes = ['B', 'KB', 'MB', 'GB', 'TB', 'PB'];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
  }

  function getPhaseIcon(phase: string): string {
    switch (phase) {
      case 'initializing': return '⏳';
      case 'scanning': return '🔍';
      case 'streaming': return '📼';
      case 'cataloging': return '📝';
      case 'completed': return '✅';
      case 'failed': return '❌';
      default: return '⏳';
    }
  }

  $: availableTapes = tapes.filter(t => t.status === 'blank' || t.status === 'active');
</script>

<div class="page-header">
  <h1>Backup Jobs</h1>
  <button class="btn btn-primary" on:click={() => { showCreateModal = true; resetForm(); }}>
    + Create Job
  </button>
</div>

{#if error}
  <div class="card error-card">
    <p>{error}</p>
  </div>
{/if}

{#if loading}
  <p>Loading...</p>
{:else}
  {#if activeJobs.length > 0}
    <div class="active-operations">
      <h2>Running Operations</h2>
      {#each activeJobs as job}
        <div class="terminal-card">
          <div class="terminal-header">
            <span class="terminal-title">{getPhaseIcon(job.phase)} {job.job_name}</span>
            <span class="terminal-phase">{job.phase}</span>
          </div>
          <div class="terminal-meta">
            {#if job.total_files > 0}
              <span>Files: {job.file_count}/{job.total_files}</span>
            {/if}
            {#if job.total_bytes > 0}
              <span>Size: {formatBytes(job.total_bytes)}</span>
            {/if}
            <span>Started: {new Date(job.start_time).toLocaleTimeString()}</span>
          </div>
          <div class="terminal-output">
            {#each job.log_lines as line}
              <div class="terminal-line">{line}</div>
            {/each}
          </div>
        </div>
      {/each}
    </div>
  {/if}

  <div class="card">
    <table>
      <thead>
        <tr>
          <th>Name</th>
          <th>Source</th>
          <th>Pool</th>
          <th>Type</th>
          <th>Schedule</th>
          <th>Last Run</th>
          <th>Next Run</th>
          <th>Status</th>
          <th>Actions</th>
        </tr>
      </thead>
      <tbody>
        {#each jobs as job}
          <tr>
            <td><strong>{job.name}</strong></td>
            <td>{job.source_name}</td>
            <td>{job.pool_name}</td>
            <td>
              <span class="badge {job.backup_type === 'full' ? 'badge-info' : 'badge-warning'}">
                {job.backup_type}
              </span>
            </td>
            <td><code>{job.schedule_cron || 'Manual'}</code></td>
            <td>{formatDate(job.last_run_at)}</td>
            <td>{formatDate(job.next_run_at)}</td>
            <td>
              <span class="badge {job.enabled ? 'badge-success' : 'badge-danger'}">
                {job.enabled ? 'Enabled' : 'Disabled'}
              </span>
            </td>
            <td>
              <div class="actions">
                <button class="btn btn-success" on:click={() => openRunModal(job)}>Run</button>
                <button class="btn btn-secondary" on:click={() => handleToggle(job)}>
                  {job.enabled ? 'Disable' : 'Enable'}
                </button>
                <button class="btn btn-danger" on:click={() => handleDelete(job)}>Delete</button>
              </div>
            </td>
          </tr>
        {/each}
        {#if jobs.length === 0}
          <tr>
            <td colspan="9" class="no-data">No jobs found. Create a job to get started.</td>
          </tr>
        {/if}
      </tbody>
    </table>
  </div>
{/if}

<!-- Create Modal -->
{#if showCreateModal}
  <div class="modal-overlay" on:click={() => showCreateModal = false}>
    <div class="modal" on:click|stopPropagation>
      <h2>Create Backup Job</h2>
      <form on:submit|preventDefault={handleCreate}>
        <div class="form-group">
          <label for="name">Job Name</label>
          <input type="text" id="name" bind:value={formData.name} required placeholder="e.g., Daily Backup" />
        </div>
        <div class="form-group">
          <label for="source">Source</label>
          <select id="source" bind:value={formData.source_id} required>
            <option value={0} disabled>Select a source</option>
            {#each sources as source}
              <option value={source.id}>{source.name}</option>
            {/each}
          </select>
        </div>
        <div class="form-group">
          <label for="pool">Tape Pool</label>
          <select id="pool" bind:value={formData.pool_id} required>
            <option value={0} disabled>Select a pool</option>
            {#each pools as pool}
              <option value={pool.id}>{pool.name}</option>
            {/each}
          </select>
        </div>
        <div class="form-group">
          <label for="type">Backup Type</label>
          <select id="type" bind:value={formData.backup_type}>
            <option value="full">Full</option>
            <option value="incremental">Incremental</option>
          </select>
        </div>
        <div class="form-group">
          <label for="schedule">Schedule (cron)</label>
          <input type="text" id="schedule" bind:value={formData.schedule_cron} 
            placeholder="e.g., 0 0 2 * * * (2am daily)" />
          <small>Leave empty for manual-only jobs</small>
        </div>
        <div class="form-group">
          <label for="retention">Retention (days)</label>
          <input type="number" id="retention" bind:value={formData.retention_days} min="1" />
        </div>
        <div class="modal-actions">
          <button type="button" class="btn btn-secondary" on:click={() => showCreateModal = false}>Cancel</button>
          <button type="submit" class="btn btn-primary">Create</button>
        </div>
      </form>
    </div>
  </div>
{/if}

<!-- Run Modal -->
{#if showRunModal && selectedJob}
  <div class="modal-overlay" on:click={() => showRunModal = false}>
    <div class="modal" on:click|stopPropagation>
      <h2>Run Backup Job</h2>
      <p>Running job: <strong>{selectedJob.name}</strong></p>
      <form on:submit|preventDefault={handleRunJob}>
        <div class="form-group">
          <label for="run-tape">Select Tape</label>
          <select id="run-tape" bind:value={runFormData.tape_id} required>
            <option value={0} disabled>Select a tape</option>
            {#each availableTapes as tape}
              <option value={tape.id}>{tape.label} ({tape.status})</option>
            {/each}
          </select>
        </div>
        <div class="form-group">
          <label for="run-type">Backup Type</label>
          <select id="run-type" bind:value={runFormData.backup_type}>
            <option value="full">Full</option>
            <option value="incremental">Incremental</option>
          </select>
        </div>
        <div class="modal-actions">
          <button type="button" class="btn btn-secondary" on:click={() => showRunModal = false}>Cancel</button>
          <button type="submit" class="btn btn-success">Start Backup</button>
        </div>
      </form>
    </div>
  </div>
{/if}

<style>
  .error-card {
    background: #f8d7da;
    color: #721c24;
  }

  code {
    background: #f0f0f0;
    padding: 0.2rem 0.4rem;
    border-radius: 4px;
    font-size: 0.8rem;
  }

  small {
    display: block;
    margin-top: 0.25rem;
    color: #666;
    font-size: 0.75rem;
  }

  .actions {
    display: flex;
    gap: 0.5rem;
  }

  .no-data {
    text-align: center;
    color: #666;
    padding: 2rem;
  }

  .modal-overlay {
    position: fixed;
    top: 0;
    left: 0;
    right: 0;
    bottom: 0;
    background: rgba(0, 0, 0, 0.5);
    display: flex;
    justify-content: center;
    align-items: center;
    z-index: 1000;
  }

  .modal {
    background: white;
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

  .active-operations {
    margin-bottom: 1.5rem;
  }

  .active-operations h2 {
    margin: 0 0 1rem;
    font-size: 1rem;
    color: #333;
  }

  .terminal-card {
    background: #1e1e2e;
    border-radius: 12px;
    overflow: hidden;
    margin-bottom: 1rem;
  }

  .terminal-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    padding: 0.75rem 1rem;
    background: #2a2a3e;
    border-bottom: 1px solid #333;
  }

  .terminal-title {
    color: #cdd6f4;
    font-weight: 600;
    font-size: 0.9rem;
  }

  .terminal-phase {
    color: #f9e2af;
    font-size: 0.75rem;
    text-transform: uppercase;
    font-weight: 600;
  }

  .terminal-meta {
    display: flex;
    gap: 1.5rem;
    padding: 0.5rem 1rem;
    background: #252537;
    color: #a6adc8;
    font-size: 0.8rem;
    font-family: monospace;
  }

  .terminal-output {
    padding: 0.75rem 1rem;
    max-height: 200px;
    overflow-y: auto;
    font-family: 'Courier New', monospace;
    font-size: 0.8rem;
    line-height: 1.5;
  }

  .terminal-line {
    color: #a6e3a1;
    white-space: pre-wrap;
    word-break: break-all;
  }
</style>
