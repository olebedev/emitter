package emitter

import (
	"path"
)

type Matcher interface {
	Match(pattern, name string) (matched bool, err error)
}

func DefaultMatcher() Matcher {
	return &PathMatch{}
}

type PathMatch struct {
}

func (p *PathMatch) Match(pattern, name string) (matched bool, err error) {
	return path.Match(pattern, name)
}
