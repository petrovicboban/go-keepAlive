package main

import (
	"testing"
	"reflect"
)

func TestDifference(t *testing.T) {
	a := []string{"a", "c"}
	b := []string{"b", "a", "c"}
	expectedResult := []string{"b"}

	diff := difference(a, b)
	if !reflect.DeepEqual(diff, expectedResult) {
		t.Fatalf("Expected %s but got %s", expectedResult, diff)
	}
}

