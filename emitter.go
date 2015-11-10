/*
Package emitter implements channel based pubsub pattern.
The design goals are:
	- fully functional and safety with no any internals goroutines
	- simple to understand and use
	- make the code readable and minimalistic
*/
package emitter

import (
	"errors"
	"path"
	"sync"
)

// Flags used to describe what behavior
// do you expect.
type flag int

const (
	// Reset only to clear previously defined flags.
	// Example:
	// ee.Use("*", Reset) // clears flags for this pattern
	Reset flag = 0
	// Once indicates to remove the listener after first sending.
	Once flag = 1 << iota
	// Void indicates to skip sending.
	Void
	// Skip indicates to skip sending if channel is blocked.
	Skip
	// Close indicates to drop listener if channel is blocked.
	Close
)

// New returns just created Emitter interface. Capacity argument
// will be used to create channels with given capacity
func New(capacity uint) Emitter {
	return &eventEmitter{
		listeners: make(map[string][]listener),
		capacity:  capacity,
		// Predefined flags for service events.
		// They can be changed as any other flags
		// via `Use` method. It need to avoid dead
		// lock when unbuffered channel was created.
		flags: map[string]flag{"listener:*": Reset | Skip},
	}
}

// Emitter is an interface that allow to emit, receive
// event, close receiver channel, get info
// about topics and listeners
type Emitter interface {
	// Use registers flags for the pattern, returns an error if pattern
	// invalid or flags are not specified.
	Use(string, ...flag) error
	// On returns a channel that will receive events. As optional second
	// argument it takes flag type to describe behavior what you expect.
	On(string, ...flag) <-chan Event
	// Off unsubscribe all listeners which were covered by
	// topic(it can be pattern) as well.
	Off(string, ...<-chan Event) error
	// Emit emits an event with the rest arguments to all
	// listeners which were covered by topic(it can be pattern).
	Emit(string, ...interface{}) error
	// Listeners returns slice of listeners which were covered by
	// topic(it can be pattern) and error if pattern is invalid.
	Listeners(string) ([]<-chan Event, error)
	// Topics returns all existing topics.
	Topics() []string
}

type eventEmitter struct {
	mu        sync.Mutex
	listeners map[string][]listener
	capacity  uint
	flags     map[string]flag
}

func newListener(capacity uint, flags ...flag) listener {
	var f flag
	// reduce the flags
	for _, item := range flags {
		f |= item
	}
	return listener{
		ch:    make(chan Event, capacity),
		flags: f,
	}
}

type listener struct {
	ch    chan Event
	flags flag
}

