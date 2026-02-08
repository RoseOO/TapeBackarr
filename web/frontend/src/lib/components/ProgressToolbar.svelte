<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import * as api from '$lib/api/client';
  import { auth } from '$lib/stores/auth';

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
    tape_estimated_seconds_remaining: number;
    compression: string;
    start_time: string;
    updated_at: string;
  }

  let activeJobs: ActiveJob[] = [];
  let pollInterval: ReturnType<typeof setInterval>;

  $: isAuthenticated = $auth.isAuthenticated;

  onMount(async () => {
    await loadActiveJobs();
    pollInterval = setInterval(loadActiveJobs, 2000);
  });

  onDestroy(() => {
    if (pollInterval) clearInterval(pollInterval);
  });

  async function loadActiveJobs() {
    if (!isAuthenticated) return;
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

  function getTapeProgressPercent(job: ActiveJob): number {
    if (job.tape_capacity_bytes <= 0) return 0;
    const totalUsed = job.tape_used_bytes + job.bytes_written;
    return Math.min(100, (totalUsed / job.tape_capacity_bytes) * 100);
  }

  function getPhaseIcon(phase: string): string {
    switch (phase) {
      case 'initializing': return '⏳';
      case 'scanning': return '🔍';
      case 'streaming': return '📼';
      case 'cataloging': return '📝';
      case 'completed': return '✅';
      case 'failed': return '❌';
      case 'cancelled': return '🚫';
      default: return '⏳';
    }
  }

  $: hasActiveJobs = activeJobs.length > 0;
</script>

{#if isAuthenticated && hasActiveJobs}
  <div class="progress-toolbar">
    {#each activeJobs as job}
      <div class="toolbar-job">
        <div class="toolbar-row">
          <span class="toolbar-title">
            {getPhaseIcon(job.phase)} {job.job_name}
            {#if job.status === 'paused'}
              <span class="status-badge paused">PAUSED</span>
            {/if}
          </span>
          <span class="toolbar-tape">
            {#if job.tape_label}📼 {job.tape_label}{/if}
            {#if job.device_path} ({job.device_path}){/if}
          </span>
          <span class="toolbar-stats">
            {#if job.phase === 'streaming'}
              {formatBytes(job.bytes_written)} / {formatBytes(job.total_bytes)}
              · {formatSpeed(job.write_speed)}
              · Job ETA: {formatETA(job.estimated_seconds_remaining)}
              {#if job.tape_estimated_seconds_remaining > 0}
                · Tape ETA: {formatETA(job.tape_estimated_seconds_remaining)}
              {/if}
            {:else if job.phase === 'scanning'}
              Scanning...
            {:else if job.phase === 'cataloging'}
              Files: {job.file_count}/{job.total_files}
            {:else}
              {job.phase}
            {/if}
          </span>
        </div>
        <div class="toolbar-bars">
          <div class="bar-group">
            <span class="bar-label">Job</span>
            <div class="progress-bar">
              <div class="progress-fill job-fill" style="width: {getProgressPercent(job)}%"></div>
            </div>
            <span class="bar-percent">{getProgressPercent(job).toFixed(1)}%</span>
          </div>
          {#if job.tape_capacity_bytes > 0}
            <div class="bar-group">
              <span class="bar-label">Tape</span>
              <div class="progress-bar">
                <div class="progress-fill tape-fill" style="width: {getTapeProgressPercent(job)}%"></div>
              </div>
              <span class="bar-percent">{getTapeProgressPercent(job).toFixed(1)}%</span>
            </div>
          {/if}
        </div>
      </div>
    {/each}
  </div>
{/if}

<style>
  .progress-toolbar {
    position: fixed;
    bottom: 35px;
    left: 240px;
    right: 0;
    z-index: 910;
    background: #1a1a2e;
    border-top: 2px solid #16c784;
    font-family: 'Consolas', 'Monaco', 'Courier New', monospace;
    font-size: 0.75rem;
    color: #e0e0e0;
    padding: 0.4rem 1rem;
  }

  .toolbar-job {
    display: flex;
    flex-direction: column;
    gap: 0.25rem;
  }

  .toolbar-job + .toolbar-job {
    border-top: 1px solid #333;
    padding-top: 0.25rem;
    margin-top: 0.25rem;
  }

  .toolbar-row {
    display: flex;
    align-items: center;
    gap: 1rem;
    flex-wrap: wrap;
  }

  .toolbar-title {
    font-weight: 700;
    color: #cdd6f4;
    flex-shrink: 0;
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

  .toolbar-tape {
    color: #a6adc8;
    flex-shrink: 0;
  }

  .toolbar-stats {
    color: #89dceb;
    margin-left: auto;
  }

  .toolbar-bars {
    display: flex;
    gap: 1rem;
    align-items: center;
  }

  .bar-group {
    display: flex;
    align-items: center;
    gap: 0.4rem;
    flex: 1;
  }

  .bar-label {
    color: #666;
    font-size: 0.65rem;
    width: 2.5em;
    flex-shrink: 0;
  }

  .progress-bar {
    flex: 1;
    height: 6px;
    background: #333;
    border-radius: 3px;
    overflow: hidden;
    min-width: 80px;
  }

  .progress-fill {
    height: 100%;
    border-radius: 3px;
    transition: width 0.5s ease;
  }

  .job-fill {
    background: linear-gradient(90deg, #4a4aff, #16c784);
  }

  .tape-fill {
    background: linear-gradient(90deg, #f39c12, #e74c3c);
  }

  .bar-percent {
    color: #888;
    font-size: 0.65rem;
    width: 3.5em;
    text-align: right;
    flex-shrink: 0;
  }
</style>
