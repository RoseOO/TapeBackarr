<script lang="ts">
  import { onMount } from 'svelte';
  import { api } from '$lib/api/client';

  interface APIKey {
    id: number;
    name: string;
    key_prefix: string;
    role: string;
    last_used_at: string | null;
    expires_at: string | null;
    created_at: string;
  }

  let keys: APIKey[] = [];
  let loading = true;
  let error = '';
  let showCreateModal = false;
  let newKey = '';

  let formData = {
    name: '',
    role: 'readonly',
    expires_in_days: 0,
  };

  onMount(async () => {
    await loadKeys();
  });

  async function loadKeys() {
    loading = true;
    try {
      keys = await api.get('/api-keys');
      if (!Array.isArray(keys)) keys = [];
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to load API keys';
    } finally {
      loading = false;
    }
  }

  async function handleCreate() {
    try {
      const body: any = { name: formData.name, role: formData.role };
      if (formData.expires_in_days > 0) body.expires_in_days = formData.expires_in_days;
      const result = await api.post('/api-keys', body);
      newKey = result.key;
      showCreateModal = false;
      await loadKeys();
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to create API key';
    }
  }

  async function handleDelete(id: number) {
    if (!confirm('Delete this API key? This cannot be undone.')) return;
    try {
      await api.delete(`/api-keys/${id}`);
      await loadKeys();
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to delete key';
    }
  }

  function formatDate(d: string | null): string {
    if (!d) return 'Never';
    return new Date(d).toLocaleString();
  }
</script>

<div class="page-header">
  <h1>🔑 API Keys</h1>
  <button class="btn btn-primary" on:click={() => { showCreateModal = true; newKey = ''; }}>
    + Generate API Key
  </button>
</div>

{#if error}
  <div class="card" style="background: var(--badge-danger-bg); color: var(--badge-danger-text);">
    <p>{error}</p>
    <button class="btn btn-secondary" on:click={() => error = ''}>Dismiss</button>
  </div>
{/if}

{#if newKey}
  <div class="card" style="background: var(--badge-success-bg); color: var(--badge-success-text); border: 2px solid var(--accent-success);">
    <h3>🔑 New API Key Created</h3>
    <p><strong>Store this key securely — it will not be shown again!</strong></p>
    <div style="background: var(--bg-input); padding: 0.75rem; border-radius: 6px; font-family: monospace; word-break: break-all; margin: 0.5rem 0;">
      {newKey}
    </div>
    <p style="font-size: 0.8rem;">Use this key in the <code>X-API-Key</code> header for API requests.</p>
    <button class="btn btn-secondary" on:click={() => newKey = ''}>Dismiss</button>
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
          <th>Key Prefix</th>
          <th>Role</th>
          <th>Last Used</th>
          <th>Expires</th>
          <th>Created</th>
          <th>Actions</th>
        </tr>
      </thead>
      <tbody>
        {#each keys as key}
          <tr>
            <td><strong>{key.name}</strong></td>
            <td><code>{key.key_prefix}...</code></td>
            <td><span class="badge {key.role === 'admin' ? 'badge-danger' : key.role === 'operator' ? 'badge-warning' : 'badge-info'}">{key.role}</span></td>
            <td>{formatDate(key.last_used_at)}</td>
            <td>{key.expires_at ? formatDate(key.expires_at) : 'Never'}</td>
            <td>{formatDate(key.created_at)}</td>
            <td><button class="btn btn-danger" on:click={() => handleDelete(key.id)}>Delete</button></td>
          </tr>
        {/each}
        {#if keys.length === 0}
          <tr><td colspan="7" style="text-align: center; color: var(--text-muted);">No API keys created yet.</td></tr>
        {/if}
      </tbody>
    </table>
  </div>
{/if}

{#if showCreateModal}
  <div class="modal-overlay" on:click={() => showCreateModal = false}>
    <div class="modal" on:click|stopPropagation={() => {}}>
      <h2>Generate API Key</h2>
      <form on:submit|preventDefault={handleCreate}>
        <div class="form-group">
          <label for="key-name">Key Name</label>
          <input type="text" id="key-name" bind:value={formData.name} required placeholder="e.g., CI Pipeline" />
        </div>
        <div class="form-group">
          <label for="key-role">Permissions (Role)</label>
          <select id="key-role" bind:value={formData.role}>
            <option value="readonly">Read Only</option>
            <option value="operator">Operator</option>
            <option value="admin">Admin</option>
          </select>
        </div>
        <div class="form-group">
          <label for="key-expiry">Expires In (days, 0 = never)</label>
          <input type="number" id="key-expiry" bind:value={formData.expires_in_days} min="0" />
        </div>
        <div class="modal-actions">
          <button type="button" class="btn btn-secondary" on:click={() => showCreateModal = false}>Cancel</button>
          <button type="submit" class="btn btn-primary">Generate</button>
        </div>
      </form>
    </div>
  </div>
{/if}

<style>
  code {
    background: var(--code-bg);
    padding: 0.1rem 0.3rem;
    border-radius: 3px;
    font-size: 0.8rem;
  }

  .modal-overlay {
    position: fixed;
    top: 0; left: 0; right: 0; bottom: 0;
    background: rgba(0,0,0,0.5);
    display: flex;
    justify-content: center;
    align-items: center;
    z-index: 1000;
  }

  .modal {
    background: var(--bg-card);
    padding: 2rem;
    border-radius: 12px;
    width: 100%;
    max-width: 400px;
  }

  .modal h2 { margin: 0 0 1.5rem; }

  .modal-actions {
    display: flex;
    gap: 0.75rem;
    justify-content: flex-end;
    margin-top: 1.5rem;
  }
</style>
