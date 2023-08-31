module github.com/mendersoftware/mender-connect

go 1.17

replace github.com/urfave/cli/v2 => github.com/mendersoftware/cli/v2 v2.1.1-minimal

require (
	github.com/creack/pty v1.1.18
	github.com/go-ozzo/ozzo-validation/v4 v4.3.0
	github.com/gorilla/websocket v1.5.0
	github.com/mendersoftware/go-lib-micro v0.0.0-20221025103319-e1f941fb3145
	github.com/pkg/errors v0.9.1
	github.com/satori/go.uuid v1.2.0
	github.com/sirupsen/logrus v1.9.0
	github.com/stretchr/testify v1.8.4
	github.com/urfave/cli/v2 v2.25.0
	github.com/vmihailenco/msgpack/v5 v5.3.5
)

require (
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/kr/pretty v0.3.0 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/rogpeppe/go-internal v1.8.0 // indirect
	github.com/stretchr/objx v0.5.0 // indirect
	github.com/vmihailenco/tagparser/v2 v2.0.0 // indirect
	golang.org/x/sys v0.0.0-20220715151400-c0bba94af5f8 // indirect
	gopkg.in/check.v1 v1.0.0-20201130134442-10cb98267c6c // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)