// Use registers flags for the pattern, returns an error if pattern
// invalid or flags are not specified.
func (e *eventEmitter) Use(pattern string, flags ...flag) error {
	if _, err := path.Match(pattern, "---"); err != nil {
		return err
	}

	if len(flags) == 0 {
		return errors.New("At least one flag must be specified")
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	// reduce the flags
	var f flag
	for _, item := range flags {
		if item == Reset {
			delete(e.flags, pattern)
			return nil
		}
		f |= item
	}

	e.flags[pattern] = f
	return nil
}

func (e *eventEmitter) getFlags(topic string) flag {
	var f flag
	for pattern, v := range e.flags {
		if match, _ := path.Match(pattern, topic); match {
			f |= v
		} else if match, _ := path.Match(topic, pattern); match {
			f |= v
		}
	}
	return f
}

// On returns a channel that will receive events. As optional second
// argument it takes flag type to describe behavior what you expect.
func (e *eventEmitter) On(topic string, flags ...flag) <-chan Event {
	e.mu.Lock()
	l := newListener(e.capacity, flags...)
	if listeners, ok := e.listeners[topic]; ok {
		e.listeners[topic] = append(listeners, l)
	} else {
		e.listeners[topic] = []listener{l}
	}
	e.mu.Unlock()
	e.Emit("listener:new", readOnly(l.ch), topic)
	return l.ch
}

// Off unsubscribe all listeners which were covered by
// topic(it can be pattern) as well.
func (e *eventEmitter) Off(topic string, channels ...<-chan Event) error {
	e.mu.Lock()
	match, err := e.matched(topic)
	if err != nil {
		defer e.mu.Unlock()
		return err
	}

	for _, _topic := range match {
		if listeners, ok := e.listeners[_topic]; ok {

			if len(channels) == 0 {
				for i := len(listeners) - 1; i >= 0; i-- {
					close(listeners[i].ch)
					defer e.Emit("listener:remove", readOnly(listeners[i].ch), _topic)
					listeners = drop(listeners, i)
				}

			} else {
				for chi := range channels {
					curr := channels[chi]
					for i := len(listeners) - 1; i >= 0; i-- {
						if curr == listeners[i].ch {
							close(listeners[i].ch)
							defer e.Emit("listener:remove", readOnly(listeners[i].ch), _topic)
							listeners = drop(listeners, i)
						}
					}
				}
			}
			e.listeners[_topic] = listeners
		}
		if len(e.listeners[_topic]) == 0 {
			delete(e.listeners, _topic)
		}
	}
	defer e.mu.Unlock()
	return nil
}

// Listeners returns slice of listeners which were covered by
// topic(it can be pattern) and error if pattern is invalid.
func (e *eventEmitter) Listeners(topic string) ([]<-chan Event, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	var acc []<-chan Event
	match, err := e.matched(topic)
	if err != nil {
		return acc, err
	}

	for _, _topic := range match {
		list := e.listeners[_topic]
		for i := range e.listeners[_topic] {
			acc = append(acc, list[i].ch)
		}
	}

	return acc, nil
}

// Topics returns all existing topics.
func (e *eventEmitter) Topics() []string {
	acc := make([]string, len(e.listeners))
	i := 0
	for k := range e.listeners {
		acc[i] = k
		i++
	}
	return acc
}

func readOnly(ch chan Event) <-chan Event { return ch }

// Emit emits an event with the rest arguments to all
// listeners which were covered by topic(it can be pattern).
func (e *eventEmitter) Emit(topic string, args ...interface{}) error {
	e.mu.Lock()
	match, err := e.matched(topic)
	if err != nil {
		e.mu.Unlock()
		return err
	}

	var closed []chan Event
	for _, _topic := range match {
		listeners := e.listeners[_topic]
		topicFlags := e.getFlags(_topic)
		// whole topic is skipping
		if (topicFlags | Void) == topicFlags {
			continue
		}

	Loop:
		for i := len(listeners) - 1; i >= 0; i-- {
			lstnr := listeners[i]
			flags := lstnr.flags | topicFlags

			// unwind the flags
			isOnce := (flags | Once) == flags
			isVoid := (flags | Void) == flags
			isSkip := (flags | Skip) == flags
			isClose := (flags | Close) == flags

			if isVoid {
				continue Loop
			}

			if !send(lstnr.ch, Event{Topic: _topic, Args: args}, !(isSkip || isClose)) {
				// if not sent
				if isClose {
					close(lstnr.ch)
					closed = append(closed, lstnr.ch)
					e.listeners[_topic] = drop(listeners, i)
				}

			} else {
				// if event was sent successfully
				if isOnce {
					close(lstnr.ch)
					closed = append(closed, lstnr.ch)
					e.listeners[_topic] = drop(listeners, i)
				}
			}

		}
		if len(e.listeners[_topic]) == 0 {
			delete(e.listeners, _topic)
		}
	}

	for _, ch := range closed {
		defer e.Emit("listener:remove", readOnly(ch), topic)
	}

	defer e.mu.Unlock()
	return nil
}

func (e *eventEmitter) matched(topic string) ([]string, error) {
	acc := []string{}
	var err error
	for k := range e.listeners {
		if matched, err := path.Match(topic, k); err != nil {
			return []string{}, err
		} else if matched {
			acc = append(acc, k)
		} else {
			if matched, _ := path.Match(k, topic); matched {
				acc = append(acc, k)
			}
		}
	}
	return acc, err
}

func drop(l []listener, i int) []listener {
	return append(l[:i], l[i+1:]...)
}

func send(ch chan Event, e Event, wait bool) bool {
	if !wait {
		select {
		case ch <- e:
			return true
		default:
			return false
		}
	} else {
		ch <- e
	}
	return true
}
