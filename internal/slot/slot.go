package slot

import (
	"fmt"
	"sort"

	"brainwash/internal/ir"
)

// Parser is a pluggable agent-memory format.
// New formats: implement Parser, call Register in init().
type Parser interface {
	Name() ir.Slot
	Label() string
	List(cwd string) ([]ir.SessionRef, error)
	Load(ref ir.SessionRef) (*ir.Session, error)
	Write(sess *ir.Session, outCWD string, opt ir.WriteOptions) (path string, err error)
}

var registry = map[ir.Slot]Parser{}

func Register(p Parser) {
	registry[p.Name()] = p
}

func Get(name ir.Slot) (Parser, error) {
	p, ok := registry[name]
	if !ok {
		return nil, fmt.Errorf("unknown slot %q; known: %v", name, Names())
	}
	return p, nil
}

func Names() []ir.Slot {
	out := make([]ir.Slot, 0, len(registry))
	for k := range registry {
		out = append(out, k)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func All() []Parser {
	names := Names()
	out := make([]Parser, 0, len(names))
	for _, n := range names {
		out = append(out, registry[n])
	}
	return out
}

func Must(name ir.Slot) Parser {
	p, err := Get(name)
	if err != nil {
		panic(err)
	}
	return p
}
