# Emitter [![wercker status](https://app.wercker.com/status/e5a44746dc89b513ed28e8a18c5c05c2/s "wercker status")](https://app.wercker.com/project/bykey/e5a44746dc89b513ed28e8a18c5c05c2) [![Coverage Status](https://coveralls.io/repos/olebedev/emitter/badge.svg?branch=HEAD&service=github)](https://coveralls.io/github/olebedev/emitter?branch=HEAD) [![godoc](http://img.shields.io/badge/godoc-reference-blue.svg?style=flat)](https://godoc.org/github.com/olebedev/emitter) [![Code Climate](https://codeclimate.com/github/olebedev/emitter/badges/gpa.svg)](https://codeclimate.com/github/olebedev/emitter)

Package emitter implements channel based pubsub pattern. The design goals are use expressive Golang concurrency model instead of flat callbacks and the simplest API to understand and use.

## Why?
Go has expressive concurrency model but nobody doesn't use it properly for pubsub, as I see(at the end of 2015). I had implemented my own as I didn't find any acceptable. Please, read [this article](#) for more information.


## It does:

- [sync/async event emitting](#Flags) 
- [predicates/middlewares](#Middlewares)
- [bi-directional wildcard](#Wildcard)
- [discard emitting if needed](#Discard emitting)
- [merge events from different channels](#Groups)
- [shallow on demand type casting](#Event)
- [work with callbacks(traditional way)](#Middlewares)


## Brief examples

```go
e := &emitter.Emitter{}
go func(){
	<-e.Emit("change", 42) // wait for event sent successfully 
	<-e.Emit("change", 37)
	e.Off("*") // unsubscribe any listeners
}()

for event := e.On("change") {
	// do something with event.Args
	plintln(event.Int(0))
}
// listener channel was closed
```

## Wildcard
The package allows publications and subscriptions with wildcard.  This feature based on `path.Match` function.

Example:

```go
go e.Emit("something:special", 42)
event := <-e.Once("*") // grub any events
println(event.Int(0)) // will print 42

// or emit event with wildcard path
go e.Emit("*", 37) // emmit for everyone
event := <-e.Once("something:special")
println(event.Int(0)) // will print 37
```

Note that wildcard uses `path.Match`, but the lib is not return errors related for parsing. As this is not main feature. Please check the topic explicitly via `emitter.Test()` function.


## Middlewares
Important part of pubsub package is predicats. It should be allow to skip some event. There are middlewares to solve this problem.
> TODO

## Flags 
fully [sync](#Flags) via unbuffered channels or callbacks or [async](#Flags) via buffered channels and/or goroutines
> TODO

## Discard emitting
> TODO

## Callbacks only usage
> TODO

## Groups
> TODO

## Event
> TODO
