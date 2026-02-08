<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import * as api from '$lib/api/client';

  interface DashboardStats {
    total_tapes: number;
    active_tapes: number;
    total_jobs: number;
    running_jobs: number;
    recent_backups: number;
    drive_status: string;
    total_data_bytes: number;
    loaded_tape: string;
    loaded_tape_uuid: string;
    loaded_tape_pool: string;
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

  let stats: DashboardStats | null = null;
  let activeJobs: ActiveJob[] = [];
  let loading = true;
  let error = '';
  let pollInterval: ReturnType<typeof setInterval>;

  onMount(async () => {
    try {
      stats = await api.getDashboard();
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to load dashboard';
    } finally {
      loading = false;
    }

    // Poll for active jobs
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
</script>

<div class="page-header">
  <h1>Dashboard</h1>
</div>

{#if loading}
  <p>Loading...</p>
{:else if error}
  <div class="card">
    <p class="error">{error}</p>
  </div>
{:else if stats}
  <div class="stats-grid">
    <div class="stat-card">
      <div class="stat-icon">💾</div>
      <div class="stat-info">
        <div class="stat-value">{stats.total_tapes}</div>
        <div class="stat-label">Total Tapes</div>
      </div>
    </div>

    <div class="stat-card">
      <div class="stat-icon">✅</div>
      <div class="stat-info">
        <div class="stat-value">{stats.active_tapes}</div>
        <div class="stat-label">Active Tapes</div>
      </div>
    </div>

    <div class="stat-card">
      <div class="stat-icon">📦</div>
      <div class="stat-info">
        <div class="stat-value">{stats.total_jobs}</div>
        <div class="stat-label">Backup Jobs</div>
      </div>
    </div>

    <div class="stat-card">
      <div class="stat-icon">⏳</div>
      <div class="stat-info">
        <div class="stat-value">{stats.running_jobs}</div>
        <div class="stat-label">Running Jobs</div>
      </div>
    </div>

    <div class="stat-card">
      <div class="stat-icon">📊</div>
      <div class="stat-info">
        <div class="stat-value">{stats.recent_backups}</div>
        <div class="stat-label">Backups (24h)</div>
      </div>
    </div>

    <div class="stat-card">
      <div class="stat-icon">🖴</div>
      <div class="stat-info">
        <div class="stat-value">{formatBytes(stats.total_data_bytes)}</div>
        <div class="stat-label">Total Backed Up</div>
      </div>
    </div>
  </div>

  <div class="dashboard-row">
    <div class="card drive-status-card">
      <h2>Drive Status</h2>
      <div class="drive-status" class:online={stats.drive_status === 'online'} class:offline={stats.drive_status === 'offline'}>
        <span class="status-indicator"></span>
        <span class="status-text">{stats.drive_status}</span>
      </div>
      {#if stats.loaded_tape}
        <div class="loaded-tape-info">
          <h3>Loaded Tape</h3>
          <div class="tape-detail"><strong>Label:</strong> {stats.loaded_tape}</div>
          {#if stats.loaded_tape_uuid}
            <div class="tape-detail"><strong>UUID:</strong> <span class="uuid">{stats.loaded_tape_uuid}</span></div>
          {/if}
          {#if stats.loaded_tape_pool}
            <div class="tape-detail"><strong>Pool:</strong> {stats.loaded_tape_pool}</div>
          {/if}
        </div>
      {:else if stats.drive_status === 'online'}
        <div class="loaded-tape-info">
          <p class="no-tape">No labeled tape detected in drive</p>
        </div>
      {/if}
    </div>

    <div class="card quick-actions">
      <h2>Quick Actions</h2>
      <div class="action-buttons">
        <a href="/jobs" class="btn btn-primary">Create Backup Job</a>
        <a href="/tapes" class="btn btn-secondary">Manage Tapes</a>
        <a href="/restore" class="btn btn-secondary">Restore Files</a>
      </div>
    </div>
  </div>

  {#if activeJobs.length > 0}
    <div class="active-operations">
      <h2>Active Operations</h2>
      {#each activeJobs as job}
        <div class="terminal-card">
          <div class="terminal-header">
            <span class="terminal-title">{getPhaseIcon(job.phase)} {job.job_name}</span>
            <span class="terminal-phase badge badge-warning">{job.phase}</span>
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
{/if}

<style>
  .stats-grid {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
    gap: 1rem;
    margin-bottom: 1.5rem;
  }

  .stat-card {
    background: white;
    border-radius: 12px;
    padding: 1.25rem;
    display: flex;
    align-items: center;
    box-shadow: 0 2px 8px rgba(0, 0, 0, 0.08);
  }

  .stat-icon {
    font-size: 2rem;
    margin-right: 1rem;
  }

  .stat-value {
    font-size: 1.5rem;
    font-weight: 700;
    color: #333;
  }

  .stat-label {
    font-size: 0.875rem;
    color: #666;
  }

  .dashboard-row {
    display: grid;
    grid-template-columns: 1fr 2fr;
    gap: 1.5rem;
  }

  .drive-status-card h2,
  .quick-actions h2 {
    margin: 0 0 1rem;
    font-size: 1rem;
    color: #333;
  }

  .drive-status {
    display: flex;
    align-items: center;
    padding: 1rem;
    border-radius: 8px;
    background: #f5f5f5;
  }

  .drive-status.online {
    background: #d4edda;
  }

  .drive-status.offline {
    background: #f8d7da;
  }

  .status-indicator {
    width: 12px;
    height: 12px;
    border-radius: 50%;
    margin-right: 0.75rem;
  }

  .drive-status.online .status-indicator {
    background: #28a745;
  }

  .drive-status.offline .status-indicator {
    background: #dc3545;
  }

  .status-text {
    font-weight: 600;
    text-transform: capitalize;
  }

  .loaded-tape-info {
    margin-top: 1rem;
    padding: 0.75rem;
    background: #f0f0f5;
    border-radius: 8px;
  }

  .loaded-tape-info h3 {
    margin: 0 0 0.5rem;
    font-size: 0.875rem;
    color: #555;
  }

  .tape-detail {
    font-size: 0.875rem;
    margin-bottom: 0.25rem;
  }

  .tape-detail .uuid {
    font-family: monospace;
    font-size: 0.75rem;
    color: #888;
  }

  .no-tape {
    font-size: 0.875rem;
    color: #888;
    margin: 0;
    font-style: italic;
  }

  .action-buttons {
    display: flex;
    gap: 0.75rem;
    flex-wrap: wrap;
  }

  .error {
    color: #dc3545;
  }

  .active-operations {
    margin-top: 1.5rem;
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
    font-size: 0.75rem;
    text-transform: uppercase;
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
