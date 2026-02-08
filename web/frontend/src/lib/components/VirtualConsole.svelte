<script lang="ts">
  import { consoleStore, type ConsoleEntry } from '$lib/stores/console';
  import { onMount, tick } from 'svelte';
  import { auth } from '$lib/stores/auth';

  let entries: ConsoleEntry[] = [];
  let isCollapsed = true;
  let consoleBody: HTMLElement;
  let autoScroll = true;

  const unsub1 = consoleStore.subscribe(v => {
    entries = v;
    if (autoScroll && consoleBody && !isCollapsed) {
      tick().then(() => {
        if (consoleBody) {
          consoleBody.scrollTop = consoleBody.scrollHeight;
        }
      });
    }
  });

  const unsub2 = consoleStore.collapsed.subscribe(v => {
    isCollapsed = v;
  });

  $: isAuthenticated = $auth.isAuthenticated;

  function getLevelClass(level: string): string {
    switch (level) {
      case 'error': return 'log-error';
      case 'warn': return 'log-warn';
      case 'success': return 'log-success';
      case 'debug': return 'log-debug';
      default: return 'log-info';
    }
  }

  function getLevelPrefix(level: string): string {
    switch (level) {
      case 'error': return 'ERR';
      case 'warn': return 'WRN';
      case 'success': return 'OK ';
      case 'debug': return 'DBG';
      default: return 'INF';
    }
  }

  function formatTime(ts: string): string {
    try {
      const d = new Date(ts);
      return d.toLocaleTimeString('en-US', { hour12: false });
    } catch {
      return '';
    }
  }

  function handleScroll() {
    if (!consoleBody) return;
    const { scrollTop, scrollHeight, clientHeight } = consoleBody;
    autoScroll = scrollHeight - scrollTop - clientHeight < 50;
  }
</script>

{#if isAuthenticated}
  <div class="virtual-console" class:collapsed={isCollapsed}>
    <div class="console-header" on:click={() => consoleStore.toggleCollapsed()}>
      <div class="console-title">
        <span class="console-icon">🖥️</span>
        <span>System Console</span>
        {#if entries.length > 0}
          <span class="entry-count">{entries.length}</span>
        {/if}
      </div>
      <div class="console-actions">
        {#if !isCollapsed}
          <button class="console-btn" on:click|stopPropagation={() => consoleStore.clear()} title="Clear console">🗑️</button>
        {/if}
        <span class="collapse-icon">{isCollapsed ? '▲' : '▼'}</span>
      </div>
    </div>
    {#if !isCollapsed}
      <div class="console-body" bind:this={consoleBody} on:scroll={handleScroll}>
        {#if entries.length === 0}
          <div class="console-empty">No log entries yet. System events will appear here.</div>
        {:else}
          {#each entries as entry (entry.id)}
            <div class="console-line {getLevelClass(entry.level)}">
              <span class="log-time">{formatTime(entry.timestamp)}</span>
              <span class="log-level">[{getLevelPrefix(entry.level)}]</span>
              {#if entry.source}
                <span class="log-source">[{entry.source}]</span>
              {/if}
              <span class="log-message">{entry.message}</span>
            </div>
          {/each}
        {/if}
      </div>
    {/if}
  </div>
{/if}

<style>
  .virtual-console {
    position: fixed;
    bottom: 0;
    left: 240px;
    right: 0;
    z-index: 900;
    background: #1e1e2e;
    border-top: 2px solid #4a4aff;
    font-family: 'Consolas', 'Monaco', 'Courier New', monospace;
    font-size: 0.8rem;
    transition: height 0.2s ease;
  }

  .virtual-console.collapsed {
    height: auto;
  }

  .console-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    padding: 0.4rem 1rem;
    background: #2d2d44;
    cursor: pointer;
    user-select: none;
    color: #e0e0e0;
  }

  .console-header:hover {
    background: #383854;
  }

  .console-title {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    font-weight: 600;
    font-size: 0.8rem;
  }

  .console-icon {
    font-size: 0.9rem;
  }

  .entry-count {
    background: #4a4aff;
    color: white;
    padding: 0.1rem 0.4rem;
    border-radius: 10px;
    font-size: 0.65rem;
    font-weight: 700;
  }

  .console-actions {
    display: flex;
    align-items: center;
    gap: 0.5rem;
  }

  .console-btn {
    background: none;
    border: none;
    color: #a0a0b0;
    cursor: pointer;
    padding: 0.15rem 0.3rem;
    border-radius: 4px;
    font-size: 0.85rem;
  }

  .console-btn:hover {
    background: #4a4a6a;
    color: white;
  }

  .collapse-icon {
    color: #a0a0b0;
    font-size: 0.7rem;
  }

  .console-body {
    height: 200px;
    overflow-y: auto;
    padding: 0.5rem;
    color: #ccc;
  }

  .console-body::-webkit-scrollbar {
    width: 8px;
  }

  .console-body::-webkit-scrollbar-track {
    background: #1e1e2e;
  }

  .console-body::-webkit-scrollbar-thumb {
    background: #4a4a6a;
    border-radius: 4px;
  }

  .console-empty {
    color: #666;
    padding: 1rem;
    text-align: center;
    font-style: italic;
  }

  .console-line {
    padding: 0.15rem 0.25rem;
    display: flex;
    gap: 0.5rem;
    white-space: nowrap;
  }

  .console-line:hover {
    background: rgba(255, 255, 255, 0.05);
  }

  .log-time {
    color: #666;
    flex-shrink: 0;
  }

  .log-level {
    font-weight: 700;
    flex-shrink: 0;
    width: 3.5em;
  }

  .log-source {
    color: #8888cc;
    flex-shrink: 0;
  }

  .log-message {
    white-space: pre-wrap;
    word-break: break-word;
  }

  .log-info .log-level { color: #5dade2; }
  .log-warn .log-level { color: #f39c12; }
  .log-error .log-level { color: #e74c3c; }
  .log-success .log-level { color: #2ecc71; }
  .log-debug .log-level { color: #999; }

  .log-error .log-message { color: #e74c3c; }
  .log-warn .log-message { color: #f39c12; }
  .log-success .log-message { color: #2ecc71; }
</style>
