/*
Package emitter implements channel based pubsub pattern.
The design goals are:
	- fully functional and safety with no any internals goroutines
	- simple to understand and use
	- make the code readable and minimalistic
*/
package emitter

import (
	"path"
	"sync"
)

// Flag used to describe what behavior
// do you expect.
type Flag int

const (
	// FlagReset only to clear previously defined flags.
	// Example:
	// ee.Use("*", Reset) // clears flags for this pattern
	FlagReset Flag = 0
	// FlagOnce indicates to remove the listener after first sending.
	FlagOnce Flag = 1 << iota
	// FlagVoid indicates to skip sending.
	FlagVoid
	// FlagSkip indicates to skip sending if channel is blocked.
	FlagSkip
	// FlagClose indicates to drop listener if channel is blocked.
	FlagClose
	// FlagSync indicates to send an event synchronously.
	FlagSync
)

// Middlewares.

// Reset middleware resets flags
func Reset(e *Event) { e.Flags = FlagReset }

// Once middleware sets FlagOnce flag for an event
func Once(e *Event) { e.Flags = e.Flags | FlagOnce }

// Void middleware sets FlagVoid flag for an event
func Void(e *Event) { e.Flags = e.Flags | FlagVoid }

// Skip middleware sets FlagSkip flag for an event
func Skip(e *Event) { e.Flags = e.Flags | FlagSkip }

// Close middleware sets FlagClose flag for an event
func Close(e *Event) { e.Flags = e.Flags | FlagClose }

// Sync middleware sets FlagSync flag for an event
func Sync(e *Event) { e.Flags = e.Flags | FlagSync }

// New returns just created Emitter interface. Capacity argument
// will be used to create channels with given capacity
func New(capacity uint) Emitter {
	return &emitter{
		listeners:   make(map[string][]listener),
		capacity:    capacity,
		middlewares: make(map[string][]func(*Event)),
	}
}

// Emitter is an interface that allow to emit, receive
// event, close receiver channel, get info
// about topics and listeners
type Emitter interface {
	// Use registers middlewares for the pattern, returns an error if pattern
	// invalid or middlewares are not specified.
	Use(string, ...func(*Event)) error
	// On returns a channel that will receive events. As optional second
	// argument it takes func(*Event) type to describe behavior what you expect.
	On(string, ...func(*Event)) <-chan Event
	// Off unsubscribes all listeners which were covered by
	// topic, it can be pattern as well.
	Off(string, ...<-chan Event) error
	// Emit emits an event with the rest arguments to all
	// listeners which were covered by topic(it can be pattern).
	Emit(string, ...interface{}) chan error
	// Listeners returns slice of listeners which were covered by
	// topic(it can be pattern) and error if pattern is invalid.
	Listeners(string) ([]<-chan Event, error)
	// Topics returns all existing topics.
	Topics() []string
}

type emitter struct {
	mu          sync.Mutex
	listeners   map[string][]listener
	capacity    uint
	middlewares map[string][]func(*Event)
}

func newListener(capacity uint, middlewares ...func(*Event)) listener {
	return listener{
		ch:          make(chan Event, capacity),
		middlewares: middlewares,
	}
}

type listener struct {
	ch          chan Event
	middlewares []func(*Event)
}

