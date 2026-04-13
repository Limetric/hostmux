// Package sockproto defines the newline-delimited JSON wire format used
// between the hostmux daemon and its clients over a Unix socket.
package sockproto

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
)

// Op identifies the operation a message represents.
type Op string

const (
	OpRegister Op = "register"
	OpList     Op = "list"
	OpInfo     Op = "info"
	OpBye      Op = "bye"
	OpShutdown Op = "shutdown"
)

// Message is the union of every NDJSON message both directions can carry.
// Fields not relevant to a given Op are zero-valued and omitted by the JSON
// encoder via omitempty.
type Message struct {
	Op       Op       `json:"op,omitempty"`
	Hosts    []string `json:"hosts,omitempty"`
	Upstream string   `json:"upstream,omitempty"`

	// Response fields.
	Ok      bool    `json:"ok,omitempty"`
	Error   string  `json:"error,omitempty"`
	Domain  string  `json:"domain,omitempty"`
	Entries []Entry `json:"entries,omitempty"`
	// PublicHTTPS is true when the daemon serves HTTPS on its public
	// listener. Omitted by older daemons; clients should treat nil as true.
	PublicHTTPS *bool `json:"public_https,omitempty"`
	// PublicPort is the effective public listener port. Omitted by
	// daemons that predate the field; clients should treat 0 as
	// "use the scheme default" (443 for https, 80 for http).
	PublicPort int `json:"public_port,omitempty"`
}

// Entry is the on-wire shape of a routing table entry, used in list responses.
type Entry struct {
	Source   string   `json:"source"`
	Hosts    []string `json:"hosts"`
	Upstream string   `json:"upstream"`
}

// Encoder writes newline-delimited JSON Messages.
type Encoder struct {
	w io.Writer
}

// NewEncoder returns an Encoder writing to w.
func NewEncoder(w io.Writer) *Encoder {
	return &Encoder{w: w}
}

// Encode writes one Message followed by a newline.
func (e *Encoder) Encode(m *Message) error {
	b, err := json.Marshal(m)
	if err != nil {
		return fmt.Errorf("sockproto: marshal: %w", err)
	}
	b = append(b, '\n')
	_, err = e.w.Write(b)
	return err
}

// Decoder reads newline-delimited JSON Messages.
type Decoder struct {
	scanner *bufio.Scanner
}

// NewDecoder returns a Decoder reading from r.
func NewDecoder(r io.Reader) *Decoder {
	s := bufio.NewScanner(r)
	// Allow large list responses (up to 1 MiB) without surprising the user.
	buf := make([]byte, 0, 64*1024)
	s.Buffer(buf, 1024*1024)
	return &Decoder{scanner: s}
}

// Decode reads the next Message. Returns io.EOF at end of stream.
func (d *Decoder) Decode() (*Message, error) {
	if !d.scanner.Scan() {
		if err := d.scanner.Err(); err != nil {
			return nil, err
		}
		return nil, io.EOF
	}
	var m Message
	if err := json.Unmarshal(d.scanner.Bytes(), &m); err != nil {
		return nil, fmt.Errorf("sockproto: unmarshal: %w", err)
	}
	return &m, nil
}
