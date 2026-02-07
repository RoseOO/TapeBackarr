<script lang="ts">
  import { goto } from '$app/navigation';
  import { auth } from '$lib/stores/auth';
  import * as api from '$lib/api/client';

  let username = '';
  let password = '';
  let error = '';
  let loading = false;

  async function handleLogin() {
    if (!username || !password) {
      error = 'Please enter username and password';
      return;
    }

    loading = true;
    error = '';

    try {
      const result = await api.login(username, password);
      auth.login(result.token, result.user);
      goto('/dashboard');
    } catch (e) {
      error = e instanceof Error ? e.message : 'Login failed';
    } finally {
      loading = false;
    }
  }
</script>

<div class="login-container">
  <div class="login-card">
    <div class="logo">
      <h1>📼 TapeBackarr</h1>
      <p>Tape Library Management System</p>
    </div>

    <form on:submit|preventDefault={handleLogin}>
      {#if error}
        <div class="error-message">{error}</div>
      {/if}

      <div class="form-group">
        <label for="username">Username</label>
        <input
          type="text"
          id="username"
          bind:value={username}
          placeholder="Enter username"
          disabled={loading}
        />
      </div>

      <div class="form-group">
        <label for="password">Password</label>
        <input
          type="password"
          id="password"
          bind:value={password}
          placeholder="Enter password"
          disabled={loading}
        />
      </div>

      <button type="submit" class="btn btn-primary login-btn" disabled={loading}>
        {loading ? 'Logging in...' : 'Login'}
      </button>
    </form>

    <div class="default-creds">
      <p>Default credentials: <strong>admin</strong> / <strong>changeme</strong></p>
    </div>
  </div>
</div>

<style>
  .login-container {
    display: flex;
    justify-content: center;
    align-items: center;
    min-height: 100vh;
    background: linear-gradient(135deg, #1a1a2e 0%, #16213e 100%);
  }

  .login-card {
    background: white;
    padding: 2.5rem;
    border-radius: 16px;
    box-shadow: 0 10px 40px rgba(0, 0, 0, 0.2);
    width: 100%;
    max-width: 400px;
  }

  .logo {
    text-align: center;
    margin-bottom: 2rem;
  }

  .logo h1 {
    margin: 0;
    font-size: 1.75rem;
    color: #1a1a2e;
  }

  .logo p {
    margin: 0.5rem 0 0;
    color: #666;
    font-size: 0.875rem;
  }

  .error-message {
    background: #f8d7da;
    color: #721c24;
    padding: 0.75rem;
    border-radius: 6px;
    margin-bottom: 1rem;
    font-size: 0.875rem;
  }

  .login-btn {
    width: 100%;
    padding: 0.75rem;
    font-size: 1rem;
    margin-top: 0.5rem;
  }

  .default-creds {
    margin-top: 1.5rem;
    padding-top: 1rem;
    border-top: 1px solid #eee;
    text-align: center;
  }

  .default-creds p {
    margin: 0;
    font-size: 0.75rem;
    color: #999;
  }
</style>
