/*
 * Copyright (C) 2019 The "MysteriumNetwork/node" Authors.
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
 */

package nat

import (
	"net"
	"os/exec"
	"strconv"
	"strings"
	"sync"

	log "github.com/cihub/seelog"
	"github.com/pkg/errors"
)

const (
	enablePublicSharing  = "$config.EnableSharing(0)"
	enablePrivateSharing = "$config.EnableSharing(1)"
	disableSharing       = "$config.DisableSharing()"
)

type serviceICS struct {
	mu                 sync.Mutex
	ifaces             map[string]RuleForwarding // list in internal interfaces with enabled internet connection sharing
	remoteAccessStatus string
	powerShell         func(cmd string) ([]byte, error)
}

// Enable enables internet connection sharing for the public interface.
func (nat *serviceICS) Enable() error {
	if err := nat.enableRemoteAccessService(); err != nil {
		return errors.Wrap(err, "failed to Enable RemoteAccess service")
	}

	ifaceName, err := nat.getPublicInterfaceName()
	if err != nil {
		return errors.Wrap(err, "failed to get public interface name")
	}

	err = nat.applySharingConfig(enablePublicSharing, ifaceName)
	return errors.Wrap(err, "failed to enable internet connection sharing")
}

func (nat *serviceICS) enableRemoteAccessService() error {
	status, err := nat.powerShell("Get-Service RemoteAccess | foreach { $_.StartType }")
	if err != nil {
		return errors.Wrap(err, "failed to get RemoteAccess service startup type")
	}
	nat.remoteAccessStatus = string(status)

	if _, err := nat.powerShell("Set-Service -Name RemoteAccess -StartupType automatic"); err != nil {
		return errors.Wrap(err, "failed to set RemoteAccess service startup type to automatic")
	}

	_, err = nat.powerShell("Start-Service -Name RemoteAccess")
	return errors.Wrap(err, "failed to start RemoteAccess service")
}

// Add enables internet connection sharing for the local interface.
func (nat *serviceICS) Add(rule RuleForwarding) error {
	// TODO firewall rule configuration should be added here for new connections.
	ifaceName, err := getInterfaceBySubnet(rule.SourceAddress)
	if err != nil {
		return errors.Wrap(err, "failed to find suitable interface")
	}

	err = nat.applySharingConfig(enablePrivateSharing, ifaceName)
	if err != nil {
		return errors.Wrap(err, "failed to enable internet connection sharing for internal interface")
	}

	nat.mu.Lock()
	defer nat.mu.Unlock()
	nat.ifaces[ifaceName] = rule

	return nil
}

// Del disables internet connection sharing for the local interface.
func (nat *serviceICS) Del(rule RuleForwarding) error {
	// TODO firewall rule configuration should be added here for cleaning up unused connections.
	ifaceName, err := getInterfaceBySubnet(rule.SourceAddress)
	if err != nil {
		return errors.Wrap(err, "failed to find suitable interface")
	}

	err = nat.applySharingConfig(disableSharing, ifaceName)
	if err != nil {
		return errors.Wrap(err, "failed to disable internet connection sharing for internal interface")
	}

	nat.mu.Lock()
	defer nat.mu.Unlock()
	delete(nat.ifaces, ifaceName)

	return nil
}

// Disable disables internet connection sharing for the public interface.
func (nat *serviceICS) Disable() (resErr error) {
	for iface, rule := range nat.ifaces {
		if err := nat.Del(rule); err != nil {
			log.Errorf("%s Failed to cleanup internet connection sharing for '%s' interface: %v", natLogPrefix, iface, err)
			if resErr == nil {
				resErr = err
			}
		}
	}

	_, err := nat.powerShell("Set-Service -Name RemoteAccess -StartupType " + nat.remoteAccessStatus)
	if err != nil {
		err = errors.Wrap(err, "failed to revert RemoteAccess service startup type")
		log.Errorf("%s %v", natLogPrefix, err)
		if resErr == nil {
			resErr = err
		}
	}

	ifaceName, err := nat.getPublicInterfaceName()
	if err != nil {
		err = errors.Wrap(err, "failed to get public interface name")
		log.Errorf("%s %v", natLogPrefix, err)
		if resErr == nil {
			resErr = err
		}
	}

	err = nat.applySharingConfig(disableSharing, ifaceName)
	if err != nil {
		err = errors.Wrap(err, "failed to disable internet connection sharing")
		log.Errorf("%s %v", natLogPrefix, err)
		if resErr == nil {
			resErr = err
		}
	}

	return resErr
}

func (nat *serviceICS) getPublicInterfaceName() (string, error) {
	out, err := nat.powerShell(`Get-WmiObject -Class Win32_IP4RouteTable | where { $_.destination -eq '0.0.0.0' -and $_.mask -eq '0.0.0.0'} | foreach { $_.InterfaceIndex }`)
	if err != nil {
		return "", errors.Wrap(err, "failed to get interface from the default route")
	}

	ifaceID, err := strconv.Atoi(strings.TrimSpace(string(out)))
	if err != nil {
		return "", errors.Wrap(err, "failed to parse interface ID")
	}

	ifaces, err := net.Interfaces()
	if err != nil {
		return "", errors.Wrap(err, "failed to get a list of network interfaces")
	}

	for _, iface := range ifaces {
		if iface.Index == ifaceID {
			return iface.Name, nil
		}
	}

	return "", errors.New("interface not found")
}

func (nat *serviceICS) applySharingConfig(action, ifaceName string) error {
	if len(action) == 0 {
		return errors.New("empty action provided")
	}
	if len(ifaceName) == 0 {
		return errors.New("empty interface name provided")
	}

	_, err := nat.powerShell(`regsvr32 /s hnetcfg.dll;
		$netShare = New-Object -ComObject HNetCfg.HNetShare;
		$c = $netShare.EnumEveryConnection |? { $netShare.NetConnectionProps.Invoke($_).Name -eq "` + ifaceName + `" };
		$config = $netShare.INetSharingConfigurationForINetConnection.Invoke($c);` + action)
	return err
}

func getInterfaceBySubnet(subnet string) (string, error) {
	_, ipnet, err := net.ParseCIDR(subnet)
	if err != nil {
		return "", errors.Wrap(err, "failed to parse subnet from request")
	}

	ifaces, err := net.Interfaces()
	if err != nil {
		return "", errors.Wrap(err, "failed to get a list of network interfaces")
	}

	for _, iface := range ifaces {
		addrs, err := iface.Addrs()
		if err != nil {
			return "", errors.Wrap(err, "failed to get list of interface addresses")
		}

		if contains(ipnet, addrs) {
			return iface.Name, nil
		}
	}
	return "", errors.New("interface not found")
}

func contains(ipnet *net.IPNet, addrs []net.Addr) bool {
	for _, addr := range addrs {
		var ip net.IP
		switch v := addr.(type) {
		case *net.IPNet:
			ip = v.IP
		case *net.IPAddr:
			ip = v.IP
		}

		if ipnet.Contains(ip) {
			return true
		}
	}

	return false
}

func powerShell(cmd string) ([]byte, error) {
	out, err := exec.Command("powershell", "-Command", cmd).CombinedOutput()
	return out, errors.Wrap(err, string(out))
}
