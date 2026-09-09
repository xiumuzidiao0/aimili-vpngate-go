package nodes

import (
	"encoding/base64"
	"testing"
)

func TestValidateOpenVPNConfig_Safe(t *testing.T) {
	safeConf := `
client
dev tun
proto udp
remote 123.45.67.89 1194
cipher AES-128-CBC
auth SHA1
resolv-retry infinite
nobind
persist-key
persist-tun
verb 3
<ca>
-----BEGIN CERTIFICATE-----
MIIDXTCCAkWgAwIBAgIJAL7
-----END CERTIFICATE-----
</ca>
`
	err := ValidateOpenVPNConfig(safeConf)
	if err != nil {
		t.Fatalf("expected safe config to pass, got: %v", err)
	}
}

func TestValidateOpenVPNConfig_RejectUnsafe(t *testing.T) {
	unsafeConfs := []struct {
		name string
		conf string
	}{
		{
			name: "script-security injection",
			conf: "client\ndev tun\nremote 1.1.1.1 1194\nscript-security 2\nup /bin/sh\n",
		},
		{
			name: "plugin directive",
			conf: "client\ndev tun\nremote 1.1.1.1 1194\nplugin /tmp/evil.so\n",
		},
		{
			name: "missing remote",
			conf: "client\ndev tun\ncipher AES-128-CBC\n",
		},
		{
			name: "unclosed tag",
			conf: "client\ndev tun\nremote 1.1.1.1 1194\n<ca>\nfoobar\n",
		},
	}

	for _, tc := range unsafeConfs {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateOpenVPNConfig(tc.conf)
			if err == nil {
				t.Fatalf("expected unsafe config to fail, but it passed")
			}
		})
	}
}

func TestDecodeConfig(t *testing.T) {
	plain := "client\ndev tun\nremote 1.2.3.4 1194\n"
	encoded := base64.StdEncoding.EncodeToString([]byte(plain))

	decoded, err := DecodeConfig(encoded)
	if err != nil {
		t.Fatalf("failed to decode valid base64: %v", err)
	}
	if decoded != plain {
		t.Fatalf("decoded content mismatch")
	}
}
