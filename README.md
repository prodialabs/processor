# Prodia Processor

```
$ go build
$ ./processor build --source-dir testdata/ping --target-dir build --executable-name processor-ping
9:00AM INF created temporary space tmp_dir=/tmp/processor-build-1200338307
9:00AM INF initializing... tmp_dir=/tmp/processor-build-1200338307
go: creating new go.mod: module github.com/prodialabs/processor-ping
go: to add module requirements and sums:
	go mod tidy
9:00AM INF tidying... tmp_dir=/tmp/processor-build-1200338307
go: finding module for package github.com/rs/zerolog
go: finding module for package github.com/alecthomas/kong
go: finding module for package github.com/mattn/go-isatty
go: finding module for package github.com/goccy/go-json
go: found github.com/alecthomas/kong in github.com/alecthomas/kong v1.2.1
go: found github.com/goccy/go-json in github.com/goccy/go-json v0.10.3
go: found github.com/mattn/go-isatty in github.com/mattn/go-isatty v0.0.20
go: found github.com/rs/zerolog in github.com/rs/zerolog v1.33.0
9:00AM INF building... tmp_dir=/tmp/processor-build-1200338307
$ ./processor-ping run job.json input.1 input.2
...
```

When run like this:

```
$ ./processor-ping run meta.json input.1 input.2
```

* meta.json is modified in place
* output files are saved in the current directory

When run like this:

```
$ ./processor-ping run < request.form-data > response.form-data
```

* stdin should contain a multipart form-data encoded request with at least the
  appropriate meta and input parts
* stdout will be the multipart form-data encoded response including the meta
  and output parts

If we can, the default command should be `run` so that `./processor-ping
meta.json input.1 input.2` etc also works.

When run like this:

```
$ ./processor-ping bootstrap
```

* `PROCESSOR_ASSETS` will be the root of the assets tree, defaults to
  `/var/lib/processor`
* processor ping will put its assets in `$PROCESSOR_ASSETS/ping`
* assets should be anything that the processor needs to be setup once after
  install including venv, pip install, downloading updated models

The minimal meta json contains at least:

```json
{
    "kind": "meta",
    "type": "processor.ping.v1",
}
```

If no error occurred during processing, then this is a sufficient response:

```json
{
    "kind": "meta",
    "type": "processor.ping.v1",
    "done": true,
    "ok": true
}
```

If a fatal error occured, then this response is minimally viable:

```json
{
    "kind": "meta",
    "type": "processor.ping.v1",
    "done": true,
    "ok": false
}
```

Actual error payloads should be found in the `error` parts. It is expected that
a systems could simplify the stream responses as desired. For example, multiple
non-fatal errors, one final error, then a meta with done=true and ok=false
might result in the last error being sent back rather than the job's meta or
any data part. This might be the typical behavior of an HTTP endpoint handling
job requests.

# Protocol

## Part Types

### meta
### data
### log
### metric
### error

## Request

## Response

## Processing

When a processor starts it emits a ready message once it can process jobs:

`processor -> ready, ..., last`

The processor will periodically resend the ready message until a job is
received or the processor is shutting down. If it is a job, the processor
sends the last part to close out the ready cycle and indicate the job is now
being processed. Processors shouldn't start work on a job until after they have
sent the last part for the ready cycle. When the processor finishes a job it
will start back again by sending a ready message and waiting for a job.

Processors can start work as soon as they have enough information to do so. It
is possible for a processor to start writing a response before it has finished
reading the request. For example, a processor could always return the same
response (like the `yes` tool on linux). Such a tool could conceivably write
the entire response as soon as the first byte of the request is received. A
less extreme example would be a processor that transforms individual inputs and
can do them independently. That kind of processor can write an output as soon
as the input has been transformed and it can do that while also reading the
next input. In this way processors retain a substantial degree of streaming
support despite being structured messages.

The round trip of processing is:

`request -> processor -> response`

Where the request and reponse expand to these parts:

`meta, data|log|metric|error|meta, ..., last -> processor -> data|log|metric|error|meta, ..., meta, last`

A job request always begins with a meta part and ends with a last part. A job
response always ends with a meta part followed by a last part.

Sending additional meta parts in the request after the first updates the job
meta. Outputs generated before the new meta was processed use the old job
configuration. If a streaming processor accepts a new meta part it should also
emit a meta part in the response. This way consumers of the response parts can
know that parts after the meta part are using a new configuration.

A meta part cannot be updated in a "invalid" ways, including restrictions like:

* `type` must be set, not empty, and never modified
* `created_at` may only be set once if absent
* `done` may only be set to once to true
* `ok` may only be set once
* if `done` is true, then `ok` must be set
* if `done` is true, then the last part must be sent next

Similarly, when emitting meta updates in the response they must be semantically
valid updates. For instance, the request side restriction "if `done` is true,
then the last part must be sent next" also applies to the response side.

Some restrictions can be rather finessed. For instance, the processor may allow
`accept` to be changed. Or it may only allow it to be changed if no output data
has been emitted. Or it might not allow it to be changed at all. Whether this
is possible or desirable depends heavily on the processing being done.

Further restrictions may be enforced by the processor for changes to the
`config` or `state`.

Log, metric, and error parts sent as part of the request are not required to
pass through to the response. If it makes sense for a processor to do so it may
pass them through (changed or unchanged, all or in part).

Processors can be pipelined via the normal shell pipe:

`request -> processor A | processor B | ... | processor N -> response`

In order that this pipelining should function smoothly all processors need to
handle the `ready` job.

A processor does not have to return a done job. In fact, processors should use
this to bound the amount of work they do per job request while still making
incremental progress. Processors can also use this behavior to perform loops
when the job submitter resubmits jobs that aren't yet done. Non-terminating
loops are also possible if the job submitter permits infinite resubmissions.

---

Copyright (C) 2024 Prodia Inc.

Licensed under the GNU Affero General Public License, version 3
(AGPL-3.0-only). See [LICENSE](LICENSE) for the full licence text.
