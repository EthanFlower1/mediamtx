package onvif

import (
	"context"
	"fmt"

	onvifdevice "github.com/use-go/onvif/device"
	sdkdevice "github.com/use-go/onvif/sdk/device"
	onviftypes "github.com/use-go/onvif/xsd/onvif"
)

// RelayOutput represents a single relay output on an ONVIF device.
type RelayOutput struct {
	Token     string `json:"token"`
	Mode      string `json:"mode"`
	IdleState string `json:"idle_state"`
}

// GetRelayOutputs retrieves the relay outputs from an ONVIF device.
func GetRelayOutputs(xaddr, username, password string) ([]RelayOutput, error) {
	client, err := NewClient(xaddr, username, password)
	if err != nil {
		return nil, err
	}

	ctx := context.Background()
	resp, err := sdkdevice.Call_GetRelayOutputs(ctx, client.Dev, onvifdevice.GetRelayOutputs{})
	if err != nil {
		return nil, fmt.Errorf("get relay outputs: %w", err)
	}

	// The SDK returns a single RelayOutput struct. If it has a token, include it.
	var outputs []RelayOutput
	if string(resp.RelayOutputs.Token) != "" {
		outputs = append(outputs, RelayOutput{
			Token:     string(resp.RelayOutputs.Token),
			Mode:      string(resp.RelayOutputs.Properties.Mode),
			IdleState: string(resp.RelayOutputs.Properties.IdleState),
		})
	}

	return outputs, nil
}

// SetRelayOutputState sets the logical state of a relay output on an ONVIF device.
func SetRelayOutputState(xaddr, username, password, token string, active bool) error {
	client, err := NewClient(xaddr, username, password)
	if err != nil {
		return err
	}

	state := onviftypes.RelayLogicalState("inactive")
	if active {
		state = "active"
	}

	ctx := context.Background()
	_, err = sdkdevice.Call_SetRelayOutputState(ctx, client.Dev, onvifdevice.SetRelayOutputState{
		RelayOutputToken: onviftypes.ReferenceToken(token),
		LogicalState:     state,
	})
	if err != nil {
		return fmt.Errorf("set relay output state: %w", err)
	}

	return nil
}
