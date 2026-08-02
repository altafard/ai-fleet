package logstream

import "testing"

func TestParseBuildStep(t *testing.T) {
	cases := []struct {
		line  string
		step  int
		total int
		instr string
		ok    bool
	}{
		{"#5 [2/6] RUN npm ci", 2, 6, "RUN npm ci", true},
		{"#7 [stage-1 3/4] COPY . .", 3, 4, "COPY . .", true},
		{"Step 2/6 : RUN npm ci", 2, 6, "RUN npm ci", true},
		{"Step 10/12 : COPY go.mod go.sum /tmp/deps/", 10, 12, "COPY go.mod go.sum /tmp/deps/", true},
		{"#5 DONE 1.2s", 0, 0, "", false},
		{" ---> Running in 0c3a5b7", 0, 0, "", false},
		{"random text", 0, 0, "", false},
	}
	for _, c := range cases {
		s, tot, instr, ok := ParseBuildStep(c.line)
		if ok != c.ok || s != c.step || tot != c.total || instr != c.instr {
			t.Errorf("%q: got (%d,%d,%q,%v)", c.line, s, tot, instr, ok)
		}
	}
}
