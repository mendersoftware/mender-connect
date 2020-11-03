module github.com/mendersoftware/mender-shell

go 1.14

replace github.com/urfave/cli/v2 => github.com/mendersoftware/cli/v2 v2.1.1-minimal

require (
	github.com/sirupsen/logrus v1.7.0
	github.com/urfave/cli/v2 v2.2.0
)
