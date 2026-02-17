package example

import "fmt"

type Greeter struct {
	Name string
}

func NewGreeter(name string) *Greeter {
	return &Greeter{Name: name}
}

func (g *Greeter) Greet() string {
	return fmt.Sprintf("Hello, %s!", g.Name)
}

func main() {
	g := NewGreeter("world")
	fmt.Println(g.Greet())
}
