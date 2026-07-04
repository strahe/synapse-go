package storage

import (
	"net"
	"testing"
)

func TestIsPrivateAddress_IPv4MappedIPv6(t *testing.T) {
	tests := []struct {
		name string
		ip   string
		want bool
	}{
		{name: "mapped loopback", ip: "::ffff:127.0.0.1", want: true},
		{name: "mapped private", ip: "::ffff:10.0.0.1", want: true},
		{name: "mapped link local metadata", ip: "::ffff:169.254.169.254", want: true},
		{name: "mapped public", ip: "::ffff:8.8.8.8", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ip := net.ParseIP(tt.ip)
			if ip == nil {
				t.Fatalf("ParseIP(%q)=nil", tt.ip)
			}
			if got := isPrivateAddress(ip); got != tt.want {
				t.Fatalf("isPrivateAddress(%q)=%v want %v", tt.ip, got, tt.want)
			}
		})
	}
}
