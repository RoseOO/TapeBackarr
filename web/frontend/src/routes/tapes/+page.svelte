<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import * as api from '$lib/api/client';
  import { dataVersion } from '$lib/stores/livedata';

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
    encryption_key_fingerprint: string;
    encryption_key_name: string;
    format_type: string;
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

  interface BatchLabelStatus {
    running: boolean;
    completed: number;
    total: number;
    current_label: string;
    message: string;
    phase: string;
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
  let showBatchLabelModal = false;
  let showDeleteModal = false;
  let selectedTape: Tape | null = null;
  let deleteTarget: Tape | null = null;

  // Batch label form
  let batchLabelDriveId: number | null = null;
  let batchLabelPrefix = '';
  let batchLabelStartNumber = 1;
  let batchLabelCount = 1;
  let batchLabelDigits = 3;
  let batchLabelPoolId: number | null = null;
  let batchLabelFormatType = 'raw';

  // Batch label progress
  let batchLabelStatus: BatchLabelStatus | null = null;
  let batchLabelPollInterval: ReturnType<typeof setInterval> | null = null;

  // Tape operation (format/label) progress
  let tapeOpPhase = '';
  let tapeOpMessage = '';
  let tapeOpElapsed = 0;
  let tapeOpRunning = false;
  let tapeOpError = '';
  let tapeOpPollInterval: ReturnType<typeof setInterval> | null = null;

  // Batch selection
  let selectedTapes: Set<number> = new Set();
  let batchStatus = '';
  let batchPoolId: number | null = null;

  // Form data
  let formData = {
    barcode: '',
    label: '',
    pool_id: null as number | null,
    lto_type: '' as string,
    capacity_bytes: 12000000000000,
    drive_id: null as number | null,
    write_label: true,
    auto_eject: true,
    format_type: 'raw' as string,
  };

  let formatDriveId: number | null = null;
  let formatAsLTFS = false;
  let exportLocation = '';
  let labelDriveId: number | null = null;
  let labelForce = false;
  let labelAutoEject = false;
  let newLabel = '';
  let detecting = false;
  let detectedType = '';

  $: if (formData.lto_type && ltoTypes[formData.lto_type]) {
    formData.capacity_bytes = ltoTypes[formData.lto_type];
  }

  $: batchLabelPreview = generateBatchLabelPreview(batchLabelPrefix, batchLabelStartNumber, batchLabelCount, batchLabelDigits);

  $: allSelected = tapes.length > 0 && tapes.every(t => selectedTapes.has(t.id));

  function generateBatchLabelPreview(prefix: string, start: number, count: number, digits: number): string[] {
    const labels: string[] = [];
    const safeCount = Math.min(count, 50);
    for (let i = 0; i < safeCount; i++) {
      labels.push(prefix + String(start + i).padStart(digits, '0'));
    }
    return labels;
  }

  function toggleSelectAll() {
    if (allSelected) {
      selectedTapes = new Set();
    } else {
      selectedTapes = new Set(tapes.map(t => t.id));
    }
  }

  function toggleSelectTape(id: number) {
    const next = new Set(selectedTapes);
    if (next.has(id)) {
      next.delete(id);
    } else {
      next.add(id);
    }
    selectedTapes = next;
  }

  async function handleBatchUpdate() {
    if (selectedTapes.size === 0) return;
    try {
      error = '';
      const data: { tape_ids: number[]; status?: string; pool_id?: number } = {
        tape_ids: Array.from(selectedTapes),
      };
      if (batchStatus) data.status = batchStatus;
      if (batchPoolId) data.pool_id = batchPoolId;
      await api.batchUpdateTapes(data);
      showSuccess(`Updated ${selectedTapes.size} tapes`);
      selectedTapes = new Set();
      batchStatus = '';
      batchPoolId = null;
      await loadData();
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to batch update tapes';
    }
  }

  async function onDriveChange(driveId: number | null) {
    formData.drive_id = driveId;
    detectedType = '';
    if (driveId) {
      detecting = true;
      try {
        const result = await api.detectTape(driveId);
        if (result.loaded && result.lto_type) {
          detectedType = result.lto_type;
          formData.lto_type = result.lto_type;
          if (result.capacity_bytes) {
            formData.capacity_bytes = result.capacity_bytes;
          }
        }
      } catch (e) {
        console.error('Tape type detection failed:', e);
      } finally {
        detecting = false;
      }
    }
  }

  // Auto-refresh tapes when SSE events arrive
  const tapesVersion = dataVersion('tapes');
  let lastTapesVersion = 0;
  const unsubTapesVersion = tapesVersion.subscribe(v => {
    if (v > lastTapesVersion && lastTapesVersion > 0) {
      loadData();
    }
    lastTapesVersion = v;
  });

  onMount(async () => {
    await loadData();
    checkBatchLabelStatus();
    checkTapeOpStatus();
  });

  onDestroy(() => {
    stopBatchLabelPolling();
    stopTapeOpPolling();
    unsubTapesVersion();
  });

  function startBatchLabelPolling() {
    stopBatchLabelPolling();
    batchLabelPollInterval = setInterval(async () => {
      await checkBatchLabelStatus();
    }, 2000);
  }

  function stopBatchLabelPolling() {
    if (batchLabelPollInterval) {
      clearInterval(batchLabelPollInterval);
      batchLabelPollInterval = null;
    }
  }

  async function checkBatchLabelStatus() {
    try {
      const status = await api.getBatchLabelStatus();
      batchLabelStatus = status;
      if (status && status.running) {
        if (!batchLabelPollInterval) {
          startBatchLabelPolling();
        }
      } else {
        stopBatchLabelPolling();
        if (status && status.total > 0 && status.completed === status.total) {
          await loadData();
        }
      }
    } catch {
      batchLabelStatus = null;
      stopBatchLabelPolling();
    }
  }

  async function handleCancelBatchLabel() {
    try {
      await api.cancelBatchLabel();
      showSuccess('Batch label cancelled');
      stopBatchLabelPolling();
      batchLabelStatus = null;
      await loadData();
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to cancel batch label';
    }
  }

  async function handleBatchLabel() {
    if (!batchLabelDriveId || !batchLabelPrefix || batchLabelCount < 1) return;
    try {
      error = '';
      await api.batchLabelTapesFromTapes({
        drive_id: batchLabelDriveId,
        prefix: batchLabelPrefix,
        start_number: batchLabelStartNumber,
        count: batchLabelCount,
        digits: batchLabelDigits,
        pool_id: batchLabelPoolId ?? undefined,
        format_type: batchLabelFormatType,
      });
      showBatchLabelModal = false;
      showSuccess('Batch label started');
      startBatchLabelPolling();
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to start batch label';
    }
  }

  async function loadData() {
    loading = true;
    error = '';
    try {
      const [tapesData, poolsData, drivesData, ltoTypesData] = await Promise.all([
        api.getTapes(),
        api.getPools(),
        api.getDrives(),
        api.getLTOTypes()
      ]);
      tapes = Array.isArray(tapesData) ? tapesData : [];
      pools = Array.isArray(poolsData) ? poolsData : [];
      drives = Array.isArray(drivesData) ? drivesData.filter((d: Drive) => d.enabled) : [];
      ltoTypes = ltoTypesData || {};
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
        pool_id: formData.pool_id ?? undefined,
        drive_id: formData.drive_id ?? undefined,
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
        barcode: formData.barcode,
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

  function handleDelete(tape: Tape) {
    deleteTarget = tape;
    showDeleteModal = true;
  }

  async function confirmDelete() {
    if (!deleteTarget) return;
    try {
      error = '';
      await api.deleteTape(deleteTarget.id);
      showSuccess('Tape deleted');
      showDeleteModal = false;
      deleteTarget = null;
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
      if (formatAsLTFS) {
        await api.formatLTFS(formatDriveId, selectedTape.label, selectedTape.uuid || crypto.randomUUID(), '', true);
        showFormatModal = false;
        showSuccess('Tape formatted with LTFS successfully');
        await loadData();
      } else {
        tapeOpRunning = true;
        tapeOpPhase = 'checking';
        tapeOpMessage = 'Starting format…';
        tapeOpElapsed = 0;
        tapeOpError = '';
        await api.formatTape(selectedTape.id, formatDriveId, true);
        startTapeOpPolling();
      }
    } catch (e) {
      tapeOpRunning = false;
      tapeOpPhase = '';
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
      tapeOpRunning = true;
      tapeOpPhase = 'checking';
      tapeOpMessage = 'Starting label…';
      tapeOpElapsed = 0;
      tapeOpError = '';
      await api.labelTape(selectedTape.id, newLabel, labelDriveId ?? undefined, labelForce, labelAutoEject);
      startTapeOpPolling();
    } catch (e) {
      tapeOpRunning = false;
      tapeOpPhase = '';
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
      format_type: tape.format_type || 'raw',
    };
    showEditModal = true;
  }

  function openFormatModal(tape: Tape) {
    selectedTape = tape;
    formatDriveId = drives.length > 0 ? drives[0].id : null;
    formatAsLTFS = false;
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

  function openBatchLabelModal() {
    batchLabelDriveId = drives.length > 0 ? drives[0].id : null;
    batchLabelPrefix = '';
    batchLabelStartNumber = 1;
    batchLabelCount = 1;
    batchLabelDigits = 3;
    batchLabelPoolId = null;
    batchLabelFormatType = 'raw';
    showBatchLabelModal = true;
  }

  function resetForm() {
    const defaultDriveId = drives.length > 0 ? drives[0].id : null;
    formData = {
      barcode: '',
      label: '',
      pool_id: null,
      lto_type: '',
      capacity_bytes: 12000000000000,
      drive_id: defaultDriveId,
      write_label: true,
      auto_eject: true,
      format_type: 'raw',
    };
    selectedTape = null;
    detectedType = '';
    if (defaultDriveId) {
      onDriveChange(defaultDriveId);
    }
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

  function formatDuration(seconds: number): string {
    if (seconds < 60) return `${seconds}s`;
    const m = Math.floor(seconds / 60);
    const s = seconds % 60;
    return `${m}m ${s}s`;
  }

  function opPhaseLabel(phase: string): string {
    switch (phase) {
      case 'checking': return 'Checking tape…';
      case 'erasing': return 'Erasing tape…';
      case 'writing': return 'Writing label…';
      case 'verifying': return 'Verifying label…';
      case 'ejecting': return 'Ejecting tape…';
      case 'updating': return 'Updating database…';
      case 'complete': return 'Complete';
      case 'failed': return 'Failed';
      default: return phase || 'Working…';
    }
  }

  function batchPhaseLabel(phase: string): string {
    switch (phase) {
      case 'waiting': return 'Waiting…';
      case 'checking': return 'Checking tape…';
      case 'writing': return 'Writing label…';
      case 'detecting': return 'Detecting tape…';
      case 'saving': return 'Saving to database…';
      case 'ejecting': return 'Ejecting tape…';
      default: return phase || '';
    }
  }

  function startTapeOpPolling() {
    stopTapeOpPolling();
    tapeOpPollInterval = setInterval(async () => {
      await checkTapeOpStatus();
    }, 1500);
  }

  function stopTapeOpPolling() {
    if (tapeOpPollInterval) {
      clearInterval(tapeOpPollInterval);
      tapeOpPollInterval = null;
    }
  }

  async function checkTapeOpStatus() {
    try {
      const status = await api.getTapeOpStatus();
      if (status && status.running) {
        tapeOpRunning = true;
        tapeOpPhase = status.phase || '';
        tapeOpMessage = status.message || '';
        tapeOpElapsed = status.elapsed_seconds || 0;
        tapeOpError = '';
        if (!tapeOpPollInterval) {
          startTapeOpPolling();
        }
      } else if (status && status.phase === 'complete') {
        tapeOpRunning = false;
        tapeOpPhase = 'complete';
        tapeOpMessage = status.message || 'Operation complete';
        tapeOpElapsed = status.elapsed_seconds || 0;
        tapeOpError = '';
        stopTapeOpPolling();
        await loadData();
        setTimeout(() => {
          showFormatModal = false;
          showLabelModal = false;
          tapeOpPhase = '';
          tapeOpMessage = '';
          tapeOpElapsed = 0;
          showSuccess(status.op_type === 'format' ? 'Tape formatted successfully' : 'Tape labeled successfully');
        }, 1500);
      } else if (status && status.phase === 'failed') {
        tapeOpRunning = false;
        tapeOpPhase = 'failed';
        tapeOpMessage = status.message || '';
        tapeOpError = status.error || 'Operation failed';
        stopTapeOpPolling();
      } else {
        tapeOpRunning = false;
        stopTapeOpPolling();
      }
    } catch {
      // Status endpoint not available or no operation running
      tapeOpRunning = false;
      stopTapeOpPolling();
    }
  }
</script>

<div class="page-header">
  <h1>Tape Management</h1>
  <div class="header-actions">
    <button class="btn btn-secondary" on:click={openBatchLabelModal}>
      📦 Batch Label
    </button>
    <button class="btn btn-primary" on:click={() => { showCreateModal = true; resetForm(); }}>
      + Add Tape
    </button>
  </div>
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

{#if batchLabelStatus && (batchLabelStatus.running || (batchLabelStatus.total > 0 && batchLabelStatus.completed < batchLabelStatus.total))}
  <div class="card batch-progress-panel">
    <div class="batch-progress-header">
      <div style="display: flex; align-items: center; gap: 0.75rem;">
        <div class="spinner" style="width: 24px; height: 24px; border-width: 3px;"></div>
        <strong>📦 Batch Label in Progress</strong>
      </div>
      <button class="btn btn-danger btn-sm" on:click={handleCancelBatchLabel}>Cancel</button>
    </div>
    <div class="batch-progress-details">
      <span>Label: <strong>{batchLabelStatus.current_label || '—'}</strong></span>
      {#if batchLabelStatus.phase}
        <span class="badge badge-info">{batchPhaseLabel(batchLabelStatus.phase)}</span>
      {/if}
      <span>{batchLabelStatus.message || ''}</span>
    </div>
    <div class="progress-bar-outer">
      <div class="progress-bar-fill" style="width: {batchLabelStatus.total > 0 ? (batchLabelStatus.completed / batchLabelStatus.total) * 100 : 0}%"></div>
    </div>
    <div class="batch-progress-count">
      {batchLabelStatus.completed} / {batchLabelStatus.total} completed
    </div>
  </div>
{/if}

{#if selectedTapes.size > 0}
  <div class="card batch-actions-bar">
    <span class="batch-count">{selectedTapes.size} tape{selectedTapes.size !== 1 ? 's' : ''} selected</span>
    <div class="batch-controls">
      <select class="status-select" bind:value={batchStatus}>
        <option value="">Change Status...</option>
        <option value="blank">Blank</option>
        <option value="active">Active</option>
        <option value="full">Full</option>
        <option value="expired">Expired</option>
        <option value="retired">Retired</option>
        <option value="exported">Exported</option>
      </select>
      <select class="status-select" bind:value={batchPoolId}>
        <option value={null}>Change Pool...</option>
        {#each pools as pool}
          <option value={pool.id}>{pool.name}</option>
        {/each}
      </select>
      <button class="btn btn-primary btn-sm" on:click={handleBatchUpdate} disabled={!batchStatus && !batchPoolId}>Apply</button>
      <button class="btn btn-secondary btn-sm" on:click={() => { selectedTapes = new Set(); }}>Clear Selection</button>
    </div>
  </div>
{/if}

{#if loading}
  <p>Loading...</p>
{:else}
  <div class="card">
    <table>
      <thead>
        <tr>
          <th class="checkbox-col">
            <input type="checkbox" checked={allSelected} on:change={toggleSelectAll} />
          </th>
          <th>Label</th>
          <th>Type</th>
          <th>Format</th>
          <th>UUID</th>
          <th>Barcode</th>
          <th>Pool</th>
          <th>Status</th>
          <th>Usage</th>
          <th>Writes</th>
          <th>Encryption</th>
          <th>Labeled</th>
          <th>Actions</th>
        </tr>
      </thead>
      <tbody>
        {#each tapes as tape}
          <tr>
            <td class="checkbox-col">
              <input type="checkbox" checked={selectedTapes.has(tape.id)} on:change={() => toggleSelectTape(tape.id)} />
            </td>
            <td><strong>{tape.label}</strong></td>
            <td>{tape.lto_type || '-'}</td>
            <td>
              {#if tape.format_type === 'ltfs'}
                <span class="badge badge-info">LTFS</span>
              {:else}
                <span class="badge badge-secondary">Raw</span>
              {/if}
            </td>
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
            <td>
              {#if tape.encryption_key_fingerprint}
                <span class="badge badge-warning" title="Fingerprint: {tape.encryption_key_fingerprint}">🔒 {tape.encryption_key_name}</span>
              {:else}
                <span class="text-muted">—</span>
              {/if}
            </td>
            <td>{tape.labeled_at ? new Date(tape.labeled_at).toLocaleString() : 'No'}</td>
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
            <td colspan="12" class="no-data">No tapes found. Add a tape to get started.</td>
          </tr>
        {/if}
      </tbody>
    </table>
  </div>
{/if}

<!-- Create Modal -->
{#if showCreateModal}
  <div class="modal-overlay" on:click={() => showCreateModal = false}>
    <div class="modal" on:click|stopPropagation={() => {}}>
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
          <small>Auto-detected from tape density code when a drive is selected, or choose manually</small>
        </div>
        <div class="form-group">
          <label for="capacity">Capacity</label>
          <input type="text" id="capacity" value={formatBytes(formData.capacity_bytes)} disabled />
          <small>Set automatically from LTO type</small>
        </div>
        {#if drives.length > 0}
          <div class="form-group">
            <label for="drive">Drive (for labeling)</label>
            <select id="drive" value={formData.drive_id || 0} on:change={(e) => onDriveChange(Number((e.target as HTMLSelectElement).value) || null)}>
              <option value={0}>No drive (software-only)</option>
              {#each drives as drive}
                <option value={drive.id}>{drive.display_name || drive.device_path}</option>
              {/each}
            </select>
          </div>
          {#if detecting}
            <p class="detect-status">Detecting tape type...</p>
          {:else if detectedType}
            <p class="detect-status detect-success">Detected: {detectedType}</p>
          {/if}
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
              <div class="form-group">
                <label for="format-type">Tape Format</label>
                <select id="format-type" bind:value={formData.format_type}>
                  <option value="raw">Raw (tar-based)</option>
                  <option value="ltfs">LTFS (Linear Tape File System)</option>
                </select>
                <small>{formData.format_type === 'ltfs' ? 'LTFS makes the tape self-describing and readable without TapeBackarr. Requires LTFS tools installed.' : 'Traditional tar-based format with TapeBackarr label.'}</small>
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
    <div class="modal" on:click|stopPropagation={() => {}}>
      <h2>Edit Tape</h2>
      <p class="modal-desc">UUID: {selectedTape.uuid || 'N/A'}</p>
      <form on:submit|preventDefault={handleUpdate}>
        <div class="form-group">
          <label for="edit-label">Label</label>
          {#if selectedTape.labeled_at}
            <input type="text" id="edit-label" bind:value={formData.label} required disabled />
            <small class="label-locked-note">Label cannot be changed after tape has been physically labelled</small>
          {:else}
            <input type="text" id="edit-label" bind:value={formData.label} required />
          {/if}
        </div>
        <div class="form-group">
          <label for="edit-barcode">Barcode</label>
          <input type="text" id="edit-barcode" bind:value={formData.barcode} placeholder="e.g., ABC123L8" />
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
  <div class="modal-overlay" on:click={() => { if (!tapeOpRunning) showFormatModal = false; }}>
    <div class="modal" on:click|stopPropagation={() => {}}>
      <h2>⚠️ Format Tape</h2>
      {#if tapeOpRunning || tapeOpPhase === 'complete' || tapeOpPhase === 'failed'}
        <div class="format-progress">
          {#if tapeOpPhase !== 'complete' && tapeOpPhase !== 'failed'}
            <div class="spinner"></div>
          {/if}
          <div class="format-progress-info">
            <strong>{opPhaseLabel(tapeOpPhase)}</strong>
            <p style="color: var(--text-muted, #888); margin: 0.25rem 0 0; font-size: 0.9rem;">{tapeOpMessage}</p>
            {#if tapeOpElapsed > 0}
              <p style="color: var(--text-muted, #888); margin: 0.25rem 0 0; font-size: 0.8rem;">
                Elapsed: {formatDuration(tapeOpElapsed)}
              </p>
            {/if}
          </div>
        </div>
        <div class="format-phases">
          <div class="format-phase" class:active={tapeOpPhase === 'checking'} class:done={['erasing', 'updating', 'complete'].includes(tapeOpPhase)}>
            <span class="phase-dot"></span> Checking tape
          </div>
          <div class="format-phase" class:active={tapeOpPhase === 'erasing'} class:done={['updating', 'complete'].includes(tapeOpPhase)}>
            <span class="phase-dot"></span> Erasing tape
          </div>
          <div class="format-phase" class:active={tapeOpPhase === 'updating'} class:done={['complete'].includes(tapeOpPhase)}>
            <span class="phase-dot"></span> Updating database
          </div>
          <div class="format-phase" class:active={tapeOpPhase === 'complete'} class:done={tapeOpPhase === 'complete'}>
            <span class="phase-dot"></span> Complete
          </div>
        </div>
        {#if tapeOpPhase === 'failed'}
          <div class="card error-card" style="margin-top: 1rem;">
            <p>{tapeOpError || 'Format failed'}</p>
          </div>
          <div class="modal-actions">
            <button class="btn btn-secondary" on:click={() => { tapeOpPhase = ''; tapeOpError = ''; showFormatModal = false; }}>Close</button>
          </div>
        {/if}
      {:else}
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
        <div class="form-group" style="margin-top: 0.5rem;">
          <label style="display: flex; align-items: center; gap: 0.5rem; cursor: pointer;">
            <input type="checkbox" bind:checked={formatAsLTFS} />
            Format as LTFS
          </label>
          <small>Use LTFS format for self-describing tape. Requires LTFS tools installed.</small>
        </div>
        <div class="modal-actions">
          <button class="btn btn-secondary" on:click={() => showFormatModal = false}>Cancel</button>
          <button class="btn btn-danger" on:click={handleFormat}>{formatAsLTFS ? 'Format as LTFS' : 'Format Tape'}</button>
        </div>
      {/if}
    </div>
  </div>
{/if}

<!-- Export Modal -->
{#if showExportModal && selectedTape}
  <div class="modal-overlay" on:click={() => showExportModal = false}>
    <div class="modal" on:click|stopPropagation={() => {}}>
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
  <div class="modal-overlay" on:click={() => { if (!tapeOpRunning) showLabelModal = false; }}>
    <div class="modal" on:click|stopPropagation={() => {}}>
      <h2>Write Tape Label</h2>
      {#if tapeOpRunning || tapeOpPhase === 'complete' || tapeOpPhase === 'failed'}
        <div class="format-progress">
          {#if tapeOpPhase !== 'complete' && tapeOpPhase !== 'failed'}
            <div class="spinner"></div>
          {/if}
          <div class="format-progress-info">
            <strong>{opPhaseLabel(tapeOpPhase)}</strong>
            <p style="color: var(--text-muted, #888); margin: 0.25rem 0 0; font-size: 0.9rem;">{tapeOpMessage}</p>
            {#if tapeOpElapsed > 0}
              <p style="color: var(--text-muted, #888); margin: 0.25rem 0 0; font-size: 0.8rem;">
                Elapsed: {formatDuration(tapeOpElapsed)}
              </p>
            {/if}
          </div>
        </div>
        <div class="format-phases">
          <div class="format-phase" class:active={tapeOpPhase === 'checking'} class:done={['verifying', 'writing', 'ejecting', 'updating', 'complete'].includes(tapeOpPhase)}>
            <span class="phase-dot"></span> Checking tape
          </div>
          <div class="format-phase" class:active={tapeOpPhase === 'verifying'} class:done={['writing', 'ejecting', 'updating', 'complete'].includes(tapeOpPhase)}>
            <span class="phase-dot"></span> Verifying label
          </div>
          <div class="format-phase" class:active={tapeOpPhase === 'writing'} class:done={['ejecting', 'updating', 'complete'].includes(tapeOpPhase)}>
            <span class="phase-dot"></span> Writing label
          </div>
          <div class="format-phase" class:active={tapeOpPhase === 'ejecting'} class:done={['updating', 'complete'].includes(tapeOpPhase)}>
            <span class="phase-dot"></span> Ejecting tape
          </div>
          <div class="format-phase" class:active={tapeOpPhase === 'updating'} class:done={['complete'].includes(tapeOpPhase)}>
            <span class="phase-dot"></span> Updating database
          </div>
          <div class="format-phase" class:active={tapeOpPhase === 'complete'} class:done={tapeOpPhase === 'complete'}>
            <span class="phase-dot"></span> Complete
          </div>
        </div>
        {#if tapeOpPhase === 'failed'}
          <div class="card error-card" style="margin-top: 1rem;">
            <p>{tapeOpError || 'Label operation failed'}</p>
          </div>
          <div class="modal-actions">
            <button class="btn btn-secondary" on:click={() => { tapeOpPhase = ''; tapeOpError = ''; showLabelModal = false; }}>Close</button>
          </div>
        {/if}
      {:else}
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
      {/if}
    </div>
  </div>
{/if}

<!-- Batch Label Modal -->
{#if showBatchLabelModal}
  <div class="modal-overlay" on:click={() => showBatchLabelModal = false}>
    <div class="modal" on:click|stopPropagation={() => {}}>
      <h2>📦 Batch Label Tapes</h2>
      <p class="modal-desc">Automatically create and label multiple tapes. Insert blank tapes one at a time when prompted.</p>
      <form on:submit|preventDefault={handleBatchLabel}>
        <div class="form-group">
          <label for="batch-drive">Drive <span class="required">*</span></label>
          <select id="batch-drive" bind:value={batchLabelDriveId} required>
            <option value={null} disabled>Select a drive...</option>
            {#each drives as drive}
              <option value={drive.id}>{drive.display_name || drive.device_path}</option>
            {/each}
          </select>
        </div>
        <div class="form-group">
          <label for="batch-prefix">Prefix <span class="required">*</span></label>
          <input type="text" id="batch-prefix" bind:value={batchLabelPrefix} required placeholder="e.g., BACKUP-" />
        </div>
        <div class="form-group">
          <label for="batch-start">Start Number</label>
          <input type="number" id="batch-start" bind:value={batchLabelStartNumber} min="0" />
        </div>
        <div class="form-group">
          <label for="batch-count">Count <span class="required">*</span></label>
          <input type="number" id="batch-count" bind:value={batchLabelCount} min="1" max="100" required />
        </div>
        <div class="form-group">
          <label for="batch-digits">Digits</label>
          <input type="number" id="batch-digits" bind:value={batchLabelDigits} min="1" max="6" />
        </div>
        <div class="form-group">
          <label for="batch-pool">Pool</label>
          <select id="batch-pool" bind:value={batchLabelPoolId}>
            <option value={null}>No Pool</option>
            {#each pools as pool}
              <option value={pool.id}>{pool.name}</option>
            {/each}
          </select>
        </div>
        <div class="form-group">
          <label for="batch-format-type">Tape Format</label>
          <select id="batch-format-type" bind:value={batchLabelFormatType}>
            <option value="raw">Raw (tar-based)</option>
            <option value="ltfs">LTFS (Linear Tape File System)</option>
          </select>
          <small>{batchLabelFormatType === 'ltfs' ? 'Each tape will be formatted with LTFS. Requires LTFS tools installed.' : 'Traditional tar-based format with TapeBackarr label.'}</small>
        </div>
        {#if batchLabelPreview.length > 0}
          <div class="batch-preview">
            <strong>Preview:</strong>
            <div class="batch-preview-labels">
              {#each batchLabelPreview as lbl}
                <span class="batch-preview-label">{lbl}</span>
              {/each}
              {#if batchLabelCount > 50}
                <span class="batch-preview-label">…and {batchLabelCount - 50} more</span>
              {/if}
            </div>
          </div>
        {/if}
        <div class="modal-actions">
          <button type="button" class="btn btn-secondary" on:click={() => showBatchLabelModal = false}>Cancel</button>
          <button type="submit" class="btn btn-primary" disabled={!batchLabelDriveId || !batchLabelPrefix || batchLabelCount < 1}>Start Batch Label</button>
        </div>
      </form>
    </div>
  </div>
{/if}

<!-- Delete Confirmation Modal -->
{#if showDeleteModal && deleteTarget}
  <div class="modal-overlay" on:click={() => { showDeleteModal = false; deleteTarget = null; }}>
    <div class="modal" on:click|stopPropagation={() => {}}>
      <h2>⚠️ Delete Tape</h2>
      <p class="modal-desc warning-text">Are you sure you want to delete tape <strong>{deleteTarget.label}</strong>? This action cannot be undone.</p>
      <div class="modal-actions">
        <button class="btn btn-secondary" on:click={() => { showDeleteModal = false; deleteTarget = null; }}>Cancel</button>
        <button class="btn btn-danger" on:click={confirmDelete}>Delete</button>
      </div>
    </div>
  </div>
{/if}

<style>
  .header-actions {
    display: flex;
    gap: 0.5rem;
    align-items: center;
  }

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

  .detect-status {
    font-size: 0.8rem;
    color: #888;
    margin: -0.5rem 0 0.5rem;
    font-style: italic;
  }

  .detect-success {
    color: #28a745;
    font-style: normal;
    font-weight: 500;
  }

  .label-locked-note {
    color: #856404;
    font-style: italic;
  }

  .checkbox-col {
    width: 2rem;
    text-align: center;
  }

  .checkbox-col input[type="checkbox"] {
    width: auto;
    margin: 0;
  }

  .batch-actions-bar {
    display: flex;
    align-items: center;
    gap: 1rem;
    padding: 0.75rem 1rem;
    margin-bottom: 1rem;
    background: #e8f0fe;
    flex-wrap: wrap;
  }

  .batch-count {
    font-weight: 600;
    font-size: 0.875rem;
  }

  .batch-controls {
    display: flex;
    gap: 0.5rem;
    align-items: center;
    flex-wrap: wrap;
  }

  .batch-progress-panel {
    background: #e8f4fd;
    border: 1px solid #b8daff;
    padding: 1rem;
    margin-bottom: 1rem;
  }

  .batch-progress-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    margin-bottom: 0.5rem;
  }

  .batch-progress-details {
    display: flex;
    gap: 1rem;
    font-size: 0.85rem;
    margin-bottom: 0.5rem;
    color: #333;
  }

  .progress-bar-outer {
    width: 100%;
    height: 10px;
    background: #d0d0d0;
    border-radius: 5px;
    overflow: hidden;
    margin-bottom: 0.25rem;
  }

  .progress-bar-fill {
    height: 100%;
    background: #4a4aff;
    border-radius: 5px;
    transition: width 0.3s ease;
  }

  .batch-progress-count {
    font-size: 0.8rem;
    color: #555;
  }

  .batch-preview {
    margin-top: 0.5rem;
    padding: 0.75rem;
    background: #f8f9fa;
    border-radius: 6px;
    font-size: 0.85rem;
  }

  .batch-preview-labels {
    display: flex;
    flex-wrap: wrap;
    gap: 0.35rem;
    margin-top: 0.35rem;
  }

  .batch-preview-label {
    background: #e0e0e0;
    padding: 0.15rem 0.5rem;
    border-radius: 4px;
    font-family: monospace;
    font-size: 0.8rem;
  }

  .spinner {
    width: 40px;
    height: 40px;
    border: 4px solid var(--border-color, #e0e0e0);
    border-top-color: var(--accent-primary, #4a4aff);
    border-radius: 50%;
    animation: spin 1s linear infinite;
    margin-bottom: 1rem;
  }

  @keyframes spin {
    to { transform: rotate(360deg); }
  }

  .format-progress {
    display: flex;
    align-items: center;
    gap: 1rem;
    padding: 1.5rem;
    background: var(--bg-input, #f8f9fa);
    border-radius: 8px;
    margin-bottom: 1rem;
  }

  .format-progress .spinner {
    flex-shrink: 0;
    margin-bottom: 0;
  }

  .format-progress-info {
    flex: 1;
  }

  .format-phases {
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
    padding: 0 0.5rem;
  }

  .format-phase {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    font-size: 0.85rem;
    color: var(--text-muted, #888);
    opacity: 0.5;
    transition: all 0.3s;
  }

  .format-phase.active {
    color: var(--accent-primary, #4a4aff);
    font-weight: 600;
    opacity: 1;
  }

  .format-phase.done {
    color: var(--badge-success-text, #22c55e);
    opacity: 0.8;
  }

  .phase-dot {
    width: 8px;
    height: 8px;
    border-radius: 50%;
    background: var(--text-muted, #888);
    flex-shrink: 0;
  }

  .format-phase.active .phase-dot {
    background: var(--accent-primary, #4a4aff);
    box-shadow: 0 0 6px var(--accent-primary, #4a4aff);
    animation: pulse-dot 1.5s infinite;
  }

  .format-phase.done .phase-dot {
    background: var(--badge-success-text, #22c55e);
  }

  @keyframes pulse-dot {
    0%, 100% { opacity: 1; }
    50% { opacity: 0.4; }
  }
</style>
