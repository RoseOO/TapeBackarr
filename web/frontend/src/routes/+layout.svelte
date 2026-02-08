<script lang="ts">
  import Sidebar from '$lib/components/Sidebar.svelte';
  import ToastNotifications from '$lib/components/ToastNotifications.svelte';
  import VirtualConsole from '$lib/components/VirtualConsole.svelte';
  import ProgressToolbar from '$lib/components/ProgressToolbar.svelte';
  import { page } from '$app/stores';
  import { auth } from '$lib/stores/auth';

  $: isLoginPage = $page.url.pathname === '/login';
  $: showSidebar = $auth.isAuthenticated && !isLoginPage;
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
  :global(body) {
    margin: 0;
    font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Oxygen, Ubuntu, Cantarell, sans-serif;
    background: #f5f6fa;
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
    background: white;
    border-radius: 12px;
    box-shadow: 0 2px 8px rgba(0, 0, 0, 0.08);
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
    background: #4a4aff;
    color: white;
  }

  :global(.btn-primary:hover) {
    background: #3a3aee;
  }

  :global(.btn-secondary) {
    background: #e0e0e0;
    color: #333;
  }

  :global(.btn-secondary:hover) {
    background: #d0d0d0;
  }

  :global(.btn-danger) {
    background: #ff4a4a;
    color: white;
  }

  :global(.btn-danger:hover) {
    background: #ee3a3a;
  }

  :global(.btn-success) {
    background: #2ecc71;
    color: white;
  }

  :global(.btn-success:hover) {
    background: #27ae60;
  }

  :global(table) {
    width: 100%;
    border-collapse: collapse;
  }

  :global(th), :global(td) {
    padding: 0.75rem 1rem;
    text-align: left;
    border-bottom: 1px solid #eee;
  }

  :global(th) {
    background: #f9f9f9;
    font-weight: 600;
    font-size: 0.875rem;
    color: #666;
  }

  :global(input), :global(select), :global(textarea) {
    width: 100%;
    padding: 0.5rem 0.75rem;
    border: 1px solid #ddd;
    border-radius: 6px;
    font-size: 0.875rem;
  }

  :global(input:focus), :global(select:focus), :global(textarea:focus) {
    outline: none;
    border-color: #4a4aff;
  }

  :global(.form-group) {
    margin-bottom: 1rem;
  }

  :global(.form-group label) {
    display: block;
    margin-bottom: 0.25rem;
    font-weight: 500;
    font-size: 0.875rem;
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
    background: #d4edda;
    color: #155724;
  }

  :global(.badge-warning) {
    background: #fff3cd;
    color: #856404;
  }

  :global(.badge-danger) {
    background: #f8d7da;
    color: #721c24;
  }

  :global(.badge-info) {
    background: #d1ecf1;
    color: #0c5460;
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
    color: #333;
  }
</style>
