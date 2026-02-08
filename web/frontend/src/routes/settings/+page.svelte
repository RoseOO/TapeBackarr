<script lang="ts">
  import { onMount } from 'svelte';
  import * as api from '$lib/api/client';

  let config: any = null;
  let loading = true;
  let saving = false;
  let error = '';
  let successMsg = '';
  let activeTab = 'server';
  let testingTelegram = false;

  // Database backup state
  let dbBackups: any[] = [];
  let dbBackupLoading = false;
  let dbBackupTapeId: number | null = null;
  let dbBackupTapes: any[] = [];
  let dbBackupRunning = false;
  let restarting = false;

  const tabs = [
    { id: 'server', label: 'Server', icon: '🖥️' },
    { id: 'tape', label: 'Tape & Drives', icon: '💾' },
    { id: 'dbbackup', label: 'Database Backup', icon: '🗄️' },
    { id: 'logging', label: 'Logging', icon: '📋' },
    { id: 'auth', label: 'Authentication', icon: '🔐' },
    { id: 'notifications', label: 'Notifications', icon: '🔔' },
    { id: 'proxmox', label: 'Proxmox', icon: '🖧' },
    { id: 'system', label: 'System', icon: '⚙️' },
  ];

  onMount(async () => {
    await loadConfig();
  });

  async function loadConfig() {
    loading = true;
    error = '';
    try {
      config = await api.getSettings();
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to load settings';
    } finally {
      loading = false;
    }
  }

  function showSuccess(msg: string) {
    successMsg = msg;
    setTimeout(() => successMsg = '', 4000);
  }

  async function loadDbBackups() {
    dbBackupLoading = true;
    try {
      const [backups, tapes] = await Promise.all([
        api.getDatabaseBackups(),
        api.getTapes(),
      ]);
      dbBackups = Array.isArray(backups) ? backups : [];
      dbBackupTapes = Array.isArray(tapes) ? tapes : [];
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to load database backups';
    } finally {
      dbBackupLoading = false;
    }
  }

  async function handleDbBackup() {
    if (!dbBackupTapeId) {
      error = 'Please select a tape for the database backup';
      return;
    }
    dbBackupRunning = true;
    error = '';
    try {
      await api.backupDatabaseToTape(dbBackupTapeId);
      showSuccess('Database backup started — check the system console for progress');
      await loadDbBackups();
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to start database backup';
    } finally {
      dbBackupRunning = false;
    }
  }

  function formatBytes(bytes: number): string {
    if (!bytes || bytes === 0) return '0 B';
    const k = 1024;
    const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
  }

  async function handleSave() {
    try {
      saving = true;
      error = '';
      await api.updateSettings(config);
      showSuccess('Settings saved. Some changes may require a restart to take effect.');
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to save settings';
    } finally {
      saving = false;
    }
  }

  async function handleTestTelegram() {
    try {
      testingTelegram = true;
      error = '';
      await api.testTelegramNotification();
      showSuccess('Test message sent successfully! Check your Telegram.');
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to send test message';
    } finally {
      testingTelegram = false;
    }
  }

  function addDrive() {
    if (!config.tape.drives) config.tape.drives = [];
    config.tape.drives = [...config.tape.drives, { device_path: '/dev/nst0', display_name: '', enabled: true }];
  }

  function removeDrive(index: number) {
    config.tape.drives = config.tape.drives.filter((_: any, i: number) => i !== index);
  }

  async function handleRestart() {
    if (!confirm('Are you sure you want to restart TapeBackarr? Active operations will be interrupted.')) return;
    restarting = true;
    error = '';
    try {
      await api.restartService();
      showSuccess('TapeBackarr is restarting... The page will reload shortly.');
      setTimeout(() => {
        window.location.reload();
      }, 5000);
    } catch (e) {
      error = e instanceof Error ? e.message : 'Failed to restart service';
    } finally {
      restarting = false;
    }
  }
</script>

<div class="page-header">
  <h1>Settings</h1>
  <button class="btn btn-primary" on:click={handleSave} disabled={saving || !config}>
    {saving ? 'Saving...' : '💾 Save Settings'}
  </button>
</div>

