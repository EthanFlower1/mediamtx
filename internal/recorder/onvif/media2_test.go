package onvif

import (
	"encoding/xml"
	"testing"
)

func TestParseCreateProfileResponse(t *testing.T) {
	raw := `<?xml version="1.0" encoding="UTF-8"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope">
  <s:Body>
    <CreateProfileResponse xmlns="http://www.onvif.org/ver20/media/wsdl">
      <Token>profile_tok_1</Token>
      <Name>TestProfile</Name>
    </CreateProfileResponse>
  </s:Body>
</s:Envelope>`

	var env media2Envelope
	if err := xml.Unmarshal([]byte(raw), &env); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if env.Body.Fault != nil {
		t.Fatalf("unexpected fault: %s", env.Body.Fault.Faultstring)
	}
	if env.Body.CreateProfileResponse == nil {
		t.Fatal("expected CreateProfileResponse, got nil")
	}
	if env.Body.CreateProfileResponse.Token != "profile_tok_1" {
		t.Errorf("expected token 'profile_tok_1', got %q", env.Body.CreateProfileResponse.Token)
	}
	if env.Body.CreateProfileResponse.Name != "TestProfile" {
		t.Errorf("expected name 'TestProfile', got %q", env.Body.CreateProfileResponse.Name)
	}
}

func TestParseGetVideoSourceConfigurationsResponse(t *testing.T) {
	raw := `<?xml version="1.0" encoding="UTF-8"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope">
  <s:Body>
    <GetVideoSourceConfigurationsResponse xmlns="http://www.onvif.org/ver20/media/wsdl">
      <Configurations token="vsc_1">
        <Name>VideoSrc1</Name>
        <SourceToken>src_001</SourceToken>
        <Bounds x="0" y="0" width="1920" height="1080"/>
      </Configurations>
      <Configurations token="vsc_2">
        <Name>VideoSrc2</Name>
        <SourceToken>src_002</SourceToken>
      </Configurations>
    </GetVideoSourceConfigurationsResponse>
  </s:Body>
</s:Envelope>`

	var env media2Envelope
	if err := xml.Unmarshal([]byte(raw), &env); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if env.Body.Fault != nil {
		t.Fatalf("unexpected fault: %s", env.Body.Fault.Faultstring)
	}
	resp := env.Body.GetVideoSourceConfigurationsResponse
	if resp == nil {
		t.Fatal("expected GetVideoSourceConfigurationsResponse, got nil")
	}
	if len(resp.Configurations) != 2 {
		t.Fatalf("expected 2 configs, got %d", len(resp.Configurations))
	}

	c := resp.Configurations[0]
	if c.Token != "vsc_1" || c.Name != "VideoSrc1" || c.SourceToken != "src_001" {
		t.Errorf("unexpected first config: %+v", c)
	}
	if c.Bounds == nil {
		t.Fatal("expected bounds on first config")
	}
	if c.Bounds.Width != 1920 || c.Bounds.Height != 1080 {
		t.Errorf("unexpected bounds: %+v", c.Bounds)
	}

	c2 := resp.Configurations[1]
	if c2.Token != "vsc_2" {
		t.Errorf("unexpected second config token: %q", c2.Token)
	}
	if c2.Bounds != nil {
		t.Errorf("expected nil bounds on second config, got %+v", c2.Bounds)
	}
}

func TestParseGetVideoSourceConfigurationOptionsResponse(t *testing.T) {
	raw := `<?xml version="1.0" encoding="UTF-8"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope">
  <s:Body>
    <GetVideoSourceConfigurationOptionsResponse xmlns="http://www.onvif.org/ver20/media/wsdl">
      <Options>
        <BoundsRange>
          <XRange><Min>0</Min><Max>0</Max></XRange>
          <YRange><Min>0</Min><Max>0</Max></YRange>
          <WidthRange><Min>1</Min><Max>1920</Max></WidthRange>
          <HeightRange><Min>1</Min><Max>1080</Max></HeightRange>
        </BoundsRange>
        <MaximumNumberOfProfiles>6</MaximumNumberOfProfiles>
      </Options>
    </GetVideoSourceConfigurationOptionsResponse>
  </s:Body>
</s:Envelope>`

	var env media2Envelope
	if err := xml.Unmarshal([]byte(raw), &env); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	resp := env.Body.GetVideoSourceConfigurationOptionsResponse
	if resp == nil {
		t.Fatal("expected response, got nil")
	}
	if resp.Options.MaximumNumberOfProfiles != 6 {
		t.Errorf("expected 6 max profiles, got %d", resp.Options.MaximumNumberOfProfiles)
	}
	if resp.Options.BoundsRange == nil {
		t.Fatal("expected bounds range")
	}
	if resp.Options.BoundsRange.WidthRange.Max != 1920 {
		t.Errorf("expected max width 1920, got %d", resp.Options.BoundsRange.WidthRange.Max)
	}
}

func TestParseGetAudioSourceConfigurationsResponse(t *testing.T) {
	raw := `<?xml version="1.0" encoding="UTF-8"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope">
  <s:Body>
    <GetAudioSourceConfigurationsResponse xmlns="http://www.onvif.org/ver20/media/wsdl">
      <Configurations token="asc_1">
        <Name>AudioInput1</Name>
        <SourceToken>audio_src_001</SourceToken>
      </Configurations>
    </GetAudioSourceConfigurationsResponse>
  </s:Body>
</s:Envelope>`

	var env media2Envelope
	if err := xml.Unmarshal([]byte(raw), &env); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	resp := env.Body.GetAudioSourceConfigurationsResponse
	if resp == nil {
		t.Fatal("expected response, got nil")
	}
	if len(resp.Configurations) != 1 {
		t.Fatalf("expected 1 config, got %d", len(resp.Configurations))
	}
	c := resp.Configurations[0]
	if c.Token != "asc_1" || c.Name != "AudioInput1" || c.SourceToken != "audio_src_001" {
		t.Errorf("unexpected config: %+v", c)
	}
}

func TestParseSOAPFault(t *testing.T) {
	raw := `<?xml version="1.0" encoding="UTF-8"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope">
  <s:Body>
    <s:Fault>
      <faultstring>Action not supported</faultstring>
    </s:Fault>
  </s:Body>
</s:Envelope>`

	var env media2Envelope
	if err := xml.Unmarshal([]byte(raw), &env); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if env.Body.Fault == nil {
		t.Fatal("expected fault, got nil")
	}
	if env.Body.Fault.Faultstring != "Action not supported" {
		t.Errorf("unexpected faultstring: %q", env.Body.Fault.Faultstring)
	}
}
