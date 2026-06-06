package cli

import (
	"fmt"
	"os"

	"github.com/meshcore-dev/meshcore-go/cmd/mcr/internal/config"
	"gopkg.in/yaml.v3"
)

func cmdConfig(e *env) error {
	switch e.restArg(0) {
	case "path":
		path, err := config.Path()
		if err != nil {
			return err
		}
		if e.out.JSON {
			return e.out.JSONValue(map[string]string{"path": path})
		}
		e.out.Human("%s\n", path)
		return nil

	case "", "show":
		cfg, err := config.Load()
		if err != nil {
			return err
		}
		if e.out.JSON {
			return e.out.JSONValue(cfg)
		}
		data, err := yaml.Marshal(cfg)
		if err != nil {
			return err
		}
		_, err = os.Stdout.Write(data)
		return err

	default:
		return fmt.Errorf("unknown config subcommand %q", e.restArg(0))
	}
}
