package ports

type EventConsumer interface {
	Listen() error
}
