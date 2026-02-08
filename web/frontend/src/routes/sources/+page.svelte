<script lang="ts">
  import { onMount } from 'svelte';
  import * as api from '$lib/api/client';

  interface Source {
    id: number;
    name: string;
    source_type: string;
    path: string;
    include_patterns: string;
    exclude_patterns: string;
    enabled: boolean;
    created_at: string;
  }

  let sources: Source[] = [];
  let loading = true;
  let error = '';
  let showCreateModal = false;
  let showEditModal = false;
  let selectedSource: Source | null = null;

  let formData = {
    name: '',
    source_type: 'local',
    path: '',
    include_patterns: [] as string[],
    exclude_patterns: [] as string[],
  };

  let includeInput = '';
  let excludeInput = '';

  onMount(async () => {
    await loadData();
  });

  async function loadData() {
    loading = true;
    error = '';
    try {
      const result = await api.getSources();
      sources = Array.isArray(result) ? result : [];
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to load sources';
    } finally {
      loading = false;
    }
  }

  async function handleCreate() {
    try {
      await api.createSource({
        ...formData,
        include_patterns: formData.include_patterns.length > 0 ? formData.include_patterns : undefined,
        exclude_patterns: formData.exclude_patterns.length > 0 ? formData.exclude_patterns : undefined,
      });
      showCreateModal = false;
      resetForm();
      await loadData();
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to create source';
    }
  }

  async function handleUpdate() {
    if (!selectedSource) return;
    try {
      await api.updateSource(selectedSource.id, {
        name: formData.name,
        path: formData.path,
        include_patterns: formData.include_patterns,
        exclude_patterns: formData.exclude_patterns,
      });
      showEditModal = false;
      await loadData();
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to update source';
    }
  }

  async function handleDelete(source: Source) {
    if (!confirm(`Delete source "${source.name}"?`)) return;
    try {
      await api.deleteSource(source.id);
      await loadData();
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to delete source';
    }
  }

  async function handleToggle(source: Source) {
    try {
      await api.updateSource(source.id, { enabled: !source.enabled });
      await loadData();
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to update source';
    }
  }

  function openEditModal(source: Source) {
    selectedSource = source;
    formData = {
      name: source.name,
      source_type: source.source_type,
      path: source.path,
      include_patterns: parsePatterns(source.include_patterns),
      exclude_patterns: parsePatterns(source.exclude_patterns),
    };
    includeInput = '';
    excludeInput = '';
    showEditModal = true;
  }

  function parsePatterns(json: string): string[] {
    if (!json) return [];
    try {
      return JSON.parse(json);
    } catch {
      return [];
    }
  }

  function addIncludePattern() {
    if (includeInput.trim()) {
      formData.include_patterns = [...formData.include_patterns, includeInput.trim()];
      includeInput = '';
    }
  }

  function addExcludePattern() {
    if (excludeInput.trim()) {
      formData.exclude_patterns = [...formData.exclude_patterns, excludeInput.trim()];
      excludeInput = '';
    }
  }

  function removeIncludePattern(index: number) {
    formData.include_patterns = formData.include_patterns.filter((_, i) => i !== index);
  }

  function removeExcludePattern(index: number) {
    formData.exclude_patterns = formData.exclude_patterns.filter((_, i) => i !== index);
  }

  function resetForm() {
    formData = {
      name: '',
      source_type: 'local',
      path: '',
      include_patterns: [],
      exclude_patterns: [],
    };
    includeInput = '';
    excludeInput = '';
    selectedSource = null;
  }

  function getSourceTypeIcon(type: string): string {
    switch (type) {
      case 'local': return '📁';
      case 'smb': return '🖥️';
      case 'nfs': return '🌐';
      default: return '📂';
    }
  }
</script>

