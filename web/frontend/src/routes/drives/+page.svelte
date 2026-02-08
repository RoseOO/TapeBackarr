<script lang="ts">
  import { onMount } from 'svelte';
  import { api } from '$lib/api/client';

  interface Drive {
    id: number;
    device_path: string;
    display_name: string;
    serial_number: string;
    model: string;
    status: string;
    current_tape_id: number | null;
    enabled: boolean;
    created_at: string;
  }

  let drives: Drive[] = [];
  let loading = true;
  let error = '';
  let showAddModal = false;
  let newDrive = {
    device_path: '',
    display_name: '',
    serial_number: '',
    model: ''
  };

  onMount(async () => {
    await loadDrives();
  });

  async function loadDrives() {
    loading = true;
    try {
      drives = await api.get('/drives');
    } catch (e) {
      error = 'Failed to load drives';
    } finally {
      loading = false;
    }
  }

  async function addDrive() {
    try {
      await api.post('/drives', newDrive);
      showAddModal = false;
      newDrive = { device_path: '', display_name: '', serial_number: '', model: '' };
      await loadDrives();
    } catch (e) {
      error = 'Failed to add drive';
    }
  }

  async function deleteDrive(id: number) {
    if (!confirm('Are you sure you want to remove this drive?')) return;
    try {
      await api.delete(`/drives/${id}`);
      await loadDrives();
    } catch (e) {
      error = 'Failed to delete drive';
    }
  }

  async function selectDrive(id: number) {
    try {
      await api.post(`/drives/${id}/select`, {});
      await loadDrives();
    } catch (e) {
      error = 'Failed to select drive';
    }
  }

  async function ejectTape(id: number) {
    try {
      await api.post(`/drives/${id}/eject`, {});
      await loadDrives();
    } catch (e) {
      error = 'Failed to eject tape';
    }
  }

  async function rewindTape(id: number) {
    try {
      await api.post(`/drives/${id}/rewind`, {});
      await loadDrives();
    } catch (e) {
      error = 'Failed to rewind tape';
    }
  }

  function getStatusBadge(status: string) {
    switch (status) {
      case 'ready': return 'badge-success';
      case 'busy': return 'badge-warning';
      case 'offline': return 'badge-danger';
      case 'error': return 'badge-danger';
      default: return 'badge-info';
    }
  }
</script>

<div class="page-header">
  <h1>Tape Drives</h1>
  <button class="btn btn-primary" on:click={() => showAddModal = true}>Add Drive</button>
</div>

{#if error}
  <div class="alert alert-error">{error}</div>
{/if}

{#if loading}
  <p>Loading...</p>
{:else}
  <div class="card">
    <table>
      <thead>
        <tr>
          <th>Name</th>
          <th>Device Path</th>
          <th>Model</th>
          <th>Status</th>
          <th>Current Tape</th>
          <th>Actions</th>
        </tr>
      </thead>
      <tbody>
        {#each drives as drive}
          <tr>
            <td>{drive.display_name || drive.device_path}</td>
            <td><code>{drive.device_path}</code></td>
            <td>{drive.model || '-'}</td>
            <td><span class="badge {getStatusBadge(drive.status)}">{drive.status}</span></td>
            <td>{drive.current_tape_id ? `Tape #${drive.current_tape_id}` : 'No tape'}</td>
            <td>
              <button class="btn btn-secondary btn-sm" on:click={() => selectDrive(drive.id)}>Select</button>
              <button class="btn btn-secondary btn-sm" on:click={() => rewindTape(drive.id)}>Rewind</button>
              <button class="btn btn-secondary btn-sm" on:click={() => ejectTape(drive.id)}>Eject</button>
              <button class="btn btn-danger btn-sm" on:click={() => deleteDrive(drive.id)}>Remove</button>
            </td>
          </tr>
        {:else}
          <tr>
            <td colspan="6">No drives configured. Add a drive to get started.</td>
          </tr>
        {/each}
      </tbody>
    </table>
  </div>
{/if}

{#if showAddModal}
  <div class="modal-backdrop" on:click={() => showAddModal = false}>
    <div class="modal" on:click|stopPropagation>
      <h2>Add Tape Drive</h2>
      <form on:submit|preventDefault={addDrive}>
        <div class="form-group">
          <label for="device_path">Device Path</label>
          <input id="device_path" type="text" bind:value={newDrive.device_path} placeholder="/dev/nst0" required />
        </div>
        <div class="form-group">
          <label for="display_name">Display Name</label>
          <input id="display_name" type="text" bind:value={newDrive.display_name} placeholder="Primary LTO Drive" />
        </div>
        <div class="form-group">
          <label for="model">Model</label>
          <input id="model" type="text" bind:value={newDrive.model} placeholder="LTO-8" />
        </div>
        <div class="form-group">
          <label for="serial_number">Serial Number</label>
          <input id="serial_number" type="text" bind:value={newDrive.serial_number} placeholder="Optional" />
        </div>
        <div class="form-actions">
          <button type="button" class="btn btn-secondary" on:click={() => showAddModal = false}>Cancel</button>
          <button type="submit" class="btn btn-primary">Add Drive</button>
        </div>
      </form>
    </div>
  </div>
{/if}

<style>
  .alert {
    padding: 1rem;
    border-radius: 8px;
    margin-bottom: 1rem;
  }

  .alert-error {
    background: #fee;
    color: #c00;
    border: 1px solid #fcc;
  }

  code {
    background: #f0f0f0;
    padding: 0.25rem 0.5rem;
    border-radius: 4px;
    font-family: monospace;
  }

  .btn-sm {
    padding: 0.25rem 0.5rem;
    font-size: 0.75rem;
    margin-right: 0.25rem;
  }

  .modal-backdrop {
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
    background: white;
    padding: 2rem;
    border-radius: 12px;
    max-width: 500px;
    width: 90%;
  }

  .modal h2 {
    margin: 0 0 1.5rem;
  }

  .form-actions {
    display: flex;
    gap: 1rem;
    justify-content: flex-end;
    margin-top: 1.5rem;
  }
</style>
