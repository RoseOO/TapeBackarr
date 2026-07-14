<script lang="ts">
  import { onMount } from 'svelte';
  import * as api from '$lib/api/client';

  interface Destination {
    id: number;
    name: string;
    destination_type: string;
    path: string;
    pool_id: number | null;
    enabled: boolean;
    created_at: string;
  }

  let destinations: Destination[] = [];
  let loading = true;
  let error = '';
  let showCreateModal = false;
  let showEditModal = false;
  let selectedDest: Destination | null = null;

  let formData = {
    name: '',
    destination_type: 'tape_pool',
    path: '',
    pool_id: null as number | null,
  };

  onMount(async () => {
    await loadData();
  });

  async function loadData() {
    loading = true;
    error = '';
    try {
      const result = await api.getDestinations();
      destinations = Array.isArray(result) ? result : [];
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to load destinations';
    } finally {
      loading = false;
    }
  }

  async function handleCreate() {
    try {
      await api.createDestination({
        name: formData.name,
        destination_type: formData.destination_type,
        path: formData.destination_type === 'file' ? formData.path : undefined,
        pool_id: formData.destination_type === 'tape_pool' ? formData.pool_id : undefined,
      });
      showCreateModal = false;
      resetForm();
      await loadData();
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to create destination';
    }
  }

  async function handleUpdate() {
    if (!selectedDest) return;
    try {
      await api.updateDestination(selectedDest.id, {
        name: formData.name,
        destination_type: formData.destination_type,
        path: formData.destination_type === 'file' ? formData.path : undefined,
        pool_id: formData.destination_type === 'tape_pool' ? formData.pool_id : undefined,
      });
      showEditModal = false;
      await loadData();
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to update destination';
    }
  }

  async function handleDelete(dest: Destination) {
    if (!confirm(`Delete destination "${dest.name}"?`)) return;
    try {
      await api.deleteDestination(dest.id);
      await loadData();
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to delete destination';
    }
  }

  async function handleToggle(dest: Destination) {
    try {
      await api.updateDestination(dest.id, { enabled: !dest.enabled });
      await loadData();
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to update destination';
    }
  }

  function openEditModal(dest: Destination) {
    selectedDest = dest;
    formData = {
      name: dest.name,
      destination_type: dest.destination_type,
      path: dest.path,
      pool_id: dest.pool_id,
    };
    showEditModal = true;
  }

  function resetForm() {
    formData = {
      name: '',
      destination_type: 'tape_pool',
      path: '',
      pool_id: null,
    };
    selectedDest = null;
  }

  function getDestTypeLabel(type: string): string {
    switch (type) {
      case 'tape_pool': return 'Tape Pool';
      case 'file': return 'File / NFS';
      default: return type;
    }
  }

  function getDestTypeIcon(type: string): string {
    switch (type) {
      case 'tape_pool': return '';
      case 'file': return '';
      default: return '';
    }
  }
</script>

