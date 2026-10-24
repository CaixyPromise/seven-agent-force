package hostinfo

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/host"
	"github.com/shirou/gopsutil/v4/load"
	"github.com/shirou/gopsutil/v4/mem"
)

type Interface struct {
	Name      string   `json:"name"`
	Addresses []string `json:"addresses"`
	MAC       string   `json:"mac,omitempty"`
}

type Resources struct {
	LogicalCPUCores   int     `json:"logicalCpuCores"`
	CPUUsage          float64 `json:"cpuUsagePercent"`
	CPUUsageAvailable bool    `json:"cpuUsageAvailable"`
	Load1             float64 `json:"load1"`
	MemoryTotal       uint64  `json:"memoryTotalBytes"`
	MemoryUsed        uint64  `json:"memoryUsedBytes"`
	MemoryAvailable   uint64  `json:"memoryAvailableBytes"`
	MemoryUsage       float64 `json:"memoryUsagePercent"`
	DiskTotal         uint64  `json:"workspaceDiskTotalBytes"`
	DiskUsed          uint64  `json:"workspaceDiskUsedBytes"`
	DiskAvailable     uint64  `json:"workspaceDiskAvailableBytes"`
	DiskUsage         float64 `json:"workspaceDiskUsagePercent"`
}

const cpuSampleInterval = time.Second

type cpuPercentSampler func(context.Context, time.Duration, bool) ([]float64, error)

type Snapshot struct {
	Hostname         string
	PrimaryInterface string
	PrimaryMAC       string
	ReportedIP       string
	Interfaces       json.RawMessage
	Resources        json.RawMessage
	DockerReady      bool
}

func HostBootID(ctx context.Context) (string, error) {
	if body, err := os.ReadFile("/proc/sys/kernel/random/boot_id"); err == nil {
		value := strings.TrimSpace(string(body))
		if value != "" {
			return "boot_" + strings.ReplaceAll(value, "-", ""), nil
		}
	}
	bootTime, err := host.BootTimeWithContext(ctx)
	if err != nil {
		return "", err
	}
	hostname, _ := os.Hostname()
	sum := sha256.Sum256([]byte(runtime.GOOS + "\x00" + hostname + "\x00" + time.Unix(int64(bootTime), 0).UTC().Format(time.RFC3339)))
	return "boot_" + hex.EncodeToString(sum[:16]), nil
}

func Collect(ctx context.Context, workspaceRoot string) (Snapshot, error) {
	hostname, _ := os.Hostname()
	interfaces, primary, ip, mac, err := networkInterfaces()
	if err != nil {
		return Snapshot{}, err
	}
	memory, err := mem.VirtualMemoryWithContext(ctx)
	if err != nil {
		return Snapshot{}, err
	}
	root, err := filepath.Abs(workspaceRoot)
	if err != nil {
		return Snapshot{}, err
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return Snapshot{}, err
	}
	filesystem, err := disk.UsageWithContext(ctx, root)
	if err != nil {
		return Snapshot{}, err
	}
	cores, _ := cpu.CountsWithContext(ctx, true)
	usage, usageAvailable := sampleCPUUsage(ctx, cpu.PercentWithContext)
	load1 := float64(0)
	if average, loadErr := load.AvgWithContext(ctx); loadErr == nil {
		load1 = average.Load1
	}
	resources, _ := json.Marshal(Resources{
		LogicalCPUCores: cores, CPUUsage: usage, CPUUsageAvailable: usageAvailable, Load1: load1,
		MemoryTotal: memory.Total, MemoryUsed: memory.Used, MemoryAvailable: memory.Available, MemoryUsage: memory.UsedPercent,
		DiskTotal: filesystem.Total, DiskUsed: filesystem.Used, DiskAvailable: filesystem.Free, DiskUsage: filesystem.UsedPercent,
	})
	interfacesJSON, _ := json.Marshal(interfaces)
	dockerCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	dockerReady := exec.CommandContext(dockerCtx, "docker", "info", "--format", "{{json .ServerVersion}}").Run() == nil
	return Snapshot{Hostname: hostname, PrimaryInterface: primary, PrimaryMAC: mac, ReportedIP: ip, Interfaces: interfacesJSON, Resources: resources, DockerReady: dockerReady}, nil
}

func sampleCPUUsage(ctx context.Context, sample cpuPercentSampler) (float64, bool) {
	percent, err := sample(ctx, cpuSampleInterval, false)
	if err != nil || len(percent) != 1 || percent[0] < 0 || percent[0] > 100 {
		return 0, false
	}
	return percent[0], true
}

func networkInterfaces() ([]Interface, string, string, string, error) {
	all, err := net.Interfaces()
	if err != nil {
		return nil, "", "", "", err
	}
	result := make([]Interface, 0, len(all))
	for _, item := range all {
		if item.Flags&net.FlagUp == 0 || item.Flags&net.FlagLoopback != 0 || ignoredInterface(item.Name) {
			continue
		}
		addresses, _ := item.Addrs()
		values := make([]string, 0, len(addresses))
		for _, address := range addresses {
			value := address.String()
			if parsed, _, parseErr := net.ParseCIDR(value); parseErr == nil && parsed.IsLoopback() {
				continue
			}
			values = append(values, value)
		}
		if len(values) == 0 {
			continue
		}
		result = append(result, Interface{Name: item.Name, Addresses: values, MAC: strings.ToUpper(item.HardwareAddr.String())})
	}
	if len(result) == 0 {
		return nil, "", "", "", errors.New("no usable network interface found")
	}
	sort.SliceStable(result, func(i, j int) bool { return interfaceRank(result[i].Name) < interfaceRank(result[j].Name) })
	primary := result[0]
	reported := ""
	for _, address := range primary.Addresses {
		ip, _, err := net.ParseCIDR(address)
		if err == nil && ip.To4() != nil {
			reported = ip.String()
			break
		}
	}
	if reported == "" && len(primary.Addresses) > 0 {
		reported = strings.Split(primary.Addresses[0], "/")[0]
	}
	return result, primary.Name, reported, primary.MAC, nil
}

func ignoredInterface(name string) bool {
	lower := strings.ToLower(name)
	for _, prefix := range []string{"docker", "br-", "veth", "virbr", "utun", "awdl", "llw", "bridge"} {
		if strings.HasPrefix(lower, prefix) {
			return true
		}
	}
	return false
}

func interfaceRank(name string) int {
	lower := strings.ToLower(name)
	if strings.HasPrefix(lower, "en") || strings.HasPrefix(lower, "eth") {
		return 0
	}
	if strings.HasPrefix(lower, "tailscale") || strings.HasPrefix(lower, "tun") {
		return 2
	}
	return 1
}
