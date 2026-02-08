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

  let keys: EncryptionKey[] = [];
  let loading = true;
  let error = '';
  let successMsg = '';
  let showCreateModal = false;
  let showImportModal = false;
  let showKeySheetModal = false;
  let newKeyBase64 = '';

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
    await loadKeys();
  });

  async function loadKeys() {
    try {
      const result = await api.getEncryptionKeys();
      keys = result.keys || [];
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to load encryption keys';
    } finally {
      loading = false;
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

  <div class="card info-card">
    <h3>About Encryption</h3>
    <div class="info-grid">
      <div class="info-item">
        <span class="badge badge-info">AES-256-GCM</span>
        <p>All keys use AES-256-GCM authenticated encryption, providing both confidentiality and integrity for your tape backups.</p>
      </div>
      <div class="info-item">
        <span class="badge badge-warning">Key Safety</span>
        <p>Keys are shown only once when created. Use the Key Sheet feature to create a paper backup of all key fingerprints and recovery information.</p>
      </div>
      <div class="info-item">
        <span class="badge badge-success">Usage</span>
        <p>Assign encryption keys to backup jobs to automatically encrypt data written to tape. Keys in active use cannot be deleted.</p>
      </div>
    </div>
  </div>
{/if}

<!-- Create Key Modal -->
{#if showCreateModal}
  <div class="modal-overlay" on:click={() => showCreateModal = false} role="presentation">
    <div class="modal" on:click|stopPropagation role="dialog" aria-modal="true" tabindex="-1">
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
    <div class="modal" on:click|stopPropagation role="dialog" aria-modal="true" tabindex="-1">
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
    <div class="modal modal-wide" on:click|stopPropagation role="dialog" aria-modal="true" tabindex="-1">
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
</style>
