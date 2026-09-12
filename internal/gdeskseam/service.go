package gdeskseam

// Service is one thing this box runs, as the operator dump reports it.
//
// The question it answers is the first one somebody asks of a desk that looks
// dead: what is up, on which port, and is anybody on it. That is why a row is
// four short fields and not a health report — anything longer would be a
// monitoring product, and this is a page somebody opens for ninety seconds.
//
// The seam defines the shape but produces no rows. Where the desk listens, what
// answers a role, and what senses the room are all the assembly's knowledge
// (gdeskfront), and it hands them in through ListenConfig.Services.
type Service struct {
	// Name is the machine name, English like the ring's own lines. The console
	// translates the state and the column headings, not this.
	Name string `json:"name"`

	// Where is an address with a port when something listens here, and plain
	// words when nothing does. A provider is somewhere else and a pipe has no
	// port at all; saying so beats an empty cell that reads as "unknown".
	Where string `json:"where"`

	// Port is the TCP port this row is about, when it is about one.
	//
	// A field of its own rather than something read back out of Where, because
	// "which port" is the question this table is opened for, and a page that
	// parsed it out of an address would be guessing at text the desk already
	// knows. Zero means the row has no port — facts on a pipe, or a role
	// nothing is bound to — which is an answer and not a missing value, so no
	// pointer is needed: nothing serves on port 0.
	Port int `json:"port,omitempty"`

	// State is one of the four words below.
	State string `json:"state"`

	// Clients is how many callers are on it, when this desk counts them.
	//
	// A pointer because "not counted" and "nobody" are different answers: an
	// HTTP route holds nobody between two requests, and a zero there would
	// read as a service nobody uses.
	Clients *int `json:"clients,omitempty"`

	// Note is what the name cannot say — a model, why a role is silent, or
	// which sibling produces something this desk does not run yet.
	Note string `json:"note,omitempty"`
}

const (
	// ServiceListening is a port this process holds right now.
	ServiceListening = "listening"

	// ServiceOutbound is something this desk calls: the port belongs to
	// somebody else, so a client count here would be somebody else's number.
	ServiceOutbound = "outbound"

	// ServiceSilent is a role nothing was bound to. The desk runs without it
	// and says so rather than failing to start (gdesk.Silence).
	ServiceSilent = "silent"

	// ServiceDeclared is a service that belongs on this box and is not running
	// in this process: a contract with a sibling that has no reader here yet.
	//
	// It is in the list on purpose. Leaving it out makes the list read as
	// complete, and somebody spends an afternoon looking for the row instead
	// of reading that the work has not been done. A table that mixes evidence
	// with intention silently is the failure this word prevents.
	ServiceDeclared = "declared"
)

// ServiceStates is every state a row may carry.
//
// A list and not only four constants because the console renders each state as
// a Polish word, and that translation is a second place the vocabulary lives.
// A fifth state added here fails the console's test until the page has a word
// for it, which is the only mechanism that keeps the two in step.
var ServiceStates = []string{ServiceListening, ServiceOutbound, ServiceSilent, ServiceDeclared}
