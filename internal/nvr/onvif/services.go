package onvif

import (
	"context"
	"log"
	"strings"

	onvifgo "github.com/EthanFlower1/onvif-go"
)

// ServiceInfo describes a single ONVIF service reported by GetServices.
type ServiceInfo struct {
	Namespace string       `json:"namespace"`
	XAddr     string       `json:"xaddr"`
	Version   OnvifVersion `json:"version"`
}

// OnvifVersion mirrors the library type for JSON serialisation.
type OnvifVersion struct {
	Major int `json:"major"`
	Minor int `json:"minor"`
}

// DetailedCapabilities holds the per-service capability structs obtained
// via each service's GetServiceCapabilities operation.  Fields are nil
// when the service is absent or the query failed.
type DetailedCapabilities struct {
	Device    *DeviceCapabilities    `json:"device,omitempty"`
	Media     *MediaCapabilities     `json:"media,omitempty"`
	Media2    *Media2Capabilities    `json:"media2,omitempty"`
	PTZ       *PTZCapabilities       `json:"ptz,omitempty"`
	Imaging   *ImagingCapabilities   `json:"imaging,omitempty"`
	Events    *EventCapabilities     `json:"events,omitempty"`
	Analytics *AnalyticsCapabilities `json:"analytics,omitempty"`
	DeviceIO  *DeviceIOCapabilities  `json:"device_io,omitempty"`
	Recording *RecordingCapabilities `json:"recording,omitempty"`
	Replay    *ReplayCapabilities    `json:"replay,omitempty"`
	Search    *SearchCapabilities    `json:"search,omitempty"`
}

// --- Per-service capability structs (mirror onvif-go types) ---

// DeviceCapabilities mirrors onvifgo.DeviceServiceCapabilities with JSON tags.
type DeviceCapabilities struct {
	Network  *NetworkCaps  `json:"network,omitempty"`
	Security *SecurityCaps `json:"security,omitempty"`
	System   *SystemCaps   `json:"system,omitempty"`
}

// NetworkCaps mirrors onvifgo.NetworkCapabilities.
type NetworkCaps struct {
	IPFilter          bool `json:"ip_filter"`
	ZeroConfiguration bool `json:"zero_configuration"`
	IPVersion6        bool `json:"ip_version6"`
	DynDNS            bool `json:"dyn_dns"`
}

// SecurityCaps mirrors onvifgo.SecurityCapabilities.
type SecurityCaps struct {
	TLS11                bool `json:"tls11"`
	TLS12                bool `json:"tls12"`
	OnboardKeyGeneration bool `json:"onboard_key_generation"`
	AccessPolicyConfig   bool `json:"access_policy_config"`
	X509Token            bool `json:"x509_token"`
	SAMLToken            bool `json:"saml_token"`
	KerberosToken        bool `json:"kerberos_token"`
	RELToken             bool `json:"rel_token"`
}

// SystemCaps mirrors onvifgo.SystemCapabilities.
type SystemCaps struct {
	DiscoveryResolve  bool `json:"discovery_resolve"`
	DiscoveryBye      bool `json:"discovery_bye"`
	RemoteDiscovery   bool `json:"remote_discovery"`
	SystemBackup      bool `json:"system_backup"`
	SystemLogging     bool `json:"system_logging"`
	FirmwareUpgrade   bool `json:"firmware_upgrade"`
}

// MediaCapabilities mirrors onvifgo.MediaServiceCapabilities.
type MediaCapabilities struct {
	SnapshotURI             bool `json:"snapshot_uri"`
	Rotation                bool `json:"rotation"`
	VideoSourceMode         bool `json:"video_source_mode"`
	OSD                     bool `json:"osd"`
	TemporaryOSDText        bool `json:"temporary_osd_text"`
	EXICompression          bool `json:"exi_compression"`
	MaximumNumberOfProfiles int  `json:"maximum_number_of_profiles"`
	RTPMulticast            bool `json:"rtp_multicast"`
	RTPTCP                  bool `json:"rtp_tcp"`
	RTPRTSPTCP              bool `json:"rtp_rtsp_tcp"`
}

// Media2Capabilities mirrors onvifgo.Media2ServiceCapabilities.
type Media2Capabilities struct {
	SnapshotUri     bool `json:"snapshot_uri"`
	Rotation        bool `json:"rotation"`
	VideoSourceMode bool `json:"video_source_mode"`
	OSD             bool `json:"osd"`
	Mask            bool `json:"mask"`
	SourceMask      bool `json:"source_mask"`
}

