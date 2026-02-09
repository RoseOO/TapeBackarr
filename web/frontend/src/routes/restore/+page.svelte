<script lang="ts">
  import { onMount } from 'svelte';
  import * as api from '$lib/api/client';

  interface BackupSet {
    id: number;
    job_id: number;
    job_name: string;
    tape_id: number;
    tape_label: string;
    backup_type: string;
    start_time: string;
    end_time: string | null;
    status: string;
    file_count: number;
    total_bytes: number;
    encrypted: boolean;
    encryption_key_id: number | null;
    compressed: boolean;
    compression_type: string;
    pool_name: string | null;
  }

  interface CatalogEntry {
    id: number;
    backup_set_id: number;
    file_path: string;
    file_size: number;
    mod_time: string;
    checksum: string;
    block_offset: number;
    tape_id: number;
    tape_label: string;
  }

  interface TapeRequirement {
    tape: {
      id: number;
      label: string;
      status: string;
    };
    file_count: number;
    total_bytes: number;
    order: number;
  }

  interface TapeDrive {
    id: number;
    device_path: string;
    display_name: string;
    vendor: string;
    model: string;
    status: string;
    enabled: boolean;
    current_tape: string;
  }

  let backupSets: BackupSet[] = [];
  let selectedSet: BackupSet | null = null;
  let catalogEntries: CatalogEntry[] = [];
  let selectedFiles: string[] = [];
  let searchQuery = '';
  let searchResults: CatalogEntry[] = [];
  let loading = true;
  let searching = false;
  let catalogLoading = false;
  let error = '';
  let showRestoreModal = false;
  let restoreStep: 'config' | 'confirm' | 'running' | 'done' = 'config';
  let restoreResult: { files_restored: number } | null = null;
  let restoreError = '';
  let requiredTapes: TapeRequirement[] = [];
  let filterStatus = 'all';
  let filterType = 'all';
  let sortBy = 'date';
  let drives: TapeDrive[] = [];
  let selectedDriveId: number | null = null;

  let restoreFormData = {
    dest_path: '/restore',
    verify: true,
    overwrite: false,
  };

  // Raw Read Tape state
  interface RawReadFile {
    path: string;
    size: number;
  }
  interface RawReadResult {
    has_header: boolean;
    tape_label?: string;
    tape_uuid?: string;
    tape_pool?: string;
    label_time?: number;
    encryption?: string;
    compression?: string;
    files_found: number;
    bytes_restored: number;
    start_time: string;
    end_time: string;
    errors?: string[];
    log_messages: string[];
    file_list?: RawReadFile[];
  }
  let showRawRead = false;
  let rawReadRunning = false;
  let rawReadResult: RawReadResult | null = null;
  let rawReadError = '';
  let rawReadDriveId: number | null = null;
  let rawReadDestPath = '/restore/raw-read';
  let rawReadOverwrite = false;

  async function handleRawReadTape() {
    if (rawReadDriveId === null) {
      rawReadError = 'Please select a tape drive';
      return;
    }
    rawReadRunning = true;
    rawReadError = '';
    rawReadResult = null;
    try {
      const result = await api.rawReadTape({
        drive_id: rawReadDriveId,
        dest_path: rawReadDestPath,
        overwrite: rawReadOverwrite,
      });
      rawReadResult = result as RawReadResult;
    } catch (e) {
      rawReadError = e instanceof Error ? e.message : 'Raw tape read failed';
    } finally {
      rawReadRunning = false;
    }
  }

  onMount(async () => {
    await Promise.all([loadBackupSets(), loadDrives()]);
  });

  async function loadDrives() {
    try {
      const result = await api.getDrives();
      drives = (Array.isArray(result) ? result : []).filter((d: TapeDrive) => d.enabled);
      if (drives.length > 0 && selectedDriveId === null) {
        selectedDriveId = drives[0].id;
      }
      if (drives.length > 0 && rawReadDriveId === null) {
        rawReadDriveId = drives[0].id;
      }
    } catch (e) {
      // Drives are optional; don't block restore if they fail to load
    }
  }

  async function loadBackupSets() {
    loading = true;
    error = '';
    try {
      const result = await api.getBackupSets();
      backupSets = Array.isArray(result) ? result : [];
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to load backup sets';
    } finally {
      loading = false;
    }
  }

  async function loadCatalog(set: BackupSet) {
    selectedSet = set;
    selectedFiles = [];
    catalogLoading = true;
    try {
      const result = await api.getBackupFiles(set.id);
      catalogEntries = Array.isArray(result) ? result : [];
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to load catalog';
      catalogEntries = [];
    } finally {
      catalogLoading = false;
    }
  }

  async function handleSearch() {
    if (!searchQuery.trim()) {
      searchResults = [];
      return;
    }
    searching = true;
    try {
      searchResults = await api.searchCatalog(searchQuery);
    } catch (e) {
      error = e instanceof Error ? e.message : 'Search failed';
    } finally {
      searching = false;
    }
  }

  async function planRestore() {
    if (!selectedSet) return;
    restoreStep = 'config';
    restoreResult = null;
    restoreError = '';
    try {
      const result = await api.getRestorePlan(
        selectedSet.id,
        selectedFiles.length > 0 ? selectedFiles : undefined,
        restoreFormData.dest_path
      );
      requiredTapes = result.required_tapes;
      showRestoreModal = true;
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to plan restore';
    }
  }

  async function executeRestore() {
    if (!selectedSet) return;
    restoreStep = 'running';
    restoreError = '';
    try {
      const result = await api.runRestore({
        backup_set_id: selectedSet.id,
        file_paths: selectedFiles.length > 0 ? selectedFiles : undefined,
        dest_path: restoreFormData.dest_path,
        verify: restoreFormData.verify,
        overwrite: restoreFormData.overwrite,
        drive_id: selectedDriveId ?? undefined,
      });
      restoreResult = result;
      restoreStep = 'done';
    } catch (e) {
      restoreError = e instanceof Error ? e.message : 'Restore failed';
      restoreStep = 'confirm';
    }
  }

  function toggleFileSelection(path: string) {
    if (selectedFiles.includes(path)) {
      selectedFiles = selectedFiles.filter(f => f !== path);
    } else {
      selectedFiles = [...selectedFiles, path];
    }
  }

  function selectAllFiles() {
    selectedFiles = catalogEntries.map(e => e.file_path);
  }

  function deselectAllFiles() {
    selectedFiles = [];
  }

  function handleOverlayClick() {
    if (restoreStep !== 'running') showRestoreModal = false;
  }

  function formatBytes(bytes: number): string {
    if (bytes === 0) return '0 B';
    const k = 1024;
    const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
  }

  function formatDate(dateStr: string | null): string {
    if (!dateStr) return '-';
    return new Date(dateStr).toLocaleString();
  }

  function formatRelativeTime(dateStr: string | null): string {
    if (!dateStr) return '-';
    const date = new Date(dateStr);
    const now = new Date();
    const diffMs = now.getTime() - date.getTime();
    const diffSec = Math.floor(diffMs / 1000);
    if (diffSec < 60) return 'Just now';
    const diffMin = Math.floor(diffSec / 60);
    if (diffMin < 60) return `${diffMin}m ago`;
    const diffHr = Math.floor(diffMin / 60);
    if (diffHr < 24) return `${diffHr}h ago`;
    const diffDay = Math.floor(diffHr / 24);
    if (diffDay < 30) return `${diffDay}d ago`;
    return date.toLocaleDateString();
  }

  function getStatusBadgeClass(status: string): string {
    switch (status) {
      case 'completed': return 'badge-success';
      case 'running': return 'badge-warning';
      case 'failed': return 'badge-danger';
      default: return 'badge-info';
    }
  }

  function getStatusIcon(status: string): string {
    switch (status) {
      case 'completed': return '✅';
      case 'running': return '⏳';
      case 'failed': return '❌';
      case 'cancelled': return '🚫';
      default: return '📋';
    }
  }

  async function handleDeleteBackupSet(set: BackupSet) {
    if (!confirm(`Delete backup set "${set.job_name}" (${set.status})? This removes the record from the database but does not erase data from tape.`)) return;
    try {
      await api.deleteBackupSet(set.id);
      if (selectedSet?.id === set.id) {
        selectedSet = null;
        catalogEntries = [];
        selectedFiles = [];
      }
      await loadBackupSets();
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to delete backup set';
    }
  }

  async function handleCancelBackupSet(set: BackupSet) {
    if (!confirm(`Cancel stuck backup set "${set.job_name}"? This will mark it as cancelled so it can be deleted.`)) return;
    try {
      await api.cancelBackupSet(set.id);
      await loadBackupSets();
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to cancel backup set';
    }
  }

  $: filteredSets = backupSets
    .filter(s => filterStatus === 'all' || s.status === filterStatus)
    .filter(s => filterType === 'all' || s.backup_type === filterType)
    .sort((a, b) => {
      if (sortBy === 'date') return new Date(b.start_time).getTime() - new Date(a.start_time).getTime();
      if (sortBy === 'size') return b.total_bytes - a.total_bytes;
      if (sortBy === 'name') return a.job_name.localeCompare(b.job_name);
      return 0;
    });

  $: selectedFilesSize = catalogEntries
    .filter(e => selectedFiles.includes(e.file_path))
    .reduce((sum, e) => sum + e.file_size, 0);

  $: totalCatalogSize = catalogEntries.reduce((sum, e) => sum + e.file_size, 0);

  $: restoreTotalFiles = requiredTapes.reduce((sum, t) => sum + t.file_count, 0);
  $: restoreTotalBytes = requiredTapes.reduce((sum, t) => sum + t.total_bytes, 0);
