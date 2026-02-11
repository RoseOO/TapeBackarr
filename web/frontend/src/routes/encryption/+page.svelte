<script lang="ts">
  import { onMount } from 'svelte';
  import * as api from '$lib/api/client';
  import { auth } from '$lib/stores/auth';

  interface EncryptionKey {
    id: number;
    name: string;
    algorithm: string;
    key_fingerprint: string;
    description: string;
    created_at: string;
    updated_at: string;
  }

  interface Drive {
    id: number;
    device_path: string;
    display_name: string;
    vendor: string;
    model: string;
    status: string;
    enabled: boolean;
  }

  interface HwEncryptionStatus {
    supported: boolean;
    enabled: boolean;
    algorithm: string;
    mode: string;
    error: string;
  }

  let keys: EncryptionKey[] = [];
  let drives: Drive[] = [];
  let loading = true;
  let error = '';
  let successMsg = '';
  let showCreateModal = false;
  let showImportModal = false;
  let showKeySheetModal = false;
  let newKeyBase64 = '';

  // Hardware encryption state
  let hwSelectedDriveId: number | null = null;
  let hwSelectedKeyId: number | null = null;
  let hwStatus: HwEncryptionStatus | null = null;
  let hwLoading = false;
  let hwError = '';

  let createForm = {
    name: '',
    description: '',
  };

  let importForm = {
    name: '',
    key_base64: '',
    description: '',
  };

  let keySheetText = '';

  $: isAdmin = $auth.user?.role === 'admin';

  onMount(async () => {
    await Promise.all([loadKeys(), loadDrives()]);
  });

  async function loadKeys() {
    loading = true;
    error = '';
    try {
      const result = await api.getEncryptionKeys();
      keys = result?.keys || [];
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to load encryption keys';
    } finally {
      loading = false;
    }
  }

  async function loadDrives() {
    try {
      const result = await api.getDrives();
      drives = Array.isArray(result) ? result.filter((d: Drive) => d.enabled) : [];
    } catch {
      // Drives are optional for this page
      drives = [];
    }
  }

  async function loadHwStatus() {
    if (!hwSelectedDriveId) {
      hwStatus = null;
      return;
    }
    hwLoading = true;
    hwError = '';
    try {
      hwStatus = await api.getHardwareEncryption(hwSelectedDriveId);
    } catch (e) {
      hwError = e instanceof Error ? e.message : 'Failed to get hardware encryption status';
      hwStatus = null;
    } finally {
      hwLoading = false;
    }
  }

  async function handleEnableHwEncryption() {
    if (!hwSelectedDriveId || !hwSelectedKeyId) return;
    hwLoading = true;
    hwError = '';
    try {
      await api.setHardwareEncryption(hwSelectedDriveId, hwSelectedKeyId);
      showSuccess('Hardware encryption enabled on drive');
      await loadHwStatus();
    } catch (e) {
      hwError = e instanceof Error ? e.message : 'Failed to enable hardware encryption';
    } finally {
      hwLoading = false;
    }
  }

  async function handleDisableHwEncryption() {
    if (!hwSelectedDriveId) return;
    if (!confirm('Disable hardware encryption on this drive? Data written after this will be unencrypted.')) return;
    hwLoading = true;
    hwError = '';
    try {
      await api.clearHardwareEncryption(hwSelectedDriveId);
      showSuccess('Hardware encryption disabled on drive');
      await loadHwStatus();
    } catch (e) {
      hwError = e instanceof Error ? e.message : 'Failed to disable hardware encryption';
    } finally {
      hwLoading = false;
    }
  }

  function showSuccess(msg: string) {
    successMsg = msg;
    setTimeout(() => successMsg = '', 4000);
  }

  async function handleCreate() {
    try {
      error = '';
      const result = await api.createEncryptionKey(createForm);
      newKeyBase64 = result.key_base64;
      showCreateModal = false;
      createForm = { name: '', description: '' };
      showSuccess('Encryption key created successfully. Save the key shown below — it will not be displayed again.');
      await loadKeys();
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to create encryption key';
    }
  }

  async function handleImport() {
    try {
      error = '';
      await api.importEncryptionKey(importForm);
      showImportModal = false;
      importForm = { name: '', key_base64: '', description: '' };
      showSuccess('Encryption key imported successfully');
      await loadKeys();
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to import encryption key';
    }
  }

  async function handleDelete(key: EncryptionKey) {
    if (!confirm(`Delete encryption key "${key.name}"? This cannot be undone. Keys in use by backup jobs cannot be deleted.`)) return;
    try {
      error = '';
      await api.deleteEncryptionKey(key.id);
      showSuccess('Encryption key deleted');
      await loadKeys();
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to delete encryption key';
    }
  }

  async function handleDownloadKeySheet() {
    try {
      error = '';
      const text = await api.getKeySheetText();
      const blob = new Blob([text], { type: 'text/plain' });
      const url = URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = url;
      a.download = 'tapebackarr-keysheet.txt';
      a.click();
      URL.revokeObjectURL(url);
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to download key sheet';
    }
  }

  async function handleViewKeySheet() {
    try {
      error = '';
      keySheetText = await api.getKeySheetText();
      showKeySheetModal = true;
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to load key sheet';
    }
  }

  function dismissNewKey() {
    newKeyBase64 = '';
  }

  function copyToClipboard(text: string) {
    navigator.clipboard.writeText(text).then(() => {
      showSuccess('Copied to clipboard');
    }).catch(() => {
      error = 'Failed to copy to clipboard';
    });
  }

  function formatDate(dateStr: string): string {
    return new Date(dateStr).toLocaleString();
  }

  function truncateFingerprint(fp: string): string {
    if (fp.length > 16) {
      return fp.substring(0, 8) + '...' + fp.substring(fp.length - 8);
    }
    return fp;
  }
