---
authors: sterling
state: draft
---

# Job Service RFD

## What

Implement a grpc service that provides the following features:
* Start a process
	* Provide resource control (CPU, Mem, Disk IO) via Cgroups
* Stop a running process
* Get the status of a running process
* Stream the output of a running process
	* Concurrent clients should be supported
	* Clients that connect stream from the beginning of the output. (potential issue - see below)

Write a CLI tool to interact with the service above

## Details

### Job Service API

Initial design of the JobService api is represented in the interface below

```go
type JobService interface {
		Start(authID string, cmd []string, limits ResourceLimit) (*Command, error)
		Get(authID string, jobID int32) (*Command, error)
		Stop(authID string, jobID int32) error
		Stream(authID string, jobID int32, writer io.Writer) error
}

// different command statuses
var (
	StatusRunning 	= "running"
	StatusStopped 	= "stopped"
	StatusFinished 	= "finished"
)
```

`authID` is a unique string used for basic authorization purposes - a user with an `authID` `foo` must not be able to
perform actions on processes that were created by user with a different `authID`. See the authorization section for more details.  

ResourceLimit is a data structure than contains the CPU, Memory, and Disk IO resource limit.  

`Command` is some data type that contains data/state for a Command.  

### GRPC API

The server will be accessible by exposing a GRPC server that interacts with the job service lib.

```proto
service JobService {
	rpc Get(GetRequest) returns (Job);
	rpc Start(StartRequest) returns(Job);
	rpc Stop(StopRequest) returns(StopResponse);
	rpc Stream(StreamRequest) returns(stream StreamResponse);
}
```

### Process Execution Lifecycle

For both starting and stopping a process, this library will use Go's std lib package `os/exec` to interact with
commands.

#### Starting a process

The `*Cmd.Start()` method will be used to start a process.

#### Stopping a process

We will stop a process if it has already been started by using `*os.Process.Kill()` located on the `Cmd` struct.

### Streaming output

There are 3 functional behaviors for streaming the output
1. clients get a continuous stream of data
1. clients that connect start streaming from the beginning
1. multiple concurrent client can connect to the stream

For clients to get a continuous stream of data, we will use grpc's server streaming rpc.

```proto
message StreamResponse { 
	bytes data = 1;
}

service Streamer {
	rpc Stream(StreamRequest) returns(stream StreamResponse)
}
```

To accomplish bullets 2 and 3, we will need to write a data structure that does the following
1. stores the complete output of the process
2. support 1 writer
3. support multiple readers

A potential implementation may look like the following:

```go
type Buf struct {
	sync.RWMutex
	buf []byte
}

// Process's stdout will write to buf
// expecting only a single writer.
func (b *Buf) Write(p []byte) (int, error) { 
	b.Lock()
	defer b.Unlock()
  // logic
}

// GetReader returns a io.Reader than can safely read from buf
// can handle multiple readers
func (b *Buf) GetReader() io.Reader {
	// each concurrent reader has its own closure. the variable offset represents the offset that it reads from buf.
	var offset int
	return func(p []byte) (int, error) {
		b.RLock()
		defer b.RUnlock()
		// use offset here
	}
}

```

Single Writer, multiple readers.
Readers take a `RLock`, writers take a `Lock`.

A potential problem with this approach is if the writer is blocked, it will block readers from advancing, even if
readers are not at the tail.

### Resource Limits

TODO: why v1 over v2?

We will use linux's v1 Cgroups to perform some simple CPU, Memory, and Disk IO resource control on processes. Each limit will be
based per process, thus, current design will create a directory whose name is the job id underneath the different
resource controllers. Below is an example:

```
/sys/fs/cgroup/{cpu,memory,blkio}/:job_id
```

The PID of the process will then be appended to the `tasks` file.

### Authorization

In this system, the authorization primitive will be the authID, a string that is used to uniquely identify different
users. Because we will not be implementing a user system in this service, the identity of the user will be based off of
the `Subject` field in the x509 certificate that is presented (because mTLS is required). Other considerations were
made, such as basing identity off of the certificate, but if attributes other than identity changes, then that would be
considered a new user. The obvious downside of this is if the `Subject` field changes, that would be considered a new
"user" in the system.

### Authentication

In order to authenticate and secure communication between server and client, mutual-TLS will be used.
TLS 1.0 and 1.1 have been deprecated, thus the minimum TLS versions required by the server will be TLS 1.2 and 1.3.

As long as the pair of TLS version and cipher suite chosen is robust and secure enough to secure communication for the
forseeable future, I don't have a strong opinion on the details of either one.

However, if I were to consider some trade offs, it would be the following
* TLS 1.3 removes insecure cipher suits available in 1.2. Thus it would be harder to select an insecure suite. There is
	also some performance benefits because it reduces round trips.
* TLS 1.2 has support for a wider range of clients. 

If choosing TLS 1.2, I would ensure that the cipher suite chosen has no known vulnerabilities or security issues. KEX
generates a ephemeral pub-key pair (uses ECDHE) for forward secrecy. Good performance in terms of processing the keys
(generally speaking RSA with higher bit size requires more processing, than ECDSA keys). Hashing algorithm free from
collision attack (no SHA1).

A strong cipher suite for TLS 1.2 that addresses the above concerns is `ECDHE-ECDSA-AES256-GCM-SHA384`.

### Security Considerations / Limitations

We need to consider safeguards against the types of commands/processes that can be created through this API. This api
currently has no safeguards to prevent the user from running commands that may be deleterious to the machine hosting
this service. There are no access controls on what the user can and cannot run in terms of commands.

The machine has no network ingress/egress safeguards. A user can curl a bash script and run it. A user can probably
exfiltrate data as well. 

There is currently no rate limiting on the service - ie there are no application level bounds on the number of
concurrent clients that can be streaming an output. At some point, it will reach a system level upper limit (likely
running out of fds).

### Performance Considerations

This is not a HA system. An improvements can been to load balance requests and run multiple nodes of this application 
in the back (loadbalancing considerations will be routing requests to certain nodes because data is kept in memory).

### UX - CLI Experience

This section provides some examples CLI commands for each of the different supported APIs.

> urfave/cli library will be used to simplify the code for this exercise

Preliminary design of the CLI UX

```
$ client --help

USAGE:
   client command [command options] [arguments...]

COMMANDS:
   get      
   start    
   stop     
   stream   

GLOBAL OPTIONS:
   --ca-cert value      path to the CA certificate file
   --client-cert value  path to client certificate file 
   --client-key value   path to client key file 
   --target value       target address of server 
```

To run the command `ls -al`

```
$ client start ls -al
> job id: 1
```


To stop a job with job id 1

```
$ client stop --id=1
```

To get a job with job id 1

```
$ client get --id=1
```

To stream a job with job id 1

```
$ client stream --id=1
> ..
> ..
```

## Open Questions
* The ability to stream the data starting from the beginning of the process creation means that the process's output
	must be stored/buffered in-memory to provide good performance. An application issue that needs to be addressed is
	limited system memory. If a long running process generates enough data to use up all the available memory of the
	system, the program will fail. A possible solution is to set a maximum limit on the size that is stored for a given
	process. The data type could just be a byte slice but act as a ring buffer, where if data reaches that maximum size,
	new data will overwrite the oldest data.
