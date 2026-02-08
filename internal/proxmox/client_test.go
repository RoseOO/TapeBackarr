package proxmox

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNewClient(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *ClientConfig
		wantErr bool
	}{
		{
			name:    "empty host",
			cfg:     &ClientConfig{},
			wantErr: true,
		},
		{
			name: "valid config with token",
			cfg: &ClientConfig{
				Host:        "localhost",
				Port:        8006,
				TokenID:     "root@pam!test",
				TokenSecret: "secret",
			},
			wantErr: false,
		},
		{
			name: "default port",
			cfg: &ClientConfig{
				Host:        "localhost",
				TokenID:     "root@pam!test",
				TokenSecret: "secret",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := NewClient(tt.cfg)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewClient() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && client == nil {
				t.Errorf("NewClient() returned nil client")
			}
		})
	}
}

func TestClientConfig_Defaults(t *testing.T) {
	cfg := &ClientConfig{
		Host:        "test-host",
		TokenID:     "test@pam!token",
		TokenSecret: "secret",
	}

	client, err := NewClient(cfg)
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	// Check defaults were applied
	expectedURL := "https://test-host:8006/api2/json"
	if client.baseURL != expectedURL {
		t.Errorf("baseURL = %v, want %v", client.baseURL, expectedURL)
	}
}

func TestClient_GetNodes(t *testing.T) {
	// Create mock server
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api2/json/nodes" {
			response := map[string]interface{}{
				"data": []map[string]interface{}{
					{
						"node":   "pve1",
						"status": "online",
						"cpu":    0.5,
						"maxcpu": 8,
						"mem":    8589934592,
						"maxmem": 17179869184,
					},
					{
						"node":   "pve2",
						"status": "online",
						"cpu":    0.3,
						"maxcpu": 8,
						"mem":    4294967296,
						"maxmem": 17179869184,
					},
				},
			}
			json.NewEncoder(w).Encode(response)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	// Create client pointing to mock server
	client := &Client{
		baseURL:    server.URL + "/api2/json",
		httpClient: server.Client(),
		tokenName:  "test@pam!token",
		apiToken:   "secret",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	nodes, err := client.GetNodes(ctx)
	if err != nil {
		t.Fatalf("GetNodes() error = %v", err)
	}

	if len(nodes) != 2 {
		t.Errorf("GetNodes() returned %d nodes, want 2", len(nodes))
	}

	if nodes[0].Node != "pve1" {
		t.Errorf("nodes[0].Node = %v, want pve1", nodes[0].Node)
	}

	if nodes[0].Status != "online" {
		t.Errorf("nodes[0].Status = %v, want online", nodes[0].Status)
	}
}

func TestClient_GetNodeVMs(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api2/json/nodes/pve1/qemu" {
			response := map[string]interface{}{
				"data": []map[string]interface{}{
					{
						"vmid":   100,
						"name":   "test-vm",
						"status": "running",
						"cpus":   4,
						"mem":    4294967296,
						"maxmem": 8589934592,
					},
				},
			}
			json.NewEncoder(w).Encode(response)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	client := &Client{
		baseURL:    server.URL + "/api2/json",
		httpClient: server.Client(),
		tokenName:  "test@pam!token",
		apiToken:   "secret",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	vms, err := client.GetNodeVMs(ctx, "pve1")
	if err != nil {
		t.Fatalf("GetNodeVMs() error = %v", err)
	}

	if len(vms) != 1 {
		t.Errorf("GetNodeVMs() returned %d VMs, want 1", len(vms))
	}

	if vms[0].VMID != 100 {
		t.Errorf("vms[0].VMID = %v, want 100", vms[0].VMID)
	}

	if vms[0].Name != "test-vm" {
		t.Errorf("vms[0].Name = %v, want test-vm", vms[0].Name)
	}

	if vms[0].Node != "pve1" {
		t.Errorf("vms[0].Node = %v, want pve1", vms[0].Node)
	}
}

func TestClient_GetNodeLXCs(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api2/json/nodes/pve1/lxc" {
			response := map[string]interface{}{
				"data": []map[string]interface{}{
					{
						"vmid":   200,
						"name":   "test-ct",
						"status": "running",
						"cpus":   2,
						"mem":    1073741824,
						"maxmem": 2147483648,
					},
				},
			}
			json.NewEncoder(w).Encode(response)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	client := &Client{
		baseURL:    server.URL + "/api2/json",
		httpClient: server.Client(),
		tokenName:  "test@pam!token",
		apiToken:   "secret",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	lxcs, err := client.GetNodeLXCs(ctx, "pve1")
	if err != nil {
		t.Fatalf("GetNodeLXCs() error = %v", err)
	}

	if len(lxcs) != 1 {
		t.Errorf("GetNodeLXCs() returned %d LXCs, want 1", len(lxcs))
	}

	if lxcs[0].VMID != 200 {
		t.Errorf("lxcs[0].VMID = %v, want 200", lxcs[0].VMID)
	}

	if lxcs[0].Name != "test-ct" {
		t.Errorf("lxcs[0].Name = %v, want test-ct", lxcs[0].Name)
	}
}

func TestClient_IsClusterMode(t *testing.T) {
	tests := []struct {
		name       string
		response   interface{}
		wantCluster bool
	}{
		{
			name: "cluster mode",
			response: map[string]interface{}{
				"data": []map[string]interface{}{
					{"type": "cluster", "name": "test-cluster"},
					{"type": "node", "name": "pve1"},
				},
			},
			wantCluster: true,
		},
		{
			name: "standalone mode",
			response: map[string]interface{}{
				"data": []map[string]interface{}{
					{"type": "node", "name": "pve1"},
				},
			},
			wantCluster: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/api2/json/cluster/status" {
					json.NewEncoder(w).Encode(tt.response)
					return
				}
				http.NotFound(w, r)
			}))
			defer server.Close()

			client := &Client{
				baseURL:    server.URL + "/api2/json",
				httpClient: server.Client(),
				tokenName:  "test@pam!token",
				apiToken:   "secret",
			}

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			isCluster, err := client.IsClusterMode(ctx)
			if err != nil {
				t.Fatalf("IsClusterMode() error = %v", err)
			}

			if isCluster != tt.wantCluster {
				t.Errorf("IsClusterMode() = %v, want %v", isCluster, tt.wantCluster)
			}
		})
	}
}

func TestBackupMode_String(t *testing.T) {
	tests := []struct {
		mode BackupMode
		want string
	}{
		{BackupModeSnapshot, "snapshot"},
		{BackupModeSuspend, "suspend"},
		{BackupModeStop, "stop"},
	}

	for _, tt := range tests {
		if string(tt.mode) != tt.want {
			t.Errorf("BackupMode = %v, want %v", string(tt.mode), tt.want)
		}
	}
}

func TestGuestType_String(t *testing.T) {
	tests := []struct {
		guestType GuestType
		want      string
	}{
		{GuestTypeVM, "qemu"},
		{GuestTypeLXC, "lxc"},
	}

	for _, tt := range tests {
		if string(tt.guestType) != tt.want {
			t.Errorf("GuestType = %v, want %v", string(tt.guestType), tt.want)
		}
	}
}
