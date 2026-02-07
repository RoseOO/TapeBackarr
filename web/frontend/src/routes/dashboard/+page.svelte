<script lang="ts">
  import { onMount } from 'svelte';
  import * as api from '$lib/api/client';

  interface DashboardStats {
    total_tapes: number;
    active_tapes: number;
    total_jobs: number;
    running_jobs: number;
    recent_backups: number;
    drive_status: string;
    total_data_bytes: number;
  }

  let stats: DashboardStats | null = null;
  let loading = true;
  let error = '';

  onMount(async () => {
    try {
      stats = await api.getDashboard();
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to load dashboard';
    } finally {
      loading = false;
    }
  });

  function formatBytes(bytes: number): string {
    if (bytes === 0) return '0 B';
    const k = 1024;
    const sizes = ['B', 'KB', 'MB', 'GB', 'TB', 'PB'];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
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

  .action-buttons {
    display: flex;
    gap: 0.75rem;
    flex-wrap: wrap;
  }

  .error {
    color: #dc3545;
  }
</style>
