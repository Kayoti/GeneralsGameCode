// This file holds the account creation limit of SERVICE.md section 4.

package accounts

import (
	"net"
	"time"
)

// accountCreationWindow is the period the account creation limit counts over.
const accountCreationWindow = time.Hour

// creationAllowed reports whether address may create an account at now. With
// the limit at 0, or for a loopback address, every creation is allowed. The
// caller holds the lock.
func (service *Service) creationAllowed(address string, now time.Time) bool {
	if !service.limitApplies(address) {
		return true
	}
	return len(service.pruneCreations(address, now)) < service.accountsPerAddressPerHour
}

// recordCreation counts an account created from address at now. The caller holds the lock.
func (service *Service) recordCreation(address string, now time.Time) {
	if !service.limitApplies(address) {
		return
	}
	recent := service.pruneCreations(address, now)
	service.creations[address] = append(recent, now)
}

// limitApplies reports whether the creation limit counts creations from address.
func (service *Service) limitApplies(address string) bool {
	return service.accountsPerAddressPerHour > 0 && !isLoopback(address)
}

// pruneCreations keeps the creations of address within the last hour and
// returns them. The caller holds the lock.
func (service *Service) pruneCreations(address string, now time.Time) []time.Time {
	var recent []time.Time
	for _, created := range service.creations[address] {
		if now.Sub(created) < accountCreationWindow {
			recent = append(recent, created)
		}
	}
	if len(recent) == 0 {
		delete(service.creations, address)
	} else {
		service.creations[address] = recent
	}
	return recent
}

// isLoopback reports whether address is a loopback IP address. Loopback is never
// limited, so several instances on one PC can be tested (SERVICE.md section 4).
func isLoopback(address string) bool {
	ip := net.ParseIP(address)
	return ip != nil && ip.IsLoopback()
}
