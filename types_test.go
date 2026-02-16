package treesitter

import (
	"testing"
)

func TestLengthAdd(t *testing.T) {
	tests := []struct {
		name string
		a, b Length
		want Length
	}{
		{
			name: "zero plus zero",
			a:    Length{},
			b:    Length{},
			want: Length{},
		},
		{
			name: "same line",
			a:    Length{Bytes: 5, Point: Point{Row: 0, Column: 5}},
			b:    Length{Bytes: 3, Point: Point{Row: 0, Column: 3}},
			want: Length{Bytes: 8, Point: Point{Row: 0, Column: 8}},
		},
		{
			name: "b crosses lines",
			a:    Length{Bytes: 10, Point: Point{Row: 2, Column: 7}},
			b:    Length{Bytes: 15, Point: Point{Row: 1, Column: 4}},
			want: Length{Bytes: 25, Point: Point{Row: 3, Column: 4}},
		},
		{
			name: "b on same line (row=0)",
			a:    Length{Bytes: 10, Point: Point{Row: 3, Column: 5}},
			b:    Length{Bytes: 2, Point: Point{Row: 0, Column: 2}},
			want: Length{Bytes: 12, Point: Point{Row: 3, Column: 7}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := LengthAdd(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("LengthAdd(%v, %v) = %v, want %v", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestLengthSub(t *testing.T) {
	tests := []struct {
		name string
		a, b Length
		want Length
	}{
		{
			name: "same line",
			a:    Length{Bytes: 8, Point: Point{Row: 0, Column: 8}},
			b:    Length{Bytes: 3, Point: Point{Row: 0, Column: 3}},
			want: Length{Bytes: 5, Point: Point{Row: 0, Column: 5}},
		},
		{
			name: "different lines",
			a:    Length{Bytes: 25, Point: Point{Row: 3, Column: 4}},
			b:    Length{Bytes: 10, Point: Point{Row: 2, Column: 7}},
			want: Length{Bytes: 15, Point: Point{Row: 1, Column: 4}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := LengthSub(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("LengthSub(%v, %v) = %v, want %v", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestParseActionEntryTypes(t *testing.T) {
	// Verify parse action entry types are distinct.
	types := []ParseActionType{
		ParseActionTypeHeader,
		ParseActionTypeShift,
		ParseActionTypeReduce,
		ParseActionTypeAccept,
		ParseActionTypeRecover,
	}
	seen := make(map[ParseActionType]bool)
	for _, ty := range types {
		if seen[ty] {
			t.Errorf("duplicate ParseActionType: %d", ty)
		}
		seen[ty] = true
	}
}
