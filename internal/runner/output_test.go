package runner

import (
	"errors"
	"reflect"
	"testing"
)

func TestLineWriter(t *testing.T) {
	tests := []struct {
		name    string
		writes  []string
		maxLine int
		want    []string
	}{
		{name: "whole lines", writes: []string{"a\nb\n"}, maxLine: 64, want: []string{"a\n", "b\n"}},
		{name: "split across writes", writes: []string{"ab", "c\nd", "e\n"}, maxLine: 64, want: []string{"abc\n", "de\n"}},
		{name: "last line without newline", writes: []string{"a\nrest"}, maxLine: 64, want: []string{"a\n", "rest"}},
		{name: "long line in pieces", writes: []string{"abcdefg\n"}, maxLine: 3, want: []string{"abc", "def", "g\n"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got []string
			w := newLineWriter(newOutputBudget(1<<20, nil), tt.maxLine, func(line []byte) {
				got = append(got, string(line))
			})
			for _, s := range tt.writes {
				if _, err := w.Write([]byte(s)); err != nil {
					t.Fatalf("Write: %v", err)
				}
			}
			w.Flush()
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("lines = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestOutputBudgetIsSharedAndStopsAtTheCap(t *testing.T) {
	exceeded := 0
	b := newOutputBudget(10, func() { exceeded++ })
	var lines []string
	collect := func(line []byte) { lines = append(lines, string(line)) }
	stdout := newLineWriter(b, 64, collect)
	stderr := newLineWriter(b, 64, collect)

	if _, err := stdout.Write([]byte("12345\n")); err != nil {
		t.Fatalf("first write: %v", err)
	}
	if _, err := stderr.Write([]byte("123\n")); err != nil {
		t.Fatalf("write up to the cap: %v", err)
	}
	if _, err := stdout.Write([]byte("x")); !errors.Is(err, errOutputCap) {
		t.Fatalf("write over the cap: err = %v, want errOutputCap", err)
	}
	if _, err := stderr.Write([]byte("y")); !errors.Is(err, errOutputCap) {
		t.Fatalf("write after the cap: err = %v, want errOutputCap", err)
	}
	stdout.Flush()
	if !b.Exceeded() || exceeded != 1 {
		t.Errorf("Exceeded = %v, onExceed calls = %d; want true, 1", b.Exceeded(), exceeded)
	}
	if want := []string{"12345\n", "123\n"}; !reflect.DeepEqual(lines, want) {
		t.Errorf("lines = %q, want %q", lines, want)
	}
}
