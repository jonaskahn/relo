// Localized builtins: help, usage, and completion text.
package cli

import (
	"strings"

	"github.com/spf13/cobra"
)

const (
	helpFlagName        = "help"
	noDescriptionsFlag  = "no-descriptions"
	commandPathTemplate = "{{.CommandPath}}"
)

func localizeBuiltins(root *cobra.Command, localized ui) {
	root.InitDefaultHelpCmd()
	root.InitDefaultCompletionCmd()
	root.SetUsageTemplate(localizedUsageTemplate(root.UsageTemplate(), localized))
	localizeHelpCommand(root, localized)
	localizeCompletionCommand(root, localized)
	localizeHelpFlags(root, localized)
}

func localizedUsageTemplate(template string, localized ui) string {
	return strings.NewReplacer(usageReplacements(localized)...).Replace(template)
}

func usageReplacements(localized ui) []string {
	return []string{
		"Usage:{{", localized.text("cli.usage.title", nil) + "{{",
		"\nAliases:\n", "\n" + localized.text("cli.usage.aliases", nil) + "\n",
		"\nExamples:\n", "\n" + localized.text("cli.usage.examples", nil) + "\n",
		"\nAvailable Commands:{{", "\n" + localized.text("cli.usage.commands", nil) + "{{",
		"\nAdditional Commands:{{", "\n" + localized.text("cli.usage.more_commands", nil) + "{{",
		"\nFlags:\n", "\n" + localized.text("cli.usage.flags", nil) + "\n",
		"\nGlobal Flags:\n", "\n" + localized.text("cli.usage.global_flags", nil) + "\n",
		"\nAdditional help topics:{{", "\n" + localized.text("cli.usage.topics", nil) + "{{",
		`Use "{{.CommandPath}} [command] --help" for more information about a command.`,
		localized.text("cli.usage.footer", map[string]any{"CommandPath": commandPathTemplate}),
	}
}

func localizeHelpCommand(root *cobra.Command, localized ui) {
	help := subcommand(root, helpFlagName)
	if help == nil {
		return
	}
	help.Short = localized.text("cli.help.short", nil)
	help.Long = localized.text("cli.help.long", map[string]any{"Name": root.DisplayName()})
}

func localizeCompletionCommand(root *cobra.Command, localized ui) {
	completion := subcommand(root, "completion")
	if completion == nil {
		return
	}
	completion.Short = localized.text("cli.completion.short", nil)
	completion.Long = localized.text("cli.completion.long", map[string]any{"Name": root.DisplayName()})
	for _, shell := range completion.Commands() {
		shell.Short = localized.text("cli.completion.shell", map[string]any{"Shell": shell.Name()})
		if flag := shell.Flags().Lookup(noDescriptionsFlag); flag != nil {
			flag.Usage = localized.text("cli.completion.no_descriptions", nil)
		}
	}
}

func localizeHelpFlags(cmd *cobra.Command, localized ui) {
	cmd.InitDefaultHelpFlag()
	if flag := cmd.Flags().Lookup(helpFlagName); flag != nil {
		flag.Usage = localized.text("cli.help.flag", map[string]any{"Name": cmd.DisplayName()})
	}
	for _, child := range cmd.Commands() {
		localizeHelpFlags(child, localized)
	}
}

func subcommand(parent *cobra.Command, name string) *cobra.Command {
	for _, child := range parent.Commands() {
		if child.Name() == name {
			return child
		}
	}
	return nil
}
