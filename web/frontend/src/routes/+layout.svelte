<script lang="ts">
  import Sidebar from '$lib/components/Sidebar.svelte';
  import ToastNotifications from '$lib/components/ToastNotifications.svelte';
  import VirtualConsole from '$lib/components/VirtualConsole.svelte';
  import ProgressToolbar from '$lib/components/ProgressToolbar.svelte';
  import { page } from '$app/stores';
  import { auth } from '$lib/stores/auth';
  import { theme } from '$lib/stores/theme';
  import { onMount } from 'svelte';

  $: isLoginPage = $page.url.pathname === '/login';
  $: showSidebar = $auth.isAuthenticated && !isLoginPage;

  onMount(() => {
    theme.init();
  });
</script>

<svelte:head>
  <title>TapeBackarr</title>
</svelte:head>

<div class="app" class:with-sidebar={showSidebar}>
  {#if showSidebar}
    <Sidebar />
  {/if}
  <main class:full-width={!showSidebar}>
    <slot />
  </main>
  {#if showSidebar}
    <ToastNotifications />
    <ProgressToolbar />
    <VirtualConsole />
  {/if}
</div>

<style>
  :global(:root),
  :global([data-theme="dark"]) {
    --bg-primary: #0f0f1a;
    --bg-secondary: #1a1a2e;
    --bg-card: #1e1e32;
    --bg-card-hover: #252540;
    --bg-input: #2a2a44;
    --border-color: #2d2d44;
    --text-primary: #e0e0f0;
    --text-secondary: #a0a0b8;
    --text-muted: #666680;
    --accent-primary: #4a4aff;
    --accent-primary-hover: #3a3aee;
    --accent-success: #2ecc71;
    --accent-success-hover: #27ae60;
    --accent-danger: #ff4a4a;
    --accent-danger-hover: #ee3a3a;
    --accent-warning: #f39c12;
    --table-header-bg: #1a1a2e;
    --table-border: #2d2d44;
    --badge-success-bg: #1a3a2a;
    --badge-success-text: #2ecc71;
    --badge-warning-bg: #3a2a1a;
    --badge-warning-text: #f39c12;
    --badge-danger-bg: #3a1a1a;
    --badge-danger-text: #ff4a4a;
    --badge-info-bg: #1a2a3a;
    --badge-info-text: #5dade2;
    --shadow: 0 2px 8px rgba(0, 0, 0, 0.3);
    --code-bg: #2a2a44;
  }

  :global([data-theme="light"]) {
    --bg-primary: #f5f6fa;
    --bg-secondary: #ffffff;
    --bg-card: #ffffff;
    --bg-card-hover: #f9f9f9;
    --bg-input: #ffffff;
    --border-color: #e0e0e0;
    --text-primary: #333333;
    --text-secondary: #666666;
    --text-muted: #999999;
    --accent-primary: #4a4aff;
    --accent-primary-hover: #3a3aee;
    --accent-success: #2ecc71;
    --accent-success-hover: #27ae60;
    --accent-danger: #ff4a4a;
    --accent-danger-hover: #ee3a3a;
    --accent-warning: #f39c12;
    --table-header-bg: #f9f9f9;
    --table-border: #eeeeee;
    --badge-success-bg: #d4edda;
    --badge-success-text: #155724;
    --badge-warning-bg: #fff3cd;
    --badge-warning-text: #856404;
    --badge-danger-bg: #f8d7da;
    --badge-danger-text: #721c24;
    --badge-info-bg: #d1ecf1;
    --badge-info-text: #0c5460;
    --shadow: 0 2px 8px rgba(0, 0, 0, 0.08);
    --code-bg: #f0f0f0;
  }

  :global(body) {
    margin: 0;
    font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Oxygen, Ubuntu, Cantarell, sans-serif;
    background: var(--bg-primary);
    color: var(--text-primary);
  }

  :global(*) {
    box-sizing: border-box;
  }

  .app {
    min-height: 100vh;
  }

  .app.with-sidebar main {
    margin-left: 240px;
  }

  main {
    padding: 2rem;
    padding-bottom: 3rem;
    min-height: 100vh;
  }

  main.full-width {
    margin-left: 0;
  }

  :global(.card) {
    background: var(--bg-card);
    border-radius: 12px;
    box-shadow: var(--shadow);
    padding: 1.5rem;
    margin-bottom: 1.5rem;
  }

  :global(.btn) {
    padding: 0.5rem 1rem;
    border: none;
    border-radius: 6px;
    cursor: pointer;
    font-size: 0.875rem;
    font-weight: 500;
    transition: all 0.2s ease;
  }

  :global(.btn-primary) {
    background: var(--accent-primary);
    color: white;
  }

  :global(.btn-primary:hover) {
    background: var(--accent-primary-hover);
  }

  :global(.btn-secondary) {
    background: var(--bg-input);
    color: var(--text-primary);
    border: 1px solid var(--border-color);
  }

  :global(.btn-secondary:hover) {
    background: var(--bg-card-hover);
  }

  :global(.btn-danger) {
    background: var(--accent-danger);
    color: white;
  }

  :global(.btn-danger:hover) {
    background: var(--accent-danger-hover);
  }

  :global(.btn-success) {
    background: var(--accent-success);
    color: white;
  }

  :global(.btn-success:hover) {
    background: var(--accent-success-hover);
  }

  :global(table) {
    width: 100%;
    border-collapse: collapse;
  }

  :global(th), :global(td) {
    padding: 0.75rem 1rem;
    text-align: left;
    border-bottom: 1px solid var(--table-border);
  }

  :global(th) {
    background: var(--table-header-bg);
    font-weight: 600;
    font-size: 0.875rem;
    color: var(--text-secondary);
  }

  :global(input), :global(select), :global(textarea) {
    width: 100%;
    padding: 0.5rem 0.75rem;
    border: 1px solid var(--border-color);
    border-radius: 6px;
    font-size: 0.875rem;
    background: var(--bg-input);
    color: var(--text-primary);
  }

  :global(input:focus), :global(select:focus), :global(textarea:focus) {
    outline: none;
    border-color: var(--accent-primary);
  }

  :global(.form-group) {
    margin-bottom: 1rem;
  }

  :global(.form-group label) {
    display: block;
    margin-bottom: 0.25rem;
    font-weight: 500;
    font-size: 0.875rem;
    color: var(--text-primary);
  }

  :global(.badge) {
    display: inline-block;
    padding: 0.25rem 0.5rem;
    border-radius: 4px;
    font-size: 0.75rem;
    font-weight: 600;
    text-transform: uppercase;
  }

  :global(.badge-success) {
    background: var(--badge-success-bg);
    color: var(--badge-success-text);
  }

  :global(.badge-warning) {
    background: var(--badge-warning-bg);
    color: var(--badge-warning-text);
  }

  :global(.badge-danger) {
    background: var(--badge-danger-bg);
    color: var(--badge-danger-text);
  }

  :global(.badge-info) {
    background: var(--badge-info-bg);
    color: var(--badge-info-text);
  }

  :global(.page-header) {
    display: flex;
    justify-content: space-between;
    align-items: center;
    margin-bottom: 1.5rem;
  }

  :global(.page-header h1) {
    margin: 0;
    font-size: 1.5rem;
    color: var(--text-primary);
  }
</style>
