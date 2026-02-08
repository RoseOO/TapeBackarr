<script lang="ts">
  import { onMount } from 'svelte';
  import * as api from '$lib/api/client';

  interface AuditLog {
    id: number;
    user_id: number;
    username: string | null;
    action: string;
    resource_type: string;
    resource_id: number;
    details: string;
    ip_address: string;
    created_at: string;
  }

  let logs: AuditLog[] = [];
  let loading = true;
  let error = '';
  let limit = 100;
  let offset = 0;
  let hasMore = true;

  onMount(async () => {
    await loadLogs();
  });

  async function loadLogs() {
    loading = true;
    try {
      const result = await api.getAuditLogs(limit, offset);
      logs = result || [];
      hasMore = logs.length === limit;
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to load logs';
    } finally {
      loading = false;
    }
  }

  async function loadMore() {
    offset += limit;
    try {
      const result = await api.getAuditLogs(limit, offset);
      const moreLogs = result || [];
      logs = [...logs, ...moreLogs];
      hasMore = moreLogs.length === limit;
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to load more logs';
    }
  }

  function formatDate(dateStr: string): string {
    return new Date(dateStr).toLocaleString();
  }

  function getActionBadgeClass(action: string): string {
    if (action.includes('delete')) return 'badge-danger';
    if (action.includes('create')) return 'badge-success';
    if (action.includes('update')) return 'badge-warning';
    if (action.includes('login')) return 'badge-info';
    return '';
  }
</script>

<div class="page-header">
  <h1>Audit Logs</h1>
</div>

{#if error}
  <div class="card error-card">
    <p>{error}</p>
    <button class="btn btn-secondary" on:click={() => error = ''}>Dismiss</button>
  </div>
{/if}

{#if loading}
  <p>Loading...</p>
{:else}
  <div class="card">
    <table>
      <thead>
        <tr>
          <th>Time</th>
          <th>User</th>
          <th>Action</th>
          <th>Resource</th>
          <th>Details</th>
          <th>IP Address</th>
        </tr>
      </thead>
      <tbody>
        {#each logs as log}
          <tr>
            <td>{formatDate(log.created_at)}</td>
            <td>{log.username || '-'}</td>
            <td>
              <span class="badge {getActionBadgeClass(log.action)}">{log.action}</span>
            </td>
            <td>{log.resource_type} #{log.resource_id}</td>
            <td class="details-cell">{log.details || '-'}</td>
            <td>{log.ip_address || '-'}</td>
          </tr>
        {/each}
        {#if logs.length === 0}
          <tr>
            <td colspan="6" class="no-data">No audit logs found.</td>
          </tr>
        {/if}
      </tbody>
    </table>
  </div>

  {#if hasMore && logs.length > 0}
    <div class="load-more">
      <button class="btn btn-secondary" on:click={loadMore}>Load More</button>
    </div>
  {/if}
{/if}

<style>
  .error-card {
    background: #f8d7da;
    color: #721c24;
    display: flex;
    justify-content: space-between;
    align-items: center;
  }

  .no-data {
    text-align: center;
    color: #666;
    padding: 2rem;
  }

  .details-cell {
    max-width: 300px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-size: 0.875rem;
    color: #666;
  }

  .load-more {
    text-align: center;
    margin-top: 1rem;
  }
</style>