{#if error}
  <div class="card error-card">
    <p>{error}</p>
    <button class="btn btn-secondary" style="font-size: 0.75rem;" on:click={() => error = ''}>Dismiss</button>
  </div>
{/if}

{#if successMsg}
  <div class="card success-card">
    <p>{successMsg}</p>
  </div>
{/if}

{#if loading}
  <p>Loading...</p>
{:else if config}
  <div class="settings-layout">
    <div class="tab-list">
      {#each tabs as tab}
        <button
          class="tab-btn"
          class:active={activeTab === tab.id}
          on:click={() => activeTab = tab.id}
        >
          <span class="tab-icon">{tab.icon}</span>
          {tab.label}
        </button>
      {/each}
    </div>

    <div class="tab-content">
      {#if activeTab === 'server'}
        <div class="settings-section">
          <h2>Server Configuration</h2>
          <div class="form-group">
            <label for="server-host">Host</label>
            <input type="text" id="server-host" bind:value={config.server.host} />
            <small>IP address to bind to (0.0.0.0 for all interfaces)</small>
          </div>
          <div class="form-group">
            <label for="server-port">Port</label>
            <input type="number" id="server-port" bind:value={config.server.port} />
          </div>
          <div class="form-group">
            <label for="static-dir">Static Files Directory</label>
            <input type="text" id="static-dir" bind:value={config.server.static_dir} />
          </div>

          <h3>Database</h3>
          <div class="form-group">
            <label for="db-path">Database Path</label>
            <input type="text" id="db-path" bind:value={config.database.path} />
          </div>
        </div>

      {:else if activeTab === 'tape'}
        <div class="settings-section">
          <h2>Tape Configuration</h2>
          <div class="form-group">
            <label for="default-device">Default Device</label>
            <input type="text" id="default-device" bind:value={config.tape.default_device} />
            <small>Default tape device path (e.g., /dev/nst0)</small>
          </div>
          <div class="form-row">
            <div class="form-group">
              <label for="buffer-size">Buffer Size (MB)</label>
              <input type="number" id="buffer-size" bind:value={config.tape.buffer_size_mb} />
            </div>
            <div class="form-group">
              <label for="block-size">Block Size (bytes)</label>
              <input type="number" id="block-size" bind:value={config.tape.block_size} />
            </div>
            <div class="form-group">
              <label for="write-retries">Write Retries</label>
              <input type="number" id="write-retries" bind:value={config.tape.write_retries} />
            </div>
          </div>
          <div class="form-group checkbox-group">
            <label>
              <input type="checkbox" bind:checked={config.tape.verify_after_write} />
              Verify After Write
            </label>
            <small>Read back data after writing to verify integrity</small>
          </div>

          <h3>Drives</h3>
          {#if config.tape.drives}
            {#each config.tape.drives as drive, i}
              <div class="drive-row">
                <div class="form-group">
                  <label>Device Path</label>
                  <input type="text" bind:value={drive.device_path} placeholder="/dev/nst0" />
                </div>
                <div class="form-group">
                  <label>Display Name</label>
                  <input type="text" bind:value={drive.display_name} placeholder="Primary LTO Drive" />
                </div>
                <div class="form-group checkbox-group" style="align-self: end;">
                  <label>
                    <input type="checkbox" bind:checked={drive.enabled} />
                    Enabled
                  </label>
                </div>
                <button class="btn btn-danger btn-sm" on:click={() => removeDrive(i)}>✕</button>
              </div>
            {/each}
          {/if}
          <button class="btn btn-secondary" on:click={addDrive}>+ Add Drive</button>
        </div>

      {:else if activeTab === 'dbbackup'}
        <div class="settings-section">
          <h2>Database Backup to Tape</h2>
          <p class="section-desc">Back up the TapeBackarr database to tape for disaster recovery. The database contains all tape catalog entries, job configurations, and system settings.</p>

          <div class="db-backup-action">
            <h3>Run Database Backup</h3>
            {#if dbBackupTapes.length === 0 && !dbBackupLoading}
              <button class="btn btn-secondary" on:click={loadDbBackups}>Load Tapes</button>
            {:else}
              <div class="form-row">
                <div class="form-group">
                  <label for="db-tape">Target Tape</label>
                  <select id="db-tape" bind:value={dbBackupTapeId}>
                    <option value={null} disabled>Select a tape...</option>
                    {#each dbBackupTapes as tape}
                      <option value={tape.id}>{tape.label} ({tape.status})</option>
                    {/each}
                  </select>
                  <small>The tape must be loaded in a drive. The backup will be written to file position 1.</small>
                </div>
                <div class="form-group" style="align-self: end;">
                  <button class="btn btn-primary" on:click={handleDbBackup} disabled={dbBackupRunning || !dbBackupTapeId}>
                    {dbBackupRunning ? 'Backing up...' : '🗄️ Backup Database to Tape'}
                  </button>
                </div>
              </div>
            {/if}
          </div>

          <h3>Backup History</h3>
          {#if dbBackupLoading}
            <p>Loading...</p>
          {:else if dbBackups.length === 0}
            <p class="no-data-text">No database backups yet. Run a backup to protect your catalog data.</p>
            {#if dbBackupTapes.length === 0}
              <button class="btn btn-secondary" on:click={loadDbBackups}>Load Backup History</button>
            {/if}
          {:else}
            <table>
              <thead>
                <tr>
                  <th>ID</th>
                  <th>Tape</th>
                  <th>Time</th>
                  <th>Size</th>
                  <th>Checksum</th>
                  <th>Status</th>
                </tr>
              </thead>
              <tbody>
                {#each dbBackups as backup}
                  <tr>
                    <td>{backup.id}</td>
                    <td>{backup.tape_id}</td>
                    <td>{backup.backup_time ? new Date(backup.backup_time).toLocaleString() : '-'}</td>
                    <td>{formatBytes(backup.file_size || 0)}</td>
                    <td class="checksum-cell">{backup.checksum ? backup.checksum.substring(0, 12) + '...' : '-'}</td>
                    <td>
                      <span class="badge {backup.status === 'completed' ? 'badge-success' : backup.status === 'failed' ? 'badge-danger' : 'badge-warning'}">
                        {backup.status}
                      </span>
                    </td>
                  </tr>
                {/each}
              </tbody>
            </table>
          {/if}
        </div>

      {:else if activeTab === 'logging'}
        <div class="settings-section">
          <h2>Logging Configuration</h2>
          <div class="form-group">
            <label for="log-level">Log Level</label>
            <select id="log-level" bind:value={config.logging.level}>
              <option value="debug">Debug</option>
              <option value="info">Info</option>
              <option value="warn">Warning</option>
              <option value="error">Error</option>
            </select>
          </div>
          <div class="form-group">
            <label for="log-format">Format</label>
            <select id="log-format" bind:value={config.logging.format}>
              <option value="json">JSON</option>
              <option value="text">Text</option>
            </select>
          </div>
          <div class="form-group">
            <label for="log-path">Output Path</label>
            <input type="text" id="log-path" bind:value={config.logging.output_path} />
          </div>
        </div>

      {:else if activeTab === 'auth'}
        <div class="settings-section">
          <h2>Authentication Configuration</h2>
          <div class="form-group">
            <label for="jwt-secret">JWT Secret</label>
            <input type="password" id="jwt-secret" bind:value={config.auth.jwt_secret} placeholder="Leave as-is to keep current value" />
            <small>Secret key for JWT token signing. Value is masked — clear to remove, or type a new value to replace.</small>
          </div>
          <div class="form-row">
            <div class="form-group">
              <label for="token-exp">Token Expiration (hours)</label>
              <input type="number" id="token-exp" bind:value={config.auth.token_expiration} />
            </div>
            <div class="form-group">
              <label for="session-timeout">Session Timeout (minutes)</label>
              <input type="number" id="session-timeout" bind:value={config.auth.session_timeout} />
            </div>
          </div>
        </div>

      {:else if activeTab === 'notifications'}
        <div class="settings-section">
          <h2>Telegram Notifications</h2>
          <div class="form-group checkbox-group">
            <label>
              <input type="checkbox" bind:checked={config.notifications.telegram.enabled} />
              Enable Telegram Notifications
            </label>
          </div>
          {#if config.notifications.telegram.enabled}
            <div class="form-group">
              <label for="tg-token">Bot Token</label>
              <input type="password" id="tg-token" bind:value={config.notifications.telegram.bot_token} />
            </div>
            <div class="form-group">
              <label for="tg-chat">Chat ID</label>
              <input type="text" id="tg-chat" bind:value={config.notifications.telegram.chat_id} />
            </div>
            <div class="form-group">
              <button class="btn btn-secondary" on:click={handleTestTelegram} disabled={testingTelegram}>
                {testingTelegram ? 'Sending...' : '📨 Send Test Message'}
              </button>
            </div>
          {/if}

          <h2>Email Notifications</h2>
          <div class="form-group checkbox-group">
            <label>
              <input type="checkbox" bind:checked={config.notifications.email.enabled} />
              Enable Email Notifications
            </label>
          </div>
          {#if config.notifications.email.enabled}
            <div class="form-row">
              <div class="form-group">
                <label for="smtp-host">SMTP Host</label>
                <input type="text" id="smtp-host" bind:value={config.notifications.email.smtp_host} />
              </div>
              <div class="form-group">
                <label for="smtp-port">SMTP Port</label>
                <input type="number" id="smtp-port" bind:value={config.notifications.email.smtp_port} />
              </div>
            </div>
            <div class="form-row">
              <div class="form-group">
                <label for="smtp-user">Username</label>
                <input type="text" id="smtp-user" bind:value={config.notifications.email.username} />
              </div>
              <div class="form-group">
                <label for="smtp-pass">Password</label>
                <input type="password" id="smtp-pass" bind:value={config.notifications.email.password} />
              </div>
            </div>
            <div class="form-row">
              <div class="form-group">
                <label for="from-email">From Email</label>
                <input type="email" id="from-email" bind:value={config.notifications.email.from_email} />
              </div>
              <div class="form-group">
                <label for="from-name">From Name</label>
                <input type="text" id="from-name" bind:value={config.notifications.email.from_name} />
              </div>
            </div>
            <div class="form-group">
              <label for="to-emails">To Emails (comma-separated)</label>
              <input type="text" id="to-emails" bind:value={config.notifications.email.to_emails} />
            </div>
            <div class="form-row">
              <div class="form-group checkbox-group">
                <label>
                  <input type="checkbox" bind:checked={config.notifications.email.use_tls} />
                  Use TLS
                </label>
              </div>
              <div class="form-group checkbox-group">
                <label>
                  <input type="checkbox" bind:checked={config.notifications.email.skip_verify} />
                  Skip TLS Verification
                </label>
              </div>
            </div>
          {/if}
        </div>

      {:else if activeTab === 'proxmox'}
        <div class="settings-section">
          <h2>Proxmox VE Integration</h2>
          <div class="form-group checkbox-group">
            <label>
              <input type="checkbox" bind:checked={config.proxmox.enabled} />
              Enable Proxmox Integration
            </label>
          </div>
          {#if config.proxmox.enabled}
            <div class="form-row">
              <div class="form-group">
                <label for="px-host">Host</label>
                <input type="text" id="px-host" bind:value={config.proxmox.host} placeholder="192.168.1.100" />
              </div>
              <div class="form-group">
                <label for="px-port">Port</label>
                <input type="number" id="px-port" bind:value={config.proxmox.port} />
              </div>
            </div>
            <div class="form-group checkbox-group">
              <label>
                <input type="checkbox" bind:checked={config.proxmox.skip_tls_verify} />
                Skip TLS Verification
              </label>
            </div>

            <h3>Authentication (Username/Password)</h3>
            <div class="form-row">
              <div class="form-group">
                <label for="px-user">Username</label>
                <input type="text" id="px-user" bind:value={config.proxmox.username} placeholder="root" />
              </div>
              <div class="form-group">
                <label for="px-pass">Password</label>
                <input type="password" id="px-pass" bind:value={config.proxmox.password} />
              </div>
              <div class="form-group">
                <label for="px-realm">Realm</label>
                <input type="text" id="px-realm" bind:value={config.proxmox.realm} placeholder="pam" />
              </div>
            </div>

            <h3>Authentication (API Token - Recommended)</h3>
            <div class="form-row">
              <div class="form-group">
                <label for="px-token-id">Token ID</label>
                <input type="text" id="px-token-id" bind:value={config.proxmox.token_id} placeholder="user@realm!tokenname" />
              </div>
              <div class="form-group">
                <label for="px-token-secret">Token Secret</label>
                <input type="password" id="px-token-secret" bind:value={config.proxmox.token_secret} />
              </div>
            </div>

            <h3>Backup Settings</h3>
            <div class="form-row">
              <div class="form-group">
                <label for="px-mode">Default Mode</label>
                <select id="px-mode" bind:value={config.proxmox.default_mode}>
                  <option value="snapshot">Snapshot</option>
                  <option value="suspend">Suspend</option>
                  <option value="stop">Stop</option>
                </select>
              </div>
              <div class="form-group">
                <label for="px-compress">Compression</label>
                <select id="px-compress" bind:value={config.proxmox.default_compress}>
                  <option value="zstd">Zstandard</option>
                  <option value="lzo">LZO</option>
                  <option value="gzip">Gzip</option>
                  <option value="">None</option>
                </select>
              </div>
            </div>
            <div class="form-group">
              <label for="px-tmpdir">Temp Directory</label>
              <input type="text" id="px-tmpdir" bind:value={config.proxmox.temp_dir} />
            </div>
          {/if}
        </div>

      {:else if activeTab === 'system'}
        <div class="settings-section">
          <h2>System Management</h2>
          <p class="section-desc">Manage the TapeBackarr service. Use the restart button after making configuration changes that require a service restart.</p>
          
          <h3>Restart Service</h3>
          <p style="font-size: 0.875rem; color: #666; margin-bottom: 1rem;">
            Restart TapeBackarr to apply configuration changes. Active backup and restore operations will be interrupted.
          </p>
          <button class="btn btn-danger" on:click={handleRestart} disabled={restarting}>
            {restarting ? '🔄 Restarting...' : '🔄 Restart TapeBackarr'}
          </button>
        </div>
      {/if}
    </div>
  </div>
{/if}

<style>
  .error-card {
    background: #f8d7da;
    color: #721c24;
    display: flex;
    justify-content: space-between;
    align-items: center;
  }

  .success-card {
    background: var(--badge-success-bg);
    color: var(--badge-success-text);
    padding: 0.75rem 1rem;
    border-radius: 8px;
    margin-bottom: 1rem;
  }

  .settings-layout {
    display: grid;
    grid-template-columns: 200px 1fr;
    gap: 1.5rem;
  }

  .tab-list {
    display: flex;
    flex-direction: column;
    gap: 0.25rem;
  }

  .tab-btn {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    padding: 0.75rem 1rem;
    border: none;
    background: var(--bg-card);
    border-radius: 8px;
    cursor: pointer;
    text-align: left;
    font-size: 0.875rem;
    color: var(--text-secondary);
    transition: all 0.2s;
  }

  .tab-btn:hover {
    background: var(--bg-card-hover);
  }

  .tab-btn.active {
    background: #4a4aff;
    color: white;
  }

  .tab-icon {
    font-size: 1.1rem;
  }

  .tab-content {
    background: var(--bg-card);
    border-radius: 12px;
    padding: 2rem;
    box-shadow: var(--shadow);
  }

  .settings-section h2 {
    margin: 0 0 1.5rem;
    font-size: 1.25rem;
    color: var(--text-primary);
  }

  .settings-section h3 {
    margin: 1.5rem 0 1rem;
    font-size: 1rem;
    color: var(--text-secondary);
    padding-top: 1rem;
    border-top: 1px solid var(--border-color);
  }

  .form-row {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(150px, 1fr));
    gap: 1rem;
  }

  .drive-row {
    display: grid;
    grid-template-columns: 1fr 1fr auto auto;
    gap: 0.75rem;
    align-items: end;
    padding: 0.75rem;
    background: var(--bg-input);
    border-radius: 8px;
    margin-bottom: 0.5rem;
  }

  .btn-sm {
    padding: 0.25rem 0.5rem;
    font-size: 0.75rem;
  }

  .checkbox-group label {
    display: flex;
    align-items: center;
    gap: 0.5rem;
    cursor: pointer;
  }

  .checkbox-group input[type="checkbox"] {
    width: auto;
  }

  small {
    display: block;
    color: var(--text-muted);
    font-size: 0.75rem;
    margin-top: 0.25rem;
  }

  .section-desc {
    color: var(--text-secondary);
    font-size: 0.875rem;
    margin-bottom: 1.5rem;
  }

  .db-backup-action {
    margin-bottom: 2rem;
  }

  .no-data-text {
    color: var(--text-muted);
    font-style: italic;
    margin-bottom: 1rem;
  }

  .checksum-cell {
    font-family: monospace;
    font-size: 0.75rem;
    color: var(--text-muted);
  }
</style>
