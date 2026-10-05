package engine

type Engine struct {
	book *Book
}

func NewEngine() *Engine {
	return &Engine{
		book: NewBook(),
	}
}

func (e *Engine) Apply(cmd Command, out []Event) []Event { panic("not implemented") }
