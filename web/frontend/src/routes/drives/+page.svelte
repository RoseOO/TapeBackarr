<script lang="ts">
  import { onMount } from 'svelte';
  import * as api from '$lib/api/client';

  interface Drive {
    id: number;
    device_path: string;
    display_name: string;
    vendor: string;
    serial_number: string;
    model: string;
    status: string;
    current_tape_id: number | null;
    current_tape: string;
    enabled: boolean;
    created_at: string;
    unknown_tape?: {
      label: string;
      uuid: string;
      pool: string;
      timestamp: number;
    } | null;
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
  let showFormatDriveModal = false;
  let formatDriveTarget: Drive | null = null;
  let formatConfirmChecked = false;
  let showAddUnknownTapeModal = false;
  let unknownTapeTarget: { drive: Drive; tape: Drive['unknown_tape'] } | null = null;
  let addTapeFormData = {
    label: '',
    barcode: '',
    pool_id: null as number | null,
    lto_type: '',
  };
  let pools: any[] = [];
  let ltoTypes: Record<string, number> = {};
  let newDrive = {
    device_path: '',
    display_name: '',
    serial_number: '',
    model: ''
  };

  let showBatchLabelModal = false;
  let batchLabelDriveId = 0;
  let batchLabelForm = { prefix: '', start_number: 1, count: 10, digits: 3, pool_id: undefined as number | undefined };
  let batchLabelRunning = false;

  onMount(async () => {
    await loadDrives();
    try {
      const [poolsData, ltoTypesData] = await Promise.all([
        api.getPools(),
        api.getLTOTypes()
      ]);
      pools = poolsData;
      ltoTypes = ltoTypesData;
    } catch (e) {
      // Non-critical
    }
  });

  async function loadDrives() {
    loading = true;
    error = '';
    try {
      const result = await api.getDrives();
      drives = Array.isArray(result) ? result : [];
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
        vendor: scanned.vendor || '',
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

  function formatBytes(bytes: number): string {
    if (bytes === 0) return '0 B';
    const k = 1024;
    const sizes = ['B', 'KB', 'MB', 'GB', 'TB', 'PB'];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
  }

  async function formatTapeInDrive() {
    if (!formatDriveTarget || !formatConfirmChecked) return;
    try {
      error = '';
      await api.formatTapeInDrive(formatDriveTarget.id, true);
      showFormatDriveModal = false;
      formatDriveTarget = null;
      formatConfirmChecked = false;
      showSuccessMsg('Tape formatted successfully');
      await loadDrives();
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to format tape';
    }
  }

  function openFormatDriveModal(drive: Drive) {
    formatDriveTarget = drive;
    formatConfirmChecked = false;
    showFormatDriveModal = true;
  }

  async function openAddUnknownTapeModal(drive: Drive) {
    if (!drive.unknown_tape) return;
    unknownTapeTarget = { drive, tape: drive.unknown_tape };
    addTapeFormData = {
      label: drive.unknown_tape.label,
      barcode: '',
      pool_id: null,
      lto_type: '',
    };
    showAddUnknownTapeModal = true;
  }

  async function addUnknownTapeToLibrary() {
    if (!unknownTapeTarget?.tape) return;
    try {
      error = '';
      const capacity = ltoTypes[addTapeFormData.lto_type] || 0;
      await api.createTape({
        barcode: addTapeFormData.barcode,
        label: unknownTapeTarget.tape.label,
        pool_id: addTapeFormData.pool_id ?? undefined,
        lto_type: addTapeFormData.lto_type,
        capacity_bytes: capacity,
      } as any);
      showAddUnknownTapeModal = false;
      showSuccessMsg('Tape added to library');
      await loadDrives();
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to add tape to library';
    }
  }

  function openBatchLabelModal(driveId: number) {
    batchLabelDriveId = driveId;
    batchLabelForm = { prefix: '', start_number: 1, count: 10, digits: 3, pool_id: undefined };
    showBatchLabelModal = true;
  }

  async function handleBatchLabel() {
    batchLabelRunning = true;
    try {
      error = '';
      await api.batchLabelTapes(batchLabelDriveId, batchLabelForm);
      showBatchLabelModal = false;
      showSuccessMsg('Batch label started successfully');
      await loadDrives();
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to start batch label';
    } finally {
      batchLabelRunning = false;
    }
  }

  function batchLabelPreview(prefix: string, start: number, count: number, digits: number): string {
    const first = prefix + String(start).padStart(digits, '0');
    const last = prefix + String(start + count - 1).padStart(digits, '0');
    return `${first} through ${last}`;
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
          <th>Vendor</th>
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
            <td>{drive.vendor || '-'}</td>
            <td>{drive.model || '-'}</td>
            <td><span class="badge {getStatusBadge(drive.status)}">{drive.status}</span></td>
            <td>{drive.enabled ? '✅' : '❌'}</td>
            <td>{drive.current_tape || (drive.current_tape_id ? `Tape #${drive.current_tape_id}` : 'No tape')}</td>
            <td>
              <button class="btn btn-secondary btn-sm" on:click={() => selectDrive(drive.id)}>Select</button>
              <button class="btn btn-secondary btn-sm" on:click={() => rewindTape(drive.id)}>Rewind</button>
              <button class="btn btn-secondary btn-sm" on:click={() => ejectTape(drive.id)}>Eject</button>
              <a href="/inspect" class="btn btn-secondary btn-sm">🔍 Inspect</a>
              <button class="btn btn-secondary btn-sm" on:click={() => openBatchLabelModal(drive.id)}>📦 Batch Label</button>
              <button class="btn btn-warning btn-sm" on:click={() => openFormatDriveModal(drive)} disabled={drive.status === 'offline'}>Format</button>
              <button class="btn btn-danger btn-sm" on:click={() => deleteDrive(drive.id)}>Remove</button>
            </td>
          </tr>
        {:else}
          <tr>
            <td colspan="8">No drives configured. Use "Scan Drives" to detect drives or add one manually.</td>
          </tr>
        {/each}
      </tbody>
    </table>
    {#each drives.filter(d => d.unknown_tape) as drive}
      <div class="unknown-tape-warning">
        <div class="warning-icon">⚠️</div>
        <div class="warning-content">
          <strong>Unknown tape detected in {drive.display_name || drive.device_path}</strong>
          <p>Tape "<strong>{drive.unknown_tape?.label}</strong>" (UUID: {drive.unknown_tape?.uuid || 'N/A'}) is loaded but not in the tape library.</p>
          <div class="warning-actions">
            <button class="btn btn-primary btn-sm" on:click={() => openAddUnknownTapeModal(drive)}>Add to Library</button>
            <button class="btn btn-warning btn-sm" on:click={() => openFormatDriveModal(drive)}>Format Tape</button>
          </div>
        </div>
      </div>
    {/each}
  </div>
{/if}

<!-- Add Drive Modal -->
{#if showAddModal}
  <div class="modal-backdrop" on:click={() => showAddModal = false}>
    <div class="modal" on:click|stopPropagation={() => {}}>
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
    <div class="modal modal-wide" on:click|stopPropagation={() => {}}>
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

<!-- Format Tape in Drive Modal -->
{#if showFormatDriveModal && formatDriveTarget}
  <div class="modal-backdrop" on:click={() => showFormatDriveModal = false}>
    <div class="modal" on:click|stopPropagation={() => {}}>
      <h2>⚠️ Format Tape</h2>
      <div class="format-warning">
        <p><strong>WARNING: This will ERASE ALL DATA on the tape currently loaded in {formatDriveTarget.display_name || formatDriveTarget.device_path}.</strong></p>
        {#if formatDriveTarget.current_tape}
          <p>Current tape: <strong>{formatDriveTarget.current_tape}</strong></p>
        {/if}
        {#if formatDriveTarget.unknown_tape}
          <p>Tape label: <strong>{formatDriveTarget.unknown_tape.label}</strong><br/>
          UUID: <code>{formatDriveTarget.unknown_tape.uuid || 'N/A'}</code></p>
        {/if}
        <p class="danger-text">This action is <strong>irreversible</strong>. All data on the tape will be permanently destroyed.</p>
      </div>
      <div class="form-group checkbox-group">
        <label class="confirm-label">
          <input type="checkbox" bind:checked={formatConfirmChecked} />
          <span>I understand this will permanently erase all data on this tape</span>
        </label>
      </div>
      <div class="form-actions">
        <button class="btn btn-secondary" on:click={() => showFormatDriveModal = false}>Cancel</button>
        <button class="btn btn-danger" on:click={formatTapeInDrive} disabled={!formatConfirmChecked}>
          🗑️ Format Tape
        </button>
      </div>
    </div>
  </div>
{/if}

<!-- Add Unknown Tape to Library Modal -->
{#if showAddUnknownTapeModal && unknownTapeTarget}
  <div class="modal-backdrop" on:click={() => showAddUnknownTapeModal = false}>
    <div class="modal" on:click|stopPropagation={() => {}}>
      <h2>Add Tape to Library</h2>
      <p class="modal-desc">This tape was found loaded in a drive but is not in the database. Add it to track it in the tape library.</p>
      <div class="tape-info-box">
        <div><strong>Label:</strong> {unknownTapeTarget.tape?.label}</div>
        <div><strong>UUID:</strong> <code>{unknownTapeTarget.tape?.uuid || 'N/A'}</code></div>
        {#if unknownTapeTarget.tape?.pool}
          <div><strong>Pool (from tape):</strong> {unknownTapeTarget.tape.pool}</div>
        {/if}
      </div>
      <form on:submit|preventDefault={addUnknownTapeToLibrary}>
        <div class="form-group">
          <label for="ut-barcode">Barcode</label>
          <input type="text" id="ut-barcode" bind:value={addTapeFormData.barcode} placeholder="Optional" />
        </div>
        <div class="form-group">
          <label for="ut-pool">Pool</label>
          <select id="ut-pool" bind:value={addTapeFormData.pool_id}>
            <option value={null}>Select a pool...</option>
            {#each pools as pool}
              <option value={pool.id}>{pool.name}</option>
            {/each}
          </select>
        </div>
        <div class="form-group">
          <label for="ut-lto">LTO Type</label>
          <select id="ut-lto" bind:value={addTapeFormData.lto_type}>
            <option value="">Select LTO type...</option>
            {#each Object.entries(ltoTypes).sort((a, b) => a[1] - b[1]) as [type, capacity]}
              <option value={type}>{type} ({formatBytes(capacity)})</option>
            {/each}
          </select>
        </div>
        <div class="form-actions">
          <button type="button" class="btn btn-secondary" on:click={() => showAddUnknownTapeModal = false}>Cancel</button>
          <button type="submit" class="btn btn-primary">Add to Library</button>
        </div>
      </form>
    </div>
  </div>
{/if}

<!-- Batch Label Modal -->
{#if showBatchLabelModal}
  <div class="modal-backdrop" on:click={() => showBatchLabelModal = false}>
    <div class="modal" on:click|stopPropagation={() => {}}>
      <h2>📦 Batch Label Tapes</h2>
      <p class="modal-desc">Automatically label multiple tapes in sequence. Insert a blank tape, and the system will detect, label, eject, and repeat.</p>
      <form on:submit|preventDefault={handleBatchLabel}>
        <div class="form-group">
          <label for="bl-prefix">Label Prefix</label>
          <input type="text" id="bl-prefix" bind:value={batchLabelForm.prefix} placeholder="e.g., NAS-OFF-" required />
        </div>
        <div class="form-group">
          <label for="bl-start">Start Number</label>
          <input type="number" id="bl-start" bind:value={batchLabelForm.start_number} min="0" required />
        </div>
        <div class="form-group">
          <label for="bl-count">Count</label>
          <input type="number" id="bl-count" bind:value={batchLabelForm.count} min="1" max="100" required />
        </div>
        <div class="form-group">
          <label for="bl-digits">Digits</label>
          <select id="bl-digits" bind:value={batchLabelForm.digits}>
            <option value={2}>2 (01-99)</option>
            <option value={3}>3 (001-999)</option>
            <option value={4}>4 (0001-9999)</option>
            <option value={5}>5 (00001-99999)</option>
            <option value={6}>6 (000001-999999)</option>
          </select>
        </div>
        <div class="form-group">
          <label for="bl-pool">Pool (optional)</label>
          <select id="bl-pool" bind:value={batchLabelForm.pool_id}>
            <option value={undefined}>No pool</option>
            {#each pools as pool}
              <option value={pool.id}>{pool.name}</option>
            {/each}
          </select>
        </div>
        {#if batchLabelForm.prefix && batchLabelForm.count > 0}
          <div class="tape-info-box">
            <strong>Preview:</strong> {batchLabelPreview(batchLabelForm.prefix, batchLabelForm.start_number, batchLabelForm.count, batchLabelForm.digits)}
          </div>
        {/if}
        <div class="form-actions">
          <button type="button" class="btn btn-secondary" on:click={() => showBatchLabelModal = false}>Cancel</button>
          <button type="submit" class="btn btn-primary" disabled={batchLabelRunning}>
            {batchLabelRunning ? 'Running...' : 'Start Batch Label'}
          </button>
        </div>
      </form>
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
    background: var(--bg-input);
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
    background: var(--bg-card);
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

  .unknown-tape-warning {
    display: flex;
    gap: 1rem;
    padding: 1rem;
    margin-top: 1rem;
    background: #fff3cd;
    border: 1px solid #ffc107;
    border-radius: 8px;
    align-items: flex-start;
  }

  .warning-icon {
    font-size: 1.5rem;
    flex-shrink: 0;
  }

  .warning-content {
    flex: 1;
  }

  .warning-content p {
    margin: 0.25rem 0;
    font-size: 0.875rem;
    color: #856404;
  }

  .warning-actions {
    display: flex;
    gap: 0.5rem;
    margin-top: 0.5rem;
  }

  .format-warning {
    background: #f8d7da;
    border: 1px solid #f5c6cb;
    border-radius: 8px;
    padding: 1rem;
    margin-bottom: 1rem;
  }

  .format-warning p {
    margin: 0.5rem 0;
    font-size: 0.875rem;
  }

  .danger-text {
    color: #721c24;
    font-weight: 500;
  }

  .confirm-label {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    cursor: pointer;
    font-size: 0.875rem;
    font-weight: 500;
  }

  .confirm-label input[type="checkbox"] {
    width: auto;
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

  .tape-info-box {
    background: #e3f2fd;
    border: 1px solid #bbdefb;
    border-radius: 8px;
    padding: 0.75rem;
    margin-bottom: 1rem;
    font-size: 0.875rem;
  }

  .tape-info-box div {
    margin: 0.25rem 0;
  }

  .tape-info-box code {
    background: #ddd;
    padding: 0.15rem 0.35rem;
    border-radius: 3px;
    font-size: 0.8rem;
  }

  .modal-desc {
    color: #666;
    font-size: 0.875rem;
    margin: 0.25rem 0 1rem;
  }

  .btn-warning {
    background: #ffc107;
    color: #212529;
  }

  .btn-warning:hover {
    background: #e0a800;
  }

  .file-list-scroll {
    max-height: 400px;
    overflow-y: auto;
    margin-top: 0.5rem;
    border: 1px solid #eee;
    border-radius: 8px;
  }

  .file-list-scroll table {
    margin: 0;
  }
</style>
