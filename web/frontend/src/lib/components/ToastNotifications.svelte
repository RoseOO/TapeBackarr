<script lang="ts">
  import { notifications, type Notification } from '$lib/stores/notifications';
  import { consoleStore } from '$lib/stores/console';
  import { onMount, onDestroy } from 'svelte';

  let visibleToasts: Notification[] = [];
  let dismissTimers: Map<string, ReturnType<typeof setTimeout>> = new Map();

  const unsubscribe = notifications.subscribe(allNotifs => {
    // Show only the most recent unread notifications as toasts
    const newToasts = allNotifs
      .filter(n => !n.read)
      .slice(0, 5);
    
    // Add auto-dismiss timers for new toasts
    for (const toast of newToasts) {
      if (!dismissTimers.has(toast.id)) {
        const timer = setTimeout(() => {
          dismissToast(toast.id);
        }, 8000);
        dismissTimers.set(toast.id, timer);
      }
    }
    
    visibleToasts = newToasts;
  });

  onMount(() => {
    notifications.connect();
  });

  onDestroy(() => {
    unsubscribe();
    for (const timer of dismissTimers.values()) {
      clearTimeout(timer);
    }
  });

  function dismissToast(id: string) {
    notifications.markRead(id);
    const timer = dismissTimers.get(id);
    if (timer) {
      clearTimeout(timer);
      dismissTimers.delete(id);
    }
  }

  function getToastIcon(type: string): string {
    switch (type) {
      case 'success': return '✅';
      case 'warning': return '⚠️';
      case 'error': return '❌';
      default: return 'ℹ️';
    }
  }

  function getToastClass(type: string): string {
    switch (type) {
      case 'success': return 'toast-success';
      case 'warning': return 'toast-warning';
      case 'error': return 'toast-error';
      default: return 'toast-info';
    }
  }

  // Also pipe notifications to the console log, tracking which ones we've already logged
  let loggedNotificationIds = new Set<string>();

  function getConsoleLevel(type: string): 'error' | 'warn' | 'success' | 'info' {
    switch (type) {
      case 'error': return 'error';
      case 'warning': return 'warn';
      case 'success': return 'success';
      default: return 'info';
    }
  }

  $: {
    for (const n of visibleToasts) {
      if (!loggedNotificationIds.has(n.id)) {
        loggedNotificationIds.add(n.id);
        consoleStore.add({
          level: getConsoleLevel(n.type),
          message: `[${n.category}] ${n.title}: ${n.message}`,
          source: n.category,
        });
      }
    }
  }
</script>

{#if visibleToasts.length > 0}
  <div class="toast-container">
    {#each visibleToasts as toast (toast.id)}
      <div class="toast {getToastClass(toast.type)}" role="alert">
        <div class="toast-icon">{getToastIcon(toast.type)}</div>
        <div class="toast-content">
          <div class="toast-title">{toast.title}</div>
          <div class="toast-message">{toast.message}</div>
        </div>
        <button class="toast-close" on:click={() => dismissToast(toast.id)}>×</button>
      </div>
    {/each}
  </div>
{/if}

<style>
  .toast-container {
    position: fixed;
    top: 1rem;
    right: 1rem;
    z-index: 9999;
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
    max-width: 400px;
  }

  .toast {
    display: flex;
    align-items: flex-start;
    padding: 0.75rem 1rem;
    border-radius: 8px;
    box-shadow: 0 4px 12px rgba(0, 0, 0, 0.15);
    animation: slideIn 0.3s ease-out;
    backdrop-filter: blur(10px);
  }

  @keyframes slideIn {
    from {
      transform: translateX(100%);
      opacity: 0;
    }
    to {
      transform: translateX(0);
      opacity: 1;
    }
  }

  .toast-success { background: #d4edda; border-left: 4px solid #28a745; color: #155724; }
  .toast-warning { background: #fff3cd; border-left: 4px solid #ffc107; color: #856404; }
  .toast-error { background: #f8d7da; border-left: 4px solid #dc3545; color: #721c24; }
  .toast-info { background: #d1ecf1; border-left: 4px solid #17a2b8; color: #0c5460; }

  .toast-icon {
    margin-right: 0.75rem;
    font-size: 1.1rem;
    flex-shrink: 0;
  }

  .toast-content {
    flex: 1;
    min-width: 0;
  }

  .toast-title {
    font-weight: 600;
    font-size: 0.875rem;
    margin-bottom: 0.15rem;
  }

  .toast-message {
    font-size: 0.8rem;
    opacity: 0.9;
    word-break: break-word;
  }

  .toast-close {
    background: none;
    border: none;
    font-size: 1.2rem;
    cursor: pointer;
    color: inherit;
    opacity: 0.6;
    padding: 0 0.25rem;
    margin-left: 0.5rem;
    flex-shrink: 0;
  }

  .toast-close:hover {
    opacity: 1;
  }
</style>
