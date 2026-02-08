package proxmox

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Client handles communication with Proxmox VE API
type Client struct {
	baseURL    string
	httpClient *http.Client
	ticket     string
	csrfToken  string
	apiToken   string // Alternative auth: API token
	tokenName  string // API token name (user@realm!tokenname)
}

// ClientConfig holds configuration for creating a Proxmox client
type ClientConfig struct {
	Host          string
	Port          int
	SkipTLSVerify bool
	Timeout       time.Duration
	// Auth option 1: Username/Password
	Username string
	Password string
	Realm    string // Usually "pam" or "pve"
	// Auth option 2: API Token
	TokenID     string // Format: user@realm!tokenname
	TokenSecret string
}

// NodeStatus represents the status of a Proxmox node
type NodeStatus struct {
	Node           string  `json:"node"`
	Status         string  `json:"status"`
	CPU            float64 `json:"cpu"`
	MaxCPU         int     `json:"maxcpu"`
	Mem            int64   `json:"mem"`
	MaxMem         int64   `json:"maxmem"`
	Disk           int64   `json:"disk"`
	MaxDisk        int64   `json:"maxdisk"`
	Uptime         int64   `json:"uptime"`
	Type           string  `json:"type"`
	SSLFingerprint string  `json:"ssl_fingerprint"`
}

// VMInfo represents a QEMU VM
type VMInfo struct {
	VMID     int     `json:"vmid"`
	Name     string  `json:"name"`
	Node     string  `json:"node,omitempty"`
	Status   string  `json:"status"`
	CPU      float64 `json:"cpu"`
	CPUs     int     `json:"cpus"`
	Mem      int64   `json:"mem"`
	MaxMem   int64   `json:"maxmem"`
	Disk     int64   `json:"disk"`
	MaxDisk  int64   `json:"maxdisk"`
	Uptime   int64   `json:"uptime"`
	Template int     `json:"template"`
	Type     string  `json:"type,omitempty"`
	Tags     string  `json:"tags,omitempty"`
	Lock     string  `json:"lock,omitempty"`
	HA       HAState `json:"ha,omitempty"`
}

// LXCInfo represents an LXC container
type LXCInfo struct {
	VMID     int     `json:"vmid"`
	Name     string  `json:"name"`
	Node     string  `json:"node,omitempty"`
	Status   string  `json:"status"`
	CPU      float64 `json:"cpu"`
	CPUs     int     `json:"cpus"`
	Mem      int64   `json:"mem"`
	MaxMem   int64   `json:"maxmem"`
	Swap     int64   `json:"swap"`
	MaxSwap  int64   `json:"maxswap"`
	Disk     int64   `json:"disk"`
	MaxDisk  int64   `json:"maxdisk"`
	Uptime   int64   `json:"uptime"`
	Template int     `json:"template"`
	Type     string  `json:"type,omitempty"`
	Tags     string  `json:"tags,omitempty"`
	Lock     string  `json:"lock,omitempty"`
}

// HAState represents High Availability state
type HAState struct {
	Managed int `json:"managed"`
}

// VMConfig represents QEMU VM configuration
type VMConfig struct {
	Name    string `json:"name"`
	Memory  int    `json:"memory"`
	Cores   int    `json:"cores"`
	Sockets int    `json:"sockets"`
	CPU     string `json:"cpu"`
	OSType  string `json:"ostype"`
	BIOS    string `json:"bios"`
	Machine string `json:"machine"`
	Boot    string `json:"boot"`
	SCSIHW  string `json:"scsihw"`
	Digest  string `json:"digest"`
	// Storage devices (scsi0, virtio0, ide0, etc.)
	Disks map[string]string `json:"-"`
	// Network devices (net0, net1, etc.)
	Networks map[string]string `json:"-"`
	// Full raw config for backup
	RawConfig map[string]interface{} `json:"-"`
}

// LXCConfig represents LXC container configuration
type LXCConfig struct {
	Hostname     string `json:"hostname"`
	Memory       int    `json:"memory"`
	Swap         int    `json:"swap"`
	Cores        int    `json:"cores"`
	OSType       string `json:"ostype"`
	Arch         string `json:"arch"`
	RootFS       string `json:"rootfs"`
	Digest       string `json:"digest"`
	Unprivileged int    `json:"unprivileged"`
	Features     string `json:"features"`
	// Mount points (mp0, mp1, etc.)
	MountPoints map[string]string `json:"-"`
	// Network devices (net0, net1, etc.)
	Networks map[string]string `json:"-"`
	// Full raw config for backup
	RawConfig map[string]interface{} `json:"-"`
}

