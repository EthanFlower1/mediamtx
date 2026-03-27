// internal/nvr/ai/merge.go
package ai

// MergeDetections combines YOLO and ONVIF detections, deduplicating where
// a YOLO box and ONVIF box overlap > 0.5 IoU and share the same class.
// YOLO detections take priority on duplicates.
func MergeDetections(yolo, onvif []Detection) []Detection {
	if len(onvif) == 0 {
		return yolo
	}
	if len(yolo) == 0 {
		return onvif
	}

	merged := make([]Detection, len(yolo))
	copy(merged, yolo)

	for _, od := range onvif {
		if isDuplicate(od, yolo) {
			continue
		}
		merged = append(merged, od)
	}
	return merged
}

func isDuplicate(onvifDet Detection, yoloDets []Detection) bool {
	for _, yd := range yoloDets {
		if yd.Class == onvifDet.Class && iouBoxes(yd.Box, onvifDet.Box) > 0.5 {
			return true
		}
	}
	return false
}

