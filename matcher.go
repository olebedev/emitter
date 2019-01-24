package emitter

import (
	"path"
)

// Matcher interface describes a pattern matcher that determines
// if the name is included as a subset of the pattern
// returns true,nil upon success
type Matcher interface {
	Match(pattern, name string) (matched bool, err error)
}

// Default matcher returns the standard Matcher (PathMatcher)
func DefaultMatcher() Matcher {
	return &PathMatch{}
}

// PathMatch is a Matcher implementation of the system path.Match
// function
type PathMatch struct {
}

func (p *PathMatch) Match(pattern, name string) (matched bool, err error) {
	return path.Match(pattern, name)
}
