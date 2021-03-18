module github.com/mendersoftware/mender-connect

go 1.14

replace github.com/urfave/cli/v2 => github.com/mendersoftware/cli/v2 v2.1.1-minimal

require (
	github.com/creack/pty v1.1.11
	github.com/go-ozzo/ozzo-validation/v4 v4.3.0
	github.com/gorilla/websocket v1.4.2
	github.com/mendersoftware/go-lib-micro v0.0.0-20210319123318-8cee8f55d7f5
	github.com/pkg/errors v0.9.1
	github.com/satori/go.uuid v1.2.0
	github.com/sirupsen/logrus v1.8.0
	github.com/stretchr/objx v0.3.0 // indirect
	github.com/stretchr/testify v1.7.0
	github.com/urfave/cli/v2 v2.2.0
	github.com/vmihailenco/msgpack/v5 v5.2.0
	golang.org/x/sys v0.0.0-20210105210732-16f7687f5001 // indirect
	gopkg.in/yaml.v3 v3.0.0-20210107192922-496545a6307b // indirect
)
