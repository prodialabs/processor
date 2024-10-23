package main

import (
	"bytes"
	"encoding/json"
	"io"
	"time"

	"github.com/calebcase/oops"
	"github.com/gofrs/uuid"
)

type PartKind string

var (
	// MetaKind contains the job metadata.
	MetaKind PartKind = "meta"

	// DataKind contains job data.
	DataKind PartKind = "data"

	// LogKind contains logs emitted while processing this job.
	LogKind PartKind = "log"

	// MetricKind contains metrics emitted while processing this job.
	MetricKind PartKind = "metric"

	// ErrorKind contains errors generated while processing this job.
	ErrorKind PartKind = "error"

	// ControlKind contains jobs for processor processor contollers (e.g.
	// ready jobs).
	ControlKind PartKind = "control"
)

type PartType string

var (
	MetaType   PartType = "processor.meta.v1"
	DataType   PartType = "processor.data.v1"
	LogType    PartType = "processor.log.v1"
	MetricType PartType = "processor.metric.v1"
	ErrorType  PartType = "processor.error.v1"

	// ReadyType is used with ready jobs. Ready jobs are sent once
	// a processor is ready to receive job requests.
	ReadyType PartType = "processor.ready.v1"

	// LastType is used to indicate this is the last part that
	// will be sent for the current job.
	LastType PartType = "processor.last.v1"
)

type Parter interface {
	isPart()
}

// Part is a logic piece of a job with a given kind and type.
type Part struct {
	Kind PartKind `json:"kind"`
	Type PartType `json:"type"`

	ContentType string `json:"content_type,omitempty"`
	Boundary    string `json:"boundary,omitempty"`
	Payload     []byte `json:"-"`
}

var _ Parter = (*Part)(nil)

func (p Part) isPart() {}

func (p *Part) LoadPayload(r io.Reader) (err error) {
	if p.Boundary == "" {
		return nil
	}

	boundary := "\n" + p.Boundary + "\n"

	w := bytes.Buffer{}

	match := []byte{}
	for {
		b := make([]byte, 1)
		_, err = r.Read(b)
		if err != nil {
			return oops.Trace(err)
		}

		_, err = w.Write(b)
		if err != nil {
			return oops.Trace(err)
		}

		match = append(match, b...)

		if string(match) == boundary {
			break
		}

		if string(match) != boundary[:len(match)] {
			match = match[:0]
		}
	}

	p.Payload = w.Bytes()

	return nil
}

// Meta is a job part that may contain metadata about the job like an
// identifier, timestamp, configuration, state, and disposition.
type Meta struct {
	Part

	// Done is true when no further processing is possible on this job. If
	// a processor understands the given job type, but the job is already
	// done, then it should copy all the job parts through unchanged.
	Done bool `json:"done"`

	// Ok is nil until Done is true. When Done is true, Ok will be true if
	// processing was successful, false if it failed, and nil if it is
	// indeterminate.
	Ok *bool `json:"status"`

	// --------------------------------------------------------------------------------
	// Everything above is critical for processing all meta requests.
	// Everything below probably can be moved to the payload since it is
	// all application specific.
	// --------------------------------------------------------------------------------

	// ID is a universally unique identifier for this job. If a processor
	// receives a nil ID it should initialize it to a new random v4 UUID.
	ID *uuid.UUID `json:"id,omitempty"`

	// CreatedAt is set to when this job was created. Once set this
	// value is immutable. If a processor receives a nil CreatedAt it
	// should initialize it to the current time.
	CreatedAt *time.Time `json:"created_at,omitempty"`

	// UpdatedAt is set to when this job was last updated.
	UpdatedAt *time.Time `json:"updated_at,omitempty"`

	// ExpiresAt is set to when this job should be considered expired.
	// Processors should terminate processing on a best effort basis if the
	// current time is after the ExpiresAt time. If a processor receives a
	// nil ExpiresAt it should initialize it to CreatedAt + the TTL for the
	// given job type.
	ExpiresAt *time.Time `json:"expires_at,omitempty"`

	// DeletedAt is non-nil when only the meta part for this job remains
	// and all other parts have been removed.
	DeletedAt *time.Time `json:"deleted_at,omitempty"`

	// Accept indicates the desired output content-type. It is meant to be
	// used as part of a process similar to the usual HTTP content
	// negotiation
	// (https://datatracker.ietf.org/doc/html/rfc7231#section-5.3.2). If a
	// processor receives a job without an accept it should use a default
	// accept value and update this to match. If content negotiation fails,
	// the processor should emit an error part, set done=true, ok=false,
	// and send the last part.
	Accept string `json:"accept,omitempty"`

	// Config contains this job's configuration. Configuration is generally
	// considered immutable once set, but processors are encouraged to
	// update Config to reflect default values used as part of the
	// processing.
	Config json.RawMessage `json:"config,omitempty"`

	// State contains this job's state values. State is meant to contain
	// all those things which result from processing, but which aren't
	// separate data parts.
	State json.RawMessage `json:"state,omitempty"`
}

// Last is a job part that indicates no more parts will arrive.
type Last struct {
	Part
}

func NewBoundary() string {
	return "\n" + uuid.Must(uuid.NewV4()).String() + ":" + uuid.Must(uuid.NewV4()).String() + "\n"
}

// Data is a job part with optional file name.
type Data struct {
	Part

	FileName string `json:"file_name,omitempty"`
}

// Log is a job data part that contains one or more log events.
type Log struct {
	Data
}

// Metric is a job data part that contains one or more metric values.
type Metric struct {
	Data
}

// Error is a job data part that contains an error message and, optionally,
// additional details.
type Error struct {
	Data

	Message string          `json:"message"`
	Details json.RawMessage `json:"details,omitempty"`
}

// Request is a job composed of a meta part with zero or more parts.
type Request struct {
	Meta  Meta
	Parts []Parter
}

// Response is a job composed of zero or more parts and a final meta part.
type Response struct {
	Parts []Parter
	Meta  Meta
}

// RoundTrip is the result of sending a request job to a processor and
// receiving the response job.
type RoundTrip struct {
	Request  Request
	Response Response
}

// Job is a conceptual unit that is worked on by a processor. It is an artifact
// that evolves from an initial state to a final done state. A job may be
// worked on, and partial progress made, by multiple processors in series. Each
// taking the last, but not yet done job, and moving it closer to a final done
// state.
type Job struct {
	Meta  Meta
	Parts []Parter
}
