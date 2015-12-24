package emitter

import "testing"

func TestGroupInternals(t *testing.T) {
	g := &Group{}
	g.init()
	expect(t, g.isInit, true)
	g.stopIfListen()
	expect(t, len(g.stop), 0)
	g.listen()
	expect(t, g.isListen, true)
	g.stopIfListen()
}

func TestGroupBasic(t *testing.T) {
	g := &Group{Cap: 5}

	e := new(Emitter)
	e2 := new(Emitter)
	e3 := new(Emitter)

	e.Use("*", Sync)
	e2.Use("*", Sync)
	e3.Use("*", Sync)

	g.Add(
		e.On("*"),
		e2.On("*"),
		e3.On("*"),
	)

	pipe := g.On()

	expect(t, len(pipe), 0)

	<-e.Emit("*", 1)
	<-e.Emit("*", 2)
	<-e2.Emit("*", 3)
	<-e3.Emit("*", 4)
	<-e3.Emit("*", 5)

	// departure/arrival order
	expect(t, (<-pipe).Int(0), 1)
	expect(t, (<-pipe).Int(0), 2)
	expect(t, (<-pipe).Int(0), 3)
	expect(t, (<-pipe).Int(0), 4)
	expect(t, (<-pipe).Int(0), 5)

	g.Off(pipe)

	_, ok := <-pipe
	expect(t, ok, false)

}

func TestGroupFlushOnOff(t *testing.T) {
	g := &Group{}
	g.On()
	expect(t, len(g.listeners), 1)
	g.Flush()
	expect(t, len(g.listeners), 0)
	g.On()
	expect(t, len(g.listeners), 1)
	g.Off()
	expect(t, len(g.listeners), 0)
}
