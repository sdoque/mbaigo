package forms

import (
	"encoding/json"
	"testing"
	"time"
)

func TestScanARoundTrip(t *testing.T) {
	var s ScanA_v1a
	s.NewForm()
	s.Angles = []float64{-10, 0, 10}
	s.Distances = []float64{2.5, 0, 3.0}
	s.Valid = []bool{true, false, true}
	s.AngleUnit = "<http://qudt.org/vocab/unit/DEG>"
	s.DistanceUnit = "<http://qudt.org/vocab/unit/M>"
	s.Timestamp = time.Now().UTC().Truncate(time.Millisecond)
	s.SweepDuration = 200 * time.Millisecond

	body, err := json.Marshal(&s)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	// Through Unpack, because that is the path a consumer actually takes: the
	// version string has to resolve in FormTypeMap or the form is undeliverable.
	back := ScanA_v1a{}
	if err := json.Unmarshal(body, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if err := back.Check(); err != nil {
		t.Errorf("round-tripped scan does not check out: %v", err)
	}
	if back.ValidCount() != 2 {
		t.Errorf("ValidCount = %d, want 2", back.ValidCount())
	}
	if back.SweepDuration != 200*time.Millisecond {
		t.Errorf("SweepDuration = %v, want 200ms", back.SweepDuration)
	}
	if _, known := FormTypeMap[back.Version]; !known {
		t.Errorf("version %q is not registered in FormTypeMap", back.Version)
	}
}

// A short array is the failure this form exists to make impossible to ignore:
// indexing angles against distances in step would put an obstacle in the wrong
// direction.
func TestScanACheckCatchesRaggedArrays(t *testing.T) {
	s := ScanA_v1a{
		Angles:    []float64{0, 1, 2},
		Distances: []float64{1, 2},
		Valid:     []bool{true, true, true},
	}
	if err := s.Check(); err == nil {
		t.Error("Check accepted 3 angles against 2 distances")
	}
}

func TestMapACheck(t *testing.T) {
	m := MapA_v1a{Width: 3, Height: 2, Cells: make([]byte, 6)}
	if err := m.Check(); err != nil {
		t.Errorf("a 3x2 map with 6 cells was rejected: %v", err)
	}
	m.Cells = make([]byte, 5)
	if err := m.Check(); err == nil {
		t.Error("Check accepted 5 cells for a 3x2 map")
	}
}

// Unknown must not collide with either observed value, or a consumer cannot
// tell unseen ground from ground seen to be empty.
func TestMapCellValuesAreDistinct(t *testing.T) {
	if MapUnknown == MapFree || MapUnknown == MapOccupied || MapFree == MapOccupied {
		t.Errorf("cell constants collide: unknown=%d free=%d occupied=%d",
			MapUnknown, MapFree, MapOccupied)
	}
}

func TestPoseARegistered(t *testing.T) {
	var p PoseA_v1a
	p.NewForm()
	if _, known := FormTypeMap[p.FormVersion()]; !known {
		t.Errorf("version %q is not registered in FormTypeMap", p.FormVersion())
	}
}
