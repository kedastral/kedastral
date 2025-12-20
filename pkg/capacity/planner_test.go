package capacity

import (
	"reflect"
	"testing"
)

func TestToReplicas_Basic(t *testing.T) {
	p := Policy{
		TargetPerPod:          50,
		Headroom:              1.2,
		MinReplicas:           1,
		MaxReplicas:           100,
		UpMaxFactorPerStep:    2.0,
		DownMaxPercentPerStep: 50,
		PrewarmWindowSteps:    0,
		RoundingMode:          "ceil",
	}
	forecast := []float64{120, 130, 125, 140, 100}
	got := ToReplicas(2, forecast, 60, p)
	// i=0 uses 120 -> 120/50*1.2 = 2.88 -> ceil = 3
	// i=1 uses 130 -> 130/50*1.2 = 3.12 -> ceil = 4
	// i=2 uses 125 -> 125/50*1.2 = 3.00 -> ceil = 3
	// i=3 uses 140 -> 140/50*1.2 = 3.36 -> ceil = 4 (but down clamp from 3 allows 1)
	// i=4 uses 100 -> 100/50*1.2 = 2.40 -> ceil = 3
	want := []int{3, 4, 3, 4, 3}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestToReplicas_ClampsUpDown(t *testing.T) {
	p := Policy{
		TargetPerPod:          100,
		Headroom:              1.2,
		MinReplicas:           1,
		MaxReplicas:           100,
		UpMaxFactorPerStep:    1.5,
		DownMaxPercentPerStep: 25,
		PrewarmWindowSteps:    0,
		RoundingMode:          "ceil",
	}
	// Raw (with headroom) -> desired before clamps:
	// v=0 => 0; 50=>0.6; 500=>6; 200=>2.4; 50=>0.6
	forecast := []float64{0, 50, 500, 200, 50}
	got := ToReplicas(2, forecast, 60, p)
	// Step 0: prev=2; raw ceil=1 -> down clamp floor(2*0.75)=1 -> 1
	// Step 1: raw ceil=1 -> prev=1, unchanged -> 1
	// Step 2: raw ceil=6 -> prev=1, up clamp ceil(1*1.5)=2 -> 2
	// Step 3: raw ceil=3 -> prev=2, up clamp ceil(2*1.5)=3 -> 3
	// Step 4: raw ceil=1 -> prev=3, down clamp floor(3*0.75)=2 -> 2
	want := []int{1, 1, 2, 3, 2}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestToReplicas_Bounds(t *testing.T) {
	p := Policy{
		TargetPerPod:          10,
		Headroom:              1.0,
		MinReplicas:           2,
		MaxReplicas:           5,
		UpMaxFactorPerStep:    10.0,
		DownMaxPercentPerStep: 100,
		PrewarmWindowSteps:    0,
		RoundingMode:          "ceil",
	}
	forecast := []float64{0, 1, 10, 1000}
	got := ToReplicas(0, forecast, 60, p)
	// min bound enforces at least 2; max bound caps at 5
	want := []int{2, 2, 2, 5}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestToReplicas_PrewarmWindow_SinglePoint(t *testing.T) {
	p := Policy{
		TargetPerPod:          100,
		Headroom:              1.0,
		MinReplicas:           0,
		MaxReplicas:           0,
		UpMaxFactorPerStep:    10.0,
		DownMaxPercentPerStep: 100,
		PrewarmWindowSteps:    0,
		RoundingMode:          "ceil",
	}
	forecast := []float64{1, 1, 1, 900, 1, 1}
	got := ToReplicas(0, forecast, 60, p)
	// i=0 uses index 0 -> 1/100=0.01 -> ceil = 1
	// i=1 uses index 1 -> 1
	// i=2 uses index 2 -> 1
	// i=3 uses index 3 -> 900/100=9
	// i=4 uses index 4 -> 1
	// i=5 uses index 5 -> 1
	want := []int{1, 1, 1, 9, 1, 1}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}
