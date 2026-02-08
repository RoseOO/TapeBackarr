<script lang="ts">
  import { onMount } from 'svelte';
  import * as api from '$lib/api/client';
  import { auth } from '$lib/stores/auth';

  interface User {
    id: number;
    username: string;
    role: string;
    created_at: string;
  }

  let users: User[] = [];
  let loading = true;
  let error = '';
  let successMsg = '';
  let showCreateModal = false;
  let showPasswordModal = false;

  let formData = {
    username: '',
    password: '',
    role: 'operator',
  };

  let passwordForm = {
    old_password: '',
    new_password: '',
    confirm_password: '',
  };

  $: currentUser = $auth.user;

  onMount(async () => {
    await loadUsers();
  });

  async function loadUsers() {
    loading = true;
    error = '';
    try {
      const result = await api.getUsers();
      users = Array.isArray(result) ? result : [];
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to load users';
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
      await api.createUser(formData);
      showCreateModal = false;
      resetForm();
      showSuccess('User created successfully');
      await loadUsers();
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to create user';
    }
  }

  async function handleDelete(user: User) {
    if (user.username === 'admin') {
      error = 'Cannot delete the default admin account';
      return;
    }
    if (!confirm(`Delete user "${user.username}"? This cannot be undone.`)) return;
    try {
      error = '';
      await api.deleteUser(user.id);
      showSuccess('User deleted');
      await loadUsers();
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to delete user';
    }
  }

  async function handleChangePassword() {
    try {
      error = '';
      if (passwordForm.new_password !== passwordForm.confirm_password) {
        error = 'New passwords do not match';
        return;
      }
      if (passwordForm.new_password.length < 6) {
        error = 'New password must be at least 6 characters';
        return;
      }
      await api.changePassword(passwordForm.old_password, passwordForm.new_password);
      showPasswordModal = false;
      passwordForm = { old_password: '', new_password: '', confirm_password: '' };
      showSuccess('Password changed successfully');
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to change password';
    }
  }

  function resetForm() {
    formData = {
      username: '',
      password: '',
      role: 'operator',
    };
  }

  function formatDate(dateStr: string): string {
    return new Date(dateStr).toLocaleDateString();
  }

  function getRoleBadgeClass(role: string): string {
    switch (role) {
      case 'admin': return 'badge-danger';
      case 'operator': return 'badge-success';
      case 'readonly': return 'badge-info';
      default: return '';
    }
  }
</script>

<div class="page-header">
  <h1>User Management</h1>
  <div class="header-actions">
    <button class="btn btn-secondary" on:click={() => { showPasswordModal = true; passwordForm = { old_password: '', new_password: '', confirm_password: '' }; }}>
      🔑 Change Password
    </button>
    <button class="btn btn-primary" on:click={() => { showCreateModal = true; resetForm(); }}>
      + Add User
    </button>
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

{#if loading}
  <p>Loading...</p>
{:else}
  <div class="card">
    <table>
      <thead>
        <tr>
          <th>Username</th>
          <th>Role</th>
          <th>Created</th>
          <th>Actions</th>
        </tr>
      </thead>
      <tbody>
        {#each users as user}
          <tr>
            <td>
              <strong>{user.username}</strong>
              {#if currentUser && user.username === currentUser.username}
                <span class="badge badge-info" style="margin-left: 0.5rem; font-size: 0.65rem;">You</span>
              {/if}
            </td>
            <td>
              <span class="badge {getRoleBadgeClass(user.role)}">{user.role}</span>
            </td>
            <td>{formatDate(user.created_at)}</td>
            <td>
              <button 
                class="btn btn-danger" 
                on:click={() => handleDelete(user)}
                disabled={user.username === 'admin'}
                title={user.username === 'admin' ? 'Cannot delete the default admin account' : ''}
              >
                Delete
              </button>
            </td>
          </tr>
        {/each}
        {#if users.length === 0}
          <tr>
            <td colspan="4" class="no-data">No users found.</td>
          </tr>
        {/if}
      </tbody>
    </table>
  </div>

  <div class="card roles-info">
    <h3>Role Permissions</h3>
    <div class="roles-grid">
      <div class="role-card">
        <span class="badge badge-danger">Admin</span>
        <p>Full access to all features including user management, settings, and destructive operations.</p>
      </div>
      <div class="role-card">
        <span class="badge badge-success">Operator</span>
        <p>Can manage tapes, create and run backup jobs, perform restores, and view logs.</p>
      </div>
      <div class="role-card">
        <span class="badge badge-info">Read-only</span>
        <p>Can view tapes, jobs, and logs but cannot make any changes.</p>
      </div>
    </div>
  </div>
{/if}

<!-- Create Modal -->
{#if showCreateModal}
  <div class="modal-overlay" on:click={() => showCreateModal = false} role="presentation">
    <div class="modal" on:click|stopPropagation={() => {}} role="dialog" aria-modal="true" tabindex="-1">
      <h2>Add New User</h2>
      <form on:submit|preventDefault={handleCreate}>
        <div class="form-group">
          <label for="username">Username</label>
          <input type="text" id="username" bind:value={formData.username} required placeholder="Enter username" />
        </div>
        <div class="form-group">
          <label for="password">Password</label>
          <input type="password" id="password" bind:value={formData.password} required placeholder="Enter password" />
        </div>
        <div class="form-group">
          <label for="role">Role</label>
          <select id="role" bind:value={formData.role}>
            <option value="admin">Admin</option>
            <option value="operator">Operator</option>
            <option value="readonly">Read-only</option>
          </select>
        </div>
        <div class="modal-actions">
          <button type="button" class="btn btn-secondary" on:click={() => showCreateModal = false}>Cancel</button>
          <button type="submit" class="btn btn-primary">Create User</button>
        </div>
      </form>
    </div>
  </div>
{/if}

<!-- Change Password Modal -->
{#if showPasswordModal}
  <div class="modal-overlay" on:click={() => showPasswordModal = false} role="presentation">
    <div class="modal" on:click|stopPropagation={() => {}} role="dialog" aria-modal="true" tabindex="-1">
      <h2>Change Password</h2>
      <p class="modal-desc">Change the password for your account ({currentUser?.username}).</p>
      <form on:submit|preventDefault={handleChangePassword}>
        <div class="form-group">
          <label for="old-password">Current Password</label>
          <input type="password" id="old-password" bind:value={passwordForm.old_password} required placeholder="Enter current password" />
        </div>
        <div class="form-group">
          <label for="new-password">New Password</label>
          <input type="password" id="new-password" bind:value={passwordForm.new_password} required placeholder="Enter new password (min 6 chars)" />
        </div>
        <div class="form-group">
          <label for="confirm-password">Confirm New Password</label>
          <input type="password" id="confirm-password" bind:value={passwordForm.confirm_password} required placeholder="Confirm new password" />
        </div>
        <div class="modal-actions">
          <button type="button" class="btn btn-secondary" on:click={() => showPasswordModal = false}>Cancel</button>
          <button type="submit" class="btn btn-primary">Change Password</button>
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

  .no-data {
    text-align: center;
    color: #666;
    padding: 2rem;
  }

  .roles-info {
    margin-top: 1.5rem;
  }

  .roles-info h3 {
    margin: 0 0 1rem;
    font-size: 1rem;
  }

  .roles-grid {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
    gap: 1rem;
  }

  .role-card {
    padding: 1rem;
    background: #f9f9f9;
    border-radius: 8px;
  }

  .role-card .badge {
    margin-bottom: 0.5rem;
  }

  .role-card p {
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
    max-width: 400px;
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
</style>
