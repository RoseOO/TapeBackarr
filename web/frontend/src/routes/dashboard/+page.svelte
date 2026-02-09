<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import * as api from '$lib/api/client';
  import { dataVersion } from '$lib/stores/livedata';

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
    loaded_tape_encrypted: boolean;
    loaded_tape_enc_key_fingerprint: string;
    loaded_tape_compression: string;
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
  }

  let stats: DashboardStats | null = null;
  let activeJobs: ActiveJob[] = [];
  let loading = true;
  let error = '';
  let pollInterval: ReturnType<typeof setInterval>;

  // Subscribe to SSE-driven data invalidation for dashboard
  const dashboardVersion = dataVersion('dashboard');
  let lastVersion = 0;
  const unsubVersion = dashboardVersion.subscribe(v => {
    if (v > lastVersion && lastVersion > 0) {
      // Data invalidated by SSE event - refresh dashboard
      refreshDashboard();
    }
    lastVersion = v;
  });

  async function refreshDashboard() {
    try {
      stats = await api.getDashboard();
    } catch { /* ignore refresh errors */ }
    await loadActiveJobs();
  }

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
    unsubVersion();
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
          {#if stats.loaded_tape_encrypted}
            <span class="badge badge-warning">🔒 Encrypted</span>
          {/if}
          {#if stats.loaded_tape_compression && stats.loaded_tape_compression !== 'none' && stats.loaded_tape_compression !== ''}
            <span class="badge badge-info">📦 {stats.loaded_tape_compression}</span>
          {/if}
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
            <span class="terminal-title">
              {getPhaseIcon(job.phase)} {job.job_name}
              {#if job.status === 'paused'}
                <span class="status-badge paused">PAUSED</span>
              {/if}
            </span>
            <span class="terminal-phase badge badge-warning">{job.phase}</span>
          </div>
          <div class="terminal-meta">
            {#if job.tape_label}
              <span>📼 {job.tape_label}</span>
            {/if}
            {#if job.bytes_written > 0}
              <span>{formatBytes(job.bytes_written)} / {formatBytes(job.total_bytes)}</span>
            {/if}
            {#if job.write_speed > 0}
              <span>⚡ {formatSpeed(job.write_speed)}</span>
            {/if}
            {#if job.estimated_seconds_remaining > 0}
              <span>ETA: {formatETA(job.estimated_seconds_remaining)}</span>
            {/if}
            <span>Started: {new Date(job.start_time).toLocaleTimeString()}</span>
          </div>
          {#if job.total_bytes > 0}
            <div class="terminal-progress">
              <div class="dash-progress-bar">
                <div class="dash-progress-fill" style="width: {getProgressPercent(job)}%"></div>
              </div>
              <span class="dash-progress-text">{getProgressPercent(job).toFixed(1)}%</span>
            </div>
          {/if}
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
    background: var(--bg-card);
    border-radius: 12px;
    padding: 1.25rem;
    display: flex;
    align-items: center;
    box-shadow: var(--shadow);
  }

  .stat-icon {
    font-size: 2rem;
    margin-right: 1rem;
  }

  .stat-value {
    font-size: 1.5rem;
    font-weight: 700;
    color: var(--text-primary);
  }

  .stat-label {
    font-size: 0.875rem;
    color: var(--text-secondary);
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
    color: var(--text-primary);
  }

  .drive-status {
    display: flex;
    align-items: center;
    padding: 1rem;
    border-radius: 8px;
    background: var(--bg-input);
  }

  .drive-status.online {
    background: var(--badge-success-bg);
  }

  .drive-status.offline {
    background: var(--badge-danger-bg);
  }

  .status-indicator {
    width: 12px;
    height: 12px;
    border-radius: 50%;
    margin-right: 0.75rem;
  }

  .drive-status.online .status-indicator {
    background: var(--accent-success);
  }

  .drive-status.offline .status-indicator {
    background: var(--accent-danger);
  }

  .status-text {
    font-weight: 600;
    text-transform: capitalize;
    color: var(--text-primary);
  }

  .loaded-tape-info {
    margin-top: 1rem;
    padding: 0.75rem;
    background: var(--bg-input);
    border-radius: 8px;
  }

  .loaded-tape-info h3 {
    margin: 0 0 0.5rem;
    font-size: 0.875rem;
    color: var(--text-secondary);
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
    color: var(--text-muted);
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
    color: var(--text-primary);
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

  .terminal-progress {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    padding: 0.4rem 1rem;
    background: #222238;
  }

  .dash-progress-bar {
    flex: 1;
    height: 8px;
    background: #333;
    border-radius: 4px;
    overflow: hidden;
  }

  .dash-progress-fill {
    height: 100%;
    border-radius: 4px;
    background: linear-gradient(90deg, #4a4aff, #16c784);
    transition: width 0.5s ease;
  }

  .dash-progress-text {
    color: #aaa;
    font-size: 0.7rem;
    font-family: monospace;
    width: 4em;
    text-align: right;
  }
</style>
