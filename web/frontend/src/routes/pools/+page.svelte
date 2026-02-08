<script lang="ts">
  import { onMount } from 'svelte';
  import * as api from '$lib/api/client';

  interface Pool {
    id: number;
    name: string;
    description: string;
    retention_days: number;
    allow_reuse: boolean;
    allocation_policy: string;
    tape_count: number;
    created_at: string;
  }

  let pools: Pool[] = [];
  let loading = true;
  let error = '';
  let successMsg = '';
  let showCreateModal = false;
  let showEditModal = false;
  let selectedPool: Pool | null = null;

  let formData = {
    name: '',
    description: '',
    retention_days: 30,
    allow_reuse: true,
    allocation_policy: 'continue',
  };

  onMount(async () => {
    await loadPools();
  });

  async function loadPools() {
    try {
      pools = await api.getPools();
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to load pools';
    } finally {
      loading = false;
    }
  }

  function showSuccessMessage(msg: string) {
    successMsg = msg;
    setTimeout(() => successMsg = '', 3000);
  }

  async function handleCreate() {
    try {
      error = '';
      if (!formData.name) {
        error = 'Pool name is required';
        return;
      }
      await api.createPool(formData as any);
      showCreateModal = false;
      resetForm();
      showSuccessMessage('Pool created');
      await loadPools();
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to create pool';
    }
  }

  async function handleUpdate() {
    if (!selectedPool) return;
    try {
      error = '';
      await api.updatePool(selectedPool.id, formData as any);
      showEditModal = false;
      showSuccessMessage('Pool updated');
      await loadPools();
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to update pool';
    }
  }

  async function handleDelete(pool: Pool) {
    if (pool.tape_count > 0) {
      error = `Cannot delete pool "${pool.name}" - it has ${pool.tape_count} tape(s) assigned`;
      return;
    }
    if (!confirm(`Delete pool "${pool.name}"?`)) return;
    try {
      error = '';
      await api.deletePool(pool.id);
      showSuccessMessage('Pool deleted');
      await loadPools();
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to delete pool';
    }
  }

  function openEditModal(pool: Pool) {
    selectedPool = pool;
    formData = {
      name: pool.name,
      description: pool.description,
      retention_days: pool.retention_days,
      allow_reuse: pool.allow_reuse,
      allocation_policy: pool.allocation_policy || 'continue',
    };
    showEditModal = true;
  }

  function resetForm() {
    formData = {
      name: '',
      description: '',
      retention_days: 30,
      allow_reuse: true,
      allocation_policy: 'continue',
    };
    selectedPool = null;
  }

  function formatRetention(days: number): string {
    if (days === 0) return 'Forever';
    if (days < 7) return `${days} day${days > 1 ? 's' : ''}`;
    const weeks = Math.floor(days / 7);
    if (days < 30) return `${weeks} week${weeks > 1 ? 's' : ''}`;
    const months = Math.floor(days / 30);
    if (days < 365) return `${months} month${months > 1 ? 's' : ''}`;
    const years = Math.floor(days / 365);
    return `${years} year${years > 1 ? 's' : ''}`;
  }
</script>

<div class="page-header">
  <h1>Media Pools</h1>
  <button class="btn btn-primary" on:click={() => { showCreateModal = true; resetForm(); }}>
    + Create Pool
  </button>
</div>