// BackupTask represents a backup task status
type BackupTask struct {
	UPID       string `json:"upid"`
	Node       string `json:"node"`
	Type       string `json:"type"`
	Status     string `json:"status"`
	ExitStatus string `json:"exitstatus"`
	StartTime  int64  `json:"starttime"`
	EndTime    int64  `json:"endtime"`
	PID        int    `json:"pid"`
	User       string `json:"user"`
}

// Storage represents a Proxmox storage
type Storage struct {
	Storage      string  `json:"storage"`
	Type         string  `json:"type"`
	Content      string  `json:"content"`
	Active       int     `json:"active"`
	Enabled      int     `json:"enabled"`
	Shared       int     `json:"shared"`
	Used         int64   `json:"used"`
	Available    int64   `json:"avail"`
	Total        int64   `json:"total"`
	UsedFraction float64 `json:"used_fraction"`
}

// apiResponse wraps Proxmox API responses
type apiResponse struct {
	Data   interface{} `json:"data"`
	Errors []struct {
		Message string `json:"message"`
		Path    string `json:"path"`
	} `json:"errors,omitempty"`
}

// NewClient creates a new Proxmox API client
func NewClient(cfg *ClientConfig) (*Client, error) {
	if cfg.Host == "" {
		return nil, fmt.Errorf("proxmox host is required")
	}

	if cfg.Port == 0 {
		cfg.Port = 8006 // Default Proxmox API port
	}

	if cfg.Timeout == 0 {
		cfg.Timeout = 30 * time.Second
	}

	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: cfg.SkipTLSVerify,
		},
	}

	client := &Client{
		baseURL: fmt.Sprintf("https://%s:%d/api2/json", cfg.Host, cfg.Port),
		httpClient: &http.Client{
			Transport: transport,
			Timeout:   cfg.Timeout,
		},
	}

	// Use API token if provided
	if cfg.TokenID != "" && cfg.TokenSecret != "" {
		client.tokenName = cfg.TokenID
		client.apiToken = cfg.TokenSecret
		return client, nil
	}

	// Otherwise authenticate with username/password
	if cfg.Username != "" && cfg.Password != "" {
		if cfg.Realm == "" {
			cfg.Realm = "pam"
		}
		if err := client.authenticate(cfg.Username, cfg.Password, cfg.Realm); err != nil {
			return nil, fmt.Errorf("authentication failed: %w", err)
		}
	}

	return client, nil
}

