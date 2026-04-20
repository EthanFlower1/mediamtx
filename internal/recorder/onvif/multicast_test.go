package onvif

import "testing"

func TestValidateMulticastAddress(t *testing.T) {
	tests := []struct {
		addr    string
		wantErr bool
	}{
		{"239.1.1.10", false},
		{"224.0.0.1", false},
		{"239.255.255.255", false},
		{"192.168.1.1", true},     // not multicast
		{"223.255.255.255", true}, // just below range
		{"240.0.0.0", true},       // just above range
		{"", true},                // empty
		{"not-an-ip", true},       // invalid
		{"256.1.1.1", true},       // invalid octet
	}
	for _, tt := range tests {
		t.Run(tt.addr, func(t *testing.T) {
			err := ValidateMulticastAddress(tt.addr)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateMulticastAddress(%q) error = %v, wantErr %v", tt.addr, err, tt.wantErr)
			}
		})
	}
}

func TestParseMulticastConfigResponse(t *testing.T) {
	xmlResp := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope"
            xmlns:trt="http://www.onvif.org/ver10/media/wsdl"
            xmlns:tt="http://www.onvif.org/ver10/schema">
  <s:Body>
    <trt:GetVideoEncoderConfigurationResponse>
      <trt:Configuration token="encoder_1">
        <tt:Name>Main Encoder</tt:Name>
        <tt:Multicast>
          <tt:Address>
            <tt:Type>IPv4</tt:Type>
            <tt:IPv4Address>239.1.1.10</tt:IPv4Address>
          </tt:Address>
          <tt:Port>5004</tt:Port>
          <tt:TTL>5</tt:TTL>
          <tt:AutoStart>false</tt:AutoStart>
        </tt:Multicast>
      </trt:Configuration>
    </trt:GetVideoEncoderConfigurationResponse>
  </s:Body>
</s:Envelope>`)

	cfg, err := parseMulticastConfigResponse(xmlResp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Address != "239.1.1.10" {
		t.Errorf("address = %q, want %q", cfg.Address, "239.1.1.10")
	}
	if cfg.Port != 5004 {
		t.Errorf("port = %d, want %d", cfg.Port, 5004)
	}
	if cfg.TTL != 5 {
		t.Errorf("ttl = %d, want %d", cfg.TTL, 5)
	}
}

func TestParseMulticastConfigResponse_Fault(t *testing.T) {
	xmlResp := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope">
  <s:Body>
    <s:Fault>
      <faultstring>not supported</faultstring>
    </s:Fault>
  </s:Body>
</s:Envelope>`)

	_, err := parseMulticastConfigResponse(xmlResp)
	if err == nil {
		t.Fatal("expected error for SOAP fault, got nil")
	}
}

func TestParseMulticastStreamUriResponse(t *testing.T) {
	xmlResp := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope"
            xmlns:trt="http://www.onvif.org/ver10/media/wsdl"
            xmlns:tt="http://www.onvif.org/ver10/schema">
  <s:Body>
    <trt:GetStreamUriResponse>
      <trt:MediaUri>
        <tt:Uri>rtp://239.1.1.10:5004</tt:Uri>
      </trt:MediaUri>
    </trt:GetStreamUriResponse>
  </s:Body>
</s:Envelope>`)

	uri, err := parseMedia1StreamUriResponse(xmlResp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if uri != "rtp://239.1.1.10:5004" {
		t.Errorf("uri = %q, want %q", uri, "rtp://239.1.1.10:5004")
	}
}
