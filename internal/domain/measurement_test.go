package domain

import "testing"

func TestClassifyMeasurementAllDefects(t *testing.T) {
	p := DefaultThresholds()
	result, err := ClassifyMeasurement(p, 1000, 850, 4, 70)
	if err != nil {
		t.Fatal(err)
	}
	if result.Passed {
		t.Fatal("预期测量不合格")
	}
	if len(result.DefectTypes) != 3 {
		t.Fatalf("预期 3 项缺陷，得到 %v", result.DefectTypes)
	}
}

func TestClassifyRejectsInvalidSupply(t *testing.T) {
	_, err := ClassifyMeasurement(DefaultThresholds(), 0, 0, 1, 0)
	if err == nil {
		t.Fatal("预期拒绝零供压")
	}
}