<div class="page-header">
  <h1>Backup Destinations</h1>
  <button class="btn btn-primary" on:click={() => { showCreateModal = true; resetForm(); }}>
    + Add Destination
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
  <div class="destinations-grid">
    {#each destinations as dest}
      <div class="dest-card card">
        <div class="dest-header">
          <span class="dest-icon">{getDestTypeIcon(dest.destination_type)}</span>
          <div class="dest-info">
            <h3>{dest.name}</h3>
            <span class="dest-type">{getDestTypeLabel(dest.destination_type)}</span>
          </div>
          <span class="badge {dest.enabled ? 'badge-success' : 'badge-danger'}">
            {dest.enabled ? 'Enabled' : 'Disabled'}
          </span>
        </div>
        {#if dest.path}
          <div class="dest-path">
            <code>{dest.path}</code>
          </div>
        {/if}
        {#if dest.pool_id}
          <div class="dest-meta">
            <span>Pool ID: {dest.pool_id}</span>
          </div>
        {/if}
        <div class="dest-actions">
          <button class="btn btn-secondary" on:click={() => openEditModal(dest)}>Edit</button>
          <button class="btn btn-secondary" on:click={() => handleToggle(dest)}>
            {dest.enabled ? 'Disable' : 'Enable'}
          </button>
          <button class="btn btn-danger" on:click={() => handleDelete(dest)}>Delete</button>
        </div>
      </div>
    {/each}
    {#if destinations.length === 0}
      <div class="card no-data">
        <p>No destinations found. Add a destination to configure where backups are stored.</p>
      </div>
    {/if}
  </div>
{/if}

<!-- Create Modal -->
{#if showCreateModal}
  <div class="modal-overlay" on:click={() => showCreateModal = false}>
    <div class="modal" on:click|stopPropagation={() => {}}>
      <h2>Add Backup Destination</h2>
      <form on:submit|preventDefault={handleCreate}>
        <div class="form-group">
          <label for="name">Name</label>
          <input type="text" id="name" bind:value={formData.name} required placeholder="e.g., NAS Backup Share" />
        </div>
        <div class="form-group">
          <label for="type">Destination Type</label>
          <select id="type" bind:value={formData.destination_type}>
            <option value="tape_pool">Tape Pool</option>
            <option value="file">File (NFS / Mounted Disk)</option>
          </select>
        </div>
        {#if formData.destination_type === 'file'}
          <div class="form-group">
            <label for="path">Output Path</label>
            <input type="text" id="path" bind:value={formData.path} required
              placeholder="e.g., /mnt/nfs/backups or /backups" />
            <span class="form-hint">Directory where tar archives will be written</span>
          </div>
        {/if}
        {#if formData.destination_type === 'tape_pool'}
          <div class="form-group">
            <label for="pool_id">Tape Pool ID (optional)</label>
            <input type="number" id="pool_id" bind:value={formData.pool_id}
              placeholder="Leave empty to use job's pool" />
          </div>
        {/if}
        <div class="modal-actions">
          <button type="button" class="btn btn-secondary" on:click={() => showCreateModal = false}>Cancel</button>
          <button type="submit" class="btn btn-primary">Create</button>
        </div>
      </form>
    </div>
  </div>
{/if}

<!-- Edit Modal -->
{#if showEditModal && selectedDest}
  <div class="modal-overlay" on:click={() => showEditModal = false}>
    <div class="modal" on:click|stopPropagation={() => {}}>
      <h2>Edit Destination</h2>
      <form on:submit|preventDefault={handleUpdate}>
        <div class="form-group">
          <label for="edit-name">Name</label>
          <input type="text" id="edit-name" bind:value={formData.name} required />
        </div>
        <div class="form-group">
          <label for="edit-type">Destination Type</label>
          <select id="edit-type" bind:value={formData.destination_type}>
            <option value="tape_pool">Tape Pool</option>
            <option value="file">File (NFS / Mounted Disk)</option>
          </select>
        </div>
        {#if formData.destination_type === 'file'}
          <div class="form-group">
            <label for="edit-path">Output Path</label>
            <input type="text" id="edit-path" bind:value={formData.path} required />
          </div>
        {/if}
        {#if formData.destination_type === 'tape_pool'}
          <div class="form-group">
            <label for="edit-pool-id">Tape Pool ID (optional)</label>
            <input type="number" id="edit-pool-id" bind:value={formData.pool_id} />
          </div>
        {/if}
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
    background: var(--badge-danger-bg);
    color: var(--badge-danger-text);
  }

  .destinations-grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(350px, 1fr));
    gap: 1rem;
  }

  .dest-card {
    padding: 1.25rem;
  }

  .dest-header {
    display: flex;
    align-items: center;
    gap: 0.75rem;
    margin-bottom: 1rem;
  }

  .dest-icon {
    font-size: 1.5rem;
  }

  .dest-info {
    flex: 1;
  }

  .dest-info h3 {
    margin: 0;
    font-size: 1rem;
  }

  .dest-type {
    font-size: 0.75rem;
    color: var(--text-muted);
    text-transform: uppercase;
  }

  .dest-path {
    background: var(--bg-input);
    padding: 0.5rem 0.75rem;
    border-radius: 6px;
    margin-bottom: 0.5rem;
  }

  .dest-path code {
    font-size: 0.875rem;
    word-break: break-all;
  }

  .dest-meta {
    font-size: 0.8rem;
    color: var(--text-muted);
    margin-bottom: 1rem;
  }

  .dest-actions {
    display: flex;
    gap: 0.5rem;
  }

  .no-data {
    text-align: center;
    color: var(--text-muted);
    padding: 2rem;
  }

  .form-hint {
    font-size: 0.75rem;
    color: var(--text-muted);
    margin-top: 0.25rem;
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
    background: var(--bg-card);
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

  .modal-actions {
    display: flex;
    gap: 0.75rem;
    justify-content: flex-end;
    margin-top: 1.5rem;
  }
</style>
