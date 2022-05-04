package net

type LocalSock interface {
	Connect() error

	Close() error

	Start()
}
