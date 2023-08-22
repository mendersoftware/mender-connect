// Copyright 2021 Northern.tech AS
//
//	Licensed under the Apache License, Version 2.0 (the "License");
//	you may not use this file except in compliance with the License.
//	You may obtain a copy of the License at
//
//	    http://www.apache.org/licenses/LICENSE-2.0
//
//	Unless required by applicable law or agreed to in writing, software
//	distributed under the License is distributed on an "AS IS" BASIS,
//	WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//	See the License for the specific language governing permissions and
//	limitations under the License.

package cli

import (
	"github.com/urfave/cli/v2"

	"github.com/northerntechhq/nt-connect/config"
)

func SetupCLI(args []string) error {
	runOptions := &runOptionsType{}
	app := &cli.App{
		Description: "",
		Name:        "mender-connect",
		Usage:       "manage and start the Mender Connect service.",
		Version:     config.ShowVersion(),
		Commands: []*cli.Command{
			{
				Name:   "daemon",
				Usage:  "Start the client as a background service.",
				Action: runOptions.handleCLIOptions,
			},
			{
				Name:   "bootstrap",
				Usage:  "Bootstrap the device's identity.",
				Action: runOptions.handleCLIOptions,
				Flags: []cli.Flag{
					&cli.BoolFlag{
						Name:  "force",
						Value: false,
					},
					&cli.StringFlag{
						Name:  "key-type",
						Value: "secp384r1",
						Usage: "Key type (choices: rsa2048|rsa3072|rsa4096|secp256r1|secp384r1|secp521r1|ed25519)",
					},
					&cli.StringSliceFlag{
						Name: "extra-identity",
						Usage: "Extra identity values " +
							"(--extra-identity key=value [--extra-identity key2=value])",
					},
				},
			},
			{
				Name:   "version",
				Usage:  "Show the version and runtime information of the binary build",
				Action: config.ShowVersionCLI,
			},
		},
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "config",
				Aliases:     []string{"c"},
				Usage:       "Configuration `FILE` path.",
				Value:       config.DefaultConfFile,
				Destination: &runOptions.config,
			},
			&cli.StringFlag{
				Name:        "fallback-config",
				Aliases:     []string{"b"},
				Usage:       "Fallback configuration `FILE` path.",
				Value:       config.DefaultFallbackConfFile,
				Destination: &runOptions.fallbackConfig,
			},
			&cli.BoolFlag{
				Name:        "debug",
				Aliases:     []string{"d"},
				Usage:       "Set the logging level to debug",
				Value:       config.DefaultDebug,
				Destination: &runOptions.debug,
			},
			&cli.BoolFlag{
				Name:        "trace",
				Aliases:     []string{"dd"},
				Usage:       "Set the logging level to trace",
				Value:       config.DefaultTrace,
				Destination: &runOptions.trace,
			},
		},
	}

	return app.Run(args)
}

func (runOptions *runOptionsType) handleCLIOptions(ctx *cli.Context) error {
	// Handle cfg flags
	cfg, err := config.LoadConfig(runOptions.config, runOptions.fallbackConfig)
	if err != nil {
		return err
	}

	cfg.Debug = runOptions.debug
	cfg.Trace = runOptions.trace

	switch ctx.Command.Name {
	case "daemon":
		err = cfg.Validate()
		if err != nil {
			return err
		}
		d, err := initDaemon(cfg)
		if err != nil {
			return err
		}
		return runDaemon(d)
	case "bootstrap":
		return bootstrap(ctx, cfg)
	default:
		cli.ShowAppHelpAndExit(ctx, 1)
	}
	return nil
}
