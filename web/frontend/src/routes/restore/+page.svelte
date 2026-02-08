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
  }

  interface CatalogEntry {
    id: number;
    backup_set_id: number;
    file_path: string;
    file_size: number;
    mod_time: string;
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

  let backupSets: BackupSet[] = [];
  let selectedSet: BackupSet | null = null;
  let catalogEntries: CatalogEntry[] = [];
  let selectedFiles: string[] = [];
  let searchQuery = '';
  let searchResults: CatalogEntry[] = [];
  let loading = true;
  let searching = false;
  let error = '';
  let showRestoreModal = false;
  let requiredTapes: TapeRequirement[] = [];

  let restoreFormData = {
    dest_path: '/restore',
    verify: true,
    overwrite: false,
  };

  onMount(async () => {
    await loadBackupSets();
  });

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
    try {
      catalogEntries = await api.getBackupFiles(set.id);
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to load catalog';
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
    try {
      const result = await api.runRestore({
        backup_set_id: selectedSet.id,
        file_paths: selectedFiles.length > 0 ? selectedFiles : undefined,
        dest_path: restoreFormData.dest_path,
        verify: restoreFormData.verify,
        overwrite: restoreFormData.overwrite,
      });
      showRestoreModal = false;
      alert(`Restore completed! ${result.files_restored} files restored.`);
    } catch (e) {
      error = e instanceof Error ? e.message : 'Restore failed';
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

  function getStatusBadgeClass(status: string): string {
    switch (status) {
      case 'completed': return 'badge-success';
      case 'running': return 'badge-warning';
      case 'failed': return 'badge-danger';
      default: return 'badge-info';
    }
  }
</script>

<div class="page-header">
  <h1>Restore</h1>
</div>

{#if error}
  <div class="card error-card">
    <p>{error}</p>
    <button class="btn btn-secondary" on:click={() => error = ''}>Dismiss</button>
  </div>
{/if}

<div class="restore-layout">
  <!-- Search Section -->
  <div class="card search-section">
    <h2>Search Catalog</h2>
    <div class="search-bar">
      <input 
        type="text" 
        bind:value={searchQuery} 
        placeholder="Search for files (e.g., *.doc, report*)"
        on:keyup={(e) => e.key === 'Enter' && handleSearch()}
      />
      <button class="btn btn-primary" on:click={handleSearch} disabled={searching}>
        {searching ? 'Searching...' : 'Search'}
      </button>
    </div>
    {#if searchResults.length > 0}
      <div class="search-results">
        <table>
          <thead>
            <tr>
              <th>File Path</th>
              <th>Size</th>
              <th>Modified</th>
            </tr>
          </thead>
          <tbody>
            {#each searchResults as result}
              <tr>
                <td>{result.file_path}</td>
                <td>{formatBytes(result.file_size)}</td>
                <td>{formatDate(result.mod_time)}</td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
    {/if}
  </div>

  <!-- Backup Sets List -->
  <div class="card backup-sets">
    <h2>Backup Sets</h2>
    {#if loading}
      <p>Loading...</p>
    {:else}
      <div class="sets-list">
        {#each backupSets as set}
          <div 
            class="set-item" 
            class:selected={selectedSet?.id === set.id}
            on:click={() => loadCatalog(set)}
          >
            <div class="set-header">
              <strong>{set.job_name}</strong>
              <span class="badge {getStatusBadgeClass(set.status)}">{set.status}</span>
            </div>
            <div class="set-meta">
              <span>📼 {set.tape_label}</span>
              <span>📅 {formatDate(set.start_time)}</span>
            </div>
            <div class="set-stats">
              <span>{set.file_count} files</span>
              <span>{formatBytes(set.total_bytes)}</span>
              <span class="badge {set.backup_type === 'full' ? 'badge-info' : 'badge-warning'}">
                {set.backup_type}
              </span>
            </div>
          </div>
        {/each}
        {#if backupSets.length === 0}
          <p class="no-data">No backup sets found.</p>
        {/if}
      </div>
    {/if}
  </div>

  <!-- File Browser -->
  <div class="card file-browser">
    <div class="browser-header">
      <h2>Files {selectedSet ? `- ${selectedSet.job_name}` : ''}</h2>
      {#if selectedSet && catalogEntries.length > 0}
        <div class="browser-actions">
          <button class="btn btn-secondary" on:click={selectAllFiles}>Select All</button>
          <button class="btn btn-secondary" on:click={deselectAllFiles}>Deselect All</button>
          <button 
            class="btn btn-success" 
            on:click={planRestore}
            disabled={!selectedSet}
          >
            Restore {selectedFiles.length > 0 ? `(${selectedFiles.length} files)` : 'All'}
          </button>
        </div>
      {/if}
    </div>
    {#if selectedSet}
      <div class="file-list">
        <table>
          <thead>
            <tr>
              <th style="width: 40px;"></th>
              <th>File Path</th>
              <th>Size</th>
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
                <td>{entry.file_path}</td>
                <td>{formatBytes(entry.file_size)}</td>
                <td>{formatDate(entry.mod_time)}</td>
              </tr>
            {/each}
            {#if catalogEntries.length === 0}
              <tr>
                <td colspan="4" class="no-data">No files in this backup set.</td>
              </tr>
            {/if}
          </tbody>
        </table>
      </div>
    {:else}
      <p class="no-data">Select a backup set to browse files.</p>
    {/if}
  </div>
</div>

<!-- Restore Modal -->
{#if showRestoreModal && selectedSet}
  <div class="modal-overlay" on:click={() => showRestoreModal = false}>
    <div class="modal" on:click|stopPropagation={() => {}}>
      <h2>Restore Files</h2>
      
      {#if requiredTapes.length > 0}
        <div class="tape-requirements">
          <h3>Required Tapes</h3>
          <p>Insert the following tape(s) in order:</p>
          <ol>
            {#each requiredTapes as req}
              <li>
                <strong>{req.tape.label}</strong> 
                <span class="badge {req.tape.status === 'active' ? 'badge-success' : 'badge-warning'}">
                  {req.tape.status}
                </span>
                <span class="tape-info">
                  {req.file_count} files, {formatBytes(req.total_bytes)}
                </span>
              </li>
            {/each}
          </ol>
        </div>
      {/if}

      <form on:submit|preventDefault={executeRestore}>
        <div class="form-group">
          <label for="dest">Destination Path</label>
          <input type="text" id="dest" bind:value={restoreFormData.dest_path} required />
        </div>
        <div class="form-group checkbox-group">
          <label>
            <input type="checkbox" bind:checked={restoreFormData.verify} />
            Verify after restore
          </label>
        </div>
        <div class="form-group checkbox-group">
          <label>
            <input type="checkbox" bind:checked={restoreFormData.overwrite} />
            Overwrite existing files
          </label>
        </div>
        <div class="modal-actions">
          <button type="button" class="btn btn-secondary" on:click={() => showRestoreModal = false}>
            Cancel
          </button>
          <button type="submit" class="btn btn-success">Start Restore</button>
        </div>
      </form>
    </div>
  </div>
{/if}

<style>
  .error-card {
    background: #f8d7da;
    color: #721c24;
    display: flex;
    justify-content: space-between;
    align-items: center;
  }

  .restore-layout {
    display: grid;
    grid-template-columns: 1fr;
    gap: 1rem;
  }

  .search-section {
    margin-bottom: 0;
  }

  .search-section h2 {
    margin: 0 0 1rem;
    font-size: 1rem;
  }

  .search-bar {
    display: flex;
    gap: 0.5rem;
  }

  .search-bar input {
    flex: 1;
  }

  .search-results {
    margin-top: 1rem;
    max-height: 200px;
    overflow-y: auto;
  }

  .backup-sets h2,
  .file-browser h2 {
    margin: 0 0 1rem;
    font-size: 1rem;
  }

  .sets-list {
    max-height: 300px;
    overflow-y: auto;
  }

  .set-item {
    padding: 0.75rem;
    border: 1px solid #eee;
    border-radius: 8px;
    margin-bottom: 0.5rem;
    cursor: pointer;
    transition: all 0.2s ease;
  }

  .set-item:hover {
    border-color: #4a4aff;
  }

  .set-item.selected {
    border-color: #4a4aff;
    background: #f0f0ff;
  }

  .set-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    margin-bottom: 0.5rem;
  }

  .set-meta {
    font-size: 0.75rem;
    color: #666;
    display: flex;
    gap: 1rem;
    margin-bottom: 0.25rem;
  }

  .set-stats {
    font-size: 0.75rem;
    display: flex;
    gap: 0.5rem;
    align-items: center;
  }

  .browser-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    margin-bottom: 1rem;
  }

  .browser-actions {
    display: flex;
    gap: 0.5rem;
  }

  .file-list {
    max-height: 400px;
    overflow-y: auto;
  }

  .file-list tr.selected {
    background: #f0f0ff;
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
    max-width: 500px;
    max-height: 90vh;
    overflow-y: auto;
  }

  .modal h2 {
    margin: 0 0 1.5rem;
  }

  .tape-requirements {
    background: #f0f0ff;
    padding: 1rem;
    border-radius: 8px;
    margin-bottom: 1.5rem;
  }

  .tape-requirements h3 {
    margin: 0 0 0.5rem;
    font-size: 0.875rem;
  }

  .tape-requirements ol {
    margin: 0.5rem 0;
    padding-left: 1.25rem;
  }

  .tape-requirements li {
    margin: 0.5rem 0;
  }

  .tape-info {
    font-size: 0.75rem;
    color: #666;
    margin-left: 0.5rem;
  }

  .checkbox-group {
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
</style>
