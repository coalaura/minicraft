package protocol

type Logger interface {
	Debugf(string, ...any)
}