// authenticate performs ticket-based authentication
func (c *Client) authenticate(username, password, realm string) error {
	data := url.Values{}
	data.Set("username", fmt.Sprintf("%s@%s", username, realm))
	data.Set("password", password)

	req, err := http.NewRequest("POST", c.baseURL+"/access/ticket", strings.NewReader(data.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("authentication failed: %s", string(body))
	}

	var result struct {
		Data struct {
			Ticket              string `json:"ticket"`
			CSRFPreventionToken string `json:"CSRFPreventionToken"`
			Username            string `json:"username"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return err
	}

	c.ticket = result.Data.Ticket
	c.csrfToken = result.Data.CSRFPreventionToken
	return nil
}

// doRequest performs an authenticated API request
func (c *Client) doRequest(ctx context.Context, method, path string, body interface{}) ([]byte, error) {
	var reqBody io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reqBody = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reqBody)
	if err != nil {
		return nil, err
	}

	// Set authentication headers
	if c.apiToken != "" {
		req.Header.Set("Authorization", fmt.Sprintf("PVEAPIToken=%s=%s", c.tokenName, c.apiToken))
	} else if c.ticket != "" {
		req.Header.Set("Cookie", fmt.Sprintf("PVEAuthCookie=%s", c.ticket))
		if method != "GET" {
			req.Header.Set("CSRFPreventionToken", c.csrfToken)
		}
	}

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("API error %d: %s", resp.StatusCode, string(respBody))
	}

	return respBody, nil
}

// GetNodes returns all nodes in the cluster (or single node for standalone)
func (c *Client) GetNodes(ctx context.Context) ([]NodeStatus, error) {
	data, err := c.doRequest(ctx, "GET", "/nodes", nil)
	if err != nil {
		return nil, err
	}

	var resp struct {
		Data []NodeStatus `json:"data"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, err
	}

	return resp.Data, nil
}

// GetNodeVMs returns all VMs on a specific node
func (c *Client) GetNodeVMs(ctx context.Context, node string) ([]VMInfo, error) {
	data, err := c.doRequest(ctx, "GET", fmt.Sprintf("/nodes/%s/qemu", node), nil)
	if err != nil {
		return nil, err
	}

	var resp struct {
		Data []VMInfo `json:"data"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, err
	}

	// Set node name on each VM
	for i := range resp.Data {
		resp.Data[i].Node = node
		resp.Data[i].Type = "qemu"
	}

	return resp.Data, nil
}

// GetNodeLXCs returns all LXC containers on a specific node
func (c *Client) GetNodeLXCs(ctx context.Context, node string) ([]LXCInfo, error) {
	data, err := c.doRequest(ctx, "GET", fmt.Sprintf("/nodes/%s/lxc", node), nil)
	if err != nil {
		return nil, err
	}

	var resp struct {
		Data []LXCInfo `json:"data"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, err
	}

	// Set node name on each LXC
	for i := range resp.Data {
		resp.Data[i].Node = node
		resp.Data[i].Type = "lxc"
	}

	return resp.Data, nil
}

// GetAllGuests returns all VMs and LXCs across all nodes
func (c *Client) GetAllGuests(ctx context.Context) ([]VMInfo, []LXCInfo, error) {
	nodes, err := c.GetNodes(ctx)
	if err != nil {
		return nil, nil, err
	}

	var allVMs []VMInfo
	var allLXCs []LXCInfo

	for _, node := range nodes {
		if node.Status != "online" {
			continue
		}

		vms, err := c.GetNodeVMs(ctx, node.Node)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to get VMs from node %s: %w", node.Node, err)
		}
		allVMs = append(allVMs, vms...)

		lxcs, err := c.GetNodeLXCs(ctx, node.Node)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to get LXCs from node %s: %w", node.Node, err)
		}
		allLXCs = append(allLXCs, lxcs...)
	}

	return allVMs, allLXCs, nil
}

// GetVMConfig returns the configuration of a specific VM
func (c *Client) GetVMConfig(ctx context.Context, node string, vmid int) (*VMConfig, error) {
	data, err := c.doRequest(ctx, "GET", fmt.Sprintf("/nodes/%s/qemu/%d/config", node, vmid), nil)
	if err != nil {
		return nil, err
	}

	var resp struct {
		Data map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, err
	}

	config := &VMConfig{
		Disks:     make(map[string]string),
		Networks:  make(map[string]string),
		RawConfig: resp.Data,
	}

	// Parse known fields
	if v, ok := resp.Data["name"].(string); ok {
		config.Name = v
	}
	if v, ok := resp.Data["memory"].(float64); ok {
		config.Memory = int(v)
	}
	if v, ok := resp.Data["cores"].(float64); ok {
		config.Cores = int(v)
	}
	if v, ok := resp.Data["sockets"].(float64); ok {
		config.Sockets = int(v)
	}
	if v, ok := resp.Data["cpu"].(string); ok {
		config.CPU = v
	}
	if v, ok := resp.Data["ostype"].(string); ok {
		config.OSType = v
	}
	if v, ok := resp.Data["bios"].(string); ok {
		config.BIOS = v
	}
	if v, ok := resp.Data["machine"].(string); ok {
		config.Machine = v
	}
	if v, ok := resp.Data["boot"].(string); ok {
		config.Boot = v
	}
	if v, ok := resp.Data["scsihw"].(string); ok {
		config.SCSIHW = v
	}
	if v, ok := resp.Data["digest"].(string); ok {
		config.Digest = v
	}

	// Parse disks and networks
	diskPrefixes := []string{"scsi", "virtio", "ide", "sata", "efidisk", "tpmstate"}
	for key, val := range resp.Data {
		if strVal, ok := val.(string); ok {
			for _, prefix := range diskPrefixes {
				if strings.HasPrefix(key, prefix) {
					config.Disks[key] = strVal
					break
				}
			}
			if strings.HasPrefix(key, "net") {
				config.Networks[key] = strVal
			}
		}
	}

	return config, nil
}