</script>

<div class="page-header">
  <h1>🔒 Encryption Keys</h1>
  <div class="header-actions">
    {#if keys.length > 0}
      <button class="btn btn-secondary" on:click={handleViewKeySheet}>
        📄 View Key Sheet
      </button>
      <button class="btn btn-secondary" on:click={handleDownloadKeySheet}>
        ⬇️ Download Key Sheet
      </button>
    {/if}
    {#if isAdmin}
      <button class="btn btn-secondary" on:click={() => { showImportModal = true; importForm = { name: '', key_base64: '', description: '' }; }}>
        📥 Import Key
      </button>
      <button class="btn btn-primary" on:click={() => { showCreateModal = true; createForm = { name: '', description: '' }; }}>
        + Generate Key
      </button>
    {/if}
  </div>
</div>

{#if error}
  <div class="card error-card">
    <p>{error}</p>
    <button class="btn btn-secondary" on:click={() => error = ''}>Dismiss</button>
  </div>
{/if}

{#if successMsg}
  <div class="card success-card">
    <p>{successMsg}</p>
  </div>
{/if}

{#if newKeyBase64}
  <div class="card key-reveal-card">
    <h3>⚠️ New Key Created — Save This Now</h3>
    <p>This is the only time this key will be shown. Copy it and store it securely.</p>
    <div class="key-display">
      <code>{newKeyBase64}</code>
      <button class="btn btn-secondary" on:click={() => copyToClipboard(newKeyBase64)}>📋 Copy</button>
    </div>
    <button class="btn btn-secondary" on:click={dismissNewKey} style="margin-top: 0.75rem;">
      I've saved this key — dismiss
    </button>
  </div>
{/if}

{#if loading}
  <p>Loading...</p>
{:else}
  <div class="card">
    <table>
      <thead>
        <tr>
          <th>Name</th>
          <th>Algorithm</th>
          <th>Fingerprint</th>
          <th>Description</th>
          <th>Created</th>
          {#if isAdmin}
            <th>Actions</th>
          {/if}
        </tr>
      </thead>
      <tbody>
        {#each keys as key}
          <tr>
            <td><strong>{key.name}</strong></td>
            <td><span class="badge badge-info">{key.algorithm}</span></td>
            <td>
              <code class="fingerprint" title={key.key_fingerprint}>
                {truncateFingerprint(key.key_fingerprint)}
              </code>
            </td>
            <td>{key.description || '—'}</td>
            <td>{formatDate(key.created_at)}</td>
            {#if isAdmin}
              <td>
                <button class="btn btn-danger" on:click={() => handleDelete(key)}>
                  Delete
                </button>
              </td>
            {/if}
          </tr>
        {/each}
        {#if keys.length === 0}
          <tr>
            <td colspan={isAdmin ? 6 : 5} class="no-data">
              No encryption keys found. Generate or import a key to enable encrypted backups.
            </td>
          </tr>
        {/if}
      </tbody>
    </table>
  </div>

  <!-- Hardware Encryption Section -->
  {#if isAdmin && drives.length > 0}
    <div class="card hw-encryption-card">
      <h3>🔐 Hardware Encryption (LTO Drive)</h3>
      <p class="section-desc">LTO-4 and later drives support AES-256-GCM encryption at the drive firmware level. Hardware encryption is transparent, operates at full tape speed, and does not require CPU resources. Keys from the key store above are used to set the drive encryption key.</p>

      {#if hwError}
        <div class="hw-error">{hwError}
          <button class="dismiss-btn" on:click={() => hwError = ''}>×</button>
        </div>
      {/if}

      <div class="hw-controls">
        <div class="hw-row">
          <div class="form-group">
            <label for="hw-drive">Drive</label>
            <select id="hw-drive" bind:value={hwSelectedDriveId} on:change={loadHwStatus}>
              <option value={null}>Select a drive...</option>
              {#each drives as drive}
                <option value={drive.id}>{drive.display_name || drive.device_path} ({drive.vendor || 'Unknown'})</option>
              {/each}
            </select>
          </div>
          <div class="form-group">
            <label for="hw-key">Encryption Key</label>
            <select id="hw-key" bind:value={hwSelectedKeyId}>
              <option value={null}>Select a key...</option>
              {#each keys as key}
                <option value={key.id}>🔒 {key.name}</option>
              {/each}
            </select>
          </div>
          <div class="hw-actions">
            <button class="btn btn-primary" on:click={handleEnableHwEncryption}
              disabled={!hwSelectedDriveId || !hwSelectedKeyId || hwLoading}>
              {hwLoading ? 'Working...' : 'Enable'}
            </button>
            <button class="btn btn-danger" on:click={handleDisableHwEncryption}
              disabled={!hwSelectedDriveId || hwLoading}>
              Disable
            </button>
          </div>
        </div>
      </div>

      {#if hwSelectedDriveId}
        <div class="hw-status-panel">
          <h4>Drive Encryption Status</h4>
          {#if hwLoading}
            <p>Checking status...</p>
          {:else if hwStatus}
            <div class="hw-status-grid">
              <div class="hw-stat">
                <span class="hw-label">Supported</span>
                <span class="badge {hwStatus.supported ? 'badge-success' : 'badge-secondary'}">{hwStatus.supported ? 'Yes' : 'No'}</span>
              </div>
              <div class="hw-stat">
                <span class="hw-label">Status</span>
                <span class="badge {hwStatus.enabled ? 'badge-success' : 'badge-secondary'}">{hwStatus.enabled ? '🔒 Encryption On' : 'Off'}</span>
              </div>
              <div class="hw-stat">
                <span class="hw-label">Mode</span>
                <span class="badge badge-info">{hwStatus.mode}</span>
              </div>
              {#if hwStatus.algorithm}
                <div class="hw-stat">
                  <span class="hw-label">Algorithm</span>
                  <span class="badge badge-info">{hwStatus.algorithm}</span>
                </div>
              {/if}
              {#if hwStatus.error}
                <div class="hw-stat hw-stat-wide">
                  <span class="hw-label">Note</span>
                  <span class="hw-note">{hwStatus.error}</span>
                </div>
              {/if}
            </div>
          {:else}
            <p class="hw-hint">Select a drive above to check hardware encryption status.</p>
          {/if}
        </div>
      {/if}
    </div>
  {/if}

  <div class="card info-card">
    <h3>About Encryption</h3>
    <div class="info-grid">
      <div class="info-item">
        <span class="badge badge-info">Software Encryption</span>
        <p>Per-file AES-256-GCM encryption applied by TapeBackarr before writing to tape. Assign an encryption key to a backup job to use this mode.</p>
      </div>
      <div class="info-item">
        <span class="badge badge-success">Hardware Encryption</span>
        <p>LTO-4+ drives encrypt data at the firmware level at full tape speed with zero CPU overhead. Enable it on a drive above — all data written while active is encrypted.</p>
      </div>
      <div class="info-item">
        <span class="badge badge-warning">Key Safety</span>
        <p>Keys are shown only once when created. Use the Key Sheet feature to create a paper backup of all key fingerprints and recovery information.</p>
      </div>
      <div class="info-item">
        <span class="badge badge-info">AES-256-GCM</span>
        <p>Both software and hardware encryption use AES-256-GCM, providing confidentiality and integrity for your tape backups.</p>
      </div>
    </div>
  </div>
{/if}

<!-- Create Key Modal -->
{#if showCreateModal}
  <div class="modal-overlay" on:click={() => showCreateModal = false} role="presentation">
    <div class="modal" on:click|stopPropagation={() => {}} role="dialog" aria-modal="true" tabindex="-1">
      <h2>Generate Encryption Key</h2>
      <p class="modal-desc">Generate a new AES-256-GCM encryption key. The key will be shown once after creation.</p>
      <form on:submit|preventDefault={handleCreate}>
        <div class="form-group">
          <label for="create-name">Key Name</label>
          <input type="text" id="create-name" bind:value={createForm.name} required placeholder="e.g., Production Backup Key" />
        </div>
        <div class="form-group">
          <label for="create-desc">Description (optional)</label>
          <textarea id="create-desc" bind:value={createForm.description} rows="2" placeholder="e.g., Primary key for nightly backups"></textarea>
        </div>
        <div class="modal-actions">
          <button type="button" class="btn btn-secondary" on:click={() => showCreateModal = false}>Cancel</button>
          <button type="submit" class="btn btn-primary">Generate Key</button>
        </div>
      </form>
    </div>
  </div>
{/if}

<!-- Import Key Modal -->
{#if showImportModal}
  <div class="modal-overlay" on:click={() => showImportModal = false} role="presentation">
    <div class="modal" on:click|stopPropagation={() => {}} role="dialog" aria-modal="true" tabindex="-1">
      <h2>Import Encryption Key</h2>
      <p class="modal-desc">Import an existing AES-256 key from a base64-encoded string.</p>
      <form on:submit|preventDefault={handleImport}>
        <div class="form-group">
          <label for="import-name">Key Name</label>
          <input type="text" id="import-name" bind:value={importForm.name} required placeholder="e.g., Restored Key" />
        </div>
        <div class="form-group">
          <label for="import-key">Key (Base64)</label>
          <input type="password" id="import-key" bind:value={importForm.key_base64} required placeholder="Paste base64-encoded key" />
        </div>
        <div class="form-group">
          <label for="import-desc">Description (optional)</label>
          <textarea id="import-desc" bind:value={importForm.description} rows="2" placeholder="e.g., Recovered from paper backup"></textarea>
        </div>
        <div class="modal-actions">
          <button type="button" class="btn btn-secondary" on:click={() => showImportModal = false}>Cancel</button>
          <button type="submit" class="btn btn-primary">Import Key</button>
        </div>
      </form>
    </div>
  </div>
{/if}

<!-- Key Sheet Modal -->
{#if showKeySheetModal}
  <div class="modal-overlay" on:click={() => showKeySheetModal = false} role="presentation">
    <div class="modal modal-wide" on:click|stopPropagation={() => {}} role="dialog" aria-modal="true" tabindex="-1">
      <h2>📄 Key Sheet</h2>
      <p class="modal-desc">Print this page or save the text as a paper backup for your encryption keys.</p>
      <pre class="keysheet-content">{keySheetText}</pre>
      <div class="modal-actions">
        <button class="btn btn-secondary" on:click={() => copyToClipboard(keySheetText)}>📋 Copy</button>
        <button class="btn btn-secondary" on:click={handleDownloadKeySheet}>⬇️ Download</button>
        <button class="btn btn-primary" on:click={() => showKeySheetModal = false}>Close</button>
      </div>
    </div>
  </div>
{/if}

<style>
  .header-actions {
    display: flex;
    gap: 0.75rem;
    flex-wrap: wrap;
  }

  .error-card {
    background: #f8d7da;
    color: #721c24;
    display: flex;
    justify-content: space-between;
    align-items: center;
  }

  .success-card {
    background: #d4edda;
    color: #155724;
    padding: 0.75rem 1rem;
    border-radius: 8px;
    margin-bottom: 1rem;
  }

  .key-reveal-card {
    background: #fff3cd;
    border: 2px solid #ffc107;
  }

  .key-reveal-card h3 {
    margin: 0 0 0.5rem;
    color: #856404;
  }

  .key-reveal-card p {
    margin: 0 0 0.75rem;
    color: #856404;
    font-size: 0.875rem;
  }

  .key-display {
    display: flex;
    gap: 0.75rem;
    align-items: center;
    background: #fff;
    padding: 0.75rem;
    border-radius: 6px;
    border: 1px solid #e0c36a;
  }

  .key-display code {
    flex: 1;
    font-size: 0.875rem;
    word-break: break-all;
    color: #333;
  }

  .fingerprint {
    background: #f0f0f0;
    padding: 0.2rem 0.4rem;
    border-radius: 4px;
    font-size: 0.8rem;
    cursor: help;
  }

  .no-data {
    text-align: center;
    color: #666;
    padding: 2rem;
  }

  .info-card {
    margin-top: 1rem;
  }

  .info-card h3 {
    margin: 0 0 1rem;
    font-size: 1rem;
  }

  .info-grid {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
    gap: 1rem;
  }

  .info-item {
    padding: 1rem;
    background: #f9f9f9;
    border-radius: 8px;
  }

  .info-item .badge {
    margin-bottom: 0.5rem;
  }

  .info-item p {
    margin: 0;
    font-size: 0.875rem;
    color: #666;
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
    max-width: 450px;
  }

  .modal-wide {
    max-width: 700px;
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

  .keysheet-content {
    background: #f9f9f9;
    padding: 1rem;
    border-radius: 8px;
    font-size: 0.75rem;
    max-height: 400px;
    overflow: auto;
    white-space: pre-wrap;
    word-break: break-all;
    border: 1px solid #eee;
  }

  /* Hardware Encryption Section */
  .hw-encryption-card {
    margin-top: 1.5rem;
    border: 1px solid #d1e7dd;
  }

  .hw-encryption-card h3 {
    margin: 0 0 0.5rem;
  }

  .section-desc {
    font-size: 0.875rem;
    color: #666;
    margin: 0 0 1.25rem;
    line-height: 1.5;
  }

  .hw-error {
    background: #f8d7da;
    color: #721c24;
    padding: 0.5rem 0.75rem;
    border-radius: 6px;
    margin-bottom: 1rem;
    display: flex;
    justify-content: space-between;
    align-items: center;
    font-size: 0.875rem;
  }

  .dismiss-btn {
    background: none;
    border: none;
    font-size: 1.25rem;
    cursor: pointer;
    color: inherit;
    padding: 0 0.25rem;
  }

  .hw-controls {
    margin-bottom: 1rem;
  }

  .hw-row {
    display: flex;
    gap: 1rem;
    align-items: flex-end;
    flex-wrap: wrap;
  }

  .hw-row .form-group {
    flex: 1;
    min-width: 180px;
  }

  .hw-actions {
    display: flex;
    gap: 0.5rem;
    padding-bottom: 0.25rem;
  }

  .hw-status-panel {
    background: #f9f9f9;
    border: 1px solid #eee;
    border-radius: 8px;
    padding: 1rem;
    margin-top: 0.5rem;
  }

  .hw-status-panel h4 {
    margin: 0 0 0.75rem;
    font-size: 0.95rem;
  }

  .hw-status-grid {
    display: flex;
    gap: 1.5rem;
    flex-wrap: wrap;
  }

  .hw-stat {
    display: flex;
    flex-direction: column;
    gap: 0.3rem;
  }

  .hw-stat-wide {
    flex-basis: 100%;
  }

  .hw-label {
    font-size: 0.8rem;
    color: #888;
    text-transform: uppercase;
    letter-spacing: 0.03em;
  }

  .hw-note {
    font-size: 0.85rem;
    color: #666;
  }

  .hw-hint {
    color: #888;
    font-size: 0.875rem;
    margin: 0;
  }
</style>
