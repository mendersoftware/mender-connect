module github.com/mendersoftware/mender-shell

go 1.14

replace github.com/urfave/cli/v2 => github.com/mendersoftware/cli/v2 v2.1.1-minimal

require (
	github.com/gorilla/websocket v1.4.2
	github.com/kr/pty v1.1.8
	github.com/pkg/errors v0.9.1
	github.com/satori/go.uuid v1.2.0
	github.com/sirupsen/logrus v1.7.0
	github.com/stretchr/objx v0.3.0 // indirect
	github.com/stretchr/testify v1.3.0
	github.com/urfave/cli/v2 v2.2.0
	github.com/vmihailenco/msgpack v4.0.4+incompatible
	google.golang.org/appengine v1.6.7 // indirect
)
