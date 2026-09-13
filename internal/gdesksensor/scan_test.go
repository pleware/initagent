package gdesksensor

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func collect(readings *[]Reading) func(Reading) {
	return func(r Reading) { *readings = append(*readings, r) }
}

func TestScanClassifiesEveryLineItCanSee(t *testing.T) {
	// A blank line in the middle and a trailing newline at the end: both are a
	// producer behaving, and neither is a fact.
	stream := fixtureCrowd + "\n\n" + fixtureEmpty + "\ncamera-0 says hello\n"

	var got []Reading
	if err := Scan(strings.NewReader(stream), collect(&got)); err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("readings = %d, want 3 (two facts and one refusal)", len(got))
	}
	if got[0].Fact.Total != 2 || got[1].Fact.Total != 0 {
		t.Errorf("facts arrived out of order: %d then %d people", got[0].Fact.Total, got[1].Fact.Total)
	}
	if got[2].Outcome != OutcomeMalformed {
		t.Errorf("last outcome = %q, want %q", got[2].Outcome, OutcomeMalformed)
	}
}

// Over the bound the stream stops, and it stops without delivering anything.
// Resynchronising in the middle of an object is how a reader starts inventing
// facts, so there is no attempt to.
func TestScanStopsOnALineOverTheBound(t *testing.T) {
	long := `{"p":2,"kind":"` + strings.Repeat("x", MaxLine) + `"}`

	var got []Reading
	err := Scan(strings.NewReader(long), collect(&got))
	if err == nil {
		t.Fatal("a line over the bound was accepted")
	}
	if !strings.Contains(err.Error(), "reading facts") {
		t.Errorf("error = %v, want it to name what failed", err)
	}
	if len(got) != 0 {
		t.Errorf("readings = %d, want none", len(got))
	}
}

// helperCommand runs this test binary again as a stand-in sensor. The trick is
// os/exec's own: a real child process, no fixture binary to build, and it works
// the same on Windows as on the appliance.
func helperCommand(t *testing.T, mode string) *exec.Cmd {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^TestSensorHelper$")
	cmd.Env = append(os.Environ(), "GDESKSENSOR_HELPER="+mode)
	cmd.Stderr = io.Discard
	return cmd
}

// TestSensorHelper is a sensor when the environment says so, and nothing at all
// otherwise. os.Exit runs before the testing framework prints its summary, which
// is what keeps `PASS` off the fact channel.
func TestSensorHelper(t *testing.T) {
	mode := os.Getenv("GDESKSENSOR_HELPER")
	if mode == "" {
		t.Skip("a helper only when a test asks for one")
	}
	switch mode {
	case "facts":
		fmt.Println(fixtureCrowd)
		fmt.Println(fixtureEmpty)
		os.Exit(0)
	case "last-word":
		fmt.Println(fixtureCrowd)
		fmt.Fprintln(os.Stderr, "camera-0: device disappeared")
		os.Exit(3)
	default:
		os.Exit(9)
	}
}

func TestRunReadsASensorToTheEnd(t *testing.T) {
	var got []Reading
	if err := Run(helperCommand(t, "facts"), collect(&got)); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("readings = %d, want 2", len(got))
	}
	for i, r := range got {
		if r.Outcome != OutcomeFact {
			t.Errorf("reading %d = %q (%s), want a fact", i, r.Outcome, r.Detail)
		}
	}
}

// A sensor that dies still said something first, and that something is the
// evidence for why it died. Reading to the end before waiting is what keeps it.
func TestRunKeepsTheLastWordOfASensorThatFailed(t *testing.T) {
	var got []Reading
	err := Run(helperCommand(t, "last-word"), collect(&got))
	if err == nil {
		t.Fatal("a sensor exiting 3 was reported as a clean run")
	}
	if !strings.Contains(err.Error(), "exited") {
		t.Errorf("error = %v, want it to say the sensor exited", err)
	}
	if len(got) != 1 || got[0].Outcome != OutcomeFact {
		t.Fatalf("readings = %+v, want the one fact it managed to send", got)
	}
}

func TestRunRefusesWhenStdoutIsTaken(t *testing.T) {
	cmd := helperCommand(t, "facts")
	cmd.Stdout = io.Discard

	err := Run(cmd, func(Reading) {})
	if err == nil {
		t.Fatal("a command whose stdout was taken elsewhere was accepted")
	}
	if !strings.Contains(err.Error(), "fact channel") {
		t.Errorf("error = %v, want it to name the fact channel", err)
	}
}

func TestRunReportsASensorThatDoesNotStart(t *testing.T) {
	err := Run(exec.Command("no-such-sensor-on-this-box"), func(Reading) {})
	if err == nil {
		t.Fatal("a missing binary was reported as a running sensor")
	}
	if !strings.Contains(err.Error(), "did not start") {
		t.Errorf("error = %v, want it to say the sensor did not start", err)
	}
}

func TestRunRefusesACommandAlreadyRunning(t *testing.T) {
	cmd := helperCommand(t, "facts")
	if err := cmd.Start(); err != nil {
		t.Fatalf("starting the stand-in sensor: %v", err)
	}
	t.Cleanup(func() { _ = cmd.Wait() })

	err := Run(cmd, func(Reading) {})
	if err == nil {
		t.Fatal("a process already running was adopted")
	}
	if !strings.Contains(err.Error(), "no pipe from") {
		t.Errorf("error = %v, want it to say there was no pipe", err)
	}
}