// GetLXCConfig returns the configuration of a specific LXC container
func (c *Client) GetLXCConfig(ctx context.Context, node string, vmid int) (*LXCConfig, error) {
	data, err := c.doRequest(ctx, "GET", fmt.Sprintf("/nodes/%s/lxc/%d/config", node, vmid), nil)
	if err != nil {
		return nil, err
	}

	var resp struct {
		Data map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, err
	}

	config := &LXCConfig{
		MountPoints: make(map[string]string),
		Networks:    make(map[string]string),
		RawConfig:   resp.Data,
	}

	// Parse known fields
	if v, ok := resp.Data["hostname"].(string); ok {
		config.Hostname = v
	}
	if v, ok := resp.Data["memory"].(float64); ok {
		config.Memory = int(v)
	}
	if v, ok := resp.Data["swap"].(float64); ok {
		config.Swap = int(v)
	}
	if v, ok := resp.Data["cores"].(float64); ok {
		config.Cores = int(v)
	}
	if v, ok := resp.Data["ostype"].(string); ok {
		config.OSType = v
	}
	if v, ok := resp.Data["arch"].(string); ok {
		config.Arch = v
	}
	if v, ok := resp.Data["rootfs"].(string); ok {
		config.RootFS = v
	}
	if v, ok := resp.Data["digest"].(string); ok {
		config.Digest = v
	}
	if v, ok := resp.Data["unprivileged"].(float64); ok {
		config.Unprivileged = int(v)
	}
	if v, ok := resp.Data["features"].(string); ok {
		config.Features = v
	}

	// Parse mount points and networks
	for key, val := range resp.Data {
		if strVal, ok := val.(string); ok {
			if strings.HasPrefix(key, "mp") {
				config.MountPoints[key] = strVal
			}
			if strings.HasPrefix(key, "net") {
				config.Networks[key] = strVal
			}
		}
	}

	return config, nil
}

// GetNodeStorage returns storage information for a node
func (c *Client) GetNodeStorage(ctx context.Context, node string) ([]Storage, error) {
	data, err := c.doRequest(ctx, "GET", fmt.Sprintf("/nodes/%s/storage", node), nil)
	if err != nil {
		return nil, err
	}

	var resp struct {
		Data []Storage `json:"data"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, err
	}

	return resp.Data, nil
}

// StartVZDump starts a vzdump backup task
func (c *Client) StartVZDump(ctx context.Context, node string, vmid int, options map[string]string) (string, error) {
	data := url.Values{}
	data.Set("vmid", fmt.Sprintf("%d", vmid))

	for k, v := range options {
		data.Set(k, v)
	}

	req, err := http.NewRequestWithContext(ctx, "POST",
		fmt.Sprintf("%s/nodes/%s/vzdump", c.baseURL, node),
		strings.NewReader(data.Encode()))
	if err != nil {
		return "", err
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if c.apiToken != "" {
		req.Header.Set("Authorization", fmt.Sprintf("PVEAPIToken=%s=%s", c.tokenName, c.apiToken))
	} else if c.ticket != "" {
		req.Header.Set("Cookie", fmt.Sprintf("PVEAuthCookie=%s", c.ticket))
		req.Header.Set("CSRFPreventionToken", c.csrfToken)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("vzdump failed: %s", string(body))
	}

	var result struct {
		Data string `json:"data"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", err
	}

	return result.Data, nil // Returns UPID (task ID)
}

