package cmd

import (
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// helpCommand is the JSON shape of `--help --json`, letting AI agents
// discover replai's interface without parsing prose.
type helpCommand struct {
	Name        string         `json:"name"`
	Usage       string         `json:"usage"`
	Description string         `json:"description"`
	Flags       []*helpFlag    `json:"flags,omitempty"`
	Subcommands []*helpCommand `json:"subcommands,omitempty"`
}

type helpFlag struct {
	Name        string `json:"name"`
	Shorthand   string `json:"shorthand,omitempty"`
	Type        string `json:"type"`
	Default     string `json:"default"`
	Description string `json:"description"`
}

type helpOut struct {
	OK      bool         `json:"ok"`
	Command *helpCommand `json:"command"`
}

func printHelpJSON(c *cobra.Command) {
	printJSON(&helpOut{OK: true, Command: describeCommand(c)})
}

func describeCommand(c *cobra.Command) *helpCommand {
	out := &helpCommand{
		Name:        c.Name(),
		Usage:       c.UseLine(),
		Description: c.Short,
	}
	collect := func(f *pflag.Flag) {
		out.Flags = append(out.Flags, &helpFlag{
			Name:        f.Name,
			Shorthand:   f.Shorthand,
			Type:        f.Value.Type(),
			Default:     f.DefValue,
			Description: f.Usage,
		})
	}
	c.LocalFlags().VisitAll(collect)
	if c == c.Root() {
		// Root help also documents the persistent flags shared by all
		// subcommands; LocalFlags on root already includes them.
	} else {
		c.InheritedFlags().VisitAll(collect)
	}
	for _, sub := range c.Commands() {
		if sub.Hidden || sub.Name() == "help" || sub.Name() == "completion" {
			continue
		}
		out.Subcommands = append(out.Subcommands, describeCommand(sub))
	}
	return out
}
