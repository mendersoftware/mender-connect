module github.com/mendersoftware/mender-connect

go 1.14

replace github.com/urfave/cli/v2 => github.com/mendersoftware/cli/v2 v2.1.1-minimal

require (
	github.com/creack/pty v1.1.18
	github.com/go-ozzo/ozzo-validation/v4 v4.3.0
	github.com/gorilla/websocket v1.5.0
	github.com/mendersoftware/go-lib-micro v0.0.0-20210407130414-8df169b86c91
	github.com/pkg/errors v0.9.1
	github.com/satori/go.uuid v1.2.0
	github.com/sirupsen/logrus v1.8.1
	github.com/stretchr/testify v1.7.0
	github.com/urfave/cli/v2 v2.2.0
	github.com/vmihailenco/msgpack/v5 v5.2.0
)