<div class="page-header">
  <h1>Backup Sources</h1>
  <button class="btn btn-primary" on:click={() => { showCreateModal = true; resetForm(); }}>
    + Add Source
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
  <div class="sources-grid">
    {#each sources as source}
      <div class="source-card card">
        <div class="source-header">
          <span class="source-icon">{getSourceTypeIcon(source.source_type)}</span>
          <div class="source-info">
            <h3>{source.name}</h3>
            <span class="source-type">{source.source_type}</span>
          </div>
          <span class="badge {source.enabled ? 'badge-success' : 'badge-danger'}">
            {source.enabled ? 'Enabled' : 'Disabled'}
          </span>
        </div>
        <div class="source-path">
          <code>{source.path}</code>
        </div>
        {#if source.include_patterns || source.exclude_patterns}
          <div class="source-patterns">
            {#if parsePatterns(source.include_patterns).length > 0}
              <div class="pattern-group">
                <span class="pattern-label">Include:</span>
                {#each parsePatterns(source.include_patterns) as pattern}
                  <span class="pattern-tag">{pattern}</span>
                {/each}
              </div>
            {/if}
            {#if parsePatterns(source.exclude_patterns).length > 0}
              <div class="pattern-group">
                <span class="pattern-label">Exclude:</span>
                {#each parsePatterns(source.exclude_patterns) as pattern}
                  <span class="pattern-tag exclude">{pattern}</span>
                {/each}
              </div>
            {/if}
          </div>
        {/if}
        <div class="source-actions">
          <button class="btn btn-secondary" on:click={() => openEditModal(source)}>Edit</button>
          <button class="btn btn-secondary" on:click={() => handleToggle(source)}>
            {source.enabled ? 'Disable' : 'Enable'}
          </button>
          <button class="btn btn-danger" on:click={() => handleDelete(source)}>Delete</button>
        </div>
      </div>
    {/each}
    {#if sources.length === 0}
      <div class="card no-data">
        <p>No sources found. Add a source to start backing up data.</p>
      </div>
    {/if}
  </div>
{/if}

<!-- Create Modal -->
{#if showCreateModal}
  <div class="modal-overlay" on:click={() => showCreateModal = false}>
    <div class="modal" on:click|stopPropagation={() => {}}>
      <h2>Add Backup Source</h2>
      <form on:submit|preventDefault={handleCreate}>
        <div class="form-group">
          <label for="name">Name</label>
          <input type="text" id="name" bind:value={formData.name} required placeholder="e.g., Documents Share" />
        </div>
        <div class="form-group">
          <label for="type">Source Type</label>
          <select id="type" bind:value={formData.source_type}>
            <option value="local">Local Filesystem</option>
            <option value="smb">SMB Share</option>
            <option value="nfs">NFS Mount</option>
          </select>
        </div>
        <div class="form-group">
          <label for="path">Path</label>
          <input type="text" id="path" bind:value={formData.path} required 
            placeholder="e.g., /mnt/data or /mnt/smb/share" />
        </div>
        <div class="form-group">
          <label>Include Patterns (glob)</label>
          <div class="pattern-input">
            <input type="text" bind:value={includeInput} placeholder="e.g., *.doc" />
            <button type="button" class="btn btn-secondary" on:click={addIncludePattern}>Add</button>
          </div>
          <div class="pattern-list">
            {#each formData.include_patterns as pattern, i}
              <span class="pattern-tag">
                {pattern}
                <button type="button" on:click={() => removeIncludePattern(i)}>×</button>
              </span>
            {/each}
          </div>
        </div>
        <div class="form-group">
          <label>Exclude Patterns (glob)</label>
          <div class="pattern-input">
            <input type="text" bind:value={excludeInput} placeholder="e.g., *.tmp" />
            <button type="button" class="btn btn-secondary" on:click={addExcludePattern}>Add</button>
          </div>
          <div class="pattern-list">
            {#each formData.exclude_patterns as pattern, i}
              <span class="pattern-tag exclude">
                {pattern}
                <button type="button" on:click={() => removeExcludePattern(i)}>×</button>
              </span>
            {/each}
          </div>
        </div>
        <div class="modal-actions">
          <button type="button" class="btn btn-secondary" on:click={() => showCreateModal = false}>Cancel</button>
          <button type="submit" class="btn btn-primary">Create</button>
        </div>
      </form>
    </div>
  </div>
{/if}

<!-- Edit Modal -->
{#if showEditModal && selectedSource}
  <div class="modal-overlay" on:click={() => showEditModal = false}>
    <div class="modal" on:click|stopPropagation={() => {}}>
      <h2>Edit Source</h2>
      <form on:submit|preventDefault={handleUpdate}>
        <div class="form-group">
          <label for="edit-name">Name</label>
          <input type="text" id="edit-name" bind:value={formData.name} required />
        </div>
        <div class="form-group">
          <label for="edit-path">Path</label>
          <input type="text" id="edit-path" bind:value={formData.path} required />
        </div>
        <div class="form-group">
          <label>Include Patterns</label>
          <div class="pattern-input">
            <input type="text" bind:value={includeInput} placeholder="e.g., *.doc" />
            <button type="button" class="btn btn-secondary" on:click={addIncludePattern}>Add</button>
          </div>
          <div class="pattern-list">
            {#each formData.include_patterns as pattern, i}
              <span class="pattern-tag">
                {pattern}
                <button type="button" on:click={() => removeIncludePattern(i)}>×</button>
              </span>
            {/each}
          </div>
        </div>
        <div class="form-group">
          <label>Exclude Patterns</label>
          <div class="pattern-input">
            <input type="text" bind:value={excludeInput} placeholder="e.g., *.tmp" />
            <button type="button" class="btn btn-secondary" on:click={addExcludePattern}>Add</button>
          </div>
          <div class="pattern-list">
            {#each formData.exclude_patterns as pattern, i}
              <span class="pattern-tag exclude">
                {pattern}
                <button type="button" on:click={() => removeExcludePattern(i)}>×</button>
              </span>
            {/each}
          </div>
        </div>
        <div class="modal-actions">
          <button type="button" class="btn btn-secondary" on:click={() => showEditModal = false}>Cancel</button>
          <button type="submit" class="btn btn-primary">Save</button>
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

  .sources-grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(350px, 1fr));
    gap: 1rem;
  }

  .source-card {
    padding: 1.25rem;
  }

  .source-header {
    display: flex;
    align-items: center;
    gap: 0.75rem;
    margin-bottom: 1rem;
  }

  .source-icon {
    font-size: 1.5rem;
  }

  .source-info {
    flex: 1;
  }

  .source-info h3 {
    margin: 0;
    font-size: 1rem;
  }

  .source-type {
    font-size: 0.75rem;
    color: #666;
    text-transform: uppercase;
  }

  .source-path {
    background: #f5f5f5;
    padding: 0.5rem 0.75rem;
    border-radius: 6px;
    margin-bottom: 1rem;
  }

  .source-path code {
    font-size: 0.875rem;
    word-break: break-all;
  }

  .source-patterns {
    margin-bottom: 1rem;
  }

  .pattern-group {
    display: flex;
    flex-wrap: wrap;
    gap: 0.5rem;
    align-items: center;
    margin-bottom: 0.5rem;
  }

  .pattern-label {
    font-size: 0.75rem;
    color: #666;
    font-weight: 500;
  }

  .pattern-tag {
    display: inline-flex;
    align-items: center;
    gap: 0.25rem;
    background: #e0e7ff;
    color: #4a4aff;
    padding: 0.2rem 0.5rem;
    border-radius: 4px;
    font-size: 0.75rem;
  }

  .pattern-tag.exclude {
    background: #fee2e2;
    color: #dc2626;
  }

  .pattern-tag button {
    background: none;
    border: none;
    cursor: pointer;
    padding: 0;
    font-size: 1rem;
    line-height: 1;
    opacity: 0.7;
  }

  .pattern-tag button:hover {
    opacity: 1;
  }

  .source-actions {
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
    max-width: 500px;
    max-height: 90vh;
    overflow-y: auto;
  }

  .modal h2 {
    margin: 0 0 1.5rem;
  }

  .pattern-input {
    display: flex;
    gap: 0.5rem;
    margin-bottom: 0.5rem;
  }

  .pattern-input input {
    flex: 1;
  }

  .pattern-list {
    display: flex;
    flex-wrap: wrap;
    gap: 0.5rem;
    min-height: 1.5rem;
  }

  .modal-actions {
    display: flex;
    gap: 0.75rem;
    justify-content: flex-end;
    margin-top: 1.5rem;
  }
</style>
