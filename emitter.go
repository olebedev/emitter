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
type Flag int

const (
	// Reset only to clear previously defined flags.
	// Example:
	// ee.Use("*", Reset) // clears flags for this pattern
	FlagReset Flag = 0
	// Once indicates to remove the listener after first sending.
	FlagOnce Flag = 1 << iota
	// Void indicates to skip sending.
	FlagVoid
	// Skip indicates to skip sending if channel is blocked.
	FlagSkip
	// Close indicates to drop listener if channel is blocked.
	FlagClose
)

type Error int

const (
	ErrorPattern Error = -1 << iota
)

// New returns just created Emitter interface. Capacity argument
// will be used to create channels with given capacity
func New(capacity uint) Emitter {
	return &emitter{
		listeners: make(map[string][]listener),
		capacity:  capacity,
		flags:     map[string]Flag{},
	}
}

// Emitter is an interface that allow to emit, receive
// event, close receiver channel, get info
// about topics and listeners
type Emitter interface {
	// Use registers flags for the pattern, returns an error if pattern
	// invalid or flags are not specified.
	Use(string, ...Flag) error
	// On returns a channel that will receive events. As optional second
	// argument it takes flag type to describe behavior what you expect.
	On(string, ...Flag) <-chan Event
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
	mu        sync.Mutex
	listeners map[string][]listener
	capacity  uint
	flags     map[string]Flag
}

func newListener(capacity uint, flags ...Flag) listener {
	var f Flag
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
	flags Flag
}

// Use registers flags for the pattern, returns an error if pattern
// invalid or flags are not specified.
func (e *emitter) Use(pattern string, flags ...Flag) error {
	if _, err := path.Match(pattern, "---"); err != nil {
		return err
	}

	if len(flags) == 0 {
		return errors.New("At least one flag must be specified")
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	// reduce the flags
	var f Flag
	for _, item := range flags {
		if item == FlagReset {
			delete(e.flags, pattern)
			return nil
		}
		f |= item
	}

	e.flags[pattern] = f
	return nil
}

func (e *emitter) getFlags(topic string) Flag {
	var f Flag
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
func (e *emitter) On(topic string, flags ...Flag) <-chan Event {
	e.mu.Lock()
	l := newListener(e.capacity, flags...)
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
	var wg sync.WaitGroup

	match, err := e.matched(topic)
	if err != nil {
		done <- err
		close(done)
		e.mu.Unlock()
		return done
	}

	// fmt.Printf("[debug] 1 %-v\n", match)
	for _, _topic := range match {
		listeners := e.listeners[_topic]
		topicFlags := e.getFlags(_topic)
		// whole topic is skipping
		if (topicFlags | FlagVoid) == topicFlags {
			continue
		}

		for i := len(listeners) - 1; i >= 0; i-- {
			lstnr := listeners[i]
			flags := lstnr.flags | topicFlags
			wg.Add(1)
			go func(lstnr listener, topicFlags Flag, _topic string) {
				// unwind the flags
				isOnce := (flags | FlagOnce) == flags
				isVoid := (flags | FlagVoid) == flags
				isSkip := (flags | FlagSkip) == flags
				isClose := (flags | FlagClose) == flags

				if isVoid {
					wg.Done()
					return
				}
				event := Event{
					Topic:         _topic,
					OriginalTopic: topic,
					Flags:         flags,
					Args:          args,
				}
				if sent, cancaled := send(
					done,
					lstnr.ch,
					event,
					!(isSkip || isClose),
				); !sent {
					// if not sent
					if isClose {
						e.Off(_topic, lstnr.ch)
					}
				} else if cancaled {
					// if sending canceled

				} else {
					// if event was sent successfully
					if isOnce {
						e.Off(_topic, lstnr.ch)
					}
				}
				wg.Done()
			}(lstnr, topicFlags, _topic)

		}

		go func(done chan error) {
			wg.Wait()
			done <- nil
			tryToClose(done)
		}(done)

	}

	e.mu.Unlock()
	return done
}

func tryToClose(c chan error) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = errors.New(r.(string))
		}
	}()
	close(c)
	return
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
	select {
	case <-done:
		break
	default:
		if !wait {
			select {
			case ch <- e:
				return true, false
			default:
				return false, false
			}
		} else {
			ch <- e
			return true, false
		}
	}
	return false, true
}
