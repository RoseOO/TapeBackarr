const API_BASE = '/api/v1';

async function fetchApi(endpoint: string, options: RequestInit = {}) {
  const token = typeof window !== 'undefined' ? localStorage.getItem('token') : null;
  
  const headers: HeadersInit = {
    'Content-Type': 'application/json',
    ...options.headers,
  };

  if (token) {
    (headers as Record<string, string>)['Authorization'] = `Bearer ${token}`;
  }

  const response = await fetch(`${API_BASE}${endpoint}`, {
    ...options,
    headers,
  });

  if (response.status === 401) {
    if (typeof window !== 'undefined') {
      localStorage.removeItem('token');
      localStorage.removeItem('user');
      window.location.href = '/login';
    }
    throw new Error('Unauthorized');
  }

  if (!response.ok) {
    const error = await response.json().catch(() => ({ error: 'Request failed' }));
    throw new Error(error.error || 'Request failed');
  }

  return response.json();
}

// Auth
export async function login(username: string, password: string) {
  return fetchApi('/auth/login', {
    method: 'POST',
    body: JSON.stringify({ username, password }),
  });
}

// Dashboard
export async function getDashboard() {
  return fetchApi('/dashboard');
}

// Tapes
export async function getTapes() {
  return fetchApi('/tapes');
}

export async function getTape(id: number) {
  return fetchApi(`/tapes/${id}`);
}

export async function createTape(data: { barcode: string; label: string; pool_id?: number; capacity_bytes: number; drive_id?: number; write_label?: boolean; auto_eject?: boolean }) {
  return fetchApi('/tapes', {
    method: 'POST',
    body: JSON.stringify(data),
  });
}

export async function updateTape(id: number, data: { label?: string; pool_id?: number; status?: string; offsite_location?: string }) {
  return fetchApi(`/tapes/${id}`, {
    method: 'PUT',
    body: JSON.stringify(data),
  });
}

export async function deleteTape(id: number) {
  return fetchApi(`/tapes/${id}`, {
    method: 'DELETE',
  });
}

export async function labelTape(id: number, label: string, driveId?: number, force?: boolean, autoEject?: boolean) {
  return fetchApi(`/tapes/${id}/label`, {
    method: 'POST',
    body: JSON.stringify({ label, drive_id: driveId, force, auto_eject: autoEject }),
  });
}

// Pools
export async function getPools() {
  return fetchApi('/pools');
}

export async function createPool(data: { name: string; description: string; retention_days: number }) {
  return fetchApi('/pools', {
    method: 'POST',
    body: JSON.stringify(data),
  });
}

export async function updatePool(id: number, data: { name?: string; description?: string; retention_days?: number }) {
  return fetchApi(`/pools/${id}`, {
    method: 'PUT',
    body: JSON.stringify(data),
  });
}

export async function deletePool(id: number) {
  return fetchApi(`/pools/${id}`, {
    method: 'DELETE',
  });
}

// Drives
export async function getDrives() {
  return fetchApi('/drives');
}

export async function getDriveStatus(id: number) {
  return fetchApi(`/drives/${id}/status`);
}

export async function ejectTape(driveId: number) {
  return fetchApi(`/drives/${driveId}/eject`, {
    method: 'POST',
  });
}

export async function rewindTape(driveId: number) {
  return fetchApi(`/drives/${driveId}/rewind`, {
    method: 'POST',
  });
}

// Tape format/erase
export async function formatTape(id: number, driveId: number, confirm: boolean) {
  return fetchApi(`/tapes/${id}/format`, {
    method: 'POST',
    body: JSON.stringify({ drive_id: driveId, confirm }),
  });
}

// Tape export (mark offsite)
export async function exportTape(id: number, offsiteLocation: string) {
  return fetchApi(`/tapes/${id}/export`, {
    method: 'POST',
    body: JSON.stringify({ offsite_location: offsiteLocation }),
  });
}

// Tape import (bring back from offsite)
export async function importTape(id: number, driveId?: number) {
  return fetchApi(`/tapes/${id}/import`, {
    method: 'POST',
    body: JSON.stringify({ drive_id: driveId }),
  });
}

// Read tape label from physical tape
export async function readTapeLabel(id: number, driveId: number) {
  return fetchApi(`/tapes/${id}/read-label?drive_id=${driveId}`);
}

// Scan for available tape drives
export async function scanDrives() {
  return fetchApi('/drives/scan');
}

// Sources
export async function getSources() {
  return fetchApi('/sources');
}

export async function getSource(id: number) {
  return fetchApi(`/sources/${id}`);
}

export async function createSource(data: { name: string; source_type: string; path: string; include_patterns?: string[]; exclude_patterns?: string[] }) {
  return fetchApi('/sources', {
    method: 'POST',
    body: JSON.stringify(data),
  });
}

export async function updateSource(id: number, data: { name?: string; path?: string; include_patterns?: string[]; exclude_patterns?: string[]; enabled?: boolean }) {
  return fetchApi(`/sources/${id}`, {
    method: 'PUT',
    body: JSON.stringify(data),
  });
}

export async function deleteSource(id: number) {
  return fetchApi(`/sources/${id}`, {
    method: 'DELETE',
  });
}

// Jobs
export async function getJobs() {
  return fetchApi('/jobs');
}

export async function getJob(id: number) {
  return fetchApi(`/jobs/${id}`);
}