// PTZCapabilities mirrors onvifgo.PTZServiceCapabilities.
type PTZCapabilities struct {
	EFlip   bool `json:"eflip"`
	Reverse bool `json:"reverse"`
}

// ImagingCapabilities mirrors onvifgo.ImagingServiceCapabilities.
type ImagingCapabilities struct {
	ImageStabilization bool `json:"image_stabilization"`
	Presets            bool `json:"presets"`
}

// EventCapabilities mirrors onvifgo.EventServiceCapabilities.
type EventCapabilities struct {
	WSSubscriptionPolicySupport                   bool     `json:"ws_subscription_policy_support"`
	WSPausableSubscriptionManagerInterfaceSupport bool     `json:"ws_pausable_subscription_manager_interface_support"`
	MaxNotificationProducers                      int      `json:"max_notification_producers"`
	MaxPullPoints                                 int      `json:"max_pull_points"`
	PersistentNotificationStorage                 bool     `json:"persistent_notification_storage"`
	EventBrokerProtocols                          []string `json:"event_broker_protocols,omitempty"`
	MaxEventBrokers                               int      `json:"max_event_brokers"`
	MetadataOverMQTT                              bool     `json:"metadata_over_mqtt"`
}

// AnalyticsCapabilities mirrors onvifgo.AnalyticsServiceCapabilities.
type AnalyticsCapabilities struct {
	RuleSupport                        bool `json:"rule_support"`
	AnalyticsModuleSupport             bool `json:"analytics_module_support"`
	CellBasedSceneDescriptionSupported bool `json:"cell_based_scene_description_supported"`
}

// DeviceIOCapabilities mirrors onvifgo.DeviceIOServiceCapabilities.
type DeviceIOCapabilities struct {
	VideoSources            int  `json:"video_sources"`
	VideoOutputs            int  `json:"video_outputs"`
	AudioSources            int  `json:"audio_sources"`
	AudioOutputs            int  `json:"audio_outputs"`
	RelayOutputs            int  `json:"relay_outputs"`
	SerialPorts             int  `json:"serial_ports"`
	DigitalInputs           int  `json:"digital_inputs"`
	DigitalInputOptions     bool `json:"digital_input_options"`
	SerialPortConfiguration bool `json:"serial_port_configuration"`
}

// RecordingCapabilities mirrors onvifgo.RecordingServiceCapabilities.
type RecordingCapabilities struct {
	DynamicRecordings          bool     `json:"dynamic_recordings"`
	DynamicTracks              bool     `json:"dynamic_tracks"`
	MaxStringLength            int      `json:"max_string_length"`
	MaxRecordings              int      `json:"max_recordings"`
	MaxRecordingJobs           int      `json:"max_recording_jobs"`
	Options                    bool     `json:"options"`
	MetadataRecording          bool     `json:"metadata_recording"`
	SupportedExportFileFormats []string `json:"supported_export_file_formats,omitempty"`
}

// ReplayCapabilities mirrors onvifgo.ReplayServiceCapabilities.
type ReplayCapabilities struct {
	ReversePlayback bool `json:"reverse_playback"`
	RTPRTSP_TCP     bool `json:"rtp_rtsp_tcp"`
}

// SearchCapabilities mirrors onvifgo.SearchServiceCapabilities.
type SearchCapabilities struct {
	MetadataSearch bool `json:"metadata_search"`
}

// getServicesDetailed calls GetServices on the underlying onvif-go client and
// returns structured ServiceInfo entries including version information.
func getServicesDetailed(ctx context.Context, dev *onvifgo.Client) []ServiceInfo {
	svcs, err := dev.GetServices(ctx, false)
	if err != nil {
		return nil
	}

	infos := make([]ServiceInfo, 0, len(svcs))
	for _, svc := range svcs {
		infos = append(infos, ServiceInfo{
			Namespace: svc.Namespace,
			XAddr:     svc.XAddr,
			Version: OnvifVersion{
				Major: svc.Version.Major,
				Minor: svc.Version.Minor,
			},
		})
	}
	return infos
}

// friendlyServiceName maps a namespace to a short name used as a key.
func friendlyServiceName(ns string) string {
	lower := strings.ToLower(ns)
	switch {
	case strings.Contains(lower, "ver20/media"):
		return "media2"
	case strings.Contains(lower, "media"):
		return "media"
	case strings.Contains(lower, "ptz"):
		return "ptz"
	case strings.Contains(lower, "imaging"):
		return "imaging"
	case strings.Contains(lower, "events"), strings.Contains(lower, "event"):
		return "events"
	case strings.Contains(lower, "analytics"):
		return "analytics"
	case strings.Contains(lower, "deviceio"):
		return "deviceio"
	case strings.Contains(lower, "recording"):
		return "recording"
	case strings.Contains(lower, "replay"):
		return "replay"
	case strings.Contains(lower, "search"):
		return "search"
	case strings.Contains(lower, "device"):
		return "device"
	default:
		return ""
	}
}

