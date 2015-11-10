package emitter

import "testing"

func TestEventTypeCast(t *testing.T) {
	ee := New(0)
	go ee.Emit("test", 10, 42.37, "value", true)
	e := <-ee.On("test")
	expect(t, e.Int(0), 10)
	expect(t, e.Float(1), 42.37)
	expect(t, e.String(2), "value")
	expect(t, e.Bool(3), true)

	expect(t, e.Int(4, 5), 5)
	expect(t, e.Float(5, 37.42), 37.42)
	expect(t, e.String(6, "_"), "_")
	expect(t, e.Bool(7, true), true)
}
