package run

type Named interface {
	Name() string
}

type Activity interface {
	Named
	Start(ctx Context) error
}