// queryDetailedCapabilities queries each supported service's
// GetServiceCapabilities and returns a populated DetailedCapabilities.
func queryDetailedCapabilities(ctx context.Context, client *Client) *DetailedCapabilities {
	dc := &DetailedCapabilities{}

	// Device service capabilities (always available).
	if devCaps, err := client.Dev.GetServiceCapabilities(ctx); err == nil && devCaps != nil {
		dc.Device = convertDeviceCaps(devCaps)
	} else if err != nil {
		log.Printf("onvif services: GetServiceCapabilities (device) failed: %v", err)
	}

	// Media (Profile S).
	if client.HasService("media") {
		if caps, err := client.Dev.GetMediaServiceCapabilities(ctx); err == nil && caps != nil {
			dc.Media = &MediaCapabilities{
				SnapshotURI:             caps.SnapshotURI,
				Rotation:                caps.Rotation,
				VideoSourceMode:         caps.VideoSourceMode,
				OSD:                     caps.OSD,
				TemporaryOSDText:        caps.TemporaryOSDText,
				EXICompression:          caps.EXICompression,
				MaximumNumberOfProfiles: caps.MaximumNumberOfProfiles,
				RTPMulticast:            caps.RTPMulticast,
				RTPTCP:                  caps.RTPTCP,
				RTPRTSPTCP:              caps.RTPRTSPTCP,
			}
		} else if err != nil {
			log.Printf("onvif services: GetMediaServiceCapabilities failed: %v", err)
		}
	}

	// Media2 (Profile T).
	if client.HasService("media2") {
		if caps, err := client.Dev.GetMedia2ServiceCapabilities(ctx); err == nil && caps != nil {
			dc.Media2 = &Media2Capabilities{
				SnapshotUri:     caps.SnapshotUri,
				Rotation:        caps.Rotation,
				VideoSourceMode: caps.VideoSourceMode,
				OSD:             caps.OSD,
				Mask:            caps.Mask,
				SourceMask:      caps.SourceMask,
			}
		} else if err != nil {
			log.Printf("onvif services: GetMedia2ServiceCapabilities failed: %v", err)
		}
	}

	// PTZ.
	if client.HasService("ptz") {
		if caps, err := client.Dev.GetPTZServiceCapabilities(ctx); err == nil && caps != nil {
			dc.PTZ = &PTZCapabilities{
				EFlip:   caps.EFlip,
				Reverse: caps.Reverse,
			}
		} else if err != nil {
			log.Printf("onvif services: GetPTZServiceCapabilities failed: %v", err)
		}
	}

	// Imaging.
	if client.HasService("imaging") {
		if caps, err := client.Dev.GetImagingServiceCapabilities(ctx); err == nil && caps != nil {
			dc.Imaging = &ImagingCapabilities{
				ImageStabilization: caps.ImageStabilization,
				Presets:            caps.Presets,
			}
		} else if err != nil {
			log.Printf("onvif services: GetImagingServiceCapabilities failed: %v", err)
		}
	}

	// Events.
	if client.HasService("events") {
		if caps, err := client.Dev.GetEventServiceCapabilities(ctx); err == nil && caps != nil {
			dc.Events = &EventCapabilities{
				WSSubscriptionPolicySupport:                   caps.WSSubscriptionPolicySupport,
				WSPausableSubscriptionManagerInterfaceSupport: caps.WSPausableSubscriptionManagerInterfaceSupport,
				MaxNotificationProducers:                      caps.MaxNotificationProducers,
				MaxPullPoints:                                 caps.MaxPullPoints,
				PersistentNotificationStorage:                 caps.PersistentNotificationStorage,
				EventBrokerProtocols:                          caps.EventBrokerProtocols,
				MaxEventBrokers:                               caps.MaxEventBrokers,
				MetadataOverMQTT:                              caps.MetadataOverMQTT,
			}
		} else if err != nil {
			log.Printf("onvif services: GetEventServiceCapabilities failed: %v", err)
		}
	}

	// Analytics.
	if client.HasService("analytics") {
		if caps, err := client.Dev.GetAnalyticsServiceCapabilities(ctx); err == nil && caps != nil {
			dc.Analytics = &AnalyticsCapabilities{
				RuleSupport:                        caps.RuleSupport,
				AnalyticsModuleSupport:             caps.AnalyticsModuleSupport,
				CellBasedSceneDescriptionSupported: caps.CellBasedSceneDescriptionSupported,
			}
		} else if err != nil {
			log.Printf("onvif services: GetAnalyticsServiceCapabilities failed: %v", err)
		}
	}

	// DeviceIO.
	if client.HasService("deviceio") {
		if caps, err := client.Dev.GetDeviceIOServiceCapabilities(ctx); err == nil && caps != nil {
			dc.DeviceIO = &DeviceIOCapabilities{
				VideoSources:            caps.VideoSources,
				VideoOutputs:            caps.VideoOutputs,
				AudioSources:            caps.AudioSources,
				AudioOutputs:            caps.AudioOutputs,
				RelayOutputs:            caps.RelayOutputs,
				SerialPorts:             caps.SerialPorts,
				DigitalInputs:           caps.DigitalInputs,
				DigitalInputOptions:     caps.DigitalInputOptions,
				SerialPortConfiguration: caps.SerialPortConfiguration,
			}
		} else if err != nil {
			log.Printf("onvif services: GetDeviceIOServiceCapabilities failed: %v", err)
		}
	}

	// Recording.
	if client.HasService("recording") {
		if caps, err := client.Dev.GetRecordingServiceCapabilities(ctx); err == nil && caps != nil {
			dc.Recording = &RecordingCapabilities{
				DynamicRecordings:          caps.DynamicRecordings,
				DynamicTracks:              caps.DynamicTracks,
				MaxStringLength:            caps.MaxStringLength,
				MaxRecordings:              caps.MaxRecordings,
				MaxRecordingJobs:           caps.MaxRecordingJobs,
				Options:                    caps.Options,
				MetadataRecording:          caps.MetadataRecording,
				SupportedExportFileFormats: caps.SupportedExportFileFormats,
			}
		} else if err != nil {
			log.Printf("onvif services: GetRecordingServiceCapabilities failed: %v", err)
		}
	}

	// Replay.
	if client.HasService("replay") {
		if caps, err := client.Dev.GetReplayServiceCapabilities(ctx); err == nil && caps != nil {
			dc.Replay = &ReplayCapabilities{
				ReversePlayback: caps.ReversePlayback,
				RTPRTSP_TCP:     caps.RTPRTSP_TCP,
			}
		} else if err != nil {
			log.Printf("onvif services: GetReplayServiceCapabilities failed: %v", err)
		}
	}

	// Search.
	if client.HasService("search") {
		if caps, err := client.Dev.GetSearchServiceCapabilities(ctx); err == nil && caps != nil {
			dc.Search = &SearchCapabilities{
				MetadataSearch: caps.MetadataSearch,
			}
		} else if err != nil {
			log.Printf("onvif services: GetSearchServiceCapabilities failed: %v", err)
		}
	}

	return dc
}

