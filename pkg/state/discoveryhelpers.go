package state

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/jaypipes/ghw"
)

// HelpersInterface defines the unified interface for all helper functions.
// This interface allows for easy mocking in unit tests by implementing mock versions
// of all the helper methods.
type HelpersInterface interface {
	// SR-IOV device utility functions
	IsSriovVF(pciAddress string) bool
	IsSriovPF(pciAddress string) bool
	GetVFList(pfPciAddress string) ([]string, error)

	// PCI device discovery functionality
	PCI() (*ghw.PCIInfo, error)
	GetPCIDeviceByAddress(address string) (*ghw.PCIDevice, error)
	GetNetworkDevices() ([]*ghw.PCIDevice, error)

	// SR-IOV capability and configuration functions
	IsSriovCapable(pciAddress string) bool
	GetSriovTotalVFs(pciAddress string) (int, error)
	GetSriovNumVFs(pciAddress string) (int, error)

	// Network interface functions
	TryGetInterfaceName(pciAddr string) string
	GetNicSriovMode(pciAddr string) string
}

// Helpers provides unified helper functionality for SR-IOV and PCI operations
type Helpers struct{}

// NewHelpers creates a new Helpers instance
func NewHelpers() HelpersInterface {
	return &Helpers{}
}

// IsSriovVF checks if a PCI device is an SR-IOV Virtual Function
func (h *Helpers) IsSriovVF(pciAddress string) bool {
	// Check if physfn symlink exists - this indicates it's a VF
	physfnPath := fmt.Sprintf("/sys/bus/pci/devices/%s/physfn", pciAddress)
	if _, err := os.Lstat(physfnPath); err == nil {
		return true
	}
	return false
}

// IsSriovPF checks if a PCI device is an SR-IOV Physical Function
func (h *Helpers) IsSriovPF(pciAddress string) bool {
	// Check if virtfn0 symlink exists - this indicates it's a PF with VFs
	virtfnPath := fmt.Sprintf("/sys/bus/pci/devices/%s/virtfn0", pciAddress)
	if _, err := os.Lstat(virtfnPath); err == nil {
		return true
	}
	return false
}

// GetVFList returns list of VFs for a given PF
func (h *Helpers) GetVFList(pfPciAddress string) ([]string, error) {
	var vfList []string

	pfPath := fmt.Sprintf("/sys/bus/pci/devices/%s", pfPciAddress)
	entries, err := os.ReadDir(pfPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read PF directory: %v", err)
	}

	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "virtfn") {
			linkPath := filepath.Join(pfPath, entry.Name())
			target, err := os.Readlink(linkPath)
			if err != nil {
				continue
			}
			// Extract PCI address from symlink target
			vfAddr := filepath.Base(target)
			vfList = append(vfList, vfAddr)
		}
	}

	return vfList, nil
}

// PCI returns PCI information using the public ghw library
func (h *Helpers) PCI() (*ghw.PCIInfo, error) {
	return ghw.PCI()
}

// GetPCIDeviceByAddress gets a specific PCI device by its address
func (h *Helpers) GetPCIDeviceByAddress(address string) (*ghw.PCIDevice, error) {
	pciInfo, err := ghw.PCI()
	if err != nil {
		return nil, err
	}

	for _, device := range pciInfo.Devices {
		if device.Address == address {
			return device, nil
		}
	}

	return nil, fmt.Errorf("PCI device with address %s not found", address)
}

// GetNetworkDevices returns only network class PCI devices
func (h *Helpers) GetNetworkDevices() ([]*ghw.PCIDevice, error) {
	pciInfo, err := ghw.PCI()
	if err != nil {
		return nil, err
	}

	var networkDevices []*ghw.PCIDevice
	for _, device := range pciInfo.Devices {
		// Network controller class is 0x02
		if device.Class != nil && strings.HasPrefix(device.Class.ID, "02") {
			networkDevices = append(networkDevices, device)
		}
	}

	return networkDevices, nil
}

// Network device constants
const (
	NetClass  = 0x02 // Network controller class
	sysBusPci = "/sys/bus/pci/devices"
)

// IsSriovCapable checks if a device supports SR-IOV
func (h *Helpers) IsSriovCapable(pciAddress string) bool {
	// Check for sriov_totalvfs file
	totalVfsPath := fmt.Sprintf("/sys/bus/pci/devices/%s/sriov_totalvfs", pciAddress)
	if _, err := os.Stat(totalVfsPath); err == nil {
		return true
	}
	return false
}

// GetSriovTotalVFs gets the total number of VFs supported by a PF
func (h *Helpers) GetSriovTotalVFs(pciAddress string) (int, error) {
	totalVfsPath := fmt.Sprintf("/sys/bus/pci/devices/%s/sriov_totalvfs", pciAddress)
	content, err := os.ReadFile(totalVfsPath)
	if err != nil {
		return 0, err
	}

	var totalVfs int
	_, err = fmt.Sscanf(strings.TrimSpace(string(content)), "%d", &totalVfs)
	if err != nil {
		return 0, err
	}

	return totalVfs, nil
}

// GetSriovNumVFs gets the current number of VFs configured for a PF
func (h *Helpers) GetSriovNumVFs(pciAddress string) (int, error) {
	numVfsPath := fmt.Sprintf("/sys/bus/pci/devices/%s/sriov_numvfs", pciAddress)
	content, err := os.ReadFile(numVfsPath)
	if err != nil {
		return 0, err
	}

	var numVfs int
	_, err = fmt.Sscanf(strings.TrimSpace(string(content)), "%d", &numVfs)
	if err != nil {
		return 0, err
	}

	return numVfs, nil
}

// TryGetInterfaceName tries to find the network interface name based on PCI address
func (h *Helpers) TryGetInterfaceName(pciAddr string) string {
	netDir := filepath.Join(sysBusPci, pciAddr, "net")
	if _, err := os.Lstat(netDir); err != nil {
		return ""
	}

	fInfos, err := os.ReadDir(netDir)
	if err != nil {
		return ""
	}

	if len(fInfos) == 0 {
		return ""
	}

	// Return the first network interface name found
	return fInfos[0].Name()
}

// GetNicSriovMode returns the interface mode (simplified implementation)
// This is a simplified version that returns "legacy" mode as fallback
func (h *Helpers) GetNicSriovMode(pciAddr string) string {
	// For simplicity, always return legacy mode
	// A full implementation would use netlink to query the eswitch mode
	return "legacy"
}
