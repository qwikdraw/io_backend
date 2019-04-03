# IO Game Server

This is the backend for [IO Game](https://github.com/qwikdraw/wasm_webgl2)  
It consists of two components, a high performance concurrent websocket server written in Go
and a user management server in Python Flask.

Find these in the `simulation` and `users` folders respectively.

## Protocol
Server-client communication is done with a [protocal buffer](https://developers.google.com/protocol-buffers/)
based message system.
Protocal buffers are used because they use bandwidth and serialization time efficient more efficently.

## Building

### Requires

* golang >= 1.11
* make
* Python3
* protoc
* proto-gen-go

### Usage

```
make
./server --addr <ip>:<port>
```
