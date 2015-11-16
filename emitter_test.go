package emitter

import (
	"reflect"
	"testing"
)

func TestFlatBasic(t *testing.T) {
	ee := New(0)
	go ee.Emit("test", nil)
	event := <-ee.On("test")
	expect(t, len(event.Args), 1)
}

func TestFlatClose(t *testing.T) {
	ee := New(0)
	ch := make(chan struct{})
	pipe := ee.On("test")
	ee.On("test", FlagClose)
	l, _ := ee.Listeners("test")
	expect(t, len(l), 2)
	go func() {
		event := <-pipe
		expect(t, len(event.Args), 1)
		ch <- struct{}{}
	}()
	ee.Emit("test", "close")
	<-ch

	go func() {
		for range pipe {
		}
		ch <- struct{}{}
	}()
	l, _ = ee.Listeners("test")
	expect(t, len(l), 1)
	ee.Off("test", pipe)

	<-ch
	expect(t, len(ee.Topics()), 0)
}

func TestBufferedBasic(t *testing.T) {
	ee := New(1)
	// ee.Use("*", OrSkip)
	ch := make(chan struct{})
	pipe := ee.On("test")
	go func() {
		event := <-pipe
		expect(t, len(event.Args), 2)
		ch <- struct{}{}
	}()
	ee.Emit("test", nil, true)
	<-ch
}

func TestOff(t *testing.T) {
	ee := New(0)
	ee.On("test")
	ee.On("test")
	expect(t, len(ee.Topics()), 1)
	l, _ := ee.Listeners("test")
	expect(t, len(l), 2)

	ee.Off("test")
	l, _ = ee.Listeners("test")
	expect(t, len(l), 0)
	expect(t, len(ee.Topics()), 0)
}

func TestRange(t *testing.T) {
	ee := New(0)
	c := 42
	go ee.Emit("test", "range", "it", c)
	for event := range ee.On("test", FlagClose) { // Close if channel is blocked
		expect(t, event.String(0), "range")
		expect(t, event.String(1), "it")
		expect(t, event.Int(2), c)
		// ee.Off("test")
		break
	}
	l, _ := ee.Listeners("test")
	expect(t, len(l), 1)
	<-ee.Emit("test", "range", "it", 42)
	l, _ = ee.Listeners("test")
	expect(t, len(l), 0)
}

func TestCloseOnBlock(t *testing.T) {
	ee := New(0)

	ee.On("test0", FlagClose)
	l, _ := ee.Listeners("test0")
	expect(t, len(l), 1)
	expect(t, len(ee.Topics()), 1)
	<-ee.Emit("test0")
	l, _ = ee.Listeners("test0")
	expect(t, len(l), 0)
	expect(t, len(ee.Topics()), 0)

	ee = New(3)
	ee.Use("test*", FlagClose)
	ee.On("test1")
	ee.On("test2")

	<-ee.Emit("test1")
	<-ee.Emit("test1")
	<-ee.Emit("test1")
	l, err := ee.Listeners("test1")
	expect(t, len(l), 1)
	expect(t, err == nil, true)
	expect(t, len(ee.Topics()), 2)
	<-ee.Emit("test1") // should raise blockedError
	// ^^^^ and remove the topic as well
	l, err = ee.Listeners("test1")
	expect(t, len(l), 0)
	expect(t, err == nil, true)
	expect(t, len(ee.Topics()), 1)
	<-ee.Emit("test2")
	<-ee.Emit("test2")
	<-ee.Emit("test2")
	expect(t, len(ee.Topics()), 1)
	l, err = ee.Listeners("test2")
	expect(t, err == nil, true)
	expect(t, len(l[0]), 3)
	<-ee.Emit("test2") // should raise blockedError
	// ^^^^ and remove the topic as well
	l, err = ee.Listeners("test2")
	expect(t, err == nil, true)
	expect(t, len(l), 0)
	expect(t, len(ee.Topics()), 0)
}

func TestInvalidPattern(t *testing.T) {
	ee := New(0)
	ee.On("test")
	list, err := ee.Listeners("\\")
	expect(t, len(list), 0)
	expect(t, err != nil, true)
	expect(t, err.Error(), "syntax error in pattern")

	err = ee.Off("\\")
	expect(t, err.Error(), "syntax error in pattern")
	err = <-ee.Emit("\\")
	expect(t, err.Error(), "syntax error in pattern")
}

func TestOnOffAll(t *testing.T) {
	ee := New(0)
	ee.On("*")
	l, err := ee.Listeners("test")
	expect(t, len(l), 1)
	expect(t, err == nil, true)

	err = ee.Off("*")
	expect(t, err == nil, true)
	l, err = ee.Listeners("test")
	expect(t, len(l), 0)
	expect(t, err == nil, true)
}

func TestOrSkipOnce(t *testing.T) {
	ee := New(0)
	pipe := ee.On("test", FlagSkip, FlagOnce)
	<-ee.Emit("test")
	l, err := ee.Listeners("test")
	expect(t, len(l), 1)
	expect(t, err == nil, true)
	go ee.Emit("test")
	<-pipe
	l, err = ee.Listeners("test")
	expect(t, len(l), 0)
	expect(t, err == nil, true)
}

func TestVoid(t *testing.T) {
	ee := New(0)
	casted := ee.(*emitter)
	expect(t, len(casted.flags), 0)
	ee.Use("*", FlagVoid)
	expect(t, len(casted.flags), 1)
	ch := make(chan struct{})
	pipe := ee.On("test")
	go func() {
		select {
		case <-pipe:
		default:
			ch <- struct{}{}
		}
	}()
	go ee.Emit("test")
	<-ch
	ee.Use("*", FlagReset)
	ee.Off("*", pipe)
	expect(t, len(casted.flags), 0)
	l, _ := ee.Listeners("*")
	expect(t, len(l), 0)
	ee.On("test", FlagVoid)
	// unblocked, sending will be skipped
	<-ee.Emit("test")
}

func TestOnceClose(t *testing.T) {
	ee := New(0)
	ee.On("test", FlagClose, FlagOnce)
	// unblocked, the listener will be
	// closed after first attempt
	<-ee.Emit("test")
}

func TestUse(t *testing.T) {
	ee := New(0)
	expect(t, ee.Use("\\").Error(), "syntax error in pattern")
	expect(t, ee.Use("-").Error(), "At least one flag must be specified")
}

func expect(t *testing.T, a interface{}, b interface{}) {
	if a != b {
		t.Errorf("Expected %v (type %v) - Got %v (type %v)", b, reflect.TypeOf(b), a, reflect.TypeOf(a))
	}
}