{#if error}
  <div class="card error-card">
    <p>{error}</p>
    <button class="btn btn-secondary" style="margin-top: 0.5rem; font-size: 0.75rem;" on:click={() => error = ''}>Dismiss</button>
  </div>
{/if}

{#if successMsg}
  <div class="card success-card">
    <p>{successMsg}</p>
  </div>
{/if}

{#if loading}
  <p>Loading...</p>
{:else}
  <div class="pools-grid">
    {#each pools as pool}
      <div class="card pool-card">
        <div class="pool-header">
          <h3>{pool.name}</h3>
          <div class="pool-actions">
            <button class="btn btn-secondary btn-sm" on:click={() => openEditModal(pool)}>Edit</button>
            <button class="btn btn-danger btn-sm" on:click={() => handleDelete(pool)}>Delete</button>
          </div>
        </div>
        <p class="pool-desc">{pool.description || 'No description'}</p>
        <div class="pool-stats">
          <div class="stat">
            <span class="stat-label">Tapes</span>
            <span class="stat-value">{pool.tape_count}</span>
          </div>
          <div class="stat">
            <span class="stat-label">Retention</span>
            <span class="stat-value">{formatRetention(pool.retention_days)}</span>
          </div>
          <div class="stat">
            <span class="stat-label">Reuse</span>
            <span class="stat-value">{pool.allow_reuse ? 'Allowed' : 'Disabled'}</span>
          </div>
          <div class="stat">
            <span class="stat-label">Allocation</span>
            <span class="stat-value">{pool.allocation_policy || 'continue'}</span>
          </div>
        </div>
      </div>
    {:else}
      <div class="card">
        <p class="no-data">No pools configured. Create a pool to organize your tapes.</p>
      </div>
    {/each}
  </div>
{/if}

<!-- Create Pool Modal -->
{#if showCreateModal}
  <div class="modal-overlay" on:click={() => showCreateModal = false}>
    <div class="modal" on:click|stopPropagation>
      <h2>Create Media Pool</h2>
      <p class="modal-desc">Pools group tapes by lifecycle policy. Each tape belongs to exactly one pool.</p>
      <form on:submit|preventDefault={handleCreate}>
        <div class="form-group">
          <label for="name">Pool Name <span class="required">*</span></label>
          <input type="text" id="name" bind:value={formData.name} required placeholder="e.g., WEEKLY, OFFSITE" />
        </div>
        <div class="form-group">
          <label for="description">Description</label>
          <input type="text" id="description" bind:value={formData.description} placeholder="e.g., Weekly rotation tapes" />
        </div>
        <div class="form-group">
          <label for="retention">Retention (days)</label>
          <input type="number" id="retention" bind:value={formData.retention_days} min="0" />
          <small>0 = retain forever. Tapes expire after this period.</small>
        </div>
        <div class="form-group checkbox-group">
          <label>
            <input type="checkbox" bind:checked={formData.allow_reuse} />
            Allow tape reuse after expiry
          </label>
          <small>When enabled, expired tapes can be formatted and reused</small>
        </div>
        <div class="form-group">
          <label for="allocation">Allocation Policy</label>
          <select id="allocation" bind:value={formData.allocation_policy}>
            <option value="continue">Continue (fill current tape first)</option>
            <option value="always-new">Always New (new tape per job)</option>
          </select>
        </div>
        <div class="modal-actions">
          <button type="button" class="btn btn-secondary" on:click={() => showCreateModal = false}>Cancel</button>
          <button type="submit" class="btn btn-primary">Create Pool</button>
        </div>
      </form>
    </div>
  </div>
{/if}

<!-- Edit Pool Modal -->
{#if showEditModal && selectedPool}
  <div class="modal-overlay" on:click={() => showEditModal = false}>
    <div class="modal" on:click|stopPropagation>
      <h2>Edit Pool: {selectedPool.name}</h2>
      <form on:submit|preventDefault={handleUpdate}>
        <div class="form-group">
          <label for="edit-name">Pool Name</label>
          <input type="text" id="edit-name" bind:value={formData.name} required />
        </div>
        <div class="form-group">
          <label for="edit-description">Description</label>
          <input type="text" id="edit-description" bind:value={formData.description} />
        </div>
        <div class="form-group">
          <label for="edit-retention">Retention (days)</label>
          <input type="number" id="edit-retention" bind:value={formData.retention_days} min="0" />
          <small>0 = retain forever</small>
        </div>
        <div class="form-group checkbox-group">
          <label>
            <input type="checkbox" bind:checked={formData.allow_reuse} />
            Allow tape reuse after expiry
          </label>
        </div>
        <div class="form-group">
          <label for="edit-allocation">Allocation Policy</label>
          <select id="edit-allocation" bind:value={formData.allocation_policy}>
            <option value="continue">Continue</option>
            <option value="always-new">Always New</option>
          </select>
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

  .success-card {
    background: #d4edda;
    color: #155724;
    padding: 0.75rem 1rem;
    border-radius: 8px;
    margin-bottom: 1rem;
  }

  .pools-grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(320px, 1fr));
    gap: 1rem;
  }

  .pool-card {
    padding: 1.25rem;
  }

  .pool-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    margin-bottom: 0.5rem;
  }

  .pool-header h3 {
    margin: 0;
    font-size: 1.1rem;
  }

  .pool-actions {
    display: flex;
    gap: 0.5rem;
  }

  .pool-desc {
    color: #666;
    font-size: 0.875rem;
    margin: 0 0 1rem;
  }

  .pool-stats {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 0.75rem;
  }

  .stat {
    display: flex;
    flex-direction: column;
  }

  .stat-label {
    font-size: 0.75rem;
    color: #888;
    text-transform: uppercase;
  }

  .stat-value {
    font-weight: 600;
    font-size: 0.95rem;
  }

  .btn-sm {
    padding: 0.25rem 0.5rem;
    font-size: 0.75rem;
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
    max-width: 480px;
  }

  .modal h2 {
    margin: 0 0 0.5rem;
  }

  .modal-desc {
    color: #666;
    font-size: 0.875rem;
    margin: 0 0 1.5rem;
  }

  .modal-actions {
    display: flex;
    gap: 0.75rem;
    justify-content: flex-end;
    margin-top: 1.5rem;
  }

  .required {
    color: #dc3545;
  }

  small {
    display: block;
    color: #888;
    font-size: 0.75rem;
    margin-top: 0.25rem;
  }

  .checkbox-group label {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    cursor: pointer;
  }

  .checkbox-group input[type="checkbox"] {
    width: auto;
  }
</style>