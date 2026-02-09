<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import * as api from '$lib/api/client';
  import { dataVersion } from '$lib/stores/livedata';

  interface Library {
    id: number;
    name: string;
    device_path: string;
    vendor: string;
    model: string;
    serial_number: string;
    num_slots: number;
    num_drives: number;
    num_import_export: number;
    barcode_reader: boolean;
    enabled: boolean;
    last_inventory_at: string | null;
    created_at: string;
  }

  interface Slot {
    id: number;
    slot_number: number;
    slot_type: string;
    tape_id: number | null;
    tape_label: string | null;
    barcode: string;
    is_empty: boolean;
    drive_id: number | null;
  }

  interface ScannedChanger {
    device_path: string;
    vendor?: string;
    model?: string;
    type: string;
  }

  let libraries: Library[] = [];
  let selectedLibrary: Library | null = null;
  let slots: Slot[] = [];
  let loading = true;
  let error = '';
  let successMsg = '';
  let showAddModal = false;
  let showScanModal = false;
  let scannedChangers: ScannedChanger[] = [];
  let scanning = false;
  let inventoryRunning = false;
  let showLoadModal = false;
  let loadForm = { slot_number: 0, drive_number: 0 };
  let showUnloadModal = false;
  let unloadForm = { slot_number: 0, drive_number: 0 };
  let showTransferModal = false;
  let transferForm = { source_slot: 0, dest_slot: 0 };

  let newLibrary = {
    name: '',
    device_path: '',
    vendor: '',
    model: '',
  };

  const tapesVersion = dataVersion('tapes');
  let lastVersion = 0;
  const unsubVersion = tapesVersion.subscribe(v => {
    if (v > lastVersion && lastVersion > 0 && selectedLibrary) {
      loadSlots(selectedLibrary.id);
    }
    lastVersion = v;
  });

  onMount(async () => {
    await loadLibraries();
  });

  onDestroy(() => {
    unsubVersion();
  });

  function showSuccess(msg: string) {
    successMsg = msg;
    setTimeout(() => successMsg = '', 3000);
  }

  async function loadLibraries() {
    loading = true;
    error = '';
    try {
      libraries = await api.getLibraries();
      if (!Array.isArray(libraries)) libraries = [];
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to load libraries';
      libraries = [];
    } finally {
      loading = false;
    }
  }

  async function loadSlots(libraryId: number) {
    try {
      slots = await api.getLibrarySlots(libraryId);
      if (!Array.isArray(slots)) slots = [];
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to load slots';
    }
  }

  async function selectLibrary(lib: Library) {
    selectedLibrary = lib;
    await loadSlots(lib.id);
  }

  async function addLibrary() {
    try {
      error = '';
      await api.createLibrary(newLibrary);
      showAddModal = false;
      newLibrary = { name: '', device_path: '', vendor: '', model: '' };
      showSuccess('Library added');
      await loadLibraries();
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to add library';
    }
  }

  async function addScannedChanger(changer: ScannedChanger) {
    try {
      error = '';
      await api.createLibrary({
        name: changer.model ? `${changer.vendor || ''} ${changer.model}`.trim() : changer.device_path,
        device_path: changer.device_path,
        vendor: changer.vendor || '',
        model: changer.model || '',
      });
      showScanModal = false;
      showSuccess('Library added from scan');
      await loadLibraries();
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to add library';
    }
  }

  async function scanForChangers() {
    scanning = true;
    try {
      error = '';
      scannedChangers = await api.scanLibraries();
      if (!Array.isArray(scannedChangers)) scannedChangers = [];
      showScanModal = true;
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to scan for changers';
    } finally {
      scanning = false;
    }
  }

  async function deleteLibrary(id: number) {
    if (!confirm('Are you sure you want to remove this tape library?')) return;
    try {
      error = '';
      await api.deleteLibrary(id);
      if (selectedLibrary?.id === id) {
        selectedLibrary = null;
        slots = [];
      }
      showSuccess('Library removed');
      await loadLibraries();
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to delete library';
    }
  }

  async function runInventory(lib: Library) {
    inventoryRunning = true;
    try {
      error = '';
      await api.libraryInventory(lib.id);
      showSuccess('Inventory completed');
      await loadLibraries();
      if (selectedLibrary?.id === lib.id) {
        await loadSlots(lib.id);
      }
    } catch (e) {
      error = e instanceof Error ? e.message : 'Inventory failed';
    } finally {
      inventoryRunning = false;
    }
  }

  async function handleLoad() {
    if (!selectedLibrary) return;
    try {
      error = '';
      await api.libraryLoad(selectedLibrary.id, loadForm.slot_number, loadForm.drive_number);
      showLoadModal = false;
      showSuccess(`Tape loaded from slot ${loadForm.slot_number} to drive ${loadForm.drive_number}`);
      await loadSlots(selectedLibrary.id);
    } catch (e) {
      error = e instanceof Error ? e.message : 'Load failed';
    }
  }

  async function handleUnload() {
    if (!selectedLibrary) return;
    try {
      error = '';
      await api.libraryUnload(selectedLibrary.id, unloadForm.slot_number, unloadForm.drive_number);
      showUnloadModal = false;
      showSuccess(`Tape unloaded from drive ${unloadForm.drive_number} to slot ${unloadForm.slot_number}`);
      await loadSlots(selectedLibrary.id);
    } catch (e) {
      error = e instanceof Error ? e.message : 'Unload failed';
    }
  }

  async function handleTransfer() {
    if (!selectedLibrary) return;
    try {
      error = '';
      await api.libraryTransfer(selectedLibrary.id, transferForm.source_slot, transferForm.dest_slot);
      showTransferModal = false;
      showSuccess(`Tape transferred from slot ${transferForm.source_slot} to slot ${transferForm.dest_slot}`);
      await loadSlots(selectedLibrary.id);
    } catch (e) {
      error = e instanceof Error ? e.message : 'Transfer failed';
    }
  }

  function formatDate(d: string | null): string {
    if (!d) return 'Never';
    return new Date(d).toLocaleString();
  }

  function getSlotIcon(slotType: string): string {
    switch (slotType) {
      case 'drive': return '🔌';
      case 'import_export': return '📬';
      default: return '📦';
    }
  }

  function isLibraryAlreadyAdded(devicePath: string): boolean {
    return libraries.some(l => l.device_path === devicePath);
  }
