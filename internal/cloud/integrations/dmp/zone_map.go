package dmp

import (
	"fmt"
	"sync"
)

// ZoneMapping maps a DMP zone/area to an NVR camera.
type ZoneMapping struct {
	// AccountID is the DMP panel account number.
	AccountID string `json:"account_id"`

	// Zone is the zone number on the panel.
	Zone int `json:"zone"`

	// Area is the area/partition number on the panel.
	Area int `json:"area"`

	// CameraID is the NVR camera ID associated with this zone.
	CameraID string `json:"camera_id"`

	// Label is a human-readable label for this zone mapping.
	Label string `json:"label,omitempty"`
}

// ZoneMapper maintains the mapping between DMP alarm zones/areas and NVR
// cameras. Thread-safe.
type ZoneMapper struct {
	mu       sync.RWMutex
	// key: "account:zone:area" -> CameraID
	mappings map[string]*ZoneMapping
}

// NewZoneMapper creates an empty zone mapper.
func NewZoneMapper() *ZoneMapper {
	return &ZoneMapper{
		mappings: make(map[string]*ZoneMapping),
	}
}

// zoneKey generates a unique key for a zone mapping.
func zoneKey(accountID string, zone, area int) string {
	return fmt.Sprintf("%s:%d:%d", accountID, zone, area)
}

// Add registers a zone-to-camera mapping.
func (zm *ZoneMapper) Add(m *ZoneMapping) {
	zm.mu.Lock()
	defer zm.mu.Unlock()
	zm.mappings[zoneKey(m.AccountID, m.Zone, m.Area)] = m
}

// Remove removes a zone mapping.
func (zm *ZoneMapper) Remove(accountID string, zone, area int) {
	zm.mu.Lock()
	defer zm.mu.Unlock()
	delete(zm.mappings, zoneKey(accountID, zone, area))
}

// Lookup finds the camera ID for a given alarm zone. It tries an exact
// match first (account+zone+area), then falls back to account+zone with
// area=0 (any area), then account+zone=0+area (panel-wide in area).
func (zm *ZoneMapper) Lookup(accountID string, zone, area int) (string, bool) {
	zm.mu.RLock()
	defer zm.mu.RUnlock()

	// Exact match.
	if m, ok := zm.mappings[zoneKey(accountID, zone, area)]; ok {
		return m.CameraID, true
	}

	// Fallback: any area.
	if area != 0 {
		if m, ok := zm.mappings[zoneKey(accountID, zone, 0)]; ok {
			return m.CameraID, true
		}
	}

	// Fallback: panel-wide for this area.
	if zone != 0 {
		if m, ok := zm.mappings[zoneKey(accountID, 0, area)]; ok {
			return m.CameraID, true
		}
	}

	// Fallback: catch-all for this account.
	if m, ok := zm.mappings[zoneKey(accountID, 0, 0)]; ok {
		return m.CameraID, true
	}

	return "", false
}

// List returns all current zone mappings.
func (zm *ZoneMapper) List() []*ZoneMapping {
	zm.mu.RLock()
	defer zm.mu.RUnlock()

	result := make([]*ZoneMapping, 0, len(zm.mappings))
	for _, m := range zm.mappings {
		result = append(result, m)
	}
	return result
}

// LoadMappings replaces all mappings with the provided list.
func (zm *ZoneMapper) LoadMappings(mappings []*ZoneMapping) {
	zm.mu.Lock()
	defer zm.mu.Unlock()

	zm.mappings = make(map[string]*ZoneMapping, len(mappings))
	for _, m := range mappings {
		zm.mappings[zoneKey(m.AccountID, m.Zone, m.Area)] = m
	}
}

// Count returns the number of registered mappings.
func (zm *ZoneMapper) Count() int {
	zm.mu.RLock()
	defer zm.mu.RUnlock()
	return len(zm.mappings)
}