</script>

<div class="page-header">
  <h1>🔄 Restore</h1>
</div>

{#if error}
  <div class="card error-card">
    <div class="error-content">
      <span class="error-icon">⚠️</span>
      <p>{error}</p>
    </div>
    <button class="btn btn-secondary" on:click={() => error = ''}>Dismiss</button>
  </div>
{/if}

<!-- Search Section -->
<div class="card search-section">
  <div class="search-section-header">
    <h2>🔍 Search Catalog</h2>
    <span class="search-hint">Search across all backup sets by file name or pattern</span>
  </div>
  <div class="search-bar">
    <input
      type="text"
      bind:value={searchQuery}
      placeholder="Search for files (e.g., *.doc, report*, /home/user/...)"
      on:keyup={(e) => e.key === 'Enter' && handleSearch()}
    />
    <button class="btn btn-primary" on:click={handleSearch} disabled={searching}>
      {searching ? '🔄 Searching...' : '🔍 Search'}
    </button>
  </div>
  {#if searchResults.length > 0}
    <div class="search-results-header">
      <span class="search-results-count">Found {searchResults.length} result{searchResults.length !== 1 ? 's' : ''}</span>
    </div>
    <div class="search-results">
      <table>
        <thead>
          <tr>
            <th>File Path</th>
            <th>Size</th>
            <th>Tape</th>
            <th>Position</th>
            <th>Modified</th>
          </tr>
        </thead>
        <tbody>
          {#each searchResults as result}
            <tr>
              <td class="file-path-cell">{result.file_path}</td>
              <td>{formatBytes(result.file_size)}</td>
              <td class="tape-label-cell">{result.tape_label || '—'}</td>
              <td class="tape-position-cell">{result.block_offset != null ? formatBytes(result.block_offset) : '—'}</td>
              <td>{formatDate(result.mod_time)}</td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>
  {/if}
</div>

<!-- Raw Read Tape Section -->
<div class="card raw-read-section">
  <div class="raw-read-header" on:click={() => showRawRead = !showRawRead} role="button" tabindex="0" on:keydown={(e) => e.key === 'Enter' && (showRawRead = !showRawRead)}>
    <h2>📼 Raw Read Tape</h2>
    <span class="raw-read-toggle">{showRawRead ? '▲' : '▼'}</span>
  </div>
  <p class="raw-read-hint">Read a tape directly without requiring it to be in the database. Supports unknown tapes and tapes from other systems.</p>

  {#if showRawRead}
    <div class="raw-read-form">
      <div class="form-row">
        <label for="raw-drive">Tape Drive</label>
        <select id="raw-drive" bind:value={rawReadDriveId}>
          {#each drives as drive}
            <option value={drive.id}>{drive.display_name || drive.device_path} ({drive.status})</option>
          {/each}
        </select>
      </div>
      <div class="form-row">
        <label for="raw-dest">Destination Path</label>
        <input id="raw-dest" type="text" bind:value={rawReadDestPath} placeholder="/restore/raw-read" />
      </div>
      <div class="form-row form-checkbox">
        <label>
          <input type="checkbox" bind:checked={rawReadOverwrite} />
          Overwrite existing files
        </label>
      </div>
      <button class="btn btn-primary" on:click={handleRawReadTape} disabled={rawReadRunning || drives.length === 0}>
        {#if rawReadRunning}
          🔄 Reading tape...
        {:else}
          📼 Read Tape
        {/if}
      </button>
      {#if drives.length === 0}
        <p class="raw-read-warning">No tape drives available. Please configure a tape drive first.</p>
      {/if}
    </div>

    {#if rawReadError}
      <div class="raw-read-error">
        <span class="error-icon">⚠️</span>
        <p>{rawReadError}</p>
      </div>
    {/if}

    {#if rawReadRunning}
      <div class="raw-read-progress">
        <div class="running-bar"><div class="running-bar-fill"></div></div>
        <p>Reading tape — this may take a while depending on tape size...</p>
      </div>
    {/if}

    {#if rawReadResult}
      <div class="raw-read-results">
        <h3>📊 Read Results</h3>

        <!-- Tape Header Info -->
        <div class="raw-read-info">
          {#if rawReadResult.has_header}
            <div class="info-badge info-badge-success">✅ TapeBackarr Header Detected</div>
            <div class="info-grid">
              <div class="info-item"><strong>Label:</strong> {rawReadResult.tape_label}</div>
              <div class="info-item"><strong>UUID:</strong> {rawReadResult.tape_uuid}</div>
              {#if rawReadResult.tape_pool}<div class="info-item"><strong>Pool:</strong> {rawReadResult.tape_pool}</div>{/if}
              {#if rawReadResult.label_time}<div class="info-item"><strong>Written:</strong> {new Date(rawReadResult.label_time * 1000).toLocaleString()}</div>{/if}
              {#if rawReadResult.encryption}<div class="info-item"><strong>Encryption:</strong> {rawReadResult.encryption}</div>{/if}
              {#if rawReadResult.compression}<div class="info-item"><strong>Compression:</strong> {rawReadResult.compression}</div>{/if}
            </div>
          {:else}
            <div class="info-badge info-badge-warning">ℹ️ No TapeBackarr Header — Unknown or Foreign Tape</div>
          {/if}
        </div>

        <!-- Stats -->
        <div class="raw-read-stats">
          <div class="stat-item"><strong>{rawReadResult.files_found}</strong> <span>files restored</span></div>
          <div class="stat-item"><strong>{formatBytes(rawReadResult.bytes_restored)}</strong> <span>total size</span></div>
          <div class="stat-item"><strong>{((new Date(rawReadResult.end_time).getTime() - new Date(rawReadResult.start_time).getTime()) / 1000).toFixed(1)}s</strong> <span>duration</span></div>
        </div>

        <!-- Errors if any -->
        {#if rawReadResult.errors && rawReadResult.errors.length > 0}
          <div class="raw-read-errors-list">
            <h4>⚠️ Warnings</h4>
            {#each rawReadResult.errors as err}
              <p class="raw-read-error-item">{err}</p>
            {/each}
          </div>
        {/if}

        <!-- File List -->
        {#if rawReadResult.file_list && rawReadResult.file_list.length > 0}
          <div class="raw-read-files">
            <h4>📁 Extracted Files ({rawReadResult.file_list.length})</h4>
            <div class="raw-read-file-list">
              <table>
                <thead>
                  <tr>
                    <th>File Path</th>
                    <th>Size</th>
                  </tr>
                </thead>
                <tbody>
                  {#each rawReadResult.file_list as file}
                    <tr>
                      <td class="file-path-cell">{file.path}</td>
                      <td>{formatBytes(file.size)}</td>
                    </tr>
                  {/each}
                </tbody>
              </table>
            </div>
          </div>
        {/if}

        <!-- Log Messages -->
        {#if rawReadResult.log_messages && rawReadResult.log_messages.length > 0}
          <div class="raw-read-log">
            <h4>📋 Activity Log</h4>
            <div class="log-output">
              {#each rawReadResult.log_messages as msg}
                <div class="log-line">{msg}</div>
              {/each}
            </div>
          </div>
        {/if}
      </div>
    {/if}
  {/if}
</div>

<!-- Main Restore Layout -->
<div class="restore-layout">
  <!-- Backup Sets Panel -->
  <div class="card backup-sets-panel">
    <div class="panel-header">
      <h2>📦 Backup Sets</h2>
      <span class="panel-count">{filteredSets.length} of {backupSets.length}</span>
    </div>

    <div class="filter-bar">
      <select bind:value={filterStatus}>
        <option value="all">All Status</option>
        <option value="completed">Completed</option>
        <option value="running">Running</option>
        <option value="failed">Failed</option>
        <option value="cancelled">Cancelled</option>
      </select>
      <select bind:value={filterType}>
        <option value="all">All Types</option>
        <option value="full">Full</option>
        <option value="incremental">Incremental</option>
      </select>
      <select bind:value={sortBy}>
        <option value="date">Sort: Date</option>
        <option value="size">Sort: Size</option>
        <option value="name">Sort: Name</option>
      </select>
    </div>

    {#if loading}
      <div class="loading-state">
        <span class="loading-spinner">⏳</span>
        <p>Loading backup sets...</p>
      </div>
    {:else}
      <div class="sets-list">
        {#each filteredSets as set}
          <div
            class="set-item"
            class:selected={selectedSet?.id === set.id}
            on:click={() => loadCatalog(set)}
            role="button"
            tabindex="0"
            on:keydown={(e) => e.key === 'Enter' && loadCatalog(set)}
          >
            <div class="set-header">
              <div class="set-title">
                <span class="set-status-icon">{getStatusIcon(set.status)}</span>
                <strong>{set.job_name}</strong>
              </div>
              <span class="badge {getStatusBadgeClass(set.status)}">{set.status}</span>
            </div>
            <div class="set-meta">
              <span class="set-meta-item">📼 {set.tape_label}</span>
              {#if set.pool_name}
                <span class="set-meta-item">📀 {set.pool_name}</span>
              {/if}
              <span class="set-meta-item">🕐 {formatRelativeTime(set.start_time)}</span>
            </div>
            <div class="set-stats">
              <span class="set-stat">{set.file_count} files</span>
              <span class="set-stat">{formatBytes(set.total_bytes)}</span>
              <span class="badge {set.backup_type === 'full' ? 'badge-info' : 'badge-warning'}">
                {set.backup_type}
              </span>
              {#if set.encrypted}
                <span class="badge badge-warning">🔒</span>
              {/if}
              {#if set.compressed}
                <span class="badge badge-info">📦 {set.compression_type}</span>
              {/if}
            </div>
            <div class="set-actions">
              {#if set.status === 'running' || set.status === 'pending'}
                <button class="btn btn-warning btn-sm" on:click|stopPropagation={() => handleCancelBackupSet(set)}>
                  ⛔ Cancel
                </button>
              {/if}
              {#if set.status === 'failed' || set.status === 'completed' || set.status === 'cancelled'}
                <button class="btn btn-danger btn-sm" on:click|stopPropagation={() => handleDeleteBackupSet(set)}>
                  🗑️ Delete
                </button>
              {/if}
            </div>
          </div>
        {/each}
        {#if filteredSets.length === 0 && !loading}
          <div class="empty-state">
            <span class="empty-icon">📭</span>
            <p>{backupSets.length === 0 ? 'No backup sets found.' : 'No sets match the current filters.'}</p>
          </div>
        {/if}
      </div>
    {/if}
  </div>

  <!-- File Browser Panel -->
  <div class="card file-browser-panel">
    {#if selectedSet}
      <!-- Selected Set Info Banner -->
      <div class="selected-set-banner">
        <div class="banner-info">
          <h2>📂 {selectedSet.job_name}</h2>
          <div class="banner-meta">
            <span>📼 {selectedSet.tape_label}</span>
            <span>📅 {formatDate(selectedSet.start_time)}</span>
            <span>{selectedSet.file_count} files</span>
            <span>{formatBytes(selectedSet.total_bytes)}</span>
          </div>
        </div>
        <div class="banner-badges">
          <span class="badge {getStatusBadgeClass(selectedSet.status)}">{selectedSet.status}</span>
          <span class="badge {selectedSet.backup_type === 'full' ? 'badge-info' : 'badge-warning'}">{selectedSet.backup_type}</span>
          {#if selectedSet.encrypted}
            <span class="badge badge-warning">🔒 Encrypted</span>
          {/if}
        </div>
      </div>

      <!-- File Selection Toolbar -->
      {#if catalogEntries.length > 0}
        <div class="file-toolbar">
          <div class="toolbar-left">
            <button class="btn btn-secondary btn-sm" on:click={selectAllFiles}>Select All</button>
            <button class="btn btn-secondary btn-sm" on:click={deselectAllFiles}>Deselect All</button>
            <span class="selection-summary">
              {#if selectedFiles.length > 0}
                {selectedFiles.length} selected ({formatBytes(selectedFilesSize)})
              {:else}
                {catalogEntries.length} files ({formatBytes(totalCatalogSize)})
              {/if}
            </span>
          </div>
          <button
            class="btn btn-success"
            on:click={planRestore}
            disabled={!selectedSet}
          >
            🔄 Restore {selectedFiles.length > 0 ? `${selectedFiles.length} Files` : 'All Files'}
          </button>
        </div>
      {/if}

      <!-- File List -->
      {#if catalogLoading}
        <div class="loading-state">
          <span class="loading-spinner">⏳</span>
          <p>Loading file catalog...</p>
        </div>
      {:else}
        <div class="file-list">
          <table>
            <thead>
              <tr>
                <th style="width: 40px;"></th>
                <th>File Path</th>
                <th>Size</th>
                <th>Tape</th>
                <th>Position</th>
                <th>Modified</th>
              </tr>
            </thead>
            <tbody>
              {#each catalogEntries as entry}
                <tr class:selected={selectedFiles.includes(entry.file_path)}>
                  <td>
                    <input
                      type="checkbox"
                      checked={selectedFiles.includes(entry.file_path)}
                      on:change={() => toggleFileSelection(entry.file_path)}
                    />
                  </td>
                  <td class="file-path-cell">{entry.file_path}</td>
                  <td>{formatBytes(entry.file_size)}</td>
                  <td class="tape-label-cell">{entry.tape_label || '—'}</td>
                  <td class="tape-position-cell">{entry.block_offset != null ? formatBytes(entry.block_offset) : '—'}</td>
                  <td>{formatDate(entry.mod_time)}</td>
                </tr>
              {/each}
              {#if catalogEntries.length === 0}
                <tr>
                  <td colspan="6" class="no-data">No files in this backup set.</td>
                </tr>
              {/if}
            </tbody>
          </table>
        </div>
      {/if}
    {:else}
      <div class="empty-state large">
        <span class="empty-icon">📂</span>
        <h3>Select a Backup Set</h3>
        <p>Choose a backup set from the list to browse and restore files.</p>
      </div>
    {/if}
  </div>
</div>

<!-- Restore Modal -->
{#if showRestoreModal && selectedSet}
  <div class="modal-overlay" on:click={handleOverlayClick}>
    <div class="modal restore-modal" on:click|stopPropagation={() => {}}>
      <!-- Modal Header with Steps -->
      <div class="modal-header">
        <h2>🔄 Restore Files</h2>
        <div class="restore-steps">
          <div class="step" class:active={restoreStep === 'config'} class:completed={restoreStep === 'confirm' || restoreStep === 'running' || restoreStep === 'done'}>
            <span class="step-number">1</span>
            <span class="step-label">Configure</span>
          </div>
          <div class="step-connector"></div>
          <div class="step" class:active={restoreStep === 'confirm'} class:completed={restoreStep === 'running' || restoreStep === 'done'}>
            <span class="step-number">2</span>
            <span class="step-label">Confirm</span>
          </div>
          <div class="step-connector"></div>
          <div class="step" class:active={restoreStep === 'running'} class:completed={restoreStep === 'done'}>
            <span class="step-number">3</span>
            <span class="step-label">Restore</span>
          </div>
        </div>
      </div>

      <!-- Step 1: Configuration -->
      {#if restoreStep === 'config'}
        <div class="modal-body">
          <!-- Restore Summary -->
          <div class="restore-summary-card">
            <h3>Restore Summary</h3>
            <div class="restore-summary-grid">
              <div class="restore-summary-item">
                <span class="summary-label">Backup Set</span>
                <span class="summary-value">{selectedSet.job_name}</span>
              </div>
              <div class="restore-summary-item">
                <span class="summary-label">Files</span>
                <span class="summary-value">{selectedFiles.length > 0 ? selectedFiles.length : selectedSet.file_count}</span>
              </div>
              <div class="restore-summary-item">
                <span class="summary-label">Total Size</span>
                <span class="summary-value">{formatBytes(selectedFiles.length > 0 ? selectedFilesSize : selectedSet.total_bytes)}</span>
              </div>
              <div class="restore-summary-item">
                <span class="summary-label">Source Tape</span>
                <span class="summary-value">📼 {selectedSet.tape_label}</span>
              </div>
            </div>
          </div>

          <form on:submit|preventDefault={() => restoreStep = 'confirm'}>
            <div class="form-group">
              <label for="drive">Tape Drive</label>
              <select id="drive" bind:value={selectedDriveId}>
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
              <span class="form-hint">Select the tape drive to read from</span>
            </div>
            <div class="form-group">
              <label for="dest">Destination Path</label>
              <input type="text" id="dest" bind:value={restoreFormData.dest_path} required />
              <span class="form-hint">Directory where files will be restored</span>
            </div>
            <div class="form-group checkbox-group">
              <label>
                <input type="checkbox" bind:checked={restoreFormData.verify} />
                Verify files after restore
              </label>
              <span class="form-hint">Ensures data integrity by verifying checksums</span>
            </div>
            <div class="form-group checkbox-group">
              <label>
                <input type="checkbox" bind:checked={restoreFormData.overwrite} />
                Overwrite existing files
              </label>
              <span class="form-hint">Replace existing files at the destination path</span>
            </div>
            <div class="modal-actions">
              <button type="button" class="btn btn-secondary" on:click={() => showRestoreModal = false}>
                Cancel
              </button>
              <button type="submit" class="btn btn-primary">
                Next →
              </button>
            </div>
          </form>
        </div>
      {/if}

      <!-- Step 2: Confirm with Tape Requirements -->
      {#if restoreStep === 'confirm'}
        <div class="modal-body">
          {#if restoreError}
            <div class="restore-error">
              <span>❌</span> {restoreError}
            </div>
          {/if}

          {#if requiredTapes.length > 0}
            <div class="tape-requirements">
              <h3>📼 Required Tapes</h3>
              <p class="tape-instruction">Insert the following tape(s) in order. The system will prompt you when a tape change is needed.</p>
              <div class="tape-timeline">
                {#each requiredTapes as req, i}
                  <div class="tape-timeline-item">
                    <div class="tape-timeline-marker">
                      <span class="tape-step-number">{i + 1}</span>
                    </div>
                    <div class="tape-timeline-content">
                      <div class="tape-timeline-header">
                        <strong>📼 {req.tape.label}</strong>
                        <span class="badge {req.tape.status === 'active' ? 'badge-success' : 'badge-warning'}">
                          {req.tape.status}
                        </span>
                      </div>
                      <div class="tape-timeline-meta">
                        <span>{req.file_count} files</span>
                        <span>{formatBytes(req.total_bytes)}</span>
                      </div>
                    </div>
                  </div>
                {/each}
              </div>
              <div class="tape-total">
                <span>Total: {restoreTotalFiles} files, {formatBytes(restoreTotalBytes)}</span>
              </div>
            </div>
          {/if}

          <div class="confirm-details">
            <h3>Restore Configuration</h3>
            <div class="confirm-grid">
              <div class="confirm-item">
                <span class="confirm-label">Destination</span>
                <span class="confirm-value"><code>{restoreFormData.dest_path}</code></span>
              </div>
              <div class="confirm-item">
                <span class="confirm-label">Verify</span>
                <span class="confirm-value">{restoreFormData.verify ? '✅ Yes' : '❌ No'}</span>
              </div>
              <div class="confirm-item">
                <span class="confirm-label">Overwrite</span>
                <span class="confirm-value">{restoreFormData.overwrite ? '⚠️ Yes' : '❌ No'}</span>
              </div>
            </div>
          </div>

          <div class="modal-actions">
            <button type="button" class="btn btn-secondary" on:click={() => restoreStep = 'config'}>
              ← Back
            </button>
            <button type="button" class="btn btn-success" on:click={executeRestore}>
              🚀 Start Restore
            </button>
          </div>
        </div>
      {/if}

      <!-- Step 3: Running -->
      {#if restoreStep === 'running'}
        <div class="modal-body restore-running">
          <div class="running-animation">
            <span class="running-icon">📼</span>
          </div>
          <h3>Restoring Files...</h3>
          <p>Please wait while files are being restored from tape. Do not eject the tape.</p>
          <div class="running-progress">
            <div class="running-bar">
              <div class="running-bar-fill"></div>
            </div>
          </div>
        </div>
      {/if}

      <!-- Step 4: Done -->
      {#if restoreStep === 'done' && restoreResult}
        <div class="modal-body restore-done">
          <div class="done-icon">✅</div>
          <h3>Restore Complete!</h3>
          <p>{restoreResult.files_restored} file{restoreResult.files_restored !== 1 ? 's' : ''} restored successfully to <code>{restoreFormData.dest_path}</code></p>
          <div class="modal-actions centered">
            <button type="button" class="btn btn-primary" on:click={() => showRestoreModal = false}>
              Done
            </button>
          </div>
        </div>
      {/if}
    </div>
  </div>
{/if}

<style>
  .error-card {
    background: var(--badge-danger-bg);
    color: var(--badge-danger-text);
    display: flex;
    justify-content: space-between;
    align-items: center;
  }

  .error-content {
    display: flex;
    align-items: center;
    gap: 0.5rem;
  }

  .error-icon {
    font-size: 1.25rem;
  }

  .error-card p {
    margin: 0;
  }

  /* Search Section */
  .search-section {
    margin-bottom: 1.5rem;
  }

  .search-section-header {
    display: flex;
    justify-content: space-between;
    align-items: baseline;
    margin-bottom: 0.75rem;
  }

  .search-section-header h2 {
    margin: 0;
    font-size: 1rem;
  }

  .search-hint {
    font-size: 0.8rem;
    color: var(--text-muted);
  }

  .search-bar {
    display: flex;
    gap: 0.5rem;
  }

  .search-bar input {
    flex: 1;
  }

  .search-results-header {
    margin-top: 0.75rem;
    padding-bottom: 0.5rem;
    border-bottom: 1px solid var(--border-color);
  }

  .search-results-count {
    font-size: 0.8rem;
    font-weight: 600;
    color: var(--text-secondary);
  }

  .search-results {
    margin-top: 0.5rem;
    max-height: 250px;
    overflow-y: auto;
  }

  .file-path-cell {
    font-family: monospace;
    font-size: 0.8rem;
    word-break: break-all;
  }

  .tape-label-cell {
    font-size: 0.8rem;
    white-space: nowrap;
  }

  .tape-position-cell {
    font-family: monospace;
    font-size: 0.8rem;
    white-space: nowrap;
  }

  /* Restore Layout */
  .restore-layout {
    display: grid;
    grid-template-columns: 1fr 2fr;
    gap: 1.5rem;
  }

  /* Backup Sets Panel */
  .panel-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    margin-bottom: 0.75rem;
  }

  .panel-header h2 {
    margin: 0;
    font-size: 1rem;
  }

  .panel-count {
    font-size: 0.8rem;
    color: var(--text-muted);
  }

  .filter-bar {
    display: flex;
    gap: 0.5rem;
    margin-bottom: 0.75rem;
  }

  .filter-bar select {
    flex: 1;
    font-size: 0.75rem;
    padding: 0.35rem 0.5rem;
  }

  .sets-list {
    max-height: 600px;
    overflow-y: auto;
  }

  .set-item {
    padding: 0.75rem;
    border: 1px solid var(--border-color);
    border-radius: 8px;
    margin-bottom: 0.5rem;
    cursor: pointer;
    transition: all 0.2s ease;
  }

  .set-item:hover {
    border-color: var(--accent-primary);
    background: var(--bg-card-hover);
  }

  .set-item.selected {
    border-color: var(--accent-primary);
    background: var(--bg-card-hover);
    box-shadow: inset 3px 0 0 var(--accent-primary);
  }

  .set-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    margin-bottom: 0.4rem;
  }

  .set-title {
    display: flex;
    align-items: center;
    gap: 0.35rem;
  }

  .set-status-icon {
    font-size: 0.85rem;
  }

  .set-meta {
    font-size: 0.75rem;
    color: var(--text-muted);
    display: flex;
    gap: 0.75rem;
    margin-bottom: 0.35rem;
    flex-wrap: wrap;
  }

  .set-meta-item {
    white-space: nowrap;
  }

  .set-stats {
    font-size: 0.75rem;
    display: flex;
    gap: 0.5rem;
    align-items: center;
    flex-wrap: wrap;
  }

  .set-stat {
    color: var(--text-secondary);
  }

  .set-actions {
    display: flex;
    gap: 0.35rem;
    margin-top: 0.35rem;
  }

  :global(.btn-sm) {
    padding: 0.2rem 0.5rem;
    font-size: 0.7rem;
  }

  /* File Browser Panel */
  .selected-set-banner {
    display: flex;
    justify-content: space-between;
    align-items: flex-start;
    padding: 1rem;
    background: var(--bg-input);
    border-radius: 8px;
    margin-bottom: 1rem;
  }

  .banner-info h2 {
    margin: 0 0 0.35rem;
    font-size: 1rem;
  }

  .banner-meta {
    display: flex;
    gap: 1rem;
    font-size: 0.8rem;
    color: var(--text-secondary);
    flex-wrap: wrap;
  }

  .banner-badges {
    display: flex;
    gap: 0.35rem;
    flex-shrink: 0;
  }

  .file-toolbar {
    display: flex;
    justify-content: space-between;
    align-items: center;
    padding: 0.75rem;
    background: var(--bg-input);
    border-radius: 8px;
    margin-bottom: 0.75rem;
  }

  .toolbar-left {
    display: flex;
    align-items: center;
    gap: 0.5rem;
  }

  .selection-summary {
    font-size: 0.8rem;
    color: var(--text-secondary);
    margin-left: 0.25rem;
  }

  .file-list {
    max-height: 500px;
    overflow-y: auto;
  }

  .file-list tr.selected {
    background: var(--bg-card-hover);
  }

  .no-data {
    text-align: center;
    color: var(--text-muted);
    padding: 2rem;
  }

  /* Empty & Loading States */
  .empty-state {
    text-align: center;
    padding: 2rem;
    color: var(--text-muted);
  }

  .empty-state.large {
    padding: 4rem 2rem;
  }

  .empty-state h3 {
    margin: 0.75rem 0 0.5rem;
    color: var(--text-secondary);
  }

  .empty-state p {
    margin: 0;
    font-size: 0.875rem;
  }

  .empty-icon {
    font-size: 2.5rem;
  }

  .loading-state {
    text-align: center;
    padding: 2rem;
    color: var(--text-muted);
  }

  .loading-spinner {
    font-size: 2rem;
    display: inline-block;
    animation: spin 1.5s linear infinite;
  }

  @keyframes spin {
    from { transform: rotate(0deg); }
    to { transform: rotate(360deg); }
  }

  /* Modal */
  .modal-overlay {
    position: fixed;
    top: 0;
    left: 0;
    right: 0;
    bottom: 0;
    background: rgba(0, 0, 0, 0.6);
    display: flex;
    justify-content: center;
    align-items: center;
    z-index: 1000;
  }

  .restore-modal {
    background: var(--bg-card);
    padding: 0;
    border-radius: 12px;
    width: 100%;
    max-width: 600px;
    max-height: 90vh;
    overflow-y: auto;
  }

  .modal-header {
    padding: 1.5rem 1.5rem 1rem;
    border-bottom: 1px solid var(--border-color);
  }

  .modal-header h2 {
    margin: 0 0 1rem;
    font-size: 1.1rem;
  }

  .modal-body {
    padding: 1.5rem;
  }

  /* Steps Indicator */
  .restore-steps {
    display: flex;
    align-items: center;
    gap: 0;
  }

  .step {
    display: flex;
    align-items: center;
    gap: 0.35rem;
    color: var(--text-muted);
  }

  .step.active {
    color: var(--accent-primary);
  }

  .step.completed {
    color: var(--accent-success);
  }

  .step-number {
    width: 24px;
    height: 24px;
    border-radius: 50%;
    display: flex;
    align-items: center;
    justify-content: center;
    font-size: 0.75rem;
    font-weight: 700;
    background: var(--bg-input);
    border: 2px solid var(--border-color);
  }

  .step.active .step-number {
    background: var(--accent-primary);
    border-color: var(--accent-primary);
    color: white;
  }

  .step.completed .step-number {
    background: var(--accent-success);
    border-color: var(--accent-success);
    color: white;
  }

  .step-label {
    font-size: 0.75rem;
    font-weight: 600;
  }

  .step-connector {
    flex: 1;
    height: 2px;
    background: var(--border-color);
    margin: 0 0.5rem;
    min-width: 20px;
  }

  /* Restore Summary Card */
  .restore-summary-card {
    background: var(--bg-input);
    padding: 1rem;
    border-radius: 8px;
    margin-bottom: 1.25rem;
  }

  .restore-summary-card h3 {
    margin: 0 0 0.75rem;
    font-size: 0.875rem;
    color: var(--text-secondary);
  }

  .restore-summary-grid {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 0.5rem;
  }

  .restore-summary-item {
    display: flex;
    flex-direction: column;
  }

  .summary-label {
    font-size: 0.7rem;
    color: var(--text-muted);
    text-transform: uppercase;
    letter-spacing: 0.05em;
  }

  .summary-value {
    font-size: 0.875rem;
    font-weight: 600;
    color: var(--text-primary);
  }

  .form-hint {
    display: block;
    font-size: 0.7rem;
    color: var(--text-muted);
    margin-top: 0.2rem;
  }

  .checkbox-group {
    display: flex;
    flex-direction: column;
    gap: 0;
  }

  .checkbox-group label {
    display: flex;
    align-items: center;
    gap: 0.5rem;
  }

  .checkbox-group input {
    width: auto;
  }

  .modal-actions {
    display: flex;
    gap: 0.75rem;
    justify-content: flex-end;
    margin-top: 1.5rem;
  }

  .modal-actions.centered {
    justify-content: center;
  }

  /* Tape Requirements */
  .tape-requirements {
    background: var(--bg-input);
    padding: 1rem;
    border-radius: 8px;
    margin-bottom: 1.25rem;
  }

  .tape-requirements h3 {
    margin: 0 0 0.35rem;
    font-size: 0.875rem;
  }

  .tape-instruction {
    font-size: 0.8rem;
    color: var(--text-secondary);
    margin: 0 0 0.75rem;
  }

  .tape-timeline {
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
  }

  .tape-timeline-item {
    display: flex;
    gap: 0.75rem;
    align-items: flex-start;
  }

  .tape-timeline-marker {
    flex-shrink: 0;
  }

  .tape-step-number {
    width: 28px;
    height: 28px;
    border-radius: 50%;
    display: flex;
    align-items: center;
    justify-content: center;
    font-size: 0.75rem;
    font-weight: 700;
    background: var(--accent-primary);
    color: white;
  }

  .tape-timeline-content {
    flex: 1;
    padding: 0.5rem 0.75rem;
    background: var(--bg-card);
    border-radius: 6px;
  }

  .tape-timeline-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    margin-bottom: 0.25rem;
  }

  .tape-timeline-meta {
    display: flex;
    gap: 0.75rem;
    font-size: 0.75rem;
    color: var(--text-secondary);
  }

  .tape-total {
    margin-top: 0.75rem;
    padding-top: 0.5rem;
    border-top: 1px solid var(--border-color);
    font-size: 0.8rem;
    font-weight: 600;
    color: var(--text-primary);
  }

  /* Confirm Details */
  .confirm-details {
    margin-bottom: 0.5rem;
  }

  .confirm-details h3 {
    margin: 0 0 0.5rem;
    font-size: 0.875rem;
  }

  .confirm-grid {
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
  }

  .confirm-item {
    display: flex;
    justify-content: space-between;
    align-items: center;
    padding: 0.5rem 0.75rem;
    background: var(--bg-input);
    border-radius: 6px;
    font-size: 0.85rem;
  }

  .confirm-label {
    color: var(--text-secondary);
  }

  .confirm-value {
    font-weight: 600;
    color: var(--text-primary);
  }

  .confirm-value code {
    background: var(--code-bg);
    padding: 0.1rem 0.35rem;
    border-radius: 3px;
    font-size: 0.8rem;
  }

  .restore-error {
    background: var(--badge-danger-bg);
    color: var(--badge-danger-text);
    padding: 0.75rem 1rem;
    border-radius: 8px;
    margin-bottom: 1rem;
    font-size: 0.85rem;
  }

  /* Running State */
  .restore-running {
    text-align: center;
    padding: 2rem 1.5rem;
  }

  .running-animation {
    font-size: 3rem;
    margin-bottom: 1rem;
  }

  .running-icon {
    display: inline-block;
    animation: pulse 1.5s ease-in-out infinite;
  }

  @keyframes pulse {
    0%, 100% { transform: scale(1); opacity: 1; }
    50% { transform: scale(1.15); opacity: 0.8; }
  }

  .restore-running h3 {
    margin: 0 0 0.5rem;
    color: var(--text-primary);
  }

  .restore-running p {
    font-size: 0.85rem;
    color: var(--text-secondary);
    margin: 0 0 1.5rem;
  }

  .running-progress {
    max-width: 300px;
    margin: 0 auto;
  }

  .running-bar {
    height: 6px;
    background: var(--bg-input);
    border-radius: 3px;
    overflow: hidden;
  }

  .running-bar-fill {
    height: 100%;
    width: 30%;
    border-radius: 3px;
    background: linear-gradient(90deg, var(--accent-primary), var(--accent-success));
    animation: progress-indeterminate 1.5s ease-in-out infinite;
  }

  @keyframes progress-indeterminate {
    0% { transform: translateX(-100%); }
    100% { transform: translateX(400%); }
  }

  /* Done State */
  .restore-done {
    text-align: center;
    padding: 2rem 1.5rem;
  }

  .done-icon {
    font-size: 3rem;
    margin-bottom: 1rem;
  }

  .restore-done h3 {
    margin: 0 0 0.5rem;
    color: var(--accent-success);
  }

  .restore-done p {
    font-size: 0.85rem;
    color: var(--text-secondary);
    margin: 0 0 1rem;
  }

  .restore-done code {
    background: var(--code-bg);
    padding: 0.1rem 0.35rem;
    border-radius: 3px;
    font-size: 0.8rem;
  }

  @media (max-width: 768px) {
    .restore-layout {
      grid-template-columns: 1fr;
    }

    .selected-set-banner {
      flex-direction: column;
      gap: 0.5rem;
    }

    .file-toolbar {
      flex-direction: column;
      gap: 0.5rem;
      align-items: stretch;
    }

    .toolbar-left {
      flex-wrap: wrap;
    }

    .restore-steps {
      flex-wrap: wrap;
    }

    .filter-bar {
      flex-direction: column;
    }
  }

  /* Raw Read Tape Section */
  .raw-read-section {
    margin-bottom: 1.5rem;
  }

  .raw-read-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    cursor: pointer;
    user-select: none;
  }

  .raw-read-header h2 {
    margin: 0;
    font-size: 1rem;
  }

  .raw-read-toggle {
    font-size: 0.8rem;
    color: var(--text-secondary);
  }

  .raw-read-hint {
    font-size: 0.8rem;
    color: var(--text-secondary);
    margin: 0.25rem 0 0.75rem;
  }

  .raw-read-form {
    display: flex;
    flex-direction: column;
    gap: 0.75rem;
    margin-bottom: 1rem;
  }

  .raw-read-form .form-row {
    display: flex;
    flex-direction: column;
    gap: 0.25rem;
  }

  .raw-read-form .form-row label {
    font-size: 0.8rem;
    font-weight: 600;
    color: var(--text-secondary);
  }

  .raw-read-form .form-checkbox label {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    font-weight: normal;
    color: var(--text-primary);
  }

  .raw-read-warning {
    font-size: 0.8rem;
    color: var(--badge-danger-text);
    margin: 0.25rem 0 0;
  }

  .raw-read-error {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    background: var(--badge-danger-bg);
    color: var(--badge-danger-text);
    padding: 0.75rem;
    border-radius: 6px;
    margin: 0.75rem 0;
  }

  .raw-read-error p {
    margin: 0;
    font-size: 0.85rem;
  }

  .raw-read-progress {
    text-align: center;
    padding: 1rem 0;
  }

  .raw-read-progress p {
    font-size: 0.85rem;
    color: var(--text-secondary);
    margin: 0.5rem 0 0;
  }

  .raw-read-results h3 {
    margin: 1rem 0 0.75rem;
    font-size: 1rem;
  }

  .raw-read-info {
    margin-bottom: 1rem;
  }

  .info-badge {
    display: inline-block;
    padding: 0.35rem 0.75rem;
    border-radius: 6px;
    font-size: 0.85rem;
    font-weight: 600;
    margin-bottom: 0.5rem;
  }

  .info-badge-success {
    background: var(--badge-success-bg);
    color: var(--badge-success-text);
  }

  .info-badge-warning {
    background: var(--badge-warning-bg);
    color: var(--badge-warning-text);
  }

  .info-grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(220px, 1fr));
    gap: 0.5rem;
    margin-top: 0.5rem;
  }

  .info-item {
    font-size: 0.85rem;
    color: var(--text-primary);
    padding: 0.35rem 0;
  }

  .info-item strong {
    color: var(--text-secondary);
  }

  .raw-read-stats {
    display: flex;
    gap: 1.5rem;
    margin-bottom: 1rem;
    flex-wrap: wrap;
  }

  .stat-item {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 0.15rem;
  }

  .stat-item strong {
    font-size: 1.25rem;
    color: var(--accent-primary);
  }

  .stat-item span {
    font-size: 0.75rem;
    color: var(--text-secondary);
  }

  .raw-read-errors-list {
    background: var(--badge-danger-bg);
    border-radius: 6px;
    padding: 0.75rem;
    margin-bottom: 1rem;
  }

  .raw-read-errors-list h4 {
    margin: 0 0 0.5rem;
    font-size: 0.85rem;
  }

  .raw-read-error-item {
    font-size: 0.8rem;
    margin: 0.25rem 0;
    word-break: break-word;
  }

  .raw-read-files h4, .raw-read-log h4 {
    margin: 0 0 0.5rem;
    font-size: 0.9rem;
  }

  .raw-read-file-list {
    max-height: 300px;
    overflow-y: auto;
    border: 1px solid var(--border-color);
    border-radius: 6px;
    margin-bottom: 1rem;
  }

  .raw-read-file-list table {
    width: 100%;
  }

  .log-output {
    background: var(--code-bg);
    border-radius: 6px;
    padding: 0.75rem;
    max-height: 250px;
    overflow-y: auto;
    font-family: 'SF Mono', 'Fira Code', 'Cascadia Code', monospace;
    font-size: 0.75rem;
    line-height: 1.5;
  }

  .log-line {
    color: var(--text-secondary);
    white-space: pre-wrap;
    word-break: break-all;
  }
</style>
