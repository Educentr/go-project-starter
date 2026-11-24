package config

import (
	"strings"
	"testing"
)

func TestRest_ListenIP(t *testing.T) {
	tests := []struct {
		name    string
		rest    Rest
		wantErr error
	}{
		{
			name: "not declared",
			rest: Rest{Name: "api", GeneratorType: "ogen", Path: []string{"api.yaml"}, Port: 8080, Version: "v1"},
		},
		{
			name: "loopback",
			rest: Rest{
				Name: "api", GeneratorType: "ogen", Path: []string{"api.yaml"}, Port: 8080, Version: "v1",
				ListenIP: "127.0.0.1",
			},
		},
		{
			name: "all interfaces",
			rest: Rest{
				Name: "api", GeneratorType: "ogen", Path: []string{"api.yaml"}, Port: 8080, Version: "v1",
				ListenIP: "0.0.0.0",
			},
		},
		{
			// The metrics server is one of the servers a project most wants on
			// loopback, and `sys` leaves Rest.IsValid before the generator-type
			// switch — so its address has to be checked ahead of that shortcut.
			name: "sys server",
			rest: Rest{
				Name: "sys", GeneratorType: "template", GeneratorTemplate: "sys", Port: 8085, Version: "v1",
				ListenIP: "127.0.0.1",
			},
		},
		{
			name: "sys server with a broken address",
			rest: Rest{
				Name: "sys", GeneratorType: "template", GeneratorTemplate: "sys", Port: 8085, Version: "v1",
				ListenIP: "127.0.0",
			},
			wantErr: ErrListenIPNotAnIP,
		},
		{
			name: "bracketed IPv6",
			rest: Rest{
				Name: "api", GeneratorType: "ogen", Path: []string{"api.yaml"}, Port: 8080, Version: "v1",
				ListenIP: "[::1]",
			},
		},
		{
			// The server joins the address and the port by concatenation, so a
			// bare IPv6 literal would reach net.Listen as "::1:8080".
			name: "bare IPv6",
			rest: Rest{
				Name: "api", GeneratorType: "ogen", Path: []string{"api.yaml"}, Port: 8080, Version: "v1",
				ListenIP: "::1",
			},
			wantErr: ErrListenIPBareIPv6,
		},
		{
			name: "unclosed bracket",
			rest: Rest{
				Name: "api", GeneratorType: "ogen", Path: []string{"api.yaml"}, Port: 8080, Version: "v1",
				ListenIP: "[::1",
			},
			wantErr: ErrListenIPNotAnIP,
		},
		{
			name: "host name",
			rest: Rest{
				Name: "api", GeneratorType: "ogen", Path: []string{"api.yaml"}, Port: 8080, Version: "v1",
				ListenIP: "localhost",
			},
			wantErr: ErrListenIPNotAnIP,
		},
		{
			name: "address with a port",
			rest: Rest{
				Name: "api", GeneratorType: "ogen", Path: []string{"api.yaml"}, Port: 8080, Version: "v1",
				ListenIP: "127.0.0.1:8080",
			},
			wantErr: ErrListenIPNotAnIP,
		},
		{
			// A client dials an address it reads from the tree; a listen
			// address on one would configure nothing.
			name: "on a client",
			rest: Rest{
				Name: "upstream", GeneratorType: GeneratorTypeOgenClient, Path: []string{"api.yaml"}, Version: "v1",
				ListenIP: "127.0.0.1",
			},
			wantErr: ErrListenIPOnClient,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ok, msg := tt.rest.validateListenIP()

			if tt.wantErr == nil {
				if !ok {
					t.Fatalf("validateListenIP() = false, %q, want true", msg)
				}

				return
			}

			if ok {
				t.Fatalf("validateListenIP() = true, want the %v error", tt.wantErr)
			}

			if !strings.Contains(msg, tt.wantErr.Error()) {
				t.Errorf("validateListenIP() message = %q, want it to report %v", msg, tt.wantErr)
			}

			if !strings.Contains(msg, tt.rest.ListenIP) {
				t.Errorf("validateListenIP() message = %q, want the offending value %q in it", msg, tt.rest.ListenIP)
			}
		})
	}
}

// TestRest_IsValidChecksListenIP checks that the address is validated through
// Rest.IsValid, the entry point config loading calls: a check reachable only
// by its own method gates nothing.
func TestRest_IsValidChecksListenIP(t *testing.T) {
	rest := Rest{
		Name: "sys", GeneratorType: "template", GeneratorTemplate: "sys", Port: 8085, Version: "v1",
		ListenIP: "nope",
	}

	ok, msg := rest.IsValid("")
	if ok {
		t.Fatal("Rest.IsValid() accepted listen_ip: nope")
	}

	if !strings.Contains(msg, ErrListenIPNotAnIP.Error()) {
		t.Errorf("Rest.IsValid() message = %q, want it to report %v", msg, ErrListenIPNotAnIP)
	}
}
