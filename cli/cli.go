// Copyright 2020 Northern.tech AS
//
//    Licensed under the Apache License, Version 2.0 (the "License");
//    you may not use this file except in compliance with the License.
//    You may obtain a copy of the License at
//
//        http://www.apache.org/licenses/LICENSE-2.0
//
//    Unless required by applicable law or agreed to in writing, software
//    distributed under the License is distributed on an "AS IS" BASIS,
//    WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//    See the License for the specific language governing permissions and
//    limitations under the License.
package cli

import (
	"github.com/urfave/cli/v2"
)

func SetupCLI(args []string) error {
	runOptions := &runOptionsType{}

	app := &cli.App{
		Description: "",
		Name:        "mender-shell",
		Usage:       "manage and start the Mender shell.",
		Version:     "",
	}
	app.Commands = []*cli.Command{
		{
			Name:   "daemon",
			Usage:  "Start the client as a background service.",
			Action: runOptions.handleCLIOptions,
		},
	}
	return app.Run(args)
}

func (runOptions *runOptionsType) handleCLIOptions(ctx *cli.Context) error {
	switch ctx.Command.Name {
	case "daemon":
		d, err := initDaemon()
		if err != nil {
			return err
		}
		return runDaemon(d)
	default:
		cli.ShowAppHelpAndExit(ctx, 1)
	}
	return nil
}
