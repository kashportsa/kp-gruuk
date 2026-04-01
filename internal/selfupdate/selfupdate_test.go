package selfupdate

import "testing"

func TestIsNewer(t *testing.T) {
	cases := []struct {
		next, current string
		want          bool
	}{
		{"v0.0.5", "v0.0.4", true},
		{"v0.1.0", "v0.0.9", true},
		{"v1.0.0", "v0.9.9", true},
		{"v0.0.4", "v0.0.4", false},
		{"v0.0.3", "v0.0.4", false},
		{"v0.0.0", "v0.0.1", false},
	}

	for _, tc := range cases {
		got := isNewer(tc.next, tc.current)
		if got != tc.want {
			t.Errorf("isNewer(%q, %q) = %v, want %v", tc.next, tc.current, got, tc.want)
		}
	}
}

func TestParseSemver(t *testing.T) {
	cases := []struct {
		input string
		want  [3]int
	}{
		{"v1.2.3", [3]int{1, 2, 3}},
		{"1.2.3", [3]int{1, 2, 3}},
		{"v0.0.4", [3]int{0, 0, 4}},
		{"v10.20.30", [3]int{10, 20, 30}},
		{"garbage", [3]int{0, 0, 0}},
	}

	for _, tc := range cases {
		got := parseSemver(tc.input)
		if got != tc.want {
			t.Errorf("parseSemver(%q) = %v, want %v", tc.input, got, tc.want)
		}
	}
}

func TestCheckAndUpdateSkipsDevBuilds(t *testing.T) {
	// dev and empty versions must never trigger a real update check
	for _, v := range []string{"dev", "", "v0.0.4-3-gabcdef", "v1.0.0-dirty"} {
		if CheckAndUpdate(v) {
			t.Errorf("CheckAndUpdate(%q) returned true — should always skip non-release builds", v)
		}
	}
}
