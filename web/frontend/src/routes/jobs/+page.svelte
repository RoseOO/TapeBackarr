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
    encryption_enabled: boolean;
    encryption_key_id: number | null;
    last_run_at: string | null;
    next_run_at: string | null;
    compression: string;
  }

  interface ActiveJob {
    job_id: number;
    job_name: string;
    backup_set_id: number;
    phase: string;
    message: string;
    status: string;
    file_count: number;
    total_files: number;
    total_bytes: number;
    bytes_written: number;
    write_speed: number;
    tape_label: string;
    tape_capacity_bytes: number;
    tape_used_bytes: number;
    device_path: string;
    estimated_seconds_remaining: number;
    start_time: string;
    updated_at: string;
    log_lines: string[];
    compression: string;
    tape_estimated_seconds_remaining: number;
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

  interface EncryptionKey {
    id: number;
    name: string;
    algorithm: string;
    key_fingerprint: string;
  }

  let jobs: Job[] = [];
  let sources: Source[] = [];
  let pools: Pool[] = [];
  let tapes: Tape[] = [];
  let encryptionKeys: EncryptionKey[] = [];
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
    encryption_key_id: null as number | null,
    compression: 'none',
  };

  let runFormData = {
    tape_id: 0,
    backup_type: 'full',
    use_pool: true,
  };

  let recommendedTape: { found: boolean; tape_id?: number; tape_label?: string; tape_status?: string; capacity_bytes?: number; used_bytes?: number; pool_name?: string; message?: string } | null = null;
  let loadingRecommendation = false;

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
    loading = true;
    error = '';
    try {
      const [jobsResult, sourcesResult, poolsResult, tapesResult, keysResult] = await Promise.all([
        api.getJobs(),
        api.getSources(),
        api.getPools(),
        api.getTapes(),
        api.getEncryptionKeys(),
      ]);
      jobs = Array.isArray(jobsResult) ? jobsResult : [];
      sources = Array.isArray(sourcesResult) ? sourcesResult : [];
      pools = Array.isArray(poolsResult) ? poolsResult : [];
      tapes = Array.isArray(tapesResult) ? tapesResult : [];
      encryptionKeys = keysResult?.keys || [];
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to load data';
    } finally {
      loading = false;
    }
  }

  async function handleCreate() {
    try {
      const payload: any = { ...formData };
      if (!payload.encryption_key_id) {
        delete payload.encryption_key_id;
      }
      await api.createJob(payload);
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
      if (runFormData.use_pool) {
        const result = await api.runJob(selectedJob.id, undefined, runFormData.backup_type, true);
        showRunModal = false;
        const tapeLabel = result?.tape_label ? ` using tape ${result.tape_label}` : '';
        alert(`Backup job started${tapeLabel}!`);
      } else {
        if (!runFormData.tape_id) {
          error = 'Please select a tape';
          return;
        }
        await api.runJob(selectedJob.id, runFormData.tape_id, runFormData.backup_type, false);
        showRunModal = false;
        alert('Backup job started!');
      }
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to start job';
    }
  }

  async function openRunModal(job: Job) {
    selectedJob = job;
    runFormData = {
      tape_id: 0,
      backup_type: job.backup_type,
      use_pool: true,
    };
    recommendedTape = null;
    showRunModal = true;

    // Fetch tape recommendation from pool
    loadingRecommendation = true;
    try {
      recommendedTape = await api.recommendTape(job.id);
    } catch {
      recommendedTape = null;
    } finally {
      loadingRecommendation = false;
    }
  }

  function resetForm() {
    formData = {
      name: '',
      source_id: 0,
      pool_id: 0,
      backup_type: 'full',
      schedule_cron: '',
      retention_days: 30,
      encryption_key_id: null as number | null,
      compression: 'none',
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

  function formatSpeed(bytesPerSec: number): string {
    if (bytesPerSec <= 0) return '---';
    return formatBytes(bytesPerSec) + '/s';
  }

  function formatETA(seconds: number): string {
    if (seconds <= 0) return '---';
    const h = Math.floor(seconds / 3600);
    const m = Math.floor((seconds % 3600) / 60);
    const s = Math.floor(seconds % 60);
    if (h > 0) return `${h}h ${m}m`;
    if (m > 0) return `${m}m ${s}s`;
    return `${s}s`;
  }

  function getProgressPercent(job: ActiveJob): number {
    if (job.total_bytes <= 0) return 0;
    return Math.min(100, (job.bytes_written / job.total_bytes) * 100);
  }

  function getTapeProgressPercent(job: ActiveJob): number {
    if (job.tape_capacity_bytes <= 0) return 0;
    const totalUsed = job.tape_used_bytes + job.bytes_written;
    return Math.min(100, (totalUsed / job.tape_capacity_bytes) * 100);
  }

  function formatElapsed(startTime: string): string {
    const start = new Date(startTime).getTime();
    const elapsed = (Date.now() - start) / 1000;
    const h = Math.floor(elapsed / 3600);
    const m = Math.floor((elapsed % 3600) / 60);
    const s = Math.floor(elapsed % 60);
    if (h > 0) return `${h}h ${m}m ${s}s`;
    if (m > 0) return `${m}m ${s}s`;
    return `${s}s`;
  }

  async function handleCancel(jobId: number) {
    if (!confirm('Are you sure you want to cancel this backup job?')) return;
    try {
      await api.cancelJob(jobId);
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to cancel job';
    }
  }

  async function handlePause(jobId: number) {
    try {
      await api.pauseJob(jobId);
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to pause job';
    }
  }

  async function handleResume(jobId: number) {
    try {
      await api.resumeJob(jobId);
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to resume job';
    }
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
            <span class="terminal-title">
              {getPhaseIcon(job.phase)} {job.job_name}
              {#if job.status === 'paused'}
                <span class="status-badge paused">PAUSED</span>
              {/if}
            </span>
            <div class="terminal-controls">
              {#if job.status === 'paused'}
                <button class="ctrl-btn resume-btn" on:click={() => handleResume(job.job_id)} title="Resume">▶ Resume</button>
              {:else if job.phase !== 'completed' && job.phase !== 'failed' && job.phase !== 'cancelled'}
                <button class="ctrl-btn pause-btn" on:click={() => handlePause(job.job_id)} title="Pause">⏸ Pause</button>
              {/if}
              {#if job.phase !== 'completed' && job.phase !== 'failed' && job.phase !== 'cancelled'}
                <button class="ctrl-btn cancel-btn" on:click={() => handleCancel(job.job_id)} title="Cancel">⏹ Cancel</button>
              {/if}
              <span class="terminal-phase">{job.phase}</span>
            </div>
          </div>
          <div class="terminal-meta">
            {#if job.tape_label}
              <span>📼 Tape: {job.tape_label}</span>
            {/if}
            {#if job.device_path}
              <span>🖴 {job.device_path}</span>
            {/if}
            <span>⏱ Elapsed: {formatElapsed(job.start_time)}</span>
            <span>Started: {new Date(job.start_time).toLocaleTimeString()}</span>
          </div>
          <div class="terminal-progress-section">
            <div class="progress-row">
              <span class="progress-label">Job Progress</span>
              <div class="progress-bar-container">
                <div class="progress-bar-track">
                  <div class="progress-bar-fill job-fill" style="width: {getProgressPercent(job)}%"></div>
                </div>
              </div>
              <span class="progress-value">{getProgressPercent(job).toFixed(1)}%</span>
            </div>
            {#if job.tape_capacity_bytes > 0}
              <div class="progress-row">
                <span class="progress-label">Tape Used</span>
                <div class="progress-bar-container">
                  <div class="progress-bar-track">
                    <div class="progress-bar-fill tape-fill" style="width: {getTapeProgressPercent(job)}%"></div>
                  </div>
                </div>
                <span class="progress-value">{getTapeProgressPercent(job).toFixed(1)}%</span>
              </div>
            {/if}
          </div>
          <div class="terminal-stats">
            <div class="stat-item">
              <span class="stat-label">Written</span>
              <span class="stat-value">{formatBytes(job.bytes_written)} / {formatBytes(job.total_bytes)}</span>
            </div>
            <div class="stat-item">
              <span class="stat-label">Speed</span>
              <span class="stat-value">{formatSpeed(job.write_speed)}</span>
            </div>
            <div class="stat-item">
              <span class="stat-label">Job ETA</span>
              <span class="stat-value">{formatETA(job.estimated_seconds_remaining)}</span>
            </div>
            {#if job.tape_estimated_seconds_remaining > 0}
              <div class="stat-item">
                <span class="stat-label">Tape ETA</span>
                <span class="stat-value">{formatETA(job.tape_estimated_seconds_remaining)}</span>
              </div>
            {/if}
            <div class="stat-item">
              <span class="stat-label">Files</span>
              <span class="stat-value">{job.file_count}/{job.total_files}</span>
            </div>
            {#if job.tape_capacity_bytes > 0}
              <div class="stat-item">
                <span class="stat-label">Tape Space</span>
                <span class="stat-value">{formatBytes(Math.max(0, job.tape_capacity_bytes - job.tape_used_bytes - job.bytes_written))} free</span>
              </div>
            {/if}
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
          <th>Encryption</th>
          <th>Compression</th>
          <th>Schedule</th>
          <th>Last Run</th>
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
            <td>
              {#if job.encryption_enabled}
                <span class="badge badge-success">🔒 Encrypted</span>
              {:else}
                <span class="badge badge-secondary">None</span>
              {/if}
            </td>
            <td>
              {#if job.compression && job.compression !== 'none'}
                <span class="badge badge-info">{job.compression}</span>
              {:else}
                <span class="badge" style="background: var(--bg-input); color: var(--text-muted)">None</span>
              {/if}
            </td>
            <td><code>{job.schedule_cron || 'Manual'}</code></td>
            <td>{formatDate(job.last_run_at)}</td>
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
            <td colspan="10" class="no-data">No jobs found. Create a job to get started.</td>
          </tr>
        {/if}
      </tbody>
    </table>
  </div>
{/if}

<!-- Create Modal -->
{#if showCreateModal}
  <div class="modal-overlay" on:click={() => showCreateModal = false}>
    <div class="modal" on:click|stopPropagation={() => {}}>
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
        <div class="form-group">
          <label for="encryption-key">Encryption Key</label>
          <select id="encryption-key" bind:value={formData.encryption_key_id}>
            <option value={null}>None (unencrypted)</option>
            {#each encryptionKeys as key}
              <option value={key.id}>🔒 {key.name}</option>
            {/each}
          </select>
          <small>Select an encryption key to encrypt backups. <a href="/encryption">Manage keys</a></small>
        </div>
        <div class="form-group">
          <label for="compression">Compression</label>
          <select id="compression" bind:value={formData.compression}>
            <option value="none">None</option>
            <option value="gzip">Gzip</option>
            <option value="zstd">Zstd</option>
          </select>
          <small>Compress data before writing to tape. Recommended for LTO tapes.</small>
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
    <div class="modal" on:click|stopPropagation={() => {}}>
      <h2>Run Backup Job</h2>
      <p>Running job: <strong>{selectedJob.name}</strong></p>
      <form on:submit|preventDefault={handleRunJob}>
        <div class="form-group">
          <label for="run-mode">Tape Selection</label>
          <select id="run-mode" bind:value={runFormData.use_pool}>
            <option value={true}>Use tape pool (recommended)</option>
            <option value={false}>Select specific tape</option>
          </select>
          <small>{runFormData.use_pool ? 'The system will automatically select the best tape from the job\'s pool' : 'Manually choose which tape to use'}</small>
        </div>
        {#if runFormData.use_pool}
          <div class="recommendation-box">
            {#if loadingRecommendation}
              <p class="rec-loading">⏳ Finding best tape from pool...</p>
            {:else if recommendedTape?.found}
              <div class="rec-found">
                <p class="rec-title">📼 Recommended tape: <strong>{recommendedTape.tape_label}</strong></p>
                <p class="rec-detail">Pool: {recommendedTape.pool_name} · Status: {recommendedTape.tape_status}</p>
                {#if recommendedTape.capacity_bytes}
                  <p class="rec-detail">Space: {formatBytes((recommendedTape.capacity_bytes || 0) - (recommendedTape.used_bytes || 0))} available</p>
                {/if}
                <p class="rec-message">{recommendedTape.message}</p>
              </div>
            {:else if recommendedTape}
              <div class="rec-warning">
                <p>⚠️ {recommendedTape.message}</p>
                <p class="rec-detail">Consider adding blank tapes to the pool or switching to manual tape selection.</p>
              </div>
            {/if}
          </div>
        {:else}
          <div class="form-group">
            <label for="run-tape">Select Tape</label>
            <select id="run-tape" bind:value={runFormData.tape_id} required>
              <option value={0} disabled>Select a tape</option>
              {#each availableTapes as tape}
                <option value={tape.id}>{tape.label} ({tape.status})</option>
              {/each}
            </select>
          </div>
        {/if}
        <div class="form-group">
          <label for="run-type">Backup Type</label>
          <select id="run-type" bind:value={runFormData.backup_type}>
            <option value="full">Full</option>
            <option value="incremental">Incremental</option>
          </select>
        </div>
        <div class="modal-actions">
          <button type="button" class="btn btn-secondary" on:click={() => showRunModal = false}>Cancel</button>
          <button type="submit" class="btn btn-success" disabled={runFormData.use_pool && recommendedTape !== null && !recommendedTape.found}>Start Backup</button>
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

  .terminal-controls {
    display: flex;
    align-items: center;
    gap: 0.5rem;
  }

  .ctrl-btn {
    background: none;
    border: 1px solid #555;
    color: #ccc;
    padding: 0.2rem 0.5rem;
    border-radius: 4px;
    font-size: 0.7rem;
    cursor: pointer;
    font-family: inherit;
  }

  .ctrl-btn:hover {
    background: #3a3a5a;
  }

  .pause-btn { border-color: #f39c12; color: #f39c12; }
  .pause-btn:hover { background: rgba(243, 156, 18, 0.15); }
  .resume-btn { border-color: #2ecc71; color: #2ecc71; }
  .resume-btn:hover { background: rgba(46, 204, 113, 0.15); }
  .cancel-btn { border-color: #e74c3c; color: #e74c3c; }
  .cancel-btn:hover { background: rgba(231, 76, 60, 0.15); }

  .status-badge {
    font-size: 0.6rem;
    padding: 0.1rem 0.3rem;
    border-radius: 3px;
    font-weight: 700;
    margin-left: 0.3rem;
  }

  .status-badge.paused {
    background: #f39c12;
    color: #000;
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

  .terminal-progress-section {
    padding: 0.5rem 1rem;
    background: #222238;
    display: flex;
    flex-direction: column;
    gap: 0.3rem;
  }

  .progress-row {
    display: flex;
    align-items: center;
    gap: 0.5rem;
  }

  .progress-label {
    color: #888;
    font-size: 0.7rem;
    width: 7em;
    flex-shrink: 0;
    font-family: monospace;
  }

  .progress-bar-container {
    flex: 1;
  }

  .progress-bar-track {
    height: 8px;
    background: #333;
    border-radius: 4px;
    overflow: hidden;
  }

  .progress-bar-fill {
    height: 100%;
    border-radius: 4px;
    transition: width 0.5s ease;
  }

  .job-fill {
    background: linear-gradient(90deg, #4a4aff, #16c784);
  }

  .tape-fill {
    background: linear-gradient(90deg, #f39c12, #e74c3c);
  }

  .progress-value {
    color: #aaa;
    font-size: 0.7rem;
    width: 4em;
    text-align: right;
    flex-shrink: 0;
    font-family: monospace;
  }

  .terminal-stats {
    display: flex;
    gap: 1.5rem;
    padding: 0.5rem 1rem;
    background: #252537;
    flex-wrap: wrap;
  }

  .stat-item {
    display: flex;
    flex-direction: column;
  }

  .stat-label {
    color: #666;
    font-size: 0.65rem;
    text-transform: uppercase;
    font-family: monospace;
  }

  .stat-value {
    color: #89dceb;
    font-size: 0.8rem;
    font-weight: 600;
    font-family: monospace;
  }

  .recommendation-box {
    background: #f8f9fa;
    border: 1px solid #dee2e6;
    border-radius: 8px;
    padding: 0.75rem;
    margin-bottom: 0.5rem;
  }

  .rec-loading {
    color: #666;
    font-style: italic;
    margin: 0;
  }

  .rec-found {
    color: #155724;
  }

  .rec-found .rec-title {
    margin: 0 0 0.25rem;
    font-size: 0.9rem;
  }

  .rec-detail {
    margin: 0.15rem 0;
    font-size: 0.8rem;
    color: #666;
  }

  .rec-message {
    margin: 0.5rem 0 0;
    font-size: 0.85rem;
    color: #0c5460;
    background: #d1ecf1;
    padding: 0.4rem 0.6rem;
    border-radius: 4px;
  }

  .rec-warning {
    color: #856404;
  }

  .rec-warning p {
    margin: 0 0 0.25rem;
    font-size: 0.85rem;
  }
</style>