// Use registers middlewares for the pattern, returns an error if pattern
// is invalid.
func (e *emitter) Use(pattern string, middlewares ...func(*Event)) error {
	if _, err := path.Match(pattern, "---"); err != nil {
		return err
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	e.middlewares[pattern] = middlewares
	if len(e.middlewares[pattern]) == 0 {
		delete(e.middlewares, pattern)
	}
	return nil
}

// On returns a channel that will receive events. As optional second
// argument it takes middlewares.
func (e *emitter) On(topic string, middlewares ...func(*Event)) <-chan Event {
	e.mu.Lock()
	l := newListener(e.capacity, middlewares...)
	if listeners, ok := e.listeners[topic]; ok {
		e.listeners[topic] = append(listeners, l)
	} else {
		e.listeners[topic] = []listener{l}
	}
	e.mu.Unlock()
	return l.ch
}

// Off unsubscribes all listeners which were covered by
// topic, it can be pattern as well.
func (e *emitter) Off(topic string, channels ...<-chan Event) error {
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
					listeners = drop(listeners, i)
				}

			} else {
				for chi := range channels {
					curr := channels[chi]
					for i := len(listeners) - 1; i >= 0; i-- {
						if curr == listeners[i].ch {
							close(listeners[i].ch)
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
	e.mu.Unlock()
	return nil
}

// Listeners returns slice of listeners which were covered by
// topic(it can be pattern) and error if pattern is invalid.
func (e *emitter) Listeners(topic string) ([]<-chan Event, error) {
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
func (e *emitter) Topics() []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	acc := make([]string, len(e.listeners))
	i := 0
	for k := range e.listeners {
		acc[i] = k
		i++
	}
	return acc
}

// Emit emits an event with the rest arguments to all
// listeners which were covered by topic(it can be pattern).
func (e *emitter) Emit(topic string, args ...interface{}) chan error {
	e.mu.Lock()
	done := make(chan error, 1)

	match, err := e.matched(topic)
	if err != nil {
		done <- err
		close(done)
		e.mu.Unlock()
		return done
	}

	var wg sync.WaitGroup
	var haveToWait bool
	for _, _topic := range match {
		listeners := e.listeners[_topic]
		event := Event{
			Topic:         _topic,
			OriginalTopic: topic,
			Args:          args,
		}

		applyMiddlewares(&event, e.getMiddlewares(_topic))

		// whole topic is skipping
		if (event.Flags | FlagVoid) == event.Flags {
			continue
		}

	Loop:
		for i := len(listeners) - 1; i >= 0; i-- {
			lstnr := listeners[i]
			evn := *(&event) // copy the event
			applyMiddlewares(&evn, lstnr.middlewares)

			if (evn.Flags | FlagVoid) == evn.Flags {
				continue Loop
			}

			if (evn.Flags | FlagSync) == evn.Flags {
				_, remove, _ := pushEvent(done, &lstnr, &evn)
				if remove {
					defer e.Off(event.Topic, lstnr.ch)
				}
			} else {
				wg.Add(1)
				haveToWait = true
				go func(lstnr listener, event *Event) {
					e.mu.Lock()
					_, remove, _ := pushEvent(done, &lstnr, event)
					if remove {
						defer e.Off(event.Topic, lstnr.ch)
					}
					wg.Done()
					e.mu.Unlock()
				}(lstnr, &evn)
			}
		}

		if haveToWait {
			go func(done chan error) {
				defer func() { recover() }()
				wg.Wait()
				close(done)
			}(done)
		} else {
			close(done)
		}

	}

	e.mu.Unlock()
	return done
}

func pushEvent(
	done chan error,
	lstnr *listener,
	event *Event,
) (success, remove bool, err error) {
	// unwind the flags
	isOnce := (event.Flags | FlagOnce) == event.Flags
	isSkip := (event.Flags | FlagSkip) == event.Flags
	isClose := (event.Flags | FlagClose) == event.Flags

	sent, canceled := send(
		done,
		lstnr.ch,
		*event,
		!(isSkip || isClose),
	)
	success = sent

	if !sent && !canceled {
		remove = isClose
		// if not sent
	} else if !canceled {
		// if event was sent successfully
		remove = isOnce
	}
	return
}

func (e *emitter) getMiddlewares(topic string) []func(*Event) {
	var acc []func(*Event)
	for pattern, v := range e.middlewares {
		if match, _ := path.Match(pattern, topic); match {
			acc = append(acc, v...)
		} else if match, _ := path.Match(topic, pattern); match {
			acc = append(acc, v...)
		}
	}
	return acc
}

func applyMiddlewares(e *Event, fns []func(*Event)) {
	for i := range fns {
		fns[i](e)
	}
}

func (e *emitter) matched(topic string) ([]string, error) {
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

func send(done chan error, ch chan Event, e Event, wait bool) (bool, bool) {

	if !wait {
		select {
		case <-done:
			break
		case ch <- e:
			return true, false
		default:
			return false, false
		}

	} else {
		select {
		case <-done:
			break
		case ch <- e:
			return true, false
		}

	}
	return false, true
}
