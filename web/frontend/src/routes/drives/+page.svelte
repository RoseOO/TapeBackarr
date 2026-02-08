<script lang="ts">
  import { onMount } from 'svelte';
  import * as api from '$lib/api/client';

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

  interface ScannedDrive {
    device_path: string;
    status: string;
    vendor?: string;
    model?: string;
    serial_number?: string;
  }

  let drives: Drive[] = [];
  let scannedDrives: ScannedDrive[] = [];
  let loading = true;
  let scanning = false;
  let error = '';
  let successMsg = '';
  let showAddModal = false;
  let showScanModal = false;
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
      drives = await api.getDrives();
    } catch (e) {
      error = 'Failed to load drives';
    } finally {
      loading = false;
    }
  }

  function showSuccessMsg(msg: string) {
    successMsg = msg;
    setTimeout(() => successMsg = '', 3000);
  }

  async function addDrive() {
    try {
      error = '';
      await api.api.post('/drives', newDrive);
      showAddModal = false;
      newDrive = { device_path: '', display_name: '', serial_number: '', model: '' };
      showSuccessMsg('Drive added');
      await loadDrives();
    } catch (e) {
      error = 'Failed to add drive';
    }
  }

  async function addScannedDrive(scanned: ScannedDrive) {
    try {
      error = '';
      await api.api.post('/drives', {
        device_path: scanned.device_path,
        display_name: scanned.model ? `${scanned.vendor || ''} ${scanned.model}`.trim() : scanned.device_path,
        serial_number: scanned.serial_number || '',
        model: scanned.model || '',
      });
      showScanModal = false;
      showSuccessMsg('Drive added from scan');
      await loadDrives();
    } catch (e) {
      error = 'Failed to add scanned drive';
    }
  }

  async function scanForDrives() {
    scanning = true;
    try {
      error = '';
      scannedDrives = await api.scanDrives();
      showScanModal = true;
    } catch (e) {
      error = 'Failed to scan for drives';
    } finally {
      scanning = false;
    }
  }

  async function deleteDrive(id: number) {
    if (!confirm('Are you sure you want to remove this drive?')) return;
    try {
      error = '';
      await api.api.delete(`/drives/${id}`);
      showSuccessMsg('Drive removed');
      await loadDrives();
    } catch (e) {
      error = 'Failed to delete drive';
    }
  }

  async function selectDrive(id: number) {
    try {
      error = '';
      await api.api.post(`/drives/${id}/select`, {});
      showSuccessMsg('Drive selected');
      await loadDrives();
    } catch (e) {
      error = 'Failed to select drive';
    }
  }

  async function ejectTape(id: number) {
    try {
      error = '';
      await api.api.post(`/drives/${id}/eject`, {});
      showSuccessMsg('Tape ejected');
      await loadDrives();
    } catch (e) {
      error = 'Failed to eject tape';
    }
  }

  async function rewindTape(id: number) {
    try {
      error = '';
      await api.api.post(`/drives/${id}/rewind`, {});
      showSuccessMsg('Tape rewound');
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

  function isDriveAlreadyAdded(devicePath: string): boolean {
    return drives.some(d => d.device_path === devicePath);
  }
</script>

<div class="page-header">
  <h1>Tape Drives</h1>
  <div class="header-actions">
    <button class="btn btn-secondary" on:click={scanForDrives} disabled={scanning}>
      {scanning ? 'Scanning...' : '🔍 Scan Drives'}
    </button>
    <button class="btn btn-primary" on:click={() => showAddModal = true}>Add Drive</button>
  </div>
</div>

{#if error}
  <div class="alert alert-error">{error}
    <button class="dismiss-btn" on:click={() => error = ''}>×</button>
  </div>
{/if}

{#if successMsg}
  <div class="alert alert-success">{successMsg}</div>
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
          <th>Enabled</th>
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
            <td>{drive.enabled ? '✅' : '❌'}</td>
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
            <td colspan="7">No drives configured. Use "Scan Drives" to detect drives or add one manually.</td>
          </tr>
        {/each}
      </tbody>
    </table>
  </div>
{/if}

<!-- Add Drive Modal -->
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

<!-- Scan Results Modal -->
{#if showScanModal}
  <div class="modal-backdrop" on:click={() => showScanModal = false}>
    <div class="modal modal-wide" on:click|stopPropagation>
      <h2>Detected Drives</h2>
      {#if scannedDrives.length === 0}
        <p>No tape drives detected. Make sure the drive is connected and the device is passed through to the container.</p>
      {:else}
        <table>
          <thead>
            <tr>
              <th>Device</th>
              <th>Vendor</th>
              <th>Model</th>
              <th>Status</th>
              <th>Action</th>
            </tr>
          </thead>
          <tbody>
            {#each scannedDrives as scanned}
              <tr>
                <td><code>{scanned.device_path}</code></td>
                <td>{scanned.vendor || '-'}</td>
                <td>{scanned.model || '-'}</td>
                <td><span class="badge badge-success">{scanned.status}</span></td>
                <td>
                  {#if isDriveAlreadyAdded(scanned.device_path)}
                    <span class="text-muted">Already added</span>
                  {:else}
                    <button class="btn btn-primary btn-sm" on:click={() => addScannedDrive(scanned)}>Add</button>
                  {/if}
                </td>
              </tr>
            {/each}
          </tbody>
        </table>
      {/if}
      <div class="form-actions">
        <button class="btn btn-secondary" on:click={() => showScanModal = false}>Close</button>
      </div>
    </div>
  </div>
{/if}

<style>
  .header-actions {
    display: flex;
    gap: 0.75rem;
  }

  .alert {
    padding: 1rem;
    border-radius: 8px;
    margin-bottom: 1rem;
    display: flex;
    justify-content: space-between;
    align-items: center;
  }

  .alert-error {
    background: #fee;
    color: #c00;
    border: 1px solid #fcc;
  }

  .alert-success {
    background: #d4edda;
    color: #155724;
    border: 1px solid #c3e6cb;
  }

  .dismiss-btn {
    background: none;
    border: none;
    font-size: 1.2rem;
    cursor: pointer;
    color: inherit;
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

  .text-muted {
    color: #999;
    font-size: 0.8rem;
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

  .modal-wide {
    max-width: 700px;
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
