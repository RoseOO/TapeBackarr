<script lang="ts">
  import { onMount } from 'svelte';
  import * as api from '$lib/api/client';

  interface Tape {
    id: number;
    barcode: string;
    label: string;
    pool_id: number | null;
    pool_name: string | null;
    status: string;
    capacity_bytes: number;
    used_bytes: number;
    write_count: number;
    last_written_at: string | null;
    created_at: string;
  }

  interface Pool {
    id: number;
    name: string;
  }

  let tapes: Tape[] = [];
  let pools: Pool[] = [];
  let loading = true;
  let error = '';
  let showCreateModal = false;
  let showEditModal = false;
  let selectedTape: Tape | null = null;

  // Form data
  let formData = {
    barcode: '',
    label: '',
    pool_id: null as number | null,
    capacity_bytes: 12000000000000, // 12TB default
  };

  onMount(async () => {
    await loadData();
  });

  async function loadData() {
    try {
      [tapes, pools] = await Promise.all([api.getTapes(), api.getPools()]);
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to load data';
    } finally {
      loading = false;
    }
  }

  async function handleCreate() {
    try {
      await api.createTape(formData);
      showCreateModal = false;
      resetForm();
      await loadData();
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to create tape';
    }
  }

  async function handleUpdate() {
    if (!selectedTape) return;
    try {
      await api.updateTape(selectedTape.id, {
        label: formData.label,
        pool_id: formData.pool_id,
        status: selectedTape.status,
      });
      showEditModal = false;
      await loadData();
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to update tape';
    }
  }

  async function handleDelete(tape: Tape) {
    if (!confirm(`Delete tape ${tape.label}?`)) return;
    try {
      await api.deleteTape(tape.id);
      await loadData();
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to delete tape';
    }
  }

  async function handleStatusChange(tape: Tape, status: string) {
    try {
      await api.updateTape(tape.id, { status });
      await loadData();
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to update status';
    }
  }

  function openEditModal(tape: Tape) {
    selectedTape = tape;
    formData = {
      barcode: tape.barcode,
      label: tape.label,
      pool_id: tape.pool_id,
      capacity_bytes: tape.capacity_bytes,
    };
    showEditModal = true;
  }

  function resetForm() {
    formData = {
      barcode: '',
      label: '',
      pool_id: null,
      capacity_bytes: 12000000000000,
    };
    selectedTape = null;
  }

  function formatBytes(bytes: number): string {
    if (bytes === 0) return '0 B';
    const k = 1024;
    const sizes = ['B', 'KB', 'MB', 'GB', 'TB', 'PB'];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
  }

  function getStatusBadgeClass(status: string): string {
    switch (status) {
      case 'blank': return 'badge-info';
      case 'active': return 'badge-success';
      case 'full': return 'badge-warning';
      case 'retired': return 'badge-danger';
      case 'offsite': return 'badge-info';
      default: return '';
    }
  }

  function getUsagePercent(tape: Tape): number {
    if (tape.capacity_bytes === 0) return 0;
    return (tape.used_bytes / tape.capacity_bytes) * 100;
  }
</script>

<div class="page-header">
  <h1>Tape Management</h1>
  <button class="btn btn-primary" on:click={() => { showCreateModal = true; resetForm(); }}>
    + Add Tape
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
  <div class="card">
    <table>
      <thead>
        <tr>
          <th>Label</th>
          <th>Barcode</th>
          <th>Pool</th>
          <th>Status</th>
          <th>Usage</th>
          <th>Writes</th>
          <th>Last Written</th>
          <th>Actions</th>
        </tr>
      </thead>
      <tbody>
        {#each tapes as tape}
          <tr>
            <td><strong>{tape.label}</strong></td>
            <td>{tape.barcode || '-'}</td>
            <td>{tape.pool_name || '-'}</td>
            <td>
              <span class="badge {getStatusBadgeClass(tape.status)}">{tape.status}</span>
            </td>
            <td>
              <div class="usage-bar">
                <div class="usage-fill" style="width: {getUsagePercent(tape)}%"></div>
              </div>
              <span class="usage-text">{formatBytes(tape.used_bytes)} / {formatBytes(tape.capacity_bytes)}</span>
            </td>
            <td>{tape.write_count}</td>
            <td>{tape.last_written_at ? new Date(tape.last_written_at).toLocaleDateString() : '-'}</td>
            <td>
              <div class="actions">
                <button class="btn btn-secondary" on:click={() => openEditModal(tape)}>Edit</button>
                <select on:change={(e) => handleStatusChange(tape, e.target.value)} value={tape.status}>
                  <option value="blank">Blank</option>
                  <option value="active">Active</option>
                  <option value="full">Full</option>
                  <option value="retired">Retired</option>
                  <option value="offsite">Offsite</option>
                </select>
                <button class="btn btn-danger" on:click={() => handleDelete(tape)}>Delete</button>
              </div>
            </td>
          </tr>
        {/each}
        {#if tapes.length === 0}
          <tr>
            <td colspan="8" class="no-data">No tapes found. Add a tape to get started.</td>
          </tr>
        {/if}
      </tbody>
    </table>
  </div>
{/if}

<!-- Create Modal -->
{#if showCreateModal}
  <div class="modal-overlay" on:click={() => showCreateModal = false}>
    <div class="modal" on:click|stopPropagation>
      <h2>Add New Tape</h2>
      <form on:submit|preventDefault={handleCreate}>
        <div class="form-group">
          <label for="barcode">Barcode</label>
          <input type="text" id="barcode" bind:value={formData.barcode} placeholder="e.g., ABC123L8" />
        </div>
        <div class="form-group">
          <label for="label">Label</label>
          <input type="text" id="label" bind:value={formData.label} required placeholder="e.g., WEEKLY-001" />
        </div>
        <div class="form-group">
          <label for="pool">Pool</label>
          <select id="pool" bind:value={formData.pool_id}>
            <option value={null}>No Pool</option>
            {#each pools as pool}
              <option value={pool.id}>{pool.name}</option>
            {/each}
          </select>
        </div>
        <div class="form-group">
          <label for="capacity">Capacity (TB)</label>
          <input type="number" id="capacity" value={formData.capacity_bytes / 1000000000000} 
            on:input={(e) => formData.capacity_bytes = parseFloat(e.target.value) * 1000000000000} />
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
{#if showEditModal && selectedTape}
  <div class="modal-overlay" on:click={() => showEditModal = false}>
    <div class="modal" on:click|stopPropagation>
      <h2>Edit Tape</h2>
      <form on:submit|preventDefault={handleUpdate}>
        <div class="form-group">
          <label for="edit-label">Label</label>
          <input type="text" id="edit-label" bind:value={formData.label} required />
        </div>
        <div class="form-group">
          <label for="edit-pool">Pool</label>
          <select id="edit-pool" bind:value={formData.pool_id}>
            <option value={null}>No Pool</option>
            {#each pools as pool}
              <option value={pool.id}>{pool.name}</option>
            {/each}
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

  .usage-bar {
    width: 100px;
    height: 8px;
    background: #e0e0e0;
    border-radius: 4px;
    overflow: hidden;
    margin-bottom: 4px;
  }

  .usage-fill {
    height: 100%;
    background: #4a4aff;
    border-radius: 4px;
  }

  .usage-text {
    font-size: 0.75rem;
    color: #666;
  }

  .actions {
    display: flex;
    gap: 0.5rem;
    align-items: center;
  }

  .actions select {
    width: auto;
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
    max-width: 400px;
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
