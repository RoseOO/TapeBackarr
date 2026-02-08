<script lang="ts">
  import { onMount } from 'svelte';
  import * as api from '$lib/api/client';

  interface Drive {
    id: number;
    name: string;
    device_path: string;
    status: string;
  }

  interface InspectResult {
    drive_id: number;
    device_path: string;
    status: string;
    label?: string;
    uuid?: string;
    pool?: string;
    lto_type?: string;
    capacity_bytes?: number;
    has_tapebackarr_label: boolean;
    label_message?: string;
    encrypted?: boolean;
    encryption_key_fingerprint?: string;
    compressed?: boolean;
    compression_type?: string;
    contents?: any[];
    contents_error?: string;
    error?: string;
    message?: string;
  }

  let drives: Drive[] = [];
  let selectedDriveId: number = 0;
  let result: InspectResult | null = null;
  let loading = false;
  let loadingDrives = true;
  let error = '';

  onMount(async () => {
    try {
      drives = await api.getDrives();
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to load drives';
    } finally {
      loadingDrives = false;
    }
  });

  async function handleInspect() {
    if (!selectedDriveId) return;
    loading = true;
    error = '';
    result = null;
    try {
      result = await api.inspectTape(selectedDriveId);
    } catch (e) {
      error = e instanceof Error ? e.message : 'Inspection failed';
    } finally {
      loading = false;
    }
  }

  async function handleScanDbBackup() {
    if (!selectedDriveId) return;
    loading = true;
    error = '';
    try {
      const scanResult = await api.api.get(`/drives/${selectedDriveId}/scan-for-db-backup`);
      result = { ...result, ...scanResult } as any;
    } catch (e) {
      error = e instanceof Error ? e.message : 'DB scan failed';
    } finally {
      loading = false;
    }
  }

  function formatBytes(bytes: number): string {
    if (!bytes) return '0 B';
    const k = 1024;
    const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
  }
</script>

<div class="page-header">
  <h1>🔍 Tape Inspector</h1>
</div>

