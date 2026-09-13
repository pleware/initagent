package gdesksensor

// FixtureCrowd and FixtureEmpty are exact bytes from pware-os-facts
// (facts.writer.as_line), kept here so other packages can replay a sensor
// without hand-written JSON.
const (
	FixtureCrowd = `{"at":"2026-03-08T20:00:00.000+00:00","faces":[{"gaze":"center","range":"near","rank":1},{"range":"far","rank":2}],"far":1,"kind":"gdesk.attendance.changed","near":1,"p":2,"sensor":"camera-0","source":"vision","total":2}`
	FixtureEmpty = `{"at":"2026-03-08T20:00:00.000+00:00","faces":[],"far":0,"kind":"gdesk.attendance.changed","near":0,"p":2,"sensor":"camera-0","source":"vision","total":0}`
)
