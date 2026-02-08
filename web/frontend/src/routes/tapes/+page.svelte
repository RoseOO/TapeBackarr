<script lang="ts">
  import { onMount } from 'svelte';
  import * as api from '$lib/api/client';

  interface Tape {
    id: number;
    uuid: string;
    barcode: string;
    label: string;
    pool_id: number | null;
    pool_name: string | null;
    lto_type: string;
    status: string;
    capacity_bytes: number;
    used_bytes: number;
    write_count: number;
    last_written_at: string | null;
    labeled_at: string | null;
    created_at: string;
  }

  interface Pool {
    id: number;
    name: string;
  }

  interface Drive {
    id: number;
    device_path: string;
    display_name: string;
    status: string;
    enabled: boolean;
  }

  let tapes: Tape[] = [];
  let pools: Pool[] = [];
  let drives: Drive[] = [];
  let ltoTypes: Record<string, number> = {};
  let loading = true;
  let error = '';
  let successMsg = '';
  let showCreateModal = false;
  let showEditModal = false;
  let showFormatModal = false;
  let showExportModal = false;
  let showLabelModal = false;
  let selectedTape: Tape | null = null;

  // Form data
  let formData = {
    barcode: '',
    label: '',
    pool_id: null as number | null,
    lto_type: '' as string,
    capacity_bytes: 12000000000000,
    drive_id: null as number | null,
    write_label: false,
    auto_eject: false,
  };

  let formatDriveId: number | null = null;
  let exportLocation = '';
  let labelDriveId: number | null = null;
  let labelForce = false;
  let labelAutoEject = false;
  let newLabel = '';

  $: if (formData.lto_type && ltoTypes[formData.lto_type]) {
    formData.capacity_bytes = ltoTypes[formData.lto_type];
  }

  onMount(async () => {
    await loadData();
  });

  async function loadData() {
    try {
      const [tapesData, poolsData, drivesData, ltoTypesData] = await Promise.all([
        api.getTapes(),
        api.getPools(),
        api.getDrives(),
        api.getLTOTypes()
      ]);
      tapes = tapesData;
      pools = poolsData;
      drives = drivesData.filter((d: Drive) => d.enabled);
      ltoTypes = ltoTypesData;
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to load data';
    } finally {
      loading = false;
    }
  }

  function showSuccess(msg: string) {
    successMsg = msg;
    setTimeout(() => successMsg = '', 3000);
  }

  async function handleCreate() {
    try {
      error = '';
      if (!formData.label) {
        error = 'Label is required';
        return;
      }
      if (!formData.pool_id) {
        error = 'Pool is required - tapes must belong to a pool';
        return;
      }
      await api.createTape({
        ...formData,
        lto_type: formData.lto_type,
        pool_id: formData.pool_id ?? undefined,
        drive_id: formData.drive_id ?? undefined,
        write_label: formData.write_label,
        auto_eject: formData.auto_eject,
      } as any);
      showCreateModal = false;
      resetForm();
      showSuccess('Tape created successfully');
      await loadData();
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to create tape';
    }
  }

  async function handleUpdate() {
    if (!selectedTape) return;
    try {
      error = '';
      await api.updateTape(selectedTape.id, {
        label: formData.label,
        pool_id: formData.pool_id ?? undefined,
        status: selectedTape.status,
      });
      showEditModal = false;
      showSuccess('Tape updated');
      await loadData();
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to update tape';
    }
  }

  async function handleDelete(tape: Tape) {
    if (!confirm(`Delete tape ${tape.label}? This cannot be undone.`)) return;
    try {
      error = '';
      await api.deleteTape(tape.id);
      showSuccess('Tape deleted');
      await loadData();
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to delete tape';
    }
  }

  async function handleStatusChange(tape: Tape, status: string) {
    try {
      error = '';
      await api.updateTape(tape.id, { status });
      showSuccess(`Tape status changed to ${status}`);
      await loadData();
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to update status';
    }
  }

  async function handleFormat() {
    if (!selectedTape || !formatDriveId) return;
    try {
      error = '';
      await api.formatTape(selectedTape.id, formatDriveId, true);
      showFormatModal = false;
      showSuccess('Tape formatted successfully');
      await loadData();
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to format tape';
    }
  }

  async function handleExport() {
    if (!selectedTape) return;
    try {
      error = '';
      await api.exportTape(selectedTape.id, exportLocation);
      showExportModal = false;
      showSuccess('Tape exported');
      await loadData();
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to export tape';
    }
  }

  async function handleImport(tape: Tape) {
    try {
      error = '';
      await api.importTape(tape.id);
      showSuccess('Tape imported');
      await loadData();
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to import tape';
    }
  }

  async function handleLabel() {
    if (!selectedTape || !newLabel) return;
    try {
      error = '';
      await api.labelTape(selectedTape.id, newLabel, labelDriveId ?? undefined, labelForce, labelAutoEject);
      showLabelModal = false;
      showSuccess('Tape labeled');
      await loadData();
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to label tape';
    }
  }

  function openEditModal(tape: Tape) {
    selectedTape = tape;
    formData = {
      barcode: tape.barcode,
      label: tape.label,
      pool_id: tape.pool_id,
      lto_type: tape.lto_type || '',
      capacity_bytes: tape.capacity_bytes,
      drive_id: null,
      write_label: false,
      auto_eject: false,
    };
    showEditModal = true;
  }

  function openFormatModal(tape: Tape) {
    selectedTape = tape;
    formatDriveId = drives.length > 0 ? drives[0].id : null;
    showFormatModal = true;
  }

  function openExportModal(tape: Tape) {
    selectedTape = tape;
    exportLocation = '';
    showExportModal = true;
  }

  function openLabelModal(tape: Tape) {
    selectedTape = tape;
    newLabel = tape.label;
    labelDriveId = drives.length > 0 ? drives[0].id : null;
    labelForce = false;
    labelAutoEject = false;
    showLabelModal = true;
  }

  function resetForm() {
    formData = {
      barcode: '',
      label: '',
      pool_id: null,
      lto_type: '',
      capacity_bytes: 12000000000000,
      drive_id: null,
      write_label: false,
      auto_eject: false,
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
      case 'expired': return 'badge-warning';
      case 'retired': return 'badge-danger';
      case 'exported': return 'badge-info';
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
  <div class="card">
    <table>
      <thead>
        <tr>
          <th>Label</th>
          <th>Type</th>
          <th>UUID</th>
          <th>Barcode</th>
          <th>Pool</th>
          <th>Status</th>
          <th>Usage</th>
          <th>Writes</th>
          <th>Labeled</th>
          <th>Actions</th>
        </tr>
      </thead>
      <tbody>
        {#each tapes as tape}
          <tr>
            <td><strong>{tape.label}</strong></td>
            <td>{tape.lto_type || '-'}</td>
            <td class="uuid-cell" title={tape.uuid || ''}>{tape.uuid && tape.uuid.length >= 8 ? tape.uuid.substring(0, 8) + '...' : (tape.uuid || '-')}</td>
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
            <td>{tape.labeled_at ? new Date(tape.labeled_at).toLocaleDateString() : 'No'}</td>
            <td>
              <div class="actions">
                <button class="btn btn-secondary btn-sm" on:click={() => openEditModal(tape)}>Edit</button>
                <button class="btn btn-secondary btn-sm" on:click={() => openLabelModal(tape)}>Label</button>
                {#if tape.status !== 'exported'}
                  <button class="btn btn-secondary btn-sm" on:click={() => openFormatModal(tape)}>Format</button>
                  <button class="btn btn-secondary btn-sm" on:click={() => openExportModal(tape)}>Export</button>
                {:else}
                  <button class="btn btn-primary btn-sm" on:click={() => handleImport(tape)}>Import</button>
                {/if}
                <select class="status-select" on:change={(e) => { handleStatusChange(tape, (e.target as HTMLSelectElement).value); }} value={tape.status}>
                  <option value="blank">Blank</option>
                  <option value="active">Active</option>
                  <option value="full">Full</option>
                  <option value="expired">Expired</option>
                  <option value="retired">Retired</option>
                  <option value="exported">Exported</option>
                </select>
                <button class="btn btn-danger btn-sm" on:click={() => handleDelete(tape)}>Delete</button>
              </div>
            </td>
          </tr>
        {/each}
        {#if tapes.length === 0}
          <tr>
            <td colspan="10" class="no-data">No tapes found. Add a tape to get started.</td>
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
      <p class="modal-desc">Labels are mandatory before a tape can be used. Assign to a pool for lifecycle management.</p>
      <form on:submit|preventDefault={handleCreate}>
        <div class="form-group">
          <label for="label">Label <span class="required">*</span></label>
          <input type="text" id="label" bind:value={formData.label} required placeholder="e.g., NAS-OFF-007" />
          <small>Human-readable label for physical identification</small>
        </div>
        <div class="form-group">
          <label for="barcode">Barcode</label>
          <input type="text" id="barcode" bind:value={formData.barcode} placeholder="e.g., ABC123L8" />
        </div>
        <div class="form-group">
          <label for="pool">Pool <span class="required">*</span></label>
          <select id="pool" bind:value={formData.pool_id} required>
            <option value={null} disabled>Select a pool...</option>
            {#each pools as pool}
              <option value={pool.id}>{pool.name}</option>
            {/each}
          </select>
          <small>A tape must belong to exactly one pool</small>
        </div>
        <div class="form-group">
          <label for="lto_type">LTO Type</label>
          <select id="lto_type" bind:value={formData.lto_type}>
            <option value="">Select LTO type...</option>
            {#each Object.entries(ltoTypes).sort((a, b) => a[1] - b[1]) as [type, capacity]}
              <option value={type}>{type} ({formatBytes(capacity)})</option>
            {/each}
          </select>
          <small>Capacity is automatically set based on LTO generation</small>
        </div>
        <div class="form-group">
          <label for="capacity">Capacity</label>
          <input type="text" id="capacity" value={formatBytes(formData.capacity_bytes)} disabled />
          <small>Set automatically from LTO type</small>
        </div>
        {#if drives.length > 0}
          <div class="form-group">
            <label for="drive">Drive (for labeling)</label>
            <select id="drive" bind:value={formData.drive_id}>
              <option value={null}>No drive (software-only)</option>
              {#each drives as drive}
                <option value={drive.id}>{drive.display_name || drive.device_path}</option>
              {/each}
            </select>
          </div>
          {#if formData.drive_id}
            <div class="form-group checkbox-group">
              <label>
                <input type="checkbox" bind:checked={formData.write_label} />
                Write label to physical tape
              </label>
              <small>Writes label data to the tape in the selected drive</small>
            </div>
            {#if formData.write_label}
              <div class="form-group checkbox-group">
                <label>
                  <input type="checkbox" bind:checked={formData.auto_eject} />
                  Auto-eject tape after labeling
                </label>
                <small>Automatically ejects the tape from the drive after writing the label</small>
              </div>
            {/if}
          {/if}
        {/if}
        <div class="modal-actions">
          <button type="button" class="btn btn-secondary" on:click={() => showCreateModal = false}>Cancel</button>
          <button type="submit" class="btn btn-primary">Create Tape</button>
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
      <p class="modal-desc">UUID: {selectedTape.uuid || 'N/A'}</p>
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

<!-- Format Modal -->
{#if showFormatModal && selectedTape}
  <div class="modal-overlay" on:click={() => showFormatModal = false}>
    <div class="modal" on:click|stopPropagation>
      <h2>⚠️ Format Tape</h2>
      <p class="modal-desc warning-text">This will erase ALL data on tape <strong>{selectedTape.label}</strong> including the label. This action cannot be undone.</p>
      <div class="form-group">
        <label for="format-drive">Select Drive</label>
        <select id="format-drive" bind:value={formatDriveId}>
          {#each drives as drive}
            <option value={drive.id}>{drive.display_name || drive.device_path}</option>
          {/each}
        </select>
        <small>The tape must be loaded in the selected drive</small>
      </div>
      <div class="modal-actions">
        <button class="btn btn-secondary" on:click={() => showFormatModal = false}>Cancel</button>
        <button class="btn btn-danger" on:click={handleFormat}>Format Tape</button>
      </div>
    </div>
  </div>
{/if}

<!-- Export Modal -->
{#if showExportModal && selectedTape}
  <div class="modal-overlay" on:click={() => showExportModal = false}>
    <div class="modal" on:click|stopPropagation>
      <h2>Export Tape</h2>
      <p class="modal-desc">Mark tape <strong>{selectedTape.label}</strong> as exported/offsite. The tape will be locked against reuse but its pool membership and catalog data will be preserved.</p>
      <div class="form-group">
        <label for="offsite-location">Offsite Location</label>
        <input type="text" id="offsite-location" bind:value={exportLocation} placeholder="e.g., Iron Mountain Vault #3" />
        <small>Where the tape is being sent</small>
      </div>
      <div class="modal-actions">
        <button class="btn btn-secondary" on:click={() => showExportModal = false}>Cancel</button>
        <button class="btn btn-primary" on:click={handleExport}>Export</button>
      </div>
    </div>
  </div>
{/if}

<!-- Label Modal -->
{#if showLabelModal && selectedTape}
  <div class="modal-overlay" on:click={() => showLabelModal = false}>
    <div class="modal" on:click|stopPropagation>
      <h2>Write Tape Label</h2>
      <p class="modal-desc">Write label data to the physical tape in the drive. UUID: {selectedTape.uuid || 'N/A'}</p>
      <form on:submit|preventDefault={handleLabel}>
        <div class="form-group">
          <label for="new-label">Label</label>
          <input type="text" id="new-label" bind:value={newLabel} required />
        </div>
        {#if drives.length > 0}
          <div class="form-group">
            <label for="label-drive">Drive</label>
            <select id="label-drive" bind:value={labelDriveId}>
              {#each drives as drive}
                <option value={drive.id}>{drive.display_name || drive.device_path}</option>
              {/each}
            </select>
            <small>The tape must be loaded in the selected drive</small>
          </div>
        {/if}
        <div class="form-group checkbox-group">
          <label>
            <input type="checkbox" bind:checked={labelForce} />
            Force overwrite existing label
          </label>
          <small>If the tape already has a label, overwrite it (tape data is not erased)</small>
        </div>
        <div class="form-group checkbox-group">
          <label>
            <input type="checkbox" bind:checked={labelAutoEject} />
            Auto-eject tape after labeling
          </label>
        </div>
        <div class="modal-actions">
          <button type="button" class="btn btn-secondary" on:click={() => showLabelModal = false}>Cancel</button>
          <button type="submit" class="btn btn-primary">Write Label</button>
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

  .uuid-cell {
    font-family: monospace;
    font-size: 0.75rem;
    color: #888;
  }

  .actions {
    display: flex;
    gap: 0.35rem;
    align-items: center;
    flex-wrap: wrap;
  }

  .status-select {
    width: auto;
    padding: 0.25rem 0.5rem;
    font-size: 0.75rem;
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

  .warning-text {
    color: #856404;
    background: #fff3cd;
    padding: 0.75rem;
    border-radius: 6px;
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
