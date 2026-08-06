package initx

import (
	"strings"
	"testing"
)

func TestINIRoundTrip(t *testing.T) {
	in := Config{Global: false, Name: "ai-fleet", Hash: "a1b2c3d4"}
	out, err := ParseINI(RenderINI(in))
	if err != nil {
		t.Fatal(err)
	}
	if out != in {
		t.Errorf("round trip: got %+v, want %+v", out, in)
	}
}

func TestRenderINIExactFormat(t *testing.T) {
	got := RenderINI(Config{Global: false, Name: "proj", Hash: "deadbeef"})
	want := "[config]\nglobal = false\n\n[project]\nname = proj\nhash = deadbeef\n"
	if got != want {
		t.Errorf("RenderINI:\n%q\nwant:\n%q", got, want)
	}
}

func TestParseINIErrors(t *testing.T) {
	cases := []struct{ name, data, wantErr string }{
		{"missing name", "[config]\nglobal = false\n\n[project]\nhash = x\n", "missing"},
		{"missing hash", "[project]\nname = x\n", "missing"},
		{"malformed line", "[project]\nname\n", "key = value"},
		{"bad bool", "[config]\nglobal = maybe\n\n[project]\nname = x\nhash = y\n", "config.global"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := ParseINI(c.data)
			if err == nil || !strings.Contains(err.Error(), c.wantErr) {
				t.Errorf("ParseINI error = %v, want containing %q", err, c.wantErr)
			}
		})
	}
}

func TestParseINITolerantOfCommentsAndSpacing(t *testing.T) {
	data := "; comment\n# comment\n[config]\n  global =  true \n\n[project]\nname=x\nhash=y\nfuture_key = ignored\n"
	c, err := ParseINI(data)
	if err != nil {
		t.Fatal(err)
	}
	if !c.Global || c.Name != "x" || c.Hash != "y" {
		t.Errorf("got %+v", c)
	}
}