export async function createJob(data: { name: string; source_id: number; pool_id: number; backup_type: string; schedule_cron?: string; retention_days: number }) {
  return fetchApi('/jobs', {
    method: 'POST',
    body: JSON.stringify(data),
  });
}

export async function updateJob(id: number, data: { name?: string; source_id?: number; pool_id?: number; backup_type?: string; schedule_cron?: string; retention_days?: number; enabled?: boolean }) {
  return fetchApi(`/jobs/${id}`, {
    method: 'PUT',
    body: JSON.stringify(data),
  });
}

export async function deleteJob(id: number) {
  return fetchApi(`/jobs/${id}`, {
    method: 'DELETE',
  });
}

export async function runJob(id: number, tapeId: number, backupType?: string) {
  return fetchApi(`/jobs/${id}/run`, {
    method: 'POST',
    body: JSON.stringify({ tape_id: tapeId, backup_type: backupType }),
  });
}

// Backup Sets
export async function getBackupSets(jobId?: number) {
  const params = jobId ? `?job_id=${jobId}` : '';
  return fetchApi(`/backup-sets${params}`);
}

export async function getBackupSet(id: number) {
  return fetchApi(`/backup-sets/${id}`);
}

export async function getBackupFiles(id: number, prefix?: string) {
  const params = prefix ? `?prefix=${encodeURIComponent(prefix)}` : '';
  return fetchApi(`/backup-sets/${id}/files${params}`);
}

// Catalog
export async function searchCatalog(query: string) {
  return fetchApi(`/catalog/search?q=${encodeURIComponent(query)}`);
}

export async function browseCatalog(backupSetId: number, prefix?: string) {
  const params = prefix ? `?prefix=${encodeURIComponent(prefix)}` : '';
  return fetchApi(`/catalog/browse/${backupSetId}${params}`);
}

// Restore
export async function getRestorePlan(backupSetId: number, filePaths?: string[], destPath?: string) {
  return fetchApi('/restore/plan', {
    method: 'POST',
    body: JSON.stringify({ backup_set_id: backupSetId, file_paths: filePaths, dest_path: destPath }),
  });
}

export async function runRestore(data: { backup_set_id: number; file_paths?: string[]; dest_path: string; verify?: boolean; overwrite?: boolean }) {
  return fetchApi('/restore/run', {
    method: 'POST',
    body: JSON.stringify(data),
  });
}

// Logs
export async function getAuditLogs(limit: number = 100, offset: number = 0) {
  return fetchApi(`/logs/audit?limit=${limit}&offset=${offset}`);
}

export async function exportLogs(startDate?: string, endDate?: string) {
  let params = '';
  if (startDate) params += `start=${startDate}`;
  if (endDate) params += `${params ? '&' : ''}end=${endDate}`;
  return fetchApi(`/logs/export${params ? '?' + params : ''}`);
}

// Users
export async function getUsers() {
  return fetchApi('/users');
}

export async function createUser(data: { username: string; password: string; role: string }) {
  return fetchApi('/users', {
    method: 'POST',
    body: JSON.stringify(data),
  });
}

export async function deleteUser(id: number) {
  return fetchApi(`/users/${id}`, {
    method: 'DELETE',
  });
}

// Change password
export async function changePassword(oldPassword: string, newPassword: string) {
  return fetchApi('/auth/change-password', {
    method: 'POST',
    body: JSON.stringify({ old_password: oldPassword, new_password: newPassword }),
  });
}

// Encryption Keys
export async function getEncryptionKeys() {
  return fetchApi('/encryption-keys');
}

export async function createEncryptionKey(data: { name: string; description?: string }) {
  return fetchApi('/encryption-keys', {
    method: 'POST',
    body: JSON.stringify(data),
  });
}

export async function importEncryptionKey(data: { name: string; key_base64: string; description?: string }) {
  return fetchApi('/encryption-keys/import', {
    method: 'POST',
    body: JSON.stringify(data),
  });
}

export async function deleteEncryptionKey(id: number) {
  return fetchApi(`/encryption-keys/${id}`, {
    method: 'DELETE',
  });
}

export async function getKeySheet() {
  return fetchApi('/encryption-keys/keysheet');
}

export async function getKeySheetText() {
  const token = typeof window !== 'undefined' ? localStorage.getItem('token') : null;
  const headers: HeadersInit = {};
  if (token) {
    headers['Authorization'] = `Bearer ${token}`;
  }
  const response = await fetch(`${API_BASE}/encryption-keys/keysheet/text`, { headers });
  if (!response.ok) {
    const error = await response.json().catch(() => ({ error: 'Request failed' }));
    throw new Error(error.error || 'Request failed');
  }
  return response.text();
}

// Settings/Config
export async function getSettings() {
  return fetchApi('/settings');
}

export async function updateSettings(data: any) {
  return fetchApi('/settings', {
    method: 'PUT',
    body: JSON.stringify(data),
  });
}

// Generic API client for pages that need direct endpoint access
export const api = {
  get: (endpoint: string) => fetchApi(endpoint),
  post: (endpoint: string, data?: any) => fetchApi(endpoint, {
    method: 'POST',
    body: data ? JSON.stringify(data) : undefined,
  }),
  put: (endpoint: string, data?: any) => fetchApi(endpoint, {
    method: 'PUT',
    body: data ? JSON.stringify(data) : undefined,
  }),
  delete: (endpoint: string) => fetchApi(endpoint, {
    method: 'DELETE',
  }),
};
