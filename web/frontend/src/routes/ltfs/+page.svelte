<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import * as api from '$lib/api/client';

  interface Drive {
    id: number;
    name: string;
    device_path: string;
    status: string;
  }

  interface BrowseEntry {
    name: string;
    path: string;
    is_dir: boolean;
    size: number;
    mod_time?: string;
  }

  interface LTFSLabel {
    magic: string;
    label: string;
    uuid: string;
    pool: string;
    format: string;
    created_at: string;
  }

  interface LTFSStatus {
    available: boolean;
    enabled: boolean;
    mounted: boolean;
    mount_point: string;
    message?: string;
    volume_info?: any;
    label?: LTFSLabel;
  }

  let drives: Drive[] = [];
  let selectedDriveId: number = 0;
  let ltfsStatus: LTFSStatus | null = null;
  let loading = false;
  let loadingDrives = true;
  let error = '';
  let successMsg = '';

  // Format modal
  let showFormatModal = false;
  let formatLabel = '';
  let formatUUID = '';
  let formatPool = '';
  let formatting = false;

  // File browser state
  let browseEntries: BrowseEntry[] = [];
  let browsePrefix = '';
  let browsePath: string[] = [];
  let browseLabel: LTFSLabel | null = null;
  let browseTotalFiles = 0;
  let browsing = false;

  // Restore modal
  let showRestoreModal = false;
  let restoreFiles: string[] = [];
  let restoreDestPath = '';
  let restoring = false;

  // Selection state
  let selectedFiles: Set<string> = new Set();
  let selectAll = false;

  // Tab state
  let activeTab: 'status' | 'browser' = 'status';

  let successTimeout: ReturnType<typeof setTimeout>;

  function clearSuccess() {
    if (successTimeout) clearTimeout(successTimeout);
    successTimeout = setTimeout(() => { successMsg = ''; }, 4000);
  }

  onMount(async () => {
    try {
      drives = await api.getDrives();
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to load drives';
    } finally {
      loadingDrives = false;
    }
    await refreshStatus();
  });

  onDestroy(() => {
    if (successTimeout) clearTimeout(successTimeout);
  });

  async function refreshStatus() {
    try {
      ltfsStatus = await api.getLTFSStatus(selectedDriveId || undefined);
    } catch (e) {
      // Silently fail — status card will show unknown
    }
  }

  async function handleMount() {
    if (!selectedDriveId) { error = 'Select a drive first'; return; }
    loading = true;
    error = '';
    try {
      await api.mountLTFS(selectedDriveId);
      successMsg = 'LTFS tape mounted successfully';
      clearSuccess();
      await refreshStatus();
      // Auto-switch to browser tab
      if (ltfsStatus?.mounted) {
        activeTab = 'browser';
        await handleBrowse('');
      }
    } catch (e) {
      error = e instanceof Error ? e.message : 'Mount failed';
    } finally {
      loading = false;
    }
  }

  async function handleUnmount() {
    loading = true;
    error = '';
    try {
      await api.unmountLTFS();
      successMsg = 'LTFS tape unmounted successfully';
      clearSuccess();
      browseEntries = [];
      browsePrefix = '';
      browsePath = [];
      activeTab = 'status';
      await refreshStatus();
    } catch (e) {
      error = e instanceof Error ? e.message : 'Unmount failed';
    } finally {
      loading = false;
    }
  }

  function openFormatModal() {
    formatLabel = '';
    formatUUID = crypto.randomUUID();
    formatPool = '';
    showFormatModal = true;
  }

  async function handleFormat() {
    if (!selectedDriveId) { error = 'Select a drive first'; return; }
    formatting = true;
    error = '';
    try {
      await api.formatLTFS(selectedDriveId, formatLabel, formatUUID, formatPool, true);
      successMsg = 'Tape formatted with LTFS successfully';
      clearSuccess();
      showFormatModal = false;
      await refreshStatus();
    } catch (e) {
      error = e instanceof Error ? e.message : 'Format failed';
    } finally {
      formatting = false;
    }
  }

  async function handleCheck() {
    if (!selectedDriveId) { error = 'Select a drive first'; return; }
    loading = true;
    error = '';
    try {
      await api.checkLTFS(selectedDriveId);
      successMsg = 'LTFS consistency check passed';
      clearSuccess();
    } catch (e) {
      error = e instanceof Error ? e.message : 'Check failed';
    } finally {
      loading = false;
    }
  }

  // ─── File Browser ────────────────────────────────────────

  async function handleBrowse(prefix: string) {
    browsing = true;
    error = '';
    browsePrefix = prefix;
    selectedFiles = new Set();
    selectAll = false;
    try {
      const result = await api.browseLTFS(prefix || undefined);
      browseEntries = result.entries || [];
      browseTotalFiles = result.total_files || 0;
      browseLabel = result.label || null;
      // Build breadcrumb path
      if (prefix) {
        browsePath = prefix.split('/').filter((p: string) => p);
      } else {
        browsePath = [];
      }
    } catch (e) {
      error = e instanceof Error ? e.message : 'Browse failed';
    } finally {
      browsing = false;
    }
  }

  function navigateToDir(entry: BrowseEntry) {
    if (entry.is_dir) {
      handleBrowse(entry.path);
    }
  }

  function navigateBreadcrumb(index: number) {
    if (index < 0) {
      handleBrowse('');
    } else {
      const path = browsePath.slice(0, index + 1).join('/');
      handleBrowse(path);
    }
  }

  function toggleFileSelection(path: string) {
    if (selectedFiles.has(path)) {
      selectedFiles.delete(path);
    } else {
      selectedFiles.add(path);
    }
    selectedFiles = new Set(selectedFiles); // trigger reactivity
    selectAll = selectedFiles.size === browseEntries.filter(e => !e.is_dir).length;
  }

  function toggleSelectAll() {
    selectAll = !selectAll;
    if (selectAll) {
      browseEntries.filter(e => !e.is_dir).forEach(e => selectedFiles.add(e.path));
    } else {
      selectedFiles.clear();
    }
    selectedFiles = new Set(selectedFiles);
  }

  function openRestoreModal() {
    restoreFiles = Array.from(selectedFiles);
    restoreDestPath = '/restore';
    showRestoreModal = true;
  }

  function openRestoreAllModal() {
    restoreFiles = [];
    restoreDestPath = '/restore';
    showRestoreModal = true;
  }

  async function handleRestore() {
    restoring = true;
    error = '';
    try {
      const result = await api.restoreLTFS(restoreFiles, restoreDestPath);
      successMsg = `Restored ${result.files} files (${formatBytes(result.total_bytes)}) to ${restoreDestPath}`;
      clearSuccess();
      showRestoreModal = false;
      selectedFiles = new Set();
      selectAll = false;
    } catch (e) {
      error = e instanceof Error ? e.message : 'Restore failed';
    } finally {
      restoring = false;
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
  <h1>📂 LTFS Management</h1>
  <p style="color: var(--text-muted); margin: 0.25rem 0 0;">Linear Tape File System — self-describing tape format</p>
</div>

{#if error}
  <div class="card" style="background: var(--badge-danger-bg); color: var(--badge-danger-text);">
    <p>{error}</p>
    <button class="btn btn-secondary" on:click={() => error = ''}>Dismiss</button>
  </div>
{/if}

{#if successMsg}
  <div class="card" style="background: var(--badge-success-bg); color: var(--badge-success-text);">
    <p>✅ {successMsg}</p>
  </div>
{/if}

<!-- Drive Selection & Actions -->
<div class="card">
  <h2>Drive & Controls</h2>
  {#if loadingDrives}
    <p>Loading drives...</p>
  {:else}
    <div style="display: flex; gap: 0.5rem; align-items: flex-end; flex-wrap: wrap;">
      <div class="form-group" style="flex: 1; min-width: 200px; margin-bottom: 0;">
        <label for="drive-select">Tape Drive</label>
        <select id="drive-select" bind:value={selectedDriveId} on:change={refreshStatus}>
          <option value={0}>Select a drive...</option>
          {#each drives as drive}
            <option value={drive.id}>{drive.name} ({drive.device_path}) — {drive.status}</option>
          {/each}
        </select>
      </div>

      {#if ltfsStatus?.mounted}
        <button class="btn btn-primary" on:click={() => { activeTab = 'browser'; handleBrowse(''); }} disabled={loading}>
          📂 Browse Files
        </button>
        <button class="btn btn-secondary" on:click={handleUnmount} disabled={loading}>
          ⏏️ Unmount
        </button>
      {:else}
        <button class="btn btn-primary" on:click={handleMount} disabled={!selectedDriveId || loading}>
          {loading ? 'Mounting...' : '💿 Mount LTFS'}
        </button>
      {/if}

      <button class="btn btn-secondary" on:click={openFormatModal} disabled={!selectedDriveId || loading || ltfsStatus?.mounted}>
        🗑️ Format LTFS
      </button>
      <button class="btn btn-secondary" on:click={handleCheck} disabled={!selectedDriveId || loading}>
        🔍 Check
      </button>
    </div>
  {/if}
</div>

<!-- Tabs -->
<div class="tabs">
  <button class="tab" class:active={activeTab === 'status'} on:click={() => activeTab = 'status'}>
    ℹ️ Status
  </button>
  <button class="tab" class:active={activeTab === 'browser'} on:click={() => { activeTab = 'browser'; if (ltfsStatus?.mounted) handleBrowse(browsePrefix); }}>
    📂 File Browser
  </button>
</div>

<!-- Status Tab -->
{#if activeTab === 'status'}
  <div class="card">
    <h2>LTFS Status</h2>
    {#if !ltfsStatus}
      <p style="color: var(--text-muted);">Select a drive and click refresh to check LTFS status.</p>
    {:else}
      <div class="label-grid">
        <div class="label-item">
          <span class="label-key">LTFS Available</span>
          <span class="label-value">
            {#if ltfsStatus.available}
              <span class="badge badge-success">✅ Installed</span>
            {:else}
              <span class="badge badge-danger">❌ Not Installed</span>
            {/if}
          </span>
        </div>
        <div class="label-item">
          <span class="label-key">LTFS Enabled</span>
          <span class="label-value">
            {#if ltfsStatus.enabled}
              <span class="badge badge-success">Enabled</span>
            {:else}
              <span class="badge badge-warning">Disabled</span>
            {/if}
          </span>
        </div>
        <div class="label-item">
          <span class="label-key">Mount Status</span>
          <span class="label-value">
            {#if ltfsStatus.mounted}
              <span class="badge badge-success">🟢 Mounted</span>
            {:else}
              <span class="badge badge-secondary">⚪ Not Mounted</span>
            {/if}
          </span>
        </div>
        <div class="label-item">
          <span class="label-key">Mount Point</span>
          <span class="label-value"><code>{ltfsStatus.mount_point}</code></span>
        </div>
      </div>

      {#if ltfsStatus.label}
        <h3 style="margin-top: 1.5rem;">Tape Label</h3>
        <div class="label-grid">
          <div class="label-item">
            <span class="label-key">Label</span>
            <span class="label-value"><strong>{ltfsStatus.label.label}</strong></span>
          </div>
          <div class="label-item">
            <span class="label-key">UUID</span>
            <span class="label-value"><code>{ltfsStatus.label.uuid}</code></span>
          </div>
          <div class="label-item">
            <span class="label-key">Pool</span>
            <span class="label-value">{ltfsStatus.label.pool || '—'}</span>
          </div>
          <div class="label-item">
            <span class="label-key">Format</span>
            <span class="label-value"><span class="badge badge-info">LTFS</span></span>
          </div>
          <div class="label-item">
            <span class="label-key">Created</span>
            <span class="label-value">{ltfsStatus.label.created_at || '—'}</span>
          </div>
        </div>
      {/if}

      {#if ltfsStatus.volume_info}
        <h3 style="margin-top: 1.5rem;">Volume Info</h3>
        <div class="label-grid">
          {#if ltfsStatus.volume_info.volume_name}
            <div class="label-item">
              <span class="label-key">Volume Name</span>
              <span class="label-value">{ltfsStatus.volume_info.volume_name}</span>
            </div>
          {/if}
          {#if ltfsStatus.volume_info.used_bytes}
            <div class="label-item">
              <span class="label-key">Used</span>
              <span class="label-value">{formatBytes(ltfsStatus.volume_info.used_bytes)}</span>
            </div>
          {/if}
          {#if ltfsStatus.volume_info.available_bytes}
            <div class="label-item">
              <span class="label-key">Available</span>
              <span class="label-value">{formatBytes(ltfsStatus.volume_info.available_bytes)}</span>
            </div>
          {/if}
        </div>
      {/if}

      {#if !ltfsStatus.available}
        <div style="margin-top: 1rem; padding: 1rem; background: var(--badge-warning-bg); color: var(--badge-warning-text); border-radius: 8px;">
          <strong>⚠️ LTFS Not Installed</strong>
          <p style="margin: 0.5rem 0 0;">Install LTFS tools (<code>mkltfs</code> and <code>ltfs</code>) to use LTFS features. See <a href="https://github.com/LinearTapeFileSystem/ltfs" target="_blank">github.com/LinearTapeFileSystem/ltfs</a> for installation instructions.</p>
        </div>
      {/if}
    {/if}
  </div>
{/if}

<!-- File Browser Tab -->
{#if activeTab === 'browser'}
  <div class="card">
    {#if !ltfsStatus?.mounted}
      <div style="text-align: center; padding: 2rem;">
        <p style="font-size: 1.2rem; color: var(--text-muted);">📂 No LTFS Volume Mounted</p>
        <p style="color: var(--text-muted);">Select a drive and click <strong>Mount LTFS</strong> to browse files on the tape.</p>
      </div>
    {:else}
      <!-- Breadcrumb Navigation -->
      <div class="breadcrumb">
        <button class="breadcrumb-item" class:active={browsePath.length === 0} on:click={() => navigateBreadcrumb(-1)}>
          🏠 Root
        </button>
        {#each browsePath as segment, i}
          <span class="breadcrumb-sep">/</span>
          <button class="breadcrumb-item" class:active={i === browsePath.length - 1} on:click={() => navigateBreadcrumb(i)}>
            {segment}
          </button>
        {/each}
        {#if browseLabel}
          <span style="margin-left: auto; font-size: 0.8rem; color: var(--text-muted);">
            Tape: <strong>{browseLabel.label}</strong> • {browseTotalFiles} files
          </span>
        {/if}
      </div>

      <!-- Action Bar -->
      <div style="display: flex; gap: 0.5rem; margin: 0.75rem 0; flex-wrap: wrap;">
        <button class="btn btn-secondary btn-sm" on:click={() => handleBrowse(browsePrefix)} disabled={browsing}>
          🔄 Refresh
        </button>
        {#if selectedFiles.size > 0}
          <button class="btn btn-primary btn-sm" on:click={openRestoreModal}>
            📥 Restore Selected ({selectedFiles.size})
          </button>
        {/if}
        <button class="btn btn-secondary btn-sm" on:click={openRestoreAllModal}>
          📥 Restore All
        </button>
      </div>

      {#if browsing}
        <div class="loading-indicator">
          <div class="spinner"></div>
          <p>Reading tape filesystem...</p>
        </div>
      {:else if browseEntries.length === 0}
        <p style="text-align: center; color: var(--text-muted); padding: 2rem;">
          {browsePrefix ? 'This directory is empty.' : 'No files found on this LTFS volume.'}
        </p>
      {:else}
        <div style="max-height: 600px; overflow-y: auto;">
          <table>
            <thead>
              <tr>
                <th style="width: 30px;">
                  <input type="checkbox" checked={selectAll} on:change={toggleSelectAll} title="Select all files" />
                </th>
                <th>Name</th>
                <th style="width: 100px;">Size</th>
                <th style="width: 160px;">Modified</th>
              </tr>
            </thead>
            <tbody>
              {#if browsePrefix}
                <tr class="clickable" on:click={() => {
                  const parts = browsePrefix.split('/').filter(p => p);
                  parts.pop();
                  handleBrowse(parts.join('/'));
                }}>
                  <td></td>
                  <td>📁 <strong>..</strong></td>
                  <td></td>
                  <td></td>
                </tr>
              {/if}
              {#each browseEntries as entry}
                <tr class:clickable={entry.is_dir} on:click={() => entry.is_dir && navigateToDir(entry)}>
                  <td on:click|stopPropagation>
                    {#if !entry.is_dir}
                      <input type="checkbox" checked={selectedFiles.has(entry.path)} on:change={() => toggleFileSelection(entry.path)} />
                    {/if}
                  </td>
                  <td>
                    {#if entry.is_dir}
                      📁 <strong>{entry.name}</strong>
                    {:else}
                      📄 {entry.name}
                    {/if}
                  </td>
                  <td>{entry.is_dir ? '' : formatBytes(entry.size)}</td>
                  <td style="font-size: 0.8rem; color: var(--text-muted);">{entry.mod_time || ''}</td>
                </tr>
              {/each}
            </tbody>
          </table>
        </div>
      {/if}
    {/if}
  </div>
{/if}

<!-- Format LTFS Modal -->
{#if showFormatModal}
  <div class="modal-overlay" on:click={() => showFormatModal = false}>
    <div class="modal" on:click|stopPropagation>
      <h2>🗑️ Format Tape with LTFS</h2>
      <div style="padding: 0.75rem; background: var(--badge-danger-bg); color: var(--badge-danger-text); border-radius: 6px; margin-bottom: 1rem;">
        <strong>⚠️ Warning:</strong> This will erase ALL data on the tape and format it with LTFS.
      </div>
      <div class="form-group">
        <label for="fmt-label">Tape Label</label>
        <input id="fmt-label" type="text" bind:value={formatLabel} placeholder="e.g. BKP001" maxlength="6" />
        <small>Up to 6 characters (LTO barcode format)</small>
      </div>
      <div class="form-group">
        <label for="fmt-uuid">UUID</label>
        <input id="fmt-uuid" type="text" bind:value={formatUUID} readonly />
        <span style="font-size: 0.75rem; color: var(--text-muted);">Auto-generated unique identifier</span>
      </div>
      <div class="form-group">
        <label for="fmt-pool">Pool (optional)</label>
        <input id="fmt-pool" type="text" bind:value={formatPool} placeholder="e.g. daily, weekly" />
      </div>
      <div class="modal-actions">
        <button class="btn btn-secondary" on:click={() => showFormatModal = false} disabled={formatting}>Cancel</button>
        <button class="btn btn-danger" on:click={handleFormat} disabled={formatting || !formatLabel}>
          {formatting ? 'Formatting...' : '🗑️ Format with LTFS'}
        </button>
      </div>
    </div>
  </div>
{/if}

<!-- Restore Modal -->
{#if showRestoreModal}
  <div class="modal-overlay" on:click={() => showRestoreModal = false}>
    <div class="modal" on:click|stopPropagation>
      <h2>📥 Restore from LTFS</h2>
      <p style="color: var(--text-muted);">
        {#if restoreFiles.length > 0}
          Restoring <strong>{restoreFiles.length}</strong> selected file(s).
        {:else}
          Restoring <strong>all files</strong> from the LTFS volume.
        {/if}
      </p>
      <div class="form-group">
        <label for="restore-dest">Destination Path</label>
        <input id="restore-dest" type="text" bind:value={restoreDestPath} placeholder="/path/to/restore/destination" />
      </div>
      {#if restoreFiles.length > 0 && restoreFiles.length <= 10}
        <div style="margin-bottom: 1rem;">
          <strong>Files:</strong>
          <ul style="max-height: 150px; overflow-y: auto; font-size: 0.85rem;">
            {#each restoreFiles as f}
              <li><code>{f}</code></li>
            {/each}
          </ul>
        </div>
      {/if}
      <div class="modal-actions">
        <button class="btn btn-secondary" on:click={() => showRestoreModal = false} disabled={restoring}>Cancel</button>
        <button class="btn btn-primary" on:click={handleRestore} disabled={restoring || !restoreDestPath}>
          {restoring ? 'Restoring...' : '📥 Start Restore'}
        </button>
      </div>
    </div>
  </div>
{/if}

<style>
  .tabs {
    display: flex;
    gap: 0;
    margin-bottom: 0;
    border-bottom: 2px solid var(--border-color);
  }

  .tab {
    padding: 0.75rem 1.5rem;
    border: none;
    background: none;
    cursor: pointer;
    color: var(--text-muted);
    font-size: 0.9rem;
    border-bottom: 2px solid transparent;
    margin-bottom: -2px;
    transition: all 0.2s;
  }

  .tab:hover {
    color: var(--text-primary);
  }

  .tab.active {
    color: var(--accent-primary);
    border-bottom-color: var(--accent-primary);
    font-weight: 600;
  }

  .breadcrumb {
    display: flex;
    align-items: center;
    gap: 0.25rem;
    padding: 0.5rem 0;
    flex-wrap: wrap;
  }

  .breadcrumb-item {
    background: none;
    border: none;
    padding: 0.25rem 0.5rem;
    cursor: pointer;
    color: var(--accent-primary);
    font-size: 0.85rem;
    border-radius: 4px;
  }

  .breadcrumb-item:hover {
    background: var(--bg-input);
  }

  .breadcrumb-item.active {
    color: var(--text-primary);
    font-weight: 600;
  }

  .breadcrumb-sep {
    color: var(--text-muted);
    font-size: 0.8rem;
  }

  tr.clickable {
    cursor: pointer;
  }

  tr.clickable:hover td {
    background: var(--bg-input);
  }

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
    grid-template-columns: repeat(auto-fill, minmax(220px, 1fr));
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

  .modal-overlay {
    position: fixed;
    top: 0;
    left: 0;
    right: 0;
    bottom: 0;
    background: rgba(0, 0, 0, 0.5);
    display: flex;
    align-items: center;
    justify-content: center;
    z-index: 1000;
  }

  .modal {
    background: var(--bg-card);
    border-radius: 12px;
    padding: 1.5rem;
    width: 90%;
    max-width: 500px;
    max-height: 90vh;
    overflow-y: auto;
    box-shadow: 0 20px 60px rgba(0, 0, 0, 0.3);
  }

  .modal-actions {
    display: flex;
    gap: 0.5rem;
    justify-content: flex-end;
    margin-top: 1rem;
  }

  .btn-sm {
    padding: 0.35rem 0.75rem;
    font-size: 0.85rem;
  }

  code {
    background: var(--code-bg);
    padding: 0.1rem 0.3rem;
    border-radius: 3px;
    font-size: 0.8rem;
  }
</style>
