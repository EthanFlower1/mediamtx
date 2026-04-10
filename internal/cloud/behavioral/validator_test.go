package behavioral_test

import (
	"errors"
	"testing"

	"github.com/bluenviron/mediamtx/internal/cloud/behavioral"
)

func TestValidateParams_Loitering_Valid(t *testing.T) {
	p := `{"roi_polygon":[[0,0],[1,0],[1,1],[0,1]],"threshold_seconds":30}`
	if err := behavioral.ValidateParams(behavioral.DetectorLoitering, p); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateParams_Loitering_MissingPolygon(t *testing.T) {
	p := `{"threshold_seconds":30}`
	err := behavioral.ValidateParams(behavioral.DetectorLoitering, p)
	if err == nil || !errors.Is(err, behavioral.ErrInvalidParams) {
		t.Errorf("expected ErrInvalidParams, got %v", err)
	}
}

func TestValidateParams_Loitering_MissingThreshold(t *testing.T) {
	p := `{"roi_polygon":[[0,0],[1,0],[1,1]]}`
	err := behavioral.ValidateParams(behavioral.DetectorLoitering, p)
	if err == nil || !errors.Is(err, behavioral.ErrInvalidParams) {
		t.Errorf("expected ErrInvalidParams, got %v", err)
	}
}

func TestValidateParams_Loitering_ThresholdZero(t *testing.T) {
	p := `{"roi_polygon":[[0,0],[1,0],[1,1]],"threshold_seconds":0}`
	err := behavioral.ValidateParams(behavioral.DetectorLoitering, p)
	if err == nil || !errors.Is(err, behavioral.ErrInvalidParams) {
		t.Errorf("expected ErrInvalidParams for zero threshold, got %v", err)
	}
}

func TestValidateParams_Loitering_TooFewPoints(t *testing.T) {
	p := `{"roi_polygon":[[0,0],[1,0]],"threshold_seconds":10}`
	err := behavioral.ValidateParams(behavioral.DetectorLoitering, p)
	if err == nil || !errors.Is(err, behavioral.ErrInvalidParams) {
		t.Errorf("expected ErrInvalidParams for <3 polygon points, got %v", err)
	}
}

func TestValidateParams_LineCrossing_Valid(t *testing.T) {
	p := `{"line_start":[0.1,0.2],"line_end":[0.9,0.8]}`
	if err := behavioral.ValidateParams(behavioral.DetectorLineCrossing, p); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateParams_LineCrossing_MissingEnd(t *testing.T) {
	p := `{"line_start":[0,0]}`
	err := behavioral.ValidateParams(behavioral.DetectorLineCrossing, p)
	if err == nil || !errors.Is(err, behavioral.ErrInvalidParams) {
		t.Errorf("expected ErrInvalidParams, got %v", err)
	}
}

func TestValidateParams_ROI_Valid(t *testing.T) {
	p := `{"roi_polygon":[[0,0],[1,0],[1,1],[0,1],[0.5,0.5]]}`
	if err := behavioral.ValidateParams(behavioral.DetectorROI, p); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateParams_ROI_Invalid(t *testing.T) {
	p := `{"roi_polygon":[[0,0],[1,0]]}`
	err := behavioral.ValidateParams(behavioral.DetectorROI, p)
	if err == nil || !errors.Is(err, behavioral.ErrInvalidParams) {
		t.Errorf("expected ErrInvalidParams, got %v", err)
	}
}

func TestValidateParams_CrowdDensity_Valid(t *testing.T) {
	p := `{"max_count":50}`
	if err := behavioral.ValidateParams(behavioral.DetectorCrowdDensity, p); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateParams_CrowdDensity_MaxCountZero(t *testing.T) {
	p := `{"max_count":0}`
	err := behavioral.ValidateParams(behavioral.DetectorCrowdDensity, p)
	if err == nil || !errors.Is(err, behavioral.ErrInvalidParams) {
		t.Errorf("expected ErrInvalidParams for max_count=0, got %v", err)
	}
}

func TestValidateParams_Tailgating_Valid(t *testing.T) {
	p := `{"threshold_seconds":5}`
	if err := behavioral.ValidateParams(behavioral.DetectorTailgating, p); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateParams_Tailgating_MissingThreshold(t *testing.T) {
	p := `{}`
	err := behavioral.ValidateParams(behavioral.DetectorTailgating, p)
	if err == nil || !errors.Is(err, behavioral.ErrInvalidParams) {
		t.Errorf("expected ErrInvalidParams, got %v", err)
	}
}

func TestValidateParams_FallDetection_EmptyParams(t *testing.T) {
	// fall_detection has no required params.
	if err := behavioral.ValidateParams(behavioral.DetectorFallDetection, "{}"); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if err := behavioral.ValidateParams(behavioral.DetectorFallDetection, ""); err != nil {
		t.Errorf("unexpected error on empty params: %v", err)
	}
}

func TestValidateParams_InvalidJSON(t *testing.T) {
	err := behavioral.ValidateParams(behavioral.DetectorROI, `not-json`)
	if err == nil || !errors.Is(err, behavioral.ErrInvalidParams) {
		t.Errorf("expected ErrInvalidParams for invalid JSON, got %v", err)
	}
}

func TestValidateParams_UnknownDetectorType(t *testing.T) {
	err := behavioral.ValidateParams(behavioral.DetectorType("unknown"), `{}`)
	if err == nil || !errors.Is(err, behavioral.ErrInvalidParams) {
		t.Errorf("expected ErrInvalidParams for unknown detector, got %v", err)
	}
}