{#if error}
  <div class="card" style="background: var(--badge-danger-bg); color: var(--badge-danger-text);">
    <p>{error}</p>
    <button class="btn btn-secondary" on:click={() => error = ''}>Dismiss</button>
  </div>
{/if}

<div class="card">
  <h2>Select Drive</h2>
  {#if loadingDrives}
    <p>Loading drives...</p>
  {:else}
    <div style="display: flex; gap: 0.5rem; align-items: flex-end;">
      <div class="form-group" style="flex: 1; margin-bottom: 0;">
        <select bind:value={selectedDriveId}>
          <option value={0}>Select a drive...</option>
          {#each drives as drive}
            <option value={drive.id}>{drive.name} ({drive.device_path}) - {drive.status}</option>
          {/each}
        </select>
      </div>
      <button class="btn btn-primary" on:click={handleInspect} disabled={!selectedDriveId || loading}>
        {loading ? 'Inspecting...' : '🔍 Inspect Tape'}
      </button>
      <button class="btn btn-secondary" on:click={handleScanDbBackup} disabled={!selectedDriveId || loading}>
        🗄️ Scan for DB Backups
      </button>
    </div>
  {/if}
</div>

{#if loading}
  <div class="card">
    <div class="loading-indicator">
      <div class="spinner"></div>
      <p>Inspecting tape... This may take a moment as the tape is being read.</p>
      <p style="font-size: 0.8rem; color: var(--text-muted);">Check the system console for live progress updates.</p>
    </div>
  </div>
{/if}

{#if result && !loading}
  <!-- Label Info -->
  <div class="card">
    <h2>Tape Label</h2>
    {#if result.has_tapebackarr_label}
      <div class="label-grid">
        <div class="label-item">
          <span class="label-key">Label</span>
          <span class="label-value"><strong>{result.label}</strong></span>
        </div>
        <div class="label-item">
          <span class="label-key">UUID</span>
          <span class="label-value"><code>{result.uuid}</code></span>
        </div>
        {#if result.pool}
          <div class="label-item">
            <span class="label-key">Pool</span>
            <span class="label-value">{result.pool}</span>
          </div>
        {/if}
        {#if result.lto_type}
          <div class="label-item">
            <span class="label-key">LTO Type</span>
            <span class="label-value">{result.lto_type}</span>
          </div>
        {/if}
        {#if result.capacity_bytes}
          <div class="label-item">
            <span class="label-key">Capacity</span>
            <span class="label-value">{formatBytes(result.capacity_bytes)}</span>
          </div>
        {/if}
        <div class="label-item">
          <span class="label-key">Encryption</span>
          <span class="label-value">
            {#if result.encrypted}
              <span class="badge badge-warning">🔒 Encrypted</span>
              <code style="margin-left: 0.5rem; font-size: 0.75rem;">{result.encryption_key_fingerprint}</code>
            {:else}
              <span class="badge badge-success">Not encrypted</span>
            {/if}
          </span>
        </div>
        <div class="label-item">
          <span class="label-key">Compression</span>
          <span class="label-value">
            {#if result.compressed}
              <span class="badge badge-info">📦 {result.compression_type}</span>
            {:else}
              <span class="badge badge-success">None</span>
            {/if}
          </span>
        </div>
      </div>
    {:else}
      <div style="padding: 1rem; background: var(--badge-warning-bg); color: var(--badge-warning-text); border-radius: 8px;">
        <strong>⚠️ No TapeBackarr Label</strong>
        <p style="margin: 0.5rem 0 0;">{result.label_message || 'This tape does not have a TapeBackarr label. It may be a foreign tape, blank, or from another backup system.'}</p>
      </div>
    {/if}
  </div>

  <!-- Contents -->
  <div class="card">
    <h2>Tape Contents {result.contents ? `(${result.contents.length} entries)` : ''}</h2>
    {#if result.contents_error}
      <div style="padding: 0.75rem; background: var(--badge-warning-bg); color: var(--badge-warning-text); border-radius: 6px; margin-bottom: 1rem;">
        {result.contents_error}
      </div>
    {/if}
    {#if result.contents && result.contents.length > 0}
      <div style="max-height: 400px; overflow-y: auto;">
        <table>
          <thead>
            <tr>
              <th>Name/Path</th>
              <th>Size</th>
              <th>Modified</th>
              <th>Type</th>
            </tr>
          </thead>
          <tbody>
            {#each result.contents as entry}
              <tr>
                <td><code style="font-size: 0.8rem;">{entry.name || entry.path || '-'}</code></td>
                <td>{entry.size ? formatBytes(entry.size) : '-'}</td>
                <td style="font-size: 0.8rem;">{entry.mod_time || entry.date || '-'}</td>
                <td>{entry.type || entry.permissions || '-'}</td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
    {:else if !result.contents_error}
      <p style="color: var(--text-muted); text-align: center;">No contents found on tape.</p>
    {/if}
  </div>
{/if}

<style>
  .loading-indicator {
    display: flex;
    flex-direction: column;
    align-items: center;
    padding: 2rem;
  }

  .spinner {
    width: 40px;
    height: 40px;
    border: 4px solid var(--border-color);
    border-top-color: var(--accent-primary);
    border-radius: 50%;
    animation: spin 1s linear infinite;
    margin-bottom: 1rem;
  }

  @keyframes spin {
    to { transform: rotate(360deg); }
  }

  .label-grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(250px, 1fr));
    gap: 0.75rem;
  }

  .label-item {
    display: flex;
    flex-direction: column;
    gap: 0.25rem;
    padding: 0.5rem;
    background: var(--bg-input);
    border-radius: 6px;
  }

  .label-key {
    font-size: 0.75rem;
    color: var(--text-muted);
    text-transform: uppercase;
    font-weight: 600;
  }

  .label-value {
    font-size: 0.9rem;
  }

  code {
    background: var(--code-bg);
    padding: 0.1rem 0.3rem;
    border-radius: 3px;
    font-size: 0.8rem;
  }
</style>
