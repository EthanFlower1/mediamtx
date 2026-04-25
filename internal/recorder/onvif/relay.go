package onvif

import (
	"context"
	"fmt"

	onvifgo "github.com/EthanFlower1/onvif-go"
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
	rawOutputs, err := client.Dev.GetRelayOutputs(ctx)
	if err != nil {
		return nil, fmt.Errorf("get relay outputs: %w", err)
	}

	var outputs []RelayOutput
	for _, o := range rawOutputs {
		if o.Token != "" {
			outputs = append(outputs, RelayOutput{
				Token:     o.Token,
				Mode:      string(o.Properties.Mode),
				IdleState: string(o.Properties.IdleState),
			})
		}
	}

	return outputs, nil
}

// SetRelayOutputState sets the logical state of a relay output on an ONVIF device.
func SetRelayOutputState(xaddr, username, password, token string, active bool) error {
	client, err := NewClient(xaddr, username, password)
	if err != nil {
		return err
	}

	state := onvifgo.RelayLogicalState("inactive")
	if active {
		state = "active"
	}

	ctx := context.Background()
	if err := client.Dev.SetRelayOutputState(ctx, token, state); err != nil {
		return fmt.Errorf("set relay output state: %w", err)
	}

	return nil
}
