module github.com/northerntechhq/nt-connect

go 1.20

replace github.com/urfave/cli/v2 => github.com/mendersoftware/cli/v2 v2.1.1-minimal

require (
	github.com/creack/pty v1.1.18
	github.com/go-ozzo/ozzo-validation/v4 v4.3.0
	github.com/gorilla/websocket v1.5.0
	github.com/mendersoftware/go-lib-micro v0.0.0-20230808081028-48c0b853fd99
	github.com/pkg/errors v0.9.1
	github.com/satori/go.uuid v1.2.0
	github.com/sirupsen/logrus v1.9.3
	github.com/stretchr/testify v1.8.4
	github.com/urfave/cli/v2 v2.0.0-00010101000000-000000000000
	github.com/vmihailenco/msgpack/v5 v5.3.5
)

require (
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/stretchr/objx v0.5.0 // indirect
	github.com/vmihailenco/tagparser/v2 v2.0.0 // indirect
	golang.org/x/sys v0.8.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)