// convertDeviceCaps converts the library's DeviceServiceCapabilities to our type.
func convertDeviceCaps(src *onvifgo.DeviceServiceCapabilities) *DeviceCapabilities {
	dc := &DeviceCapabilities{}
	if src.Network != nil {
		dc.Network = &NetworkCaps{
			IPFilter:          src.Network.IPFilter,
			ZeroConfiguration: src.Network.ZeroConfiguration,
			IPVersion6:        src.Network.IPVersion6,
			DynDNS:            src.Network.DynDNS,
		}
	}
	if src.Security != nil {
		dc.Security = &SecurityCaps{
			TLS11:                src.Security.TLS11,
			TLS12:                src.Security.TLS12,
			OnboardKeyGeneration: src.Security.OnboardKeyGeneration,
			AccessPolicyConfig:   src.Security.AccessPolicyConfig,
			X509Token:            src.Security.X509Token,
			SAMLToken:            src.Security.SAMLToken,
			KerberosToken:        src.Security.KerberosToken,
			RELToken:             src.Security.RELToken,
		}
	}
	if src.System != nil {
		dc.System = &SystemCaps{
			DiscoveryResolve: src.System.DiscoveryResolve,
			DiscoveryBye:     src.System.DiscoveryBye,
			RemoteDiscovery:  src.System.RemoteDiscovery,
			SystemBackup:     src.System.SystemBackup,
			SystemLogging:    src.System.SystemLogging,
			FirmwareUpgrade:  src.System.FirmwareUpgrade,
		}
	}
	return dc
}
