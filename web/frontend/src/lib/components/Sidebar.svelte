<script lang="ts">
  import { page } from '$app/stores';
  import { auth } from '$lib/stores/auth';
  import { theme } from '$lib/stores/theme';

  const navSections = [
    {
      label: 'Overview',
      items: [
        { href: '/dashboard', label: 'Dashboard', icon: '📊' },
      ],
    },
    {
      label: 'Storage',
      items: [
        { href: '/tapes', label: 'Tapes', icon: '💾' },
        { href: '/pools', label: 'Media Pools', icon: '🗂️' },
        { href: '/drives', label: 'Drives', icon: '🔌' },
        { href: '/inspect', label: 'Tape Inspector', icon: '🔍' },
        { href: '/libraries', label: 'Tape Libraries', icon: '🏗️' },
      ],
    },
    {
      label: 'Backup & Restore',
      items: [
        { href: '/jobs', label: 'Backup Jobs', icon: '📦' },
        { href: '/sources', label: 'Sources', icon: '📁' },
        { href: '/restore', label: 'Restore', icon: '🔄' },
      ],
    },
    {
      label: 'Monitoring',
      items: [
        { href: '/logs', label: 'Logs', icon: '📋' },
      ],
    },
    {
      label: 'Security',
      items: [
        { href: '/encryption', label: 'Encryption', icon: '🔒' },
        { href: '/api-keys', label: 'API Keys', icon: '🔑' },
      ],
    },
    {
      label: 'Infrastructure',
      items: [
        { href: '/proxmox', label: 'Proxmox', icon: '🖥️' },
      ],
    },
    {
      label: 'System',
      items: [
        { href: '/settings', label: 'Settings', icon: '⚙️' },
        { href: '/docs', label: 'Documentation', icon: '📖' },
      ],
    },
  ];

  $: currentPath = $page.url.pathname;
  $: isAdmin = $auth.user?.role === 'admin';

  let mobileOpen = false;

  function handleLogout() {
    auth.logout();
    window.location.href = '/login';
  }

  function closeMobile() {
    mobileOpen = false;
  }
</script>

<button class="mobile-toggle" on:click={() => mobileOpen = !mobileOpen} aria-label="Toggle menu">
  {mobileOpen ? '✕' : '☰'}
</button>

{#if mobileOpen}
  <div class="sidebar-overlay" on:click={closeMobile}></div>
{/if}

<nav class="sidebar" class:open={mobileOpen}>
  <div class="logo">
    <h1>📼 TapeBackarr</h1>
  </div>

  <ul class="nav-items">
    {#each navSections as section}
      <li class="nav-section-label">{section.label}</li>
      {#each section.items as item}
        <li class:active={currentPath.startsWith(item.href)}>
          <a href={item.href} on:click={closeMobile}>
            <span class="icon">{item.icon}</span>
            <span class="label">{item.label}</span>
          </a>
        </li>
      {/each}
    {/each}
    {#if isAdmin}
      <li class="nav-section-label">Admin</li>
      <li class:active={currentPath.startsWith('/users')}>
        <a href="/users" on:click={closeMobile}>
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
      <button class="theme-btn" on:click={theme.toggle}>
        {$theme === 'dark' ? '☀️ Light Mode' : '🌙 Dark Mode'}
      </button>
      <button class="logout-btn" on:click={handleLogout}>Logout</button>
    {/if}
  </div>
</nav>

<style>
  .mobile-toggle {
    display: none;
    position: fixed;
    top: 0.75rem;
    left: 0.75rem;
    z-index: 1100;
    background: #1a1a2e;
    color: white;
    border: none;
    border-radius: 8px;
    width: 40px;
    height: 40px;
    font-size: 1.25rem;
    cursor: pointer;
    align-items: center;
    justify-content: center;
  }

  .sidebar-overlay {
    display: none;
  }

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
    z-index: 1050;
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
    padding: 0.5rem 0;
    margin: 0;
    flex: 1;
    overflow-y: auto;
  }

  .nav-section-label {
    padding: 0.5rem 1.25rem 0.25rem;
    font-size: 0.65rem;
    font-weight: 700;
    text-transform: uppercase;
    letter-spacing: 0.08em;
    color: #5a5a7a;
    margin-top: 0.25rem;
  }

  .nav-items li {
    margin: 0.125rem 0.5rem;
  }

  .nav-items li.nav-section-label {
    margin: 0;
  }

  .nav-items a {
    display: flex;
    align-items: center;
    padding: 0.6rem 1rem;
    color: #a0a0b0;
    text-decoration: none;
    border-radius: 8px;
    transition: all 0.2s ease;
    font-size: 0.875rem;
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

  .theme-btn {
    width: 100%;
    padding: 0.5rem;
    background: var(--bg-input, #2d2d44);
    color: var(--text-secondary, #a0a0b0);
    border: 1px solid var(--border-color, #3d3d54);
    border-radius: 6px;
    cursor: pointer;
    transition: background 0.2s ease;
    margin-bottom: 0.5rem;
    font-size: 0.8rem;
  }

  .theme-btn:hover {
    background: var(--bg-card-hover, #3d3d54);
    color: var(--text-primary, white);
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

  @media (max-width: 768px) {
    .mobile-toggle {
      display: flex;
    }

    .sidebar-overlay {
      display: block;
      position: fixed;
      top: 0;
      left: 0;
      right: 0;
      bottom: 0;
      background: rgba(0, 0, 0, 0.5);
      z-index: 1040;
    }

    .sidebar {
      transform: translateX(-100%);
      transition: transform 0.3s ease;
    }

    .sidebar.open {
      transform: translateX(0);
    }
  }
</style>
