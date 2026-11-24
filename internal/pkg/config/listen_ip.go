package config

import (
	"fmt"
	"net"
	"strings"

	"github.com/pkg/errors"
)

// Sentinel errors of rest[].listen_ip. Use errors.Is to distinguish.
var (
	ErrListenIPNotAnIP = errors.New(
		"rest.listen_ip: expected an IP literal (127.0.0.1, 0.0.0.0, [::1])")
	ErrListenIPBareIPv6 = errors.New(
		"rest.listen_ip: an IPv6 literal has to carry its brackets")
	ErrListenIPOnClient = errors.New(
		"rest.listen_ip: a client transport dials an address instead of listening on one")
)

// validateListenIP checks the declared listen address of a REST transport.
//
// The value becomes the default of the server's `ip` key, and the server joins
// it with the port by concatenation (rest.ServerConfig: IP + ":" + Port), so
// anything but an address literal would reach net.Listen as a string it cannot
// bind — and the generator would have printed `done` over a project that
// cannot start. A host name is rejected for the same reason the key is called
// `ip`: resolving a name at bind time is a different contract, not a spelling
// of this one.
//
// An IPv6 literal is accepted only in its bracketed form: `::1` concatenated
// with the port gives `::1:8080`, which net.Listen reads as an address with
// too many colons, while `[::1]:8080` is what it expects. The error says so
// rather than accepting the value and leaving the failure to the first start.
func (r Rest) validateListenIP() (bool, string) {
	if r.ListenIP == "" {
		return true, ""
	}

	if r.GeneratorType == GeneratorTypeOgenClient {
		return false, fmt.Sprintf("%s, got: %s", ErrListenIPOnClient, r.ListenIP)
	}

	if inner, bracketed := strings.CutPrefix(r.ListenIP, "["); bracketed {
		addr, closed := strings.CutSuffix(inner, "]")
		if !closed || net.ParseIP(addr) == nil {
			return false, fmt.Sprintf("%s, got: %s", ErrListenIPNotAnIP, r.ListenIP)
		}

		return true, ""
	}

	ip := net.ParseIP(r.ListenIP)
	if ip == nil {
		return false, fmt.Sprintf("%s, got: %s", ErrListenIPNotAnIP, r.ListenIP)
	}

	if ip.To4() == nil {
		return false, fmt.Sprintf("%s, got: %s (write it as [%s])",
			ErrListenIPBareIPv6, r.ListenIP, r.ListenIP)
	}

	return true, ""
}
