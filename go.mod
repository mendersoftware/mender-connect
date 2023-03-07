module github.com/mendersoftware/mender-connect

go 1.14

replace github.com/urfave/cli/v2 => github.com/mendersoftware/cli/v2 v2.1.1-minimal

require (
	github.com/creack/pty v1.1.18
	github.com/go-ozzo/ozzo-validation/v4 v4.3.0
	github.com/gorilla/websocket v1.5.0
	github.com/mendersoftware/go-lib-micro v0.0.0-20221025103319-e1f941fb3145
	github.com/pkg/errors v0.9.1
	github.com/satori/go.uuid v1.2.0
	github.com/sirupsen/logrus v1.9.0
	github.com/stretchr/testify v1.8.2
	github.com/urfave/cli/v2 v2.25.0
	github.com/vmihailenco/msgpack/v5 v5.3.5
)
