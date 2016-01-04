# Emitter [![wercker status](https://app.wercker.com/status/e5a44746dc89b513ed28e8a18c5c05c2/s "wercker status")](https://app.wercker.com/project/bykey/e5a44746dc89b513ed28e8a18c5c05c2) [![Coverage Status](https://coveralls.io/repos/olebedev/emitter/badge.svg?branch=HEAD&service=github)](https://coveralls.io/github/olebedev/emitter?branch=HEAD) [![godoc](http://img.shields.io/badge/godoc-reference-blue.svg?style=flat)](https://godoc.org/github.com/olebedev/emitter) [![Code Climate](https://codeclimate.com/github/olebedev/emitter/badges/gpa.svg)](https://codeclimate.com/github/olebedev/emitter)

Package emitter implements channel based pubsub pattern. The design goals are use  Golang concurrency model instead of flat callbacks and the simplest API to understand and use.

## Why?
Go has expressive concurrency model but nobody doesn't use it properly for pubsub, as I see(at the end of 2015). I had implemented my own as I didn't find any acceptable. Please, read [this article](#) for more information.


## It does:

- [sync/async event emitting](#flags) 
- [predicates/middlewares](#middlewares)
- [bi-directional wildcard](#wildcard)
- [discard emitting if needed](#discard-emitting)
- [merge events from different channels](#groups)
- [shallow on demand type casting](#event)
- [work with callbacks(traditional way)](#middlewares)


## Brief example

Simple usage:
```go
e := &emitter.Emitter{}
go func(){
	<-e.Emit("change", 42) // wait for event sent successfully 
	<-e.Emit("change", 37)
	e.Off("*") // unsubscribe any listeners
}()

for event := e.On("change") {
	// do something with event.Args
	plintln(event.Int(0)) // cast first argument to int
}
// listener channel was closed
```

## Wildcard
The package allows publications and subscriptions with wildcard.  This feature based on `path.Match` function.

Example:

```go
go e.Emit("something:special", 42)
event := <-e.Once("*"} // grub any events
println(event.Int(0)) // will print 42

// or emit event with wildcard path
go e.Emit("*", 37) // emmit for everyone
event := <-e.Once("something:special")
println(event.Int(0)) // will print 37
})
```

Note that wildcard uses `path.Match`, but the lib is not return errors related for parsing. As this is not main feature. Please check the topic explicitly via `emitter.Test()` function.


## Middlewares
Important part of pubsub package is predicates. It should be allow to skip some event. Middlewares solve this problem. 
Middleware is a function that takes a pointer to the Event as first argument. All that middlewares can do is just modify the event. It allows to skip sending it needed or modify event's agruments. Or specify the mode to describe how exactly event should be emitted(see [below](#flags)).

There are two ways to add middleware into emitting flow:

- via .On("event", middlewares...)
- via .Use("event", middlewares...)

The first add middlewares ony for this listener, but second add middlewares for all events with given topic. 

Example:
```go
e.Use("*", emitter.Sync) // use synchronous mode for all events
go e.Emit("something:special", 42)

// define predicate
event := <-e.Once("*", func(ev *emitter.Event){
	if ev.Int(0) == 42 {
	    // skip sending
		ev.Flags = ev.Flags | emitter.FlagVoid
	}
})
panic("will never happen")
```


## Flags 
Flags needs to describe how exactly the event should be emitted. Available options are listed [here](https://godoc.org/github.com/olebedev/emitter#Flag).

Every event(`emitter.Event`) has field `.Flags` that contains flags as binary mask. 
Flags can be set only via middlewares(see above). 

There are several predefined middlewares to set needed flags:

- [`emitter.Once`](https://godoc.org/github.com/olebedev/emitter#Once)
- [`emitter.Close`](https://godoc.org/github.com/olebedev/emitter#Close)
- [`emitter.Void`](https://godoc.org/github.com/olebedev/emitter#Void)
- [`emitter.Skip`](https://godoc.org/github.com/olebedev/emitter#Skip)
- [`emitter.Reset`](https://godoc.org/github.com/olebedev/emitter#Reset)

You can combine it as a chain:
```go
e.Use("*", emitter.Void) // skip sending for any events
go e.Emit("surprise", 65536)
event := <-e.On("*", emitter.Reset, emitter.Sync, emitter.Once) // set custom fields for this listener
pintln(event.Int(0)) // prints 65536
```

## Discard emitting
> TODO

## Callbacks only usage
> TODO

## Groups
> TODO

## Event
> TODO
