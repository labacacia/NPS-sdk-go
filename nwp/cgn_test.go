package nwp

import "testing"

func TestEstimateCgn(t *testing.T) {
	cases := []struct {
		in   string
		want int
	}{
		{"", 0},
		{"test", 1},     // 4 bytes / 4 = 1
		{"hello", 2},    // 5 bytes -> ceil(5/4) = 2
		{"abcdefgh", 2}, // 8 bytes / 4 = 2
		{"你好", 2},       // 6 UTF-8 bytes -> ceil(6/4) = 2
	}
	for _, c := range cases {
		got := EstimateCgn(c.in)
		if got != c.want {
			t.Errorf("EstimateCgn(%q) = %d; want %d", c.in, got, c.want)
		}
	}
}

func TestEstimateCgnBytes(t *testing.T) {
	if EstimateCgnBytes([]byte{}) != 0 {
		t.Error("empty bytes should return 0")
	}
	if EstimateCgnBytes([]byte("hello")) != 2 {
		t.Error("hello should return 2")
	}
}

func TestEstimateCgnJSON(t *testing.T) {
	n, err := EstimateCgnJSON(map[string]string{"key": "val"})
	if err != nil {
		t.Fatal(err)
	}
	if n <= 0 {
		t.Error("expected positive CGN for JSON object")
	}
}

func TestTokenBudgetMetaDefaultProfile(t *testing.T) {
	m := TokenBudgetMeta{CgnLimit: 100}
	if m.DefaultProfile() != "cgn.v1" {
		t.Error("empty Profile should default to cgn.v1")
	}
}

func TestBudgetExceededError(t *testing.T) {
	err := &BudgetExceededError{Requested: 200, Limit: 100}
	if err.Requested != 200 || err.Limit != 100 {
		t.Error("wrong fields")
	}
}
