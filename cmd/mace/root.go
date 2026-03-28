package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/louiss0/mace/formatter"
	"github.com/sanity-io/litter"
	"github.com/spf13/cobra"

	"github.com/louiss0/mace/lexer"
	"github.com/louiss0/mace/parser"
	"github.com/louiss0/mace/parser/ast"
	"github.com/louiss0/mace/processor"
)

func run(args []string, stdout io.Writer, stderr io.Writer) int {
	command := newRootCommand(stdout, stderr)
	command.SetArgs(args)

	if err := command.Execute(); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}

	return 0
}

func newRootCommand(stdout io.Writer, stderr io.Writer) *cobra.Command {
	command := &cobra.Command{
		Use:           "mace",
		Short:         "Process Mace configuration files",
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	command.SetOut(stdout)
	command.SetErr(stderr)
	command.AddCommand(newJSONCommand(stdout), newNodesCommand(stdout), newSourceCommand(stdout))

	return command
}

func newJSONCommand(stdout io.Writer) *cobra.Command {
	command := &cobra.Command{
		Use:   "json <path>",
		Short: "Evaluate a Mace file and print JSON output",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			result, err := processor.New().ProcessFile(args[0])
			if err != nil {
				return err
			}

			payload, err := json.MarshalIndent(outputValue(result.Output), "", "  ")
			if err != nil {
				return fmt.Errorf("marshal json output: %w", err)
			}

			_, err = fmt.Fprintln(stdout, string(payload))
			return err
		},
	}

	return command
}

func newNodesCommand(stdout io.Writer) *cobra.Command {
	command := &cobra.Command{
		Use:   "nodes <path>",
		Short: "Parse a Mace file and print its node structure",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			file, err := parseFile(args[0])
			if err != nil {
				return err
			}

			_, err = fmt.Fprintln(stdout, litter.Sdump(file))
			return err
		},
	}

	return command
}

func newSourceCommand(stdout io.Writer) *cobra.Command {
	command := &cobra.Command{
		Use:   "source <path>",
		Short: "Parse a Mace file and print canonical Mace source",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			file, err := parseFile(args[0])
			if err != nil {
				return err
			}

			source, err := formatter.FormatFile(file)
			if err != nil {
				return err
			}

			_, err = fmt.Fprintln(stdout, source)
			return err
		},
	}

	return command
}

func outputValue(fields map[string]processor.Value) map[string]any {
	output := make(map[string]any, len(fields))
	for name, value := range fields {
		output[name] = valueToAny(value)
	}

	return output
}

func valueToAny(value processor.Value) any {
	switch value.Kind {
	case processor.ValueString:
		return value.String
	case processor.ValueInt:
		return value.Int
	case processor.ValueFloat:
		return value.Float
	case processor.ValueBoolean:
		return value.Boolean
	case processor.ValueArray:
		items := make([]any, 0, len(value.Array))
		for _, item := range value.Array {
			items = append(items, valueToAny(item))
		}
		return items
	case processor.ValueRecord:
		record := make(map[string]any, len(value.Record))
		for name, item := range value.Record {
			record[name] = valueToAny(item)
		}
		return record
	case processor.ValueUnknown:
		return nil
	default:
		return nil
	}
}

func parseFile(path string) (ast.File, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return ast.File{}, fmt.Errorf("read mace file: %w", err)
	}

	tokens, err := lex(string(contents))
	if err != nil {
		return ast.File{}, err
	}

	file, err := parser.New(tokens).ParseFile()
	if err != nil {
		return ast.File{}, err
	}

	return file, nil
}

func lex(input string) ([]lexer.Token, error) {
	lexerInstance := lexer.New(input)
	tokens := []lexer.Token{}

	for {
		token, err := lexerInstance.NextToken()
		if err != nil {
			return nil, err
		}

		tokens = append(tokens, token)
		if token.Type == lexer.TokenEOF {
			return tokens, nil
		}
	}
}
