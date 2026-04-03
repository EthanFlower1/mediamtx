package onvif

import (
	"context"
	"fmt"
	"time"

	onvifgo "github.com/EthanFlower1/onvif-go"
)

// MetadataConfigInfo holds metadata configuration details from an ONVIF device.
type MetadataConfigInfo struct {
	Token          string `json:"token"`
	Name           string `json:"name"`
	UseCount       int    `json:"use_count"`
	Analytics      bool   `json:"analytics"`
	SessionTimeout string `json:"session_timeout"`
}

func convertMetadataConfig(cfg *onvifgo.MetadataConfiguration) *MetadataConfigInfo {
	return &MetadataConfigInfo{
		Token:          cfg.Token,
		Name:           cfg.Name,
		UseCount:       cfg.UseCount,
		Analytics:      cfg.Analytics,
		SessionTimeout: cfg.SessionTimeout.String(),
	}
}

// GetMetadataConfigurations returns all metadata configurations from the device.
func GetMetadataConfigurations(xaddr, username, password string) ([]*MetadataConfigInfo, error) {
	client, err := NewClient(xaddr, username, password)
	if err != nil {
		return nil, err
	}

	ctx := context.Background()
	configs, err := client.Dev.GetMetadataConfigurations(ctx)
	if err != nil {
		return nil, fmt.Errorf("get metadata configurations: %w", err)
	}

	var result []*MetadataConfigInfo
	for _, c := range configs {
		result = append(result, convertMetadataConfig(c))
	}
	return result, nil
}

// GetMetadataConfiguration returns a single metadata configuration by token.
func GetMetadataConfiguration(xaddr, username, password, configToken string) (*MetadataConfigInfo, error) {
	client, err := NewClient(xaddr, username, password)
	if err != nil {
		return nil, err
	}

	ctx := context.Background()
	cfg, err := client.Dev.GetMetadataConfiguration(ctx, configToken)
	if err != nil {
		return nil, fmt.Errorf("get metadata configuration: %w", err)
	}
	return convertMetadataConfig(cfg), nil
}

// SetMetadataConfiguration updates a metadata configuration on the device.
func SetMetadataConfiguration(xaddr, username, password string, cfg *MetadataConfigInfo) error {
	client, err := NewClient(xaddr, username, password)
	if err != nil {
		return err
	}

	timeout, err := time.ParseDuration(cfg.SessionTimeout)
	if err != nil {
		return fmt.Errorf("parse session timeout: %w", err)
	}

	mc := &onvifgo.MetadataConfiguration{
		Token:          cfg.Token,
		Name:           cfg.Name,
		UseCount:       cfg.UseCount,
		Analytics:      cfg.Analytics,
		SessionTimeout: timeout,
	}

	ctx := context.Background()
	if err := client.Dev.SetMetadataConfiguration(ctx, mc, true); err != nil {
		return fmt.Errorf("set metadata configuration: %w", err)
	}
	return nil
}

// AddMetadataToProfile adds a metadata configuration to a profile.
func AddMetadataToProfile(xaddr, username, password, profileToken, configToken string) error {
	client, err := NewClient(xaddr, username, password)
	if err != nil {
		return err
	}

	ctx := context.Background()
	if err := client.Dev.AddMetadataConfiguration(ctx, profileToken, configToken); err != nil {
		return fmt.Errorf("add metadata to profile: %w", err)
	}
	return nil
}

// RemoveMetadataFromProfile removes the metadata configuration from a profile.
func RemoveMetadataFromProfile(xaddr, username, password, profileToken string) error {
	client, err := NewClient(xaddr, username, password)
	if err != nil {
		return err
	}

	ctx := context.Background()
	if err := client.Dev.RemoveMetadataConfiguration(ctx, profileToken); err != nil {
		return fmt.Errorf("remove metadata from profile: %w", err)
	}
	return nil
}