</script>

<div class="page-header">
  <h1>🏗️ Tape Libraries</h1>
  <div class="header-actions">
    <button class="btn btn-secondary" on:click={scanForChangers} disabled={scanning}>
      {scanning ? 'Scanning...' : '🔍 Scan Changers'}
    </button>
    <button class="btn btn-primary" on:click={() => showAddModal = true}>Add Library</button>
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
{:else if libraries.length === 0}
  <div class="card">
    <h2>No Tape Libraries Configured</h2>
    <p>Tape libraries (autochangers) allow automated tape management with robotic slot loading/unloading.</p>
    <p>Use "Scan Changers" to detect SCSI medium changer devices, or add one manually.</p>
    <p style="color: var(--text-muted); font-size: 0.875rem;">
      Requires <code>mtx</code> and <code>lsscsi</code> packages. The changer device is typically <code>/dev/sgX</code>.
    </p>
  </div>
{:else}
  <!-- Library cards -->
  <div class="library-grid">
    {#each libraries as lib}
      <div class="card library-card" class:selected={selectedLibrary?.id === lib.id} on:click={() => selectLibrary(lib)}>
        <div class="lib-header">
          <h3>🏗️ {lib.name}</h3>
          <span class="badge {lib.enabled ? 'badge-success' : 'badge-danger'}">
            {lib.enabled ? 'Enabled' : 'Disabled'}
          </span>
        </div>
        <div class="lib-meta">
          <div><strong>Device:</strong> <code>{lib.device_path}</code></div>
          {#if lib.vendor}<div><strong>Vendor:</strong> {lib.vendor}</div>{/if}
          {#if lib.model}<div><strong>Model:</strong> {lib.model}</div>{/if}
        </div>
        <div class="lib-stats">
          <span>📦 {lib.num_slots} slots</span>
          <span>🔌 {lib.num_drives} drives</span>
          {#if lib.num_import_export > 0}
            <span>📬 {lib.num_import_export} I/E</span>
          {/if}
          {#if lib.barcode_reader}
            <span class="badge badge-info">Barcode</span>
          {/if}
        </div>
        <div class="lib-footer">
          <span class="text-muted">Last inventory: {formatDate(lib.last_inventory_at)}</span>
          <div class="lib-actions">
            <button class="btn btn-secondary btn-sm" on:click|stopPropagation={() => runInventory(lib)} disabled={inventoryRunning}>
              {inventoryRunning ? '...' : '🔄 Inventory'}
            </button>
            <button class="btn btn-danger btn-sm" on:click|stopPropagation={() => deleteLibrary(lib.id)}>🗑️</button>
          </div>
        </div>
      </div>
    {/each}
  </div>

  <!-- Slot inventory for selected library -->
  {#if selectedLibrary}
    <div class="card">
      <div class="slot-header">
        <h2>{selectedLibrary.name} — Slot Inventory</h2>
        <div class="slot-actions">
          <button class="btn btn-primary btn-sm" on:click={() => { loadForm = { slot_number: 0, drive_number: 0 }; showLoadModal = true; }}>
            📥 Load Tape
          </button>
          <button class="btn btn-secondary btn-sm" on:click={() => { unloadForm = { slot_number: 0, drive_number: 0 }; showUnloadModal = true; }}>
            📤 Unload Tape
          </button>
          <button class="btn btn-secondary btn-sm" on:click={() => { transferForm = { source_slot: 0, dest_slot: 0 }; showTransferModal = true; }}>
            🔀 Transfer
          </button>
          <button class="btn btn-secondary btn-sm" on:click={() => { if (selectedLibrary) runInventory(selectedLibrary); }} disabled={inventoryRunning}>
            🔄 Refresh
          </button>
        </div>
      </div>

      {#if slots.length === 0}
        <p class="text-muted" style="text-align:center; padding: 2rem;">
          No slot data. Click "Inventory" to scan the library.
        </p>
      {:else}
        <!-- Drive slots -->
        {#if slots.filter(s => s.slot_type === 'drive').length > 0}
          <h3 class="slot-section-title">🔌 Drives</h3>
          <div class="slot-grid">
            {#each slots.filter(s => s.slot_type === 'drive') as slot}
              <div class="slot-card" class:empty={slot.is_empty} class:full={!slot.is_empty}>
                <div class="slot-num">Drive {slot.slot_number}</div>
                {#if slot.is_empty}
                  <div class="slot-content empty-slot">Empty</div>
                {:else}
                  <div class="slot-content">
                    {#if slot.tape_label}📼 {slot.tape_label}{:else if slot.barcode}🏷️ {slot.barcode}{:else}📼 Tape{/if}
                  </div>
                {/if}
              </div>
            {/each}
          </div>
        {/if}

        <!-- Storage slots -->
        {#if slots.filter(s => s.slot_type === 'storage').length > 0}
          <h3 class="slot-section-title">📦 Storage Slots</h3>
          <div class="slot-grid">
            {#each slots.filter(s => s.slot_type === 'storage') as slot}
              <div class="slot-card" class:empty={slot.is_empty} class:full={!slot.is_empty}>
                <div class="slot-num">Slot {slot.slot_number}</div>
                {#if slot.is_empty}
                  <div class="slot-content empty-slot">Empty</div>
                {:else}
                  <div class="slot-content">
                    {#if slot.tape_label}📼 {slot.tape_label}{:else if slot.barcode}🏷️ {slot.barcode}{:else}📼 Tape{/if}
                  </div>
                {/if}
              </div>
            {/each}
          </div>
        {/if}

        <!-- Import/Export slots -->
        {#if slots.filter(s => s.slot_type === 'import_export').length > 0}
          <h3 class="slot-section-title">📬 Import/Export Slots</h3>
          <div class="slot-grid">
            {#each slots.filter(s => s.slot_type === 'import_export') as slot}
              <div class="slot-card" class:empty={slot.is_empty} class:full={!slot.is_empty}>
                <div class="slot-num">I/E {slot.slot_number}</div>
                {#if slot.is_empty}
                  <div class="slot-content empty-slot">Empty</div>
                {:else}
                  <div class="slot-content">
                    {#if slot.tape_label}📼 {slot.tape_label}{:else if slot.barcode}🏷️ {slot.barcode}{:else}📼 Tape{/if}
                  </div>
                {/if}
              </div>
            {/each}
          </div>
        {/if}
      {/if}
    </div>
  {/if}
{/if}

<!-- Add Library Modal -->
{#if showAddModal}
  <div class="modal-backdrop" on:click={() => showAddModal = false}>
    <div class="modal" on:click|stopPropagation={() => {}}>
      <h2>Add Tape Library</h2>
      <form on:submit|preventDefault={addLibrary}>
        <div class="form-group">
          <label for="lib-name">Name</label>
          <input id="lib-name" type="text" bind:value={newLibrary.name} placeholder="e.g., Main Tape Library" required />
        </div>
        <div class="form-group">
          <label for="lib-device">Changer Device Path</label>
          <input id="lib-device" type="text" bind:value={newLibrary.device_path} placeholder="/dev/sg3" required />
          <small style="color: var(--text-muted)">SCSI generic device for the medium changer (use <code>lsscsi --generic</code> to find)</small>
        </div>
        <div class="form-group">
          <label for="lib-vendor">Vendor</label>
          <input id="lib-vendor" type="text" bind:value={newLibrary.vendor} placeholder="Optional" />
        </div>
        <div class="form-group">
          <label for="lib-model">Model</label>
          <input id="lib-model" type="text" bind:value={newLibrary.model} placeholder="Optional" />
        </div>
        <div class="form-actions">
          <button type="button" class="btn btn-secondary" on:click={() => showAddModal = false}>Cancel</button>
          <button type="submit" class="btn btn-primary">Add Library</button>
        </div>
      </form>
    </div>
  </div>
{/if}

<!-- Scan Results Modal -->
{#if showScanModal}
  <div class="modal-backdrop" on:click={() => showScanModal = false}>
    <div class="modal modal-wide" on:click|stopPropagation={() => {}}>
      <h2>Detected Medium Changers</h2>
      {#if scannedChangers.length === 0}
        <p>No SCSI medium changers detected. Make sure the library is connected and the device is passed through.</p>
      {:else}
        <table>
          <thead>
            <tr>
              <th>Device</th>
              <th>Vendor</th>
              <th>Model</th>
              <th>Action</th>
            </tr>
          </thead>
          <tbody>
            {#each scannedChangers as changer}
              <tr>
                <td><code>{changer.device_path}</code></td>
                <td>{changer.vendor || '-'}</td>
                <td>{changer.model || '-'}</td>
                <td>
                  {#if isLibraryAlreadyAdded(changer.device_path)}
                    <span class="text-muted">Already added</span>
                  {:else}
                    <button class="btn btn-primary btn-sm" on:click={() => addScannedChanger(changer)}>Add</button>
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

<!-- Load Tape Modal -->
{#if showLoadModal && selectedLibrary}
  <div class="modal-backdrop" on:click={() => showLoadModal = false}>
    <div class="modal" on:click|stopPropagation={() => {}}>
      <h2>📥 Load Tape into Drive</h2>
      <p class="modal-desc">Move a tape from a storage slot into a drive.</p>
      <form on:submit|preventDefault={handleLoad}>
        <div class="form-group">
          <label for="load-slot">Source Slot Number</label>
          <input type="number" id="load-slot" bind:value={loadForm.slot_number} min="1" required />
        </div>
        <div class="form-group">
          <label for="load-drive">Target Drive Number</label>
          <input type="number" id="load-drive" bind:value={loadForm.drive_number} min="0" required />
        </div>
        <div class="form-actions">
          <button type="button" class="btn btn-secondary" on:click={() => showLoadModal = false}>Cancel</button>
          <button type="submit" class="btn btn-primary">Load</button>
        </div>
      </form>
    </div>
  </div>
{/if}

<!-- Unload Tape Modal -->
{#if showUnloadModal && selectedLibrary}
  <div class="modal-backdrop" on:click={() => showUnloadModal = false}>
    <div class="modal" on:click|stopPropagation={() => {}}>
      <h2>📤 Unload Tape from Drive</h2>
      <p class="modal-desc">Move a tape from a drive back to a storage slot.</p>
      <form on:submit|preventDefault={handleUnload}>
        <div class="form-group">
          <label for="unload-slot">Destination Slot Number</label>
          <input type="number" id="unload-slot" bind:value={unloadForm.slot_number} min="1" required />
        </div>
        <div class="form-group">
          <label for="unload-drive">Source Drive Number</label>
          <input type="number" id="unload-drive" bind:value={unloadForm.drive_number} min="0" required />
        </div>
        <div class="form-actions">
          <button type="button" class="btn btn-secondary" on:click={() => showUnloadModal = false}>Cancel</button>
          <button type="submit" class="btn btn-primary">Unload</button>
        </div>
      </form>
    </div>
  </div>
{/if}

<!-- Transfer Tape Modal -->
{#if showTransferModal && selectedLibrary}
  <div class="modal-backdrop" on:click={() => showTransferModal = false}>
    <div class="modal" on:click|stopPropagation={() => {}}>
      <h2>🔀 Transfer Tape Between Slots</h2>
      <p class="modal-desc">Move a tape from one slot to another within the library.</p>
      <form on:submit|preventDefault={handleTransfer}>
        <div class="form-group">
          <label for="xfer-src">Source Slot Number</label>
          <input type="number" id="xfer-src" bind:value={transferForm.source_slot} min="1" required />
        </div>
        <div class="form-group">
          <label for="xfer-dst">Destination Slot Number</label>
          <input type="number" id="xfer-dst" bind:value={transferForm.dest_slot} min="1" required />
        </div>
        <div class="form-actions">
          <button type="button" class="btn btn-secondary" on:click={() => showTransferModal = false}>Cancel</button>
          <button type="submit" class="btn btn-primary">Transfer</button>
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
    background: var(--badge-danger-bg);
    color: var(--badge-danger-text);
    border: 1px solid var(--accent-danger);
  }

  .alert-success {
    background: var(--badge-success-bg);
    color: var(--badge-success-text);
    border: 1px solid var(--accent-success);
  }

  .dismiss-btn {
    background: none;
    border: none;
    font-size: 1.2rem;
    cursor: pointer;
    color: inherit;
  }

  .library-grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(320px, 1fr));
    gap: 1rem;
    margin-bottom: 1.5rem;
  }

  .library-card {
    cursor: pointer;
    transition: border-color 0.2s;
    border: 2px solid transparent;
  }

  .library-card:hover {
    border-color: var(--accent-primary);
  }

  .library-card.selected {
    border-color: var(--accent-primary);
  }

  .lib-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    margin-bottom: 0.75rem;
  }

  .lib-header h3 {
    margin: 0;
    font-size: 1rem;
  }

  .lib-meta {
    font-size: 0.8rem;
    color: var(--text-secondary);
    margin-bottom: 0.75rem;
  }

  .lib-meta div {
    margin: 0.15rem 0;
  }

  .lib-stats {
    display: flex;
    gap: 0.75rem;
    font-size: 0.8rem;
    margin-bottom: 0.75rem;
  }

  .lib-footer {
    display: flex;
    justify-content: space-between;
    align-items: center;
    font-size: 0.75rem;
  }

  .lib-actions {
    display: flex;
    gap: 0.25rem;
  }

  .text-muted {
    color: var(--text-muted);
  }

  .slot-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    margin-bottom: 1rem;
    flex-wrap: wrap;
    gap: 0.5rem;
  }

  .slot-header h2 {
    margin: 0;
    font-size: 1rem;
  }

  .slot-actions {
    display: flex;
    gap: 0.5rem;
    flex-wrap: wrap;
  }

  .slot-section-title {
    margin: 1rem 0 0.5rem;
    font-size: 0.875rem;
    color: var(--text-secondary);
  }

  .slot-grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(120px, 1fr));
    gap: 0.5rem;
  }

  .slot-card {
    background: var(--bg-input);
    border-radius: 8px;
    padding: 0.5rem;
    text-align: center;
    border: 1px solid var(--border-color);
    font-size: 0.75rem;
  }

  .slot-card.full {
    border-color: var(--accent-primary);
    background: var(--badge-info-bg);
  }

  .slot-num {
    font-weight: 600;
    margin-bottom: 0.25rem;
    color: var(--text-secondary);
  }

  .slot-content {
    font-size: 0.7rem;
    word-break: break-all;
  }

  .empty-slot {
    color: var(--text-muted);
    font-style: italic;
  }

  code {
    background: var(--code-bg);
    padding: 0.1rem 0.3rem;
    border-radius: 3px;
    font-size: 0.8rem;
  }

  .btn-sm {
    padding: 0.25rem 0.5rem;
    font-size: 0.75rem;
  }

  .modal-backdrop {
    position: fixed;
    top: 0; left: 0; right: 0; bottom: 0;
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

  .modal-desc {
    color: var(--text-secondary);
    font-size: 0.875rem;
    margin: 0 0 1rem;
  }

  .form-actions {
    display: flex;
    gap: 1rem;
    justify-content: flex-end;
    margin-top: 1.5rem;
  }

  small {
    display: block;
    margin-top: 0.25rem;
  }

  @media (max-width: 768px) {
    .library-grid {
      grid-template-columns: 1fr;
    }

    .slot-grid {
      grid-template-columns: repeat(auto-fill, minmax(90px, 1fr));
    }

    .slot-header {
      flex-direction: column;
      align-items: flex-start;
    }
  }
</style>
