package blastradius

import "testing"

func TestSumSignalPointsCategoryFiltering(t *testing.T) {
	signals := []Signal{
		{Name: "a", Points: 1.0, Category: "architecture"},
		{Name: "b", Points: 2.0, Category: "graph"},
		{Name: "c", Points: 4.0, Category: "duplication"},
		{Name: "d", Points: 8.0, Category: "code-metrics"},
		{Name: "e", Points: 16.0, Category: "diff-shape"},
	}
	if got := sumSignalPoints(signals, blastRadiusCategories); got != 3.0 {
		t.Fatalf("blastRadiusCategories sum = %v, want 3.0 (architecture+graph)", got)
	}
	if got := sumSignalPoints(signals, reviewPriorityCategories); got != 12.0 {
		t.Fatalf("reviewPriorityCategories sum = %v, want 12.0 (duplication+code-metrics)", got)
	}
}

func TestSumSignalPointsEmpty(t *testing.T) {
	if got := sumSignalPoints(nil, blastRadiusCategories); got != 0 {
		t.Fatalf("expected 0 for nil signals, got %v", got)
	}
}