// GetTaskStatus returns the status of a task
func (c *Client) GetTaskStatus(ctx context.Context, node, upid string) (*BackupTask, error) {
	// URL encode the UPID
	encodedUPID := url.PathEscape(upid)
	data, err := c.doRequest(ctx, "GET", fmt.Sprintf("/nodes/%s/tasks/%s/status", node, encodedUPID), nil)
	if err != nil {
		return nil, err
	}

	var resp struct {
		Data BackupTask `json:"data"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, err
	}

	return &resp.Data, nil
}

// GetTaskLog returns the log output of a task
func (c *Client) GetTaskLog(ctx context.Context, node, upid string, start, limit int) ([]string, error) {
	encodedUPID := url.PathEscape(upid)
	path := fmt.Sprintf("/nodes/%s/tasks/%s/log?start=%d&limit=%d", node, encodedUPID, start, limit)
	data, err := c.doRequest(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}

	var resp struct {
		Data []struct {
			N int    `json:"n"`
			T string `json:"t"`
		} `json:"data"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, err
	}

	var lines []string
	for _, entry := range resp.Data {
		lines = append(lines, entry.T)
	}

	return lines, nil
}

// StopVM stops a VM
func (c *Client) StopVM(ctx context.Context, node string, vmid int) (string, error) {
	data, err := c.doRequest(ctx, "POST", fmt.Sprintf("/nodes/%s/qemu/%d/status/stop", node, vmid), nil)
	if err != nil {
		return "", err
	}

	var resp struct {
		Data string `json:"data"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return "", err
	}

	return resp.Data, nil
}

// StartVM starts a VM
func (c *Client) StartVM(ctx context.Context, node string, vmid int) (string, error) {
	data, err := c.doRequest(ctx, "POST", fmt.Sprintf("/nodes/%s/qemu/%d/status/start", node, vmid), nil)
	if err != nil {
		return "", err
	}

	var resp struct {
		Data string `json:"data"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return "", err
	}

	return resp.Data, nil
}

// StopLXC stops an LXC container
func (c *Client) StopLXC(ctx context.Context, node string, vmid int) (string, error) {
	data, err := c.doRequest(ctx, "POST", fmt.Sprintf("/nodes/%s/lxc/%d/status/stop", node, vmid), nil)
	if err != nil {
		return "", err
	}

	var resp struct {
		Data string `json:"data"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return "", err
	}

	return resp.Data, nil
}

// StartLXC starts an LXC container
func (c *Client) StartLXC(ctx context.Context, node string, vmid int) (string, error) {
	data, err := c.doRequest(ctx, "POST", fmt.Sprintf("/nodes/%s/lxc/%d/status/start", node, vmid), nil)
	if err != nil {
		return "", err
	}

	var resp struct {
		Data string `json:"data"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return "", err
	}

	return resp.Data, nil
}

// DownloadBackupFile downloads a backup file as a stream
// This reads from Proxmox storage and returns an io.ReadCloser for streaming to tape
func (c *Client) DownloadBackupFile(ctx context.Context, node, storage, volumeID string) (io.ReadCloser, int64, error) {
	// Use the special download endpoint
	path := fmt.Sprintf("/nodes/%s/storage/%s/file-restore/download", node, storage)

	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+path+"?volume="+url.QueryEscape(volumeID), nil)
	if err != nil {
		return nil, 0, err
	}

	if c.apiToken != "" {
		req.Header.Set("Authorization", fmt.Sprintf("PVEAPIToken=%s=%s", c.tokenName, c.apiToken))
	} else if c.ticket != "" {
		req.Header.Set("Cookie", fmt.Sprintf("PVEAuthCookie=%s", c.ticket))
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, 0, err
	}

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, 0, fmt.Errorf("download failed: %s", string(body))
	}

	return resp.Body, resp.ContentLength, nil
}

// IsClusterMode checks if the Proxmox installation is in cluster mode
func (c *Client) IsClusterMode(ctx context.Context) (bool, error) {
	data, err := c.doRequest(ctx, "GET", "/cluster/status", nil)
	if err != nil {
		// If cluster endpoint fails, likely standalone
		return false, nil
	}

	var resp struct {
		Data []struct {
			Type string `json:"type"`
			Name string `json:"name"`
		} `json:"data"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return false, nil
	}

	// Check if there's a cluster entry
	for _, item := range resp.Data {
		if item.Type == "cluster" {
			return true, nil
		}
	}

	return false, nil
}

// GetClusterResources returns all resources across the cluster
func (c *Client) GetClusterResources(ctx context.Context, resourceType string) ([]map[string]interface{}, error) {
	path := "/cluster/resources"
	if resourceType != "" {
		path += "?type=" + resourceType
	}

	data, err := c.doRequest(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}

	var resp struct {
		Data []map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, err
	}

	return resp.Data, nil
}
