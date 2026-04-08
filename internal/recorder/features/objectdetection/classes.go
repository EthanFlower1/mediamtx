package objectdetection

// ClassMap is a map from a model's raw output class id to a human-readable
// label. Different verticals ship different maps so that the same pipeline
// can emit retail-loss-prevention events, parking events, healthcare
// events, or generic COCO-style events without re-training for every
// deployment.
//
// Keys are dense starting at 0 (the class id emitted by the model's
// classification head). Missing keys indicate "this class is not part of
// this vertical and should be dropped during label resolution".
type ClassMap map[int]string

// Label returns the label for the class id and whether it is known to the
// map. Unknown class ids produce ("", false), which the pipeline treats as
// an implicit drop.
func (m ClassMap) Label(id int) (string, bool) {
	if m == nil {
		return "", false
	}
	l, ok := m[id]
	return l, ok
}

// GenericClasses is the default COCO-ish class map used when a deployment
// has not selected a vertical. It is intentionally narrow — it covers the
// objects common NVR installs actually care about rather than the full
// 80-class COCO set.
var GenericClasses = ClassMap{
	0: "person",
	1: "bicycle",
	2: "car",
	3: "motorcycle",
	4: "bus",
	5: "truck",
	6: "dog",
	7: "cat",
	8: "package",
	9: "bag",
}

// RetailLPClasses is the retail loss-prevention vertical class map. The
// "concealed_item" class is emitted by a specialised head that looks for
// items being tucked into bags or clothing and is one of the reasons the
// LP vertical needs a distinct model from the generic one.
var RetailLPClasses = ClassMap{
	0: "person",
	1: "shopping_cart",
	2: "bag",
	3: "concealed_item",
}

// ParkingClasses is the parking / ALPR vertical class map. Detections of
// "license_plate" feed into KAI-282 (ALPR OCR) as a region-of-interest
// hint so that the OCR stage doesn't have to scan the entire frame.
var ParkingClasses = ClassMap{
	0: "car",
	1: "motorcycle",
	2: "truck",
	3: "bus",
	4: "license_plate",
}

// HealthcareClasses is the healthcare vertical class map. The
// "fall_event" class is emitted by a pose-aware model variant and is
// consumed by KAI-284 (behavioral analytics) as the fall-detection trigger.
// Keep "fall_event" reserved as id 3 — KAI-284 hard-codes it as its
// behavioral hook.
var HealthcareClasses = ClassMap{
	0: "person",
	1: "wheelchair",
	2: "stretcher",
	3: "fall_event",
}
