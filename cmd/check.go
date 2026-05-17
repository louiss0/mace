package main

import (
	"fmt"
	"io"

	"github.com/louiss0/mace/codec"
	"github.com/spf13/cobra"
)

type commandError struct {
	code    int
	message string
	quiet   bool
}

func (err *commandError) Error() string {
	return err.message
}

func newCheckCommand() *cobra.Command {
	command := &cobra.Command{
		Use:   "check <path> [path...]",
		Short: "Check JSON, YAML, or TOML files for Mace compatibility issues",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			reports := make([]codec.FileCheckReport, 0, len(args))
			hasIssues := false
			hasFailures := false

			for _, path := range args {
				report, err := codec.CheckFile(path)
				if err != nil {
					_, _ = fmt.Fprintf(command.ErrOrStderr(), "%s: %v\n", path, err)
					hasFailures = true
					continue
				}
				if report.Errors.HasIssues() {
					hasIssues = true
				}
				reports = append(reports, report)
			}

			if err := writeCheckOutput(command.OutOrStdout(), reports); err != nil {
				return err
			}

			if hasFailures {
				return &commandError{code: 2, quiet: true}
			}
			if hasIssues {
				return &commandError{code: 1, quiet: true}
			}
			return nil
		},
	}

	return command
}

func writeCheckOutput(writer io.Writer, reports []codec.FileCheckReport) error {
	if len(reports) == 0 {
		return nil
	}

	var (
		source string
		err    error
	)
	if len(reports) == 1 {
		source, err = codec.FormatCheckReport(reports[0].Errors)
	} else {
		source, err = codec.FormatFileCheckReports(reports)
	}
	if err != nil {
		return err
	}

	_, err = fmt.Fprintln(writer, source)
	return err
}
