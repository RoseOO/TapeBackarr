<script lang="ts">
  import { page } from '$app/stores';
  import { auth } from '$lib/stores/auth';

  const navItems = [
    { href: '/dashboard', label: 'Dashboard', icon: '📊' },
    { href: '/tapes', label: 'Tapes', icon: '💾' },
    { href: '/pools', label: 'Media Pools', icon: '🗂️' },
    { href: '/drives', label: 'Drives', icon: '🔌' },
    { href: '/jobs', label: 'Backup Jobs', icon: '📦' },
    { href: '/sources', label: 'Sources', icon: '📁' },
    { href: '/restore', label: 'Restore', icon: '🔄' },
    { href: '/logs', label: 'Logs', icon: '📋' },
    { href: '/docs', label: 'Documentation', icon: '📖' },
  ];

  $: currentPath = $page.url.pathname;
  $: isAdmin = $auth.user?.role === 'admin';

  function handleLogout() {
    auth.logout();
    window.location.href = '/login';
  }
</script>

<nav class="sidebar">
  <div class="logo">
    <h1>📼 TapeBackarr</h1>
  </div>

  <ul class="nav-items">
    {#each navItems as item}
      <li class:active={currentPath.startsWith(item.href)}>
        <a href={item.href}>
          <span class="icon">{item.icon}</span>
          <span class="label">{item.label}</span>
        </a>
      </li>
    {/each}
    {#if isAdmin}
      <li class:active={currentPath.startsWith('/users')}>
        <a href="/users">
          <span class="icon">👥</span>
          <span class="label">Users</span>
        </a>
      </li>
    {/if}
  </ul>

  <div class="user-section">
    {#if $auth.user}
      <div class="user-info">
        <span class="username">{$auth.user.username}</span>
        <span class="role">{$auth.user.role}</span>
      </div>
      <button class="logout-btn" on:click={handleLogout}>Logout</button>
    {/if}
  </div>
</nav>

<style>
  .sidebar {
    width: 240px;
    background: #1a1a2e;
    color: white;
    height: 100vh;
    display: flex;
    flex-direction: column;
    position: fixed;
    left: 0;
    top: 0;
  }

  .logo {
    padding: 1.5rem;
    border-bottom: 1px solid #2d2d44;
  }

  .logo h1 {
    margin: 0;
    font-size: 1.25rem;
    font-weight: 600;
  }

  .nav-items {
    list-style: none;
    padding: 1rem 0;
    margin: 0;
    flex: 1;
  }

  .nav-items li {
    margin: 0.25rem 0.5rem;
  }

  .nav-items a {
    display: flex;
    align-items: center;
    padding: 0.75rem 1rem;
    color: #a0a0b0;
    text-decoration: none;
    border-radius: 8px;
    transition: all 0.2s ease;
  }

  .nav-items a:hover {
    background: #2d2d44;
    color: white;
  }

  .nav-items li.active a {
    background: #4a4aff;
    color: white;
  }

  .icon {
    margin-right: 0.75rem;
    font-size: 1.1rem;
  }

  .user-section {
    padding: 1rem;
    border-top: 1px solid #2d2d44;
  }

  .user-info {
    display: flex;
    flex-direction: column;
    margin-bottom: 0.75rem;
  }

  .username {
    font-weight: 600;
  }

  .role {
    font-size: 0.75rem;
    color: #a0a0b0;
    text-transform: capitalize;
  }

  .logout-btn {
    width: 100%;
    padding: 0.5rem;
    background: #ff4a4a;
    color: white;
    border: none;
    border-radius: 6px;
    cursor: pointer;
    transition: background 0.2s ease;
  }

  .logout-btn:hover {
    background: #ff3333;
  }
</style>
