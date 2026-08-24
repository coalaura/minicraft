package server

type Logger interface {
	Debugf(string, ...any)
	Debugln(...any)

	Printf(string, ...any)
	Println(...any)

	Warnf(string, ...any)
	Warnln(...any)

	Errorf(string, ...any)
	Errorln(...any)
}
